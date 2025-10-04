package dns

import (
	"testing"
	"time"
)

func TestNewDNSResolver(t *testing.T) {
	config := ResolverConfig{
		QueryTimeout: 5 * time.Second,
		EnableCache:  false,
		MaxCacheTTL:  300,
	}

	resolver := NewDNSResolver(config)
	if resolver == nil {
		t.Fatal("NewDNSResolver returned nil")
	}
	if resolver.config.QueryTimeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", resolver.config.QueryTimeout)
	}
	if resolver.config.EnableCache {
		t.Error("Expected cache to be disabled")
	}
	resolver.Stop()
}

func TestUpdateNameservers(t *testing.T) {
	resolver := NewDNSResolver(ResolverConfig{})
	defer resolver.Stop()

	tests := []struct {
		name        string
		nameservers []string
		checkFunc   func(*DNSResolver) bool
	}{
		{
			name:        "default nameserver",
			nameservers: []string{"8.8.8.8"},
			checkFunc: func(r *DNSResolver) bool {
				return len(r.nameservers) == 1 && r.nameservers[0] == "8.8.8.8:53"
			},
		},
		{
			name:        "nameserver with port",
			nameservers: []string{"8.8.8.8:5353"},
			checkFunc: func(r *DNSResolver) bool {
				return len(r.nameservers) == 1 && r.nameservers[0] == "8.8.8.8:5353"
			},
		},
		{
			name:        "domain-specific nameserver",
			nameservers: []string{"example.com/10.0.0.1"},
			checkFunc: func(r *DNSResolver) bool {
				servers, ok := r.domainServers["example.com."]
				return ok && len(servers) == 1 && servers[0] == "10.0.0.1:53"
			},
		},
		{
			name:        "skip comments",
			nameservers: []string{"# comment", "8.8.8.8"},
			checkFunc: func(r *DNSResolver) bool {
				return len(r.nameservers) == 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver.UpdateNameservers(tt.nameservers)
			if !tt.checkFunc(resolver) {
				t.Errorf("Test failed for %s", tt.name)
			}
		})
	}
}

func TestStringToType(t *testing.T) {
	resolver := NewDNSResolver(ResolverConfig{})
	defer resolver.Stop()

	tests := []struct {
		input    string
		expected uint16
	}{
		{"A", 1},
		{"AAAA", 28},
		{"CNAME", 5},
		{"MX", 15},
		{"SRV", 33},
		{"TXT", 16},
		{"UNKNOWN", 1}, // defaults to A
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := resolver.stringToType(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestCacheOperations(t *testing.T) {
	resolver := NewDNSResolver(ResolverConfig{EnableCache: true, MaxCacheTTL: 300})
	defer resolver.Stop()
	time.Sleep(10 * time.Millisecond)

	records := []DNSRecord{
		{Type: "A", Name: "example.com", Target: "93.184.216.34", TTL: 300},
	}

	resolver.addToCache("example.com:A", records)

	cached, found := resolver.checkCache("example.com:A")
	if !found {
		t.Error("Expected to find cached record")
	}
	if len(cached) != 1 || cached[0].Target != "93.184.216.34" {
		t.Error("Cached record doesn't match")
	}

	resolver.ClearCache()
	_, found = resolver.checkCache("example.com:A")
	if found {
		t.Error("Cache should be empty after clear")
	}
}

func TestResolveSRVHttp(t *testing.T) {
	resolver := NewDNSResolver(ResolverConfig{})
	defer resolver.Stop()

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "regular http",
			input:    "http://example.com",
			contains: "http://example.com",
		},
		{
			name:     "regular https",
			input:    "https://example.com",
			contains: "https://example.com",
		},
		{
			name:     "no protocol adds https",
			input:    "example.com",
			contains: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.ResolveSRVHttp(tt.input)
			if result != tt.contains {
				t.Errorf("Expected %s, got %s", tt.contains, result)
			}
		})
	}
}

func TestOrderSRVRecords(t *testing.T) {
	resolver := NewDNSResolver(ResolverConfig{})
	defer resolver.Stop()

	records := []DNSRecord{
		{Type: "SRV", Target: "server1.example.com", Port: 8080, Priority: 10, Weight: 5},
		{Type: "SRV", Target: "server2.example.com", Port: 8080, Priority: 5, Weight: 10},
		{Type: "SRV", Target: "server3.example.com", Port: 8080, Priority: 5, Weight: 5},
	}

	ordered := resolver.orderSRVRecords(records)
	if len(ordered) != 3 {
		t.Errorf("Expected 3 ordered records, got %d", len(ordered))
	}

	if ordered[0].Priority != 5 {
		t.Error("First record should have lowest priority")
	}
}

func TestOrderSRVRecordsFiltersInvalid(t *testing.T) {
	resolver := NewDNSResolver(ResolverConfig{})
	defer resolver.Stop()

	records := []DNSRecord{
		{Type: "SRV", Target: "server1.example.com", Port: 8080, Priority: 10, Weight: 5},
		{Type: "SRV", Target: ".", Port: 8080, Priority: 5, Weight: 10},
		{Type: "SRV", Target: "server3.example.com", Port: 0, Priority: 5, Weight: 5},
		{Type: "A", Target: "1.2.3.4", Priority: 1},
	}

	ordered := resolver.orderSRVRecords(records)
	if len(ordered) != 1 {
		t.Errorf("Expected 1 valid record, got %d", len(ordered))
	}
}
