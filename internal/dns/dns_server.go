package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
)

type DNSServerConfig struct {
	ListenAddr   string        // Address to listen on (e.g., ":53", "127.0.0.1:5353")
	Records      []string      // DNS records in BIND-style format
	DefaultTTL   int           // Default TTL when not specified in records
	CacheTTL     int           // Cache TTL in seconds, 0 disables cache
	QueryTimeout time.Duration // Timeout for upstream queries, 0 uses 2s default
	Resolver     *DNSResolver  // Optional upstream resolver
}

type DNSRecord struct {
	Type     string // "A", "AAAA", "CNAME", "MX", "SRV"
	Name     string // fully qualified domain name (normalized)
	Target   string // IP for A/AAAA, FQDN for CNAME/MX/SRV, target for SRV
	Port     int    // only for SRV records
	Priority int    // for SRV and MX records
	Weight   int    // only for SRV records
	TTL      int    // time to live
}

type CacheEntry struct {
	Records   []DNSRecord
	ExpiresAt time.Time
}

type DNSServer struct {
	config        DNSServerConfig
	records       map[string][]DNSRecord // exact matches
	wildcards     map[string][]DNSRecord // wildcard patterns
	cache         map[string]*CacheEntry // upstream cache
	udpServer     *dns.Server            // UDP server instance
	tcpServer     *dns.Server            // TCP server instance
	cleanupTicker *time.Ticker           // Ticker for cache cleanup
	cleanupCancel context.CancelFunc     // Cancel function for cleanup goroutine
	mu            sync.RWMutex
	cacheMu       sync.RWMutex
	running       bool
}

// NewDNSServer creates a new DNS server with the given configuration
func NewDNSServer(config DNSServerConfig) (*DNSServer, error) {
	server := &DNSServer{
		config:    config,
		records:   make(map[string][]DNSRecord),
		wildcards: make(map[string][]DNSRecord),
		cache:     make(map[string]*CacheEntry),
	}

	if config.DefaultTTL <= 0 {
		server.config.DefaultTTL = 300 // 5 minutes default
	}

	if config.QueryTimeout <= 0 {
		server.config.QueryTimeout = 2 * time.Second // 2 seconds default
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
			log.Warn().Msgf("dns: Invalid DNS record: %s", recordStr)
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
		case "A", "AAAA", "CNAME":
			record.Target = parts[2]

			// Optional TTL
			if len(parts) >= 4 {
				if ttl, err := strconv.Atoi(parts[3]); err == nil {
					record.TTL = ttl
				}
			}

		case "MX":
			if len(parts) < 4 {
				log.Warn().Msgf("dns: Invalid MX record: %s", recordStr)
				continue
			}
			record.Target = parts[2]

			var err error
			if record.Priority, err = strconv.Atoi(parts[3]); err != nil {
				log.Warn().Msgf("dns: Invalid MX priority in record: %s", recordStr)
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
				log.Warn().Msgf("dns: Invalid SRV record: %s", recordStr)
				continue // Log and continue to the next record
			}
			record.Target = parts[2]

			var err error
			if record.Port, err = strconv.Atoi(parts[3]); err != nil {
				log.Warn().Msgf("dns: Invalid SRV port in record: %s", recordStr)
				continue
			}
			if record.Priority, err = strconv.Atoi(parts[4]); err != nil {
				log.Warn().Msgf("dns: Invalid SRV priority in record: %s", recordStr)
				continue
			}
			if record.Weight, err = strconv.Atoi(parts[5]); err != nil {
				log.Warn().Msgf("dns: Invalid SRV weight in record: %s", recordStr)
				continue
			}

			// Optional TTL
			if len(parts) >= 7 {
				if ttl, err := strconv.Atoi(parts[6]); err == nil {
					record.TTL = ttl
				}
			}

		default:
			log.Warn().Msgf("dns: unsupported record type: %s", record.Type)
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

	return results
}

// checkCache looks up a query in the cache
func (s *DNSServer) checkCache(key string) ([]DNSRecord, bool) {
	if s.config.CacheTTL == 0 {
		return nil, false
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, exists := s.cache[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		// Cache expired - remove it
		delete(s.cache, key)
		return nil, false
	}

	return entry.Records, true
}

// addToCache adds records to the cache
func (s *DNSServer) addToCache(key string, records []DNSRecord) {
	if s.config.CacheTTL == 0 {
		return
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache[key] = &CacheEntry{
		Records:   records,
		ExpiresAt: time.Now().Add(time.Duration(s.config.CacheTTL) * time.Second),
	}
}

// queryUpstream queries the upstream resolver for records using parallel DNS forwarding
func (s *DNSServer) queryUpstream(name string, recordType string) ([]DNSRecord, error) {
	if s.config.Resolver == nil {
		return nil, fmt.Errorf("no upstream resolver configured")
	}

	cacheKey := fmt.Sprintf("%s:%s", name, recordType)

	// Check cache first
	if records, found := s.checkCache(cacheKey); found {
		return records, nil
	}

	// Get nameservers for this query
	nameservers := s.config.Resolver.GetNameservers(name)
	if len(nameservers) == 0 {
		return nil, fmt.Errorf("no nameservers configured for %s", name)
	}

	// Create DNS query message
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), s.stringToType(recordType))
	msg.RecursionDesired = true

	// Use context for cancellation
	ctx, cancel := context.WithTimeout(context.Background(), s.config.QueryTimeout)
	defer cancel()

	// Response channel - buffered to prevent goroutine leaks
	respChan := make(chan *dns.Msg, len(nameservers))
	errChan := make(chan error, len(nameservers))

	// Query all nameservers in parallel
	for _, nameserver := range nameservers {
		go func(ns string) {
			response := s.queryNameserver(ctx, msg, ns)
			if response != nil && response.Rcode == dns.RcodeSuccess && len(response.Answer) > 0 {
				select {
				case respChan <- response:
					cancel() // Cancel other queries on first success
				case <-ctx.Done():
					// Context already cancelled
				}
			} else {
				select {
				case errChan <- fmt.Errorf("nameserver %s returned no valid response", ns):
				case <-ctx.Done():
					// Context already cancelled
				}
			}
		}(nameserver)
	}

	// Wait for first success or all failures
	select {
	case response := <-respChan:
		// Success - convert DNS RRs to our internal format
		var results []DNSRecord
		for _, rr := range response.Answer {
			if record := s.rrToRecord(rr); record != nil {
				results = append(results, *record)
			}
		}

		// Cache the results
		s.addToCache(cacheKey, results)
		return results, nil

	case <-ctx.Done():
		// Collect any errors that came in
		var errs []error
		for i := 0; i < len(nameservers); i++ {
			select {
			case err := <-errChan:
				errs = append(errs, err)
			default:
				errs = append(errs, fmt.Errorf("timeout"))
			}
		}
		return nil, fmt.Errorf("all nameservers failed: %w", errors.Join(errs...))
	}
}

// queryNameserver queries a single nameserver with both UDP and TCP fallback
func (s *DNSServer) queryNameserver(ctx context.Context, msg *dns.Msg, nameserver string) *dns.Msg {
	// Try UDP first
	client := &dns.Client{
		Net:     "udp",
		Timeout: s.config.QueryTimeout,
	}

	response, _, err := client.ExchangeContext(ctx, msg, nameserver)
	if err == nil && response != nil {
		// Check if truncated - if so, retry with TCP
		if response.Truncated {
			client.Net = "tcp"
			response, _, err = client.ExchangeContext(ctx, msg, nameserver)
		}
		if err == nil && response != nil {
			return response
		}
	}

	// If UDP failed or context cancelled, try TCP as fallback
	if ctx.Err() == nil {
		client.Net = "tcp"
		response, _, err = client.ExchangeContext(ctx, msg, nameserver)
		if err == nil && response != nil {
			return response
		}
	}

	return nil
}

// rrToRecord converts a DNS RR to our internal DNSRecord format
func (s *DNSServer) rrToRecord(rr dns.RR) *DNSRecord {
	header := rr.Header()
	record := &DNSRecord{
		Name: header.Name,
		TTL:  int(header.Ttl),
	}

	switch r := rr.(type) {
	case *dns.A:
		record.Type = "A"
		record.Target = r.A.String()
		return record

	case *dns.AAAA:
		record.Type = "AAAA"
		record.Target = r.AAAA.String()
		return record

	case *dns.CNAME:
		record.Type = "CNAME"
		record.Target = strings.TrimSuffix(r.Target, ".")
		return record

	case *dns.MX:
		record.Type = "MX"
		record.Target = strings.TrimSuffix(r.Mx, ".")
		record.Priority = int(r.Preference)
		return record

	case *dns.SRV:
		record.Type = "SRV"
		record.Target = strings.TrimSuffix(r.Target, ".")
		record.Port = int(r.Port)
		record.Priority = int(r.Priority)
		record.Weight = int(r.Weight)
		return record

	default:
		// Unsupported record type
		return nil
	}
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
		log.Warn().Msgf("dns: CNAME loop detected or depth exceeded for %s", name)
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

	// Query upstream if no local match
	return s.queryUpstream(name, recordType)
}

// ClearCache clears the upstream DNS cache
func (s *DNSServer) ClearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]*CacheEntry)
	log.Debug().Msg("dns: cache cleared")
}

// GetStats returns statistics about the DNS server
func (s *DNSServer) GetStats() map[string]interface{} {
	s.mu.RLock()
	s.cacheMu.RLock()
	defer s.mu.RUnlock()
	defer s.cacheMu.RUnlock()

	exactRecords := 0
	for _, records := range s.records {
		exactRecords += len(records)
	}

	wildcardRecords := 0
	for _, records := range s.wildcards {
		wildcardRecords += len(records)
	}

	return map[string]interface{}{
		"exact_records":    exactRecords,
		"wildcard_records": wildcardRecords,
		"cache_entries":    len(s.cache),
		"cache_enabled":    s.config.CacheTTL > 0,
		"upstream_enabled": s.config.Resolver != nil,
		"listen_addr":      s.config.ListenAddr,
	}
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
		log.Info().Msgf("dns: Starting DNS server on %s (UDP)", s.config.ListenAddr)
		if err := s.udpServer.ListenAndServe(); err != nil {
			log.Error().Err(err).Msg("dns: UDP server failed")
		}
	}()

	// Start TCP server
	go func() {
		log.Info().Msgf("dns: Starting DNS server on %s (TCP)", s.config.ListenAddr)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			log.Error().Err(err).Msg("dns: TCP server failed")
		}
	}()

	// Start cache cleanup if caching is enabled
	if s.config.CacheTTL > 0 {
		s.startCacheCleanup()
	}

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

	// Stop cache cleanup ticker
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
		if s.cleanupCancel != nil {
			s.cleanupCancel()
		}
		s.cleanupTicker = nil
		s.cleanupCancel = nil
	}

	// Shutdown both servers
	var udpErr, tcpErr error

	if s.udpServer != nil {
		udpErr = s.udpServer.Shutdown()
		if udpErr != nil {
			log.Error().Err(udpErr).Msg("dns: Failed to shutdown UDP DNS server")
		}
	}

	if s.tcpServer != nil {
		tcpErr = s.tcpServer.Shutdown()
		if tcpErr != nil {
			log.Error().Err(tcpErr).Msg("dns: Failed to shutdown TCP DNS server")
		}
	}

	s.running = false
	s.udpServer = nil
	s.tcpServer = nil

	log.Info().Msg("dns: server stopped")

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
	default:
		return "UNKNOWN"
	}
}

// startCacheCleanup starts a ticker to periodically clean expired cache entries
func (s *DNSServer) startCacheCleanup() {
	// Clean up cache every cache TTL duration, or every 5 minutes, whichever is shorter
	cleanupInterval := time.Duration(s.config.CacheTTL) * time.Second
	if cleanupInterval > 5*time.Minute {
		cleanupInterval = 5 * time.Minute
	}
	if cleanupInterval < 30*time.Second {
		cleanupInterval = 30 * time.Second
	}

	s.cleanupTicker = time.NewTicker(cleanupInterval)

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	s.cleanupCancel = cancel

	go func() {
		for {
			select {
			case <-s.cleanupTicker.C:
				s.cleanExpiredEntries()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// cleanExpiredEntries removes expired entries from the cache
func (s *DNSServer) cleanExpiredEntries() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	now := time.Now()
	removed := 0

	for key, entry := range s.cache {
		if now.After(entry.ExpiresAt) {
			delete(s.cache, key)
			removed++
		}
	}

	if removed > 0 {
		log.Debug().Msgf("dns: cache cleanup, removed %d expired entries, %d remaining", removed, len(s.cache))
	}
}
