package dns

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/miekg/dns"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

type DNSServerConfig struct {
	ListenAddr string       // Address to listen on (e.g., ":53", "127.0.0.1:5353")
	Records    []string     // DNS records in BIND-style format
	DefaultTTL int          // Default TTL when not specified in records
	Resolver   *DNSResolver // Optional upstream resolver
}

type DNSRecord struct {
	Type     string // "A", "AAAA", "CNAME", "MX", "SRV", "TXT"
	Name     string // fully qualified domain name (normalized)
	Target   string // IP for A/AAAA, FQDN for CNAME/MX/SRV, target for SRV, value for TXT
	Port     int    // only for SRV records
	Priority int    // for SRV and MX records
	Weight   int    // only for SRV records
	TTL      int    // time to live
}

type DNSServer struct {
	config    DNSServerConfig
	records   map[string][]DNSRecord // exact matches
	wildcards map[string][]DNSRecord // wildcard patterns
	udpServer *dns.Server            // UDP server instance
	tcpServer *dns.Server            // TCP server instance
	mu        sync.RWMutex
	running   bool
	logger    logger.Logger
}

// NewDNSServer creates a new DNS server with the given configuration
func NewDNSServer(config DNSServerConfig) (*DNSServer, error) {
	server := &DNSServer{
		config:    config,
		records:   make(map[string][]DNSRecord),
		wildcards: make(map[string][]DNSRecord),
		logger:    log.WithGroup("dns"),
	}

	if config.DefaultTTL <= 0 {
		server.config.DefaultTTL = 30 // 30 seconds default
	}

	err := server.parseRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to parse DNS records: %w", err)
	}

	return server, nil
}

// parseRecords parses the BIND-style record strings
// Format:
//
//	A|NAME|IP[|TTL]
//	AAAA|NAME|IPv6[|TTL]
//	CNAME|NAME|TARGET[|TTL]
//	MX|NAME|TARGET|PRIORITY[|TTL]
//	SRV|NAME|TARGET|PORT|PRIORITY|WEIGHT[|TTL]
//	TXT|NAME|VALUE[|TTL]
func (s *DNSServer) parseRecords() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing records
	s.records = make(map[string][]DNSRecord)
	s.wildcards = make(map[string][]DNSRecord)

	for _, recordStr := range s.config.Records {
		recordStr = strings.TrimSpace(recordStr)
		if recordStr == "" || strings.HasPrefix(recordStr, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.Split(recordStr, "|")
		if len(parts) < 3 {
			s.logger.Warn("Invalid DNS record:", "recordStr", recordStr)
			continue // Log and continue to the next record
		}

		record := DNSRecord{
			Type: strings.ToUpper(parts[0]),
			TTL:  s.config.DefaultTTL,
		}

		// Normalize domain name
		record.Name = strings.ToLower(parts[1])
		if !strings.HasSuffix(record.Name, ".") {
			record.Name = record.Name + "."
		}

		switch record.Type {
		case "A", "AAAA", "CNAME", "TXT":
			record.Target = parts[2]

			// Optional TTL
			if len(parts) >= 4 {
				if ttl, err := strconv.Atoi(parts[3]); err == nil {
					record.TTL = ttl
				}
			}

		case "MX":
			if len(parts) < 4 {
				s.logger.Warn("Invalid MX record:", "recordStr", recordStr)
				continue
			}
			record.Target = parts[2]

			var err error
			if record.Priority, err = strconv.Atoi(parts[3]); err != nil {
				s.logger.Warn("Invalid MX priority in record:", "recordStr", recordStr)
				continue
			}

			// Optional TTL
			if len(parts) >= 5 {
				if ttl, err := strconv.Atoi(parts[4]); err == nil {
					record.TTL = ttl
				}
			}

		case "SRV":
			if len(parts) < 6 {
				s.logger.Warn("Invalid SRV record:", "recordStr", recordStr)
				continue // Log and continue to the next record
			}
			record.Target = parts[2]

			var err error
			if record.Port, err = strconv.Atoi(parts[3]); err != nil {
				s.logger.Warn("Invalid SRV port in record:", "recordStr", recordStr)
				continue
			}
			if record.Priority, err = strconv.Atoi(parts[4]); err != nil {
				s.logger.Warn("Invalid SRV priority in record:", "recordStr", recordStr)
				continue
			}
			if record.Weight, err = strconv.Atoi(parts[5]); err != nil {
				s.logger.Warn("Invalid SRV weight in record:", "recordStr", recordStr)
				continue
			}

			// Optional TTL
			if len(parts) >= 7 {
				if ttl, err := strconv.Atoi(parts[6]); err == nil {
					record.TTL = ttl
				}
			}

		default:
			s.logger.Warn("unsupported record type:", "record_type", record.Type)
			continue
		}

		// Add to appropriate map (wildcard or exact)
		if strings.HasPrefix(record.Name, "*.") {
			// Wildcard record - store the pattern without the *
			pattern := record.Name[2:] // Remove "*."
			s.wildcards[pattern] = append(s.wildcards[pattern], record)
		} else {
			// Exact match record
			s.records[record.Name] = append(s.records[record.Name], record)
		}
	}

	return nil
}

// UpdateRecords updates the DNS records and reparses them
func (s *DNSServer) UpdateRecords(records []string) error {
	s.config.Records = records
	return s.parseRecords()
}

// findRecords looks up DNS records, checking exact matches first, then wildcards
func (s *DNSServer) findRecords(name string, recordType string) []DNSRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Normalize query name
	queryName := strings.ToLower(name)
	if !strings.HasSuffix(queryName, ".") {
		queryName = queryName + "."
	}

	var results []DNSRecord

	// Check exact matches first
	if records, exists := s.records[queryName]; exists {
		for _, record := range records {
			// Handle different record type matching rules:
			// - CNAME: Always return for any query type (will be resolved further)
			// - SRV: Only return for explicit SRV queries or when no type specified
			// - Other types: Only return if type matches or no type specified
			shouldInclude := false

			if record.Type == "CNAME" {
				// Always include CNAME records - they'll be resolved further
				shouldInclude = true
			} else if record.Type == "SRV" || record.Type == "MX" {
				// SRV and MX records only for explicit queries or no type filter
				shouldInclude = (recordType == "" || recordType == record.Type)
			} else {
				// Other record types: exact match or no type filter
				shouldInclude = (recordType == "" || record.Type == recordType)
			}

			if shouldInclude {
				results = append(results, record)
			}
		}
	}

	// If no exact match, check wildcards
	if len(results) == 0 {
		for pattern, records := range s.wildcards {
			if strings.HasSuffix(queryName, pattern) {
				prefix := strings.TrimSuffix(queryName, pattern)
				if prefix == "" || strings.HasSuffix(prefix, ".") {
					for _, record := range records {
						// Apply same type filtering rules as exact matches
						shouldInclude := false

						if record.Type == "CNAME" {
							shouldInclude = true
						} else if record.Type == "SRV" || record.Type == "MX" {
							shouldInclude = (recordType == "" || recordType == record.Type)
						} else {
							shouldInclude = (recordType == "" || record.Type == recordType)
						}

						if shouldInclude {
							// Create a copy with the actual queried name
							wildcardRecord := record
							wildcardRecord.Name = queryName
							results = append(results, wildcardRecord)
						}
					}
				}
			}
		}
	}

	return results
}

// LookupRecords performs a DNS lookup, checking local records first, then upstream
func (s *DNSServer) LookupRecords(name string, recordType string) ([]DNSRecord, error) {
	return s.lookupRecordsWithDepth(name, recordType, 0, make(map[string]bool))
}

// lookupRecordsWithDepth performs DNS lookup with CNAME loop detection
func (s *DNSServer) lookupRecordsWithDepth(name string, recordType string, depth int, visited map[string]bool) ([]DNSRecord, error) {
	// Prevent infinite CNAME loops
	const maxCNAMEDepth = 10
	if depth > maxCNAMEDepth {
		s.logger.Warn("CNAME loop detected or depth exceeded for", "name", name)
		return nil, fmt.Errorf("CNAME loop detected or depth exceeded for %s", name)
	}

	// Normalize the name for loop detection
	normalizedName := strings.ToLower(name)
	if !strings.HasSuffix(normalizedName, ".") {
		normalizedName = normalizedName + "."
	}

	if visited[normalizedName] {
		return nil, fmt.Errorf("CNAME loop detected for %s", name)
	}
	visited[normalizedName] = true

	// Check local records first
	if records := s.findRecords(name, recordType); len(records) > 0 {
		// If we found records, check if we need to follow CNAMEs
		var finalRecords []DNSRecord
		for _, record := range records {
			finalRecords = append(finalRecords, record)

			// If this is a CNAME and we're looking for A or AAAA records, follow the CNAME
			if record.Type == "CNAME" && (recordType == "A" || recordType == "AAAA") {
				cnameRecords, err := s.lookupRecordsWithDepth(record.Target, recordType, depth+1, visited)
				if err == nil && len(cnameRecords) > 0 {
					finalRecords = append(finalRecords, cnameRecords...)
				}
			}
		}

		return finalRecords, nil
	}

	// Query upstream if no local match and we have a resolver to use
	if s.config.Resolver != nil {
		return s.config.Resolver.QueryUpstream(name, recordType)
	}

	return nil, fmt.Errorf("unknown DNS record")
}

// Start starts the DNS server
func (s *DNSServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("DNS server is already running")
	}

	// Create DNS handler mux for this server instance
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleDNSRequest)

	// Create and store server instances
	s.udpServer = &dns.Server{
		Addr:    s.config.ListenAddr,
		Net:     "udp",
		Handler: mux,
	}

	s.tcpServer = &dns.Server{
		Addr:    s.config.ListenAddr,
		Net:     "tcp",
		Handler: mux,
	}

	// Start UDP server
	go func() {
		s.logger.Info("Starting DNS server on (UDP)", s.config.ListenAddr)
		if err := s.udpServer.ListenAndServe(); err != nil {
			s.logger.WithError(err).Error("UDP server failed")
		}
	}()

	// Start TCP server
	go func() {
		s.logger.Info("Starting DNS server on (TCP)", s.config.ListenAddr)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			s.logger.WithError(err).Error("TCP server failed")
		}
	}()

	s.running = true
	return nil
}

// Stop stops the DNS server
func (s *DNSServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("DNS server is not running")
	}

	// Shutdown both servers
	var udpErr, tcpErr error

	if s.udpServer != nil {
		udpErr = s.udpServer.Shutdown()
		if udpErr != nil {
			s.logger.Error("Failed to shutdown UDP DNS server", "error", udpErr)
		}
	}

	if s.tcpServer != nil {
		tcpErr = s.tcpServer.Shutdown()
		if tcpErr != nil {
			s.logger.Error("Failed to shutdown TCP DNS server", "error", tcpErr)
		}
	}

	s.running = false
	s.udpServer = nil
	s.tcpServer = nil

	s.logger.Info("server stopped")

	// Return first error encountered, if any
	if udpErr != nil {
		return udpErr
	}
	return tcpErr
}

// handleDNSRequest handles incoming DNS requests
func (s *DNSServer) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	// Handle each question in the request
	for _, question := range r.Question {
		recordType := s.typeToString(question.Qtype)

		records, err := s.LookupRecords(question.Name, recordType)
		if err != nil {
			continue // Skip this question, don't fail entire response
		}

		// Convert our internal records to DNS RRs
		for _, record := range records {
			rr := s.recordToRR(record)
			if rr != nil {
				msg.Answer = append(msg.Answer, rr)
			}
		}
	}

	// Set appropriate response code
	if len(msg.Answer) == 0 {
		msg.SetRcode(r, dns.RcodeNameError)
	} else {
		msg.SetRcode(r, dns.RcodeSuccess)
	}

	w.WriteMsg(msg)
}

// recordToRR converts our internal DNSRecord to a DNS RR
func (s *DNSServer) recordToRR(record DNSRecord) dns.RR {
	header := dns.RR_Header{
		Name:   record.Name,
		Rrtype: s.stringToType(record.Type),
		Class:  dns.ClassINET,
		Ttl:    uint32(record.TTL),
	}

	switch record.Type {
	case "A":
		if ip := net.ParseIP(record.Target); ip != nil && ip.To4() != nil {
			return &dns.A{
				Hdr: header,
				A:   ip.To4(),
			}
		}

	case "AAAA":
		if ip := net.ParseIP(record.Target); ip != nil && ip.To16() != nil {
			return &dns.AAAA{
				Hdr:  header,
				AAAA: ip.To16(),
			}
		}

	case "CNAME":
		return &dns.CNAME{
			Hdr:    header,
			Target: dns.Fqdn(record.Target),
		}

	case "MX":
		return &dns.MX{
			Hdr:        header,
			Preference: uint16(record.Priority),
			Mx:         dns.Fqdn(record.Target),
		}

	case "SRV":
		return &dns.SRV{
			Hdr:      header,
			Priority: uint16(record.Priority),
			Weight:   uint16(record.Weight),
			Port:     uint16(record.Port),
			Target:   dns.Fqdn(record.Target),
		}

	case "TXT":
		return &dns.TXT{
			Hdr: header,
			Txt: []string{record.Target},
		}
	}

	return nil
}

// stringToType converts string type to DNS type constant
func (s *DNSServer) stringToType(recordType string) uint16 {
	switch recordType {
	case "A":
		return dns.TypeA
	case "AAAA":
		return dns.TypeAAAA
	case "CNAME":
		return dns.TypeCNAME
	case "MX":
		return dns.TypeMX
	case "SRV":
		return dns.TypeSRV
	case "TXT":
		return dns.TypeTXT
	default:
		return dns.TypeNone
	}
}

// typeToString converts DNS type constant to string
func (s *DNSServer) typeToString(qtype uint16) string {
	switch qtype {
	case dns.TypeA:
		return "A"
	case dns.TypeAAAA:
		return "AAAA"
	case dns.TypeCNAME:
		return "CNAME"
	case dns.TypeMX:
		return "MX"
	case dns.TypeSRV:
		return "SRV"
	case dns.TypeTXT:
		return "TXT"
	default:
		return "UNKNOWN"
	}
}
