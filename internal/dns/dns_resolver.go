package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type ResolverConfig struct {
	QueryTimeout time.Duration // Timeout for upstream queries, 0 uses 2s default
	EnableCache  bool          // Enable/disable upstream DNS cache
	MaxCacheTTL  int           // Maximum cache TTL in seconds (0 = unlimited)
}

type CacheEntry struct {
	Records   []DNSRecord
	ExpiresAt time.Time
}

type DNSResolver struct {
	config        ResolverConfig
	nameservers   []string               // General nameservers
	domainServers map[string][]string    // Domain-specific nameservers
	cache         map[string]*CacheEntry // upstream cache
	cleanupTicker *time.Ticker           // Ticker for cache cleanup
	cleanupCancel context.CancelFunc     // Cancel function for cleanup goroutine
	mu            sync.RWMutex
	cacheMu       sync.RWMutex
}

// NewDNSResolver creates a new DNS resolver with the given nameservers
func NewDNSResolver(config ResolverConfig) *DNSResolver {
	resolver := &DNSResolver{
		config:        config,
		nameservers:   make([]string, 0),
		domainServers: make(map[string][]string),
		cache:         make(map[string]*CacheEntry),
	}

	if resolver.config.QueryTimeout == 0 {
		resolver.config.QueryTimeout = 2 * time.Second
	}

	// Start cache cleanup
	if resolver.config.EnableCache {
		resolver.startCacheCleanup()
	}

	return resolver
}

// UpdateNameservers updates the resolver's nameservers
// Format:
//
//	nameserver         -> default nameserver, port 53
//	nameserver:port    -> default nameserver with custom port
//	domain/nameserver  -> domain-specific nameserver, port 53
//	domain/nameserver:port -> domain-specific nameserver with custom port
func (r *DNSResolver) UpdateNameservers(nameservers []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing configuration
	r.nameservers = make([]string, 0)
	r.domainServers = make(map[string][]string)

	// Process nameservers
	for _, ns := range nameservers {
		ns = strings.TrimSpace(ns)
		if ns == "" || strings.HasPrefix(ns, "#") {
			continue // Skip empty lines and comments
		}

		if strings.Contains(ns, "/") {
			// Domain-specific nameserver
			parts := strings.SplitN(ns, "/", 2)
			if len(parts) != 2 {
				continue // Skip invalid entries
			}

			domain := parts[0]
			nameserver := parts[1]

			// Ensure domain ends with dot
			if !strings.HasSuffix(domain, ".") {
				domain = domain + "."
			}

			// Add port if not specified
			if _, _, err := net.SplitHostPort(nameserver); err != nil {
				nameserver = net.JoinHostPort(nameserver, "53")
			}

			// Add to domain servers
			if _, exists := r.domainServers[domain]; !exists {
				r.domainServers[domain] = make([]string, 0)
			}
			r.domainServers[domain] = append(r.domainServers[domain], nameserver)
		} else {
			// Default nameserver
			nameserver := ns

			// Add port if not specified
			if _, _, err := net.SplitHostPort(nameserver); err != nil {
				nameserver = net.JoinHostPort(nameserver, "53")
			}

			r.nameservers = append(r.nameservers, nameserver)
		}
	}

	r.ClearCache()
}

func (r *DNSResolver) SetConfig(newConfig ResolverConfig) {
	r.mu.Lock()
	oldEnableCache := r.config.EnableCache
	r.config = newConfig
	r.mu.Unlock()

	// Handle cache cleanup and cache state
	if oldEnableCache && !newConfig.EnableCache {
		// Cache was enabled, now disabled: stop cleanup and clear cache
		r.Stop()
		r.ClearCache()
	} else if !oldEnableCache && newConfig.EnableCache {
		// Cache was disabled, now enabled: start cleanup
		r.startCacheCleanup()
		r.ClearCache()
	}
}

func (r *DNSResolver) getResolvers(record string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !strings.HasSuffix(record, ".") {
		record = record + "."
	}

	// Look through the domains map to see if we have specific servers for this domain
	for domain, ns := range r.domainServers {
		if strings.HasSuffix(record, domain) {
			return ns
		}
	}

	// If no specific servers are found, use the default servers
	if len(r.nameservers) == 0 {
		return nil
	} else {
		return r.nameservers
	}
}

// checkCache looks up a query in the cache
func (r *DNSResolver) checkCache(key string) ([]DNSRecord, bool) {
	if !r.config.EnableCache {
		return nil, false
	}

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	entry, exists := r.cache[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		// Cache expired - remove it
		delete(r.cache, key)
		return nil, false
	}

	return entry.Records, true
}

// addToCache adds records to the cache
func (r *DNSResolver) addToCache(key string, records []DNSRecord) {
	if !r.config.EnableCache {
		return
	}

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// Determine the minimum TTL among the records
	minTTL := -1
	for _, rec := range records {
		recTTL := rec.TTL
		if recTTL > 0 && (minTTL == -1 || recTTL < minTTL) {
			minTTL = recTTL
		}
	}
	if minTTL <= 0 {
		minTTL = 60 // fallback to 60s if no TTL found
	}
	// Cap by MaxCacheTTL if set
	if r.config.MaxCacheTTL > 0 && minTTL > r.config.MaxCacheTTL {
		minTTL = r.config.MaxCacheTTL
	}

	r.cache[key] = &CacheEntry{
		Records:   records,
		ExpiresAt: time.Now().Add(time.Duration(minTTL) * time.Second),
	}
}

// QueryUpstream queries the upstream resolver for records using parallel DNS forwarding
func (r *DNSResolver) QueryUpstream(name string, recordType string) ([]DNSRecord, error) {
	cacheKey := fmt.Sprintf("%s:%s", name, recordType)

	// Check cache first
	if records, found := r.checkCache(cacheKey); found {
		return records, nil
	}

	// Get nameservers for this query
	nameservers := r.getResolvers(name)
	if len(nameservers) == 0 {
		// Use system resolver
		return r.querySystemResolver(name, recordType)
	}

	// Create DNS query message
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), r.stringToType(recordType))
	msg.RecursionDesired = true

	// Use context for cancellation
	ctx, cancel := context.WithTimeout(context.Background(), r.config.QueryTimeout)
	defer cancel()

	// Response channel - buffered to prevent goroutine leaks
	respChan := make(chan *dns.Msg, 1) // Only need 1 success
	errChan := make(chan error, len(nameservers))

	var wg sync.WaitGroup

	// Query all nameservers in parallel
	for _, nameserver := range nameservers {
		wg.Add(1)

		go func(ns string) {
			defer wg.Done()

			response := r.queryNameserver(ctx, msg, ns)
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

	// Close channels when all goroutines complete
	go func() {
		wg.Wait()
		close(respChan)
		close(errChan)
	}()

	// Wait for first success or all failures
	select {
	case response, ok := <-respChan:
		if !ok {
			// Channel closed, no successful response
			break
		}
		// Success - convert DNS RRs to our internal format
		var results []DNSRecord
		for _, rr := range response.Answer {
			if record := r.rrToRecord(rr); record != nil {
				results = append(results, *record)
			}
		}

		// Cache the results
		r.addToCache(cacheKey, results)
		return results, nil

	case <-ctx.Done():
		// Context timeout/cancellation
	}

	// Collect all errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		errs = append(errs, fmt.Errorf("query timeout after %v", r.config.QueryTimeout))
	}

	return nil, fmt.Errorf("all nameservers failed: %w", errors.Join(errs...))
}

// queryNameserver queries a single nameserver with both UDP and TCP fallback
func (r *DNSResolver) queryNameserver(ctx context.Context, msg *dns.Msg, nameserver string) *dns.Msg {
	// Try UDP first
	client := &dns.Client{
		Net:     "udp",
		Timeout: r.config.QueryTimeout,
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
func (r *DNSResolver) rrToRecord(rr dns.RR) *DNSRecord {
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

	case *dns.TXT:
		record.Type = "TXT"
		if len(r.Txt) > 0 {
			record.Target = strings.Join(r.Txt, " ")
		}
		return record

	default:
		// Unsupported record type
		return nil
	}
}

// stringToType converts string type to DNS type constant
func (r *DNSResolver) stringToType(recordType string) uint16 {
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
		return dns.TypeA
	}
}

// querySystemResolver uses the Go net.Resolver as a fallback
func (r *DNSResolver) querySystemResolver(name string, recordType string) ([]DNSRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.config.QueryTimeout)
	defer cancel()
	var records []DNSRecord
	var err error
	resolver := net.Resolver{}

	switch recordType {
	case "A":
		var addrs []string
		addrs, err = resolver.LookupHost(ctx, name)
		for _, addr := range addrs {
			if net.ParseIP(addr) != nil && !strings.Contains(addr, ":") {
				records = append(records, DNSRecord{Type: "A", Name: name, Target: addr, TTL: 300})
			}
		}
	case "AAAA":
		var addrs []string
		addrs, err = resolver.LookupHost(ctx, name)
		for _, addr := range addrs {
			if net.ParseIP(addr) != nil && strings.Contains(addr, ":") {
				records = append(records, DNSRecord{Type: "AAAA", Name: name, Target: addr, TTL: 300})
			}
		}
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(ctx, name)
		if err == nil {
			records = append(records, DNSRecord{Type: "CNAME", Name: name, Target: cname, TTL: 300})
		}
	case "TXT":
		var txts []string
		txts, err = resolver.LookupTXT(ctx, name)
		for _, txt := range txts {
			records = append(records, DNSRecord{Type: "TXT", Name: name, Target: txt, TTL: 300})
		}
	case "MX":
		var mxs []*net.MX
		mxs, err = resolver.LookupMX(ctx, name)
		for _, mx := range mxs {
			records = append(records, DNSRecord{Type: "MX", Name: name, Target: mx.Host, Priority: int(mx.Pref), TTL: 300})
		}
	case "SRV":
		var srvs []*net.SRV
		_, srvs, err = resolver.LookupSRV(ctx, "", "", name)
		for _, srv := range srvs {
			records = append(records, DNSRecord{
				Type:     "SRV",
				Name:     name,
				Target:   srv.Target,
				Port:     int(srv.Port),
				Priority: int(srv.Priority),
				Weight:   int(srv.Weight),
				TTL:      300,
			})
		}
	default:
		return nil, fmt.Errorf("system resolver fallback does not support type %s", recordType)
	}

	if err != nil {
		return nil, err
	}
	return records, nil
}

// ClearCache clears the upstream DNS cache
func (r *DNSResolver) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache = make(map[string]*CacheEntry)
}

// startCacheCleanup starts a ticker to periodically clean expired cache entries
func (r *DNSResolver) startCacheCleanup() {
	// Clean up cache every 5 minutes, or every 30s if MaxCacheTTL is very low
	cleanupInterval := 5 * time.Minute
	if r.config.MaxCacheTTL > 0 && time.Duration(r.config.MaxCacheTTL)*time.Second < cleanupInterval {
		cleanupInterval = time.Duration(r.config.MaxCacheTTL) * time.Second
	}
	if cleanupInterval < 30*time.Second {
		cleanupInterval = 30 * time.Second
	}

	r.cleanupTicker = time.NewTicker(cleanupInterval)

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	r.cleanupCancel = cancel

	go func() {
		for {
			select {
			case <-r.cleanupTicker.C:
				r.cleanExpiredEntries()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// cleanExpiredEntries removes expired entries from the cache
func (r *DNSResolver) cleanExpiredEntries() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	now := time.Now()
	removed := 0

	for key, entry := range r.cache {
		if now.After(entry.ExpiresAt) {
			delete(r.cache, key)
			removed++
		}
	}
}

// Stop stops the resolver and cleans up resources
func (r *DNSResolver) Stop() {
	if r.cleanupTicker != nil {
		r.cleanupTicker.Stop()
		if r.cleanupCancel != nil {
			r.cleanupCancel()
		}
		r.cleanupTicker = nil
		r.cleanupCancel = nil
	}
}

// Legacy functions for backward compatibility with existing SRV/IP lookup functionality
func (r *DNSResolver) LookupSRV(service string) ([]*net.TCPAddr, error) {
	records, err := r.QueryUpstream(service, "SRV")
	if err != nil {
		return nil, err
	}

	var tcpAddrs []*net.TCPAddr
	for _, record := range records {
		if record.Type == "SRV" {
			// Look up IPs for the target
			ips, err := r.LookupIP(record.Target)
			if err == nil {
				for _, ip := range ips {
					parsedIP := net.ParseIP(ip)
					if parsedIP != nil {
						tcpAddrs = append(tcpAddrs, &net.TCPAddr{
							IP:   parsedIP,
							Port: record.Port,
						})
					}
				}
			}
		}
	}

	if len(tcpAddrs) == 0 {
		return nil, errors.New("no such host")
	}

	return tcpAddrs, nil
}

func (r *DNSResolver) LookupIP(host string) ([]string, error) {
	var lastErr error

	// Try A records first
	records, err := r.QueryUpstream(host, "A")
	if err == nil && len(records) > 0 {
		var ips []string
		for _, record := range records {
			if record.Type == "A" {
				ips = append(ips, record.Target)
			}
		}
		if len(ips) > 0 {
			return ips, nil
		}
	} else {
		lastErr = err
	}

	// Try AAAA records
	records, err = r.QueryUpstream(host, "AAAA")
	if err == nil && len(records) > 0 {
		var ips []string
		for _, record := range records {
			if record.Type == "AAAA" {
				ips = append(ips, record.Target)
			}
		}
		if len(ips) > 0 {
			return ips, nil
		}
	} else if lastErr == nil {
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", host, lastErr)
	}
	return nil, fmt.Errorf("no IP addresses found for %s", host)
}

func (r *DNSResolver) ResolveSRVHttp(uri string) string {
	// If url starts with srv+ then remove it and resolve the actual url
	if strings.HasPrefix(uri, "srv+") || strings.HasPrefix(uri, "SRV+") {
		// Parse the url excluding the srv+ prefix
		u, err := url.Parse(uri[4:])
		if err != nil {
			return uri[4:]
		}

		hostPorts, err := r.LookupSRV(u.Host)
		if err != nil || len(hostPorts) == 0 {
			return uri[4:]
		}

		u.Host = hostPorts[0].String()
		return u.String()
	}

	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return "https://" + uri
	}

	return uri
}

// Default global resolver instance
var defaultResolver = NewDNSResolver(ResolverConfig{})

// Global convenience functions that use the default resolver
func UpdateNameservers(nameservers []string) {
	defaultResolver.UpdateNameservers(nameservers)
}

func SetConfig(newConfig ResolverConfig) {
	defaultResolver.SetConfig(newConfig)
}

func LookupSRV(service string) ([]*net.TCPAddr, error) {
	return defaultResolver.LookupSRV(service)
}

func LookupIP(host string) ([]string, error) {
	return defaultResolver.LookupIP(host)
}

func ResolveSRVHttp(uri string) string {
	return defaultResolver.ResolveSRVHttp(uri)
}

func GetDefaultResolver() *DNSResolver {
	return defaultResolver
}
