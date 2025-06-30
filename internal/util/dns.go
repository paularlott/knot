package util

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type HostPort struct {
	Host string
	Port string
}

type ResolverConfig struct {
	Nameservers   []string
	DomainServers map[string][]string
	Timeout       time.Duration
}

type DNSResolver struct {
	config ResolverConfig
	mu     sync.RWMutex
}

// NewDNSResolver creates a new DNS resolver with the given nameservers
func NewDNSResolver(nameservers []string) *DNSResolver {
	resolver := &DNSResolver{
		config: ResolverConfig{
			Nameservers:   make([]string, 0),
			DomainServers: make(map[string][]string),
			Timeout:       2 * time.Second,
		},
	}

	resolver.UpdateConfig(nameservers)

	return resolver
}

// UpdateConfig updates the resolver configuration with nameservers
// Format:
//
//	nameserver         -> default nameserver, port 53
//	nameserver:port    -> default nameserver with custom port
//	domain/nameserver  -> domain-specific nameserver, port 53
//	domain/nameserver:port -> domain-specific nameserver with custom port
func (r *DNSResolver) UpdateConfig(nameservers []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing configuration
	r.config.Nameservers = make([]string, 0)
	r.config.DomainServers = make(map[string][]string)

	// Process nameservers
	for _, ns := range nameservers {
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
			if _, exists := r.config.DomainServers[domain]; !exists {
				r.config.DomainServers[domain] = make([]string, 0)
			}
			r.config.DomainServers[domain] = append(r.config.DomainServers[domain], nameserver)
		} else {
			// Default nameserver
			nameserver := ns

			// Add port if not specified
			if _, _, err := net.SplitHostPort(nameserver); err != nil {
				nameserver = net.JoinHostPort(nameserver, "53")
			}

			r.config.Nameservers = append(r.config.Nameservers, nameserver)
		}
	}
}

func (r *DNSResolver) getResolvers(record string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !strings.HasSuffix(record, ".") {
		record = record + "."
	}

	// Look through the domains map to see if we have specific servers for this domain
	for domain, ns := range r.config.DomainServers {
		if strings.HasSuffix(record, domain) {
			log.Trace().Msgf("Using Servers for %s: %+v", domain, ns)
			return ns
		}
	}

	// If no specific servers are found, use the default servers
	if len(r.config.Nameservers) == 0 {
		log.Trace().Msgf("Using system default nameservers")
		return nil
	} else {
		log.Trace().Msgf("Using Default Servers: %+v", r.config.Nameservers)
		return r.config.Nameservers
	}
}

// Generic parallel DNS lookup function
func (r *DNSResolver) parallelLookup(servers []string, lookupFunc func(ctx context.Context, resolver *net.Resolver) (interface{}, error)) (interface{}, error) {
	if len(servers) == 0 {
		return nil, errors.New("no servers provided")
	}

	result := make(chan interface{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(servers))

	for _, server := range servers {
		go func(srv string) {
			defer wg.Done()

			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: r.config.Timeout,
					}
					return dialer.DialContext(ctx, "udp", srv)
				},
			}

			res, err := lookupFunc(ctx, resolver)
			if err == nil && res != nil {
				select {
				case result <- res:
					cancel() // Cancel context to stop other goroutines
				case <-ctx.Done():
					// Context cancelled, another goroutine succeeded or timeout
				}
			}
		}(server)
	}

	// Close result channel when all goroutines complete
	go func() {
		wg.Wait()
		close(result)
	}()

	select {
	case res, ok := <-result:
		if !ok {
			return nil, errors.New("no successful DNS lookup")
		}
		return res, nil
	case <-ctx.Done():
		return nil, errors.New("DNS lookup timeout")
	}
}

// Run a parallel SRV query against a list of servers and return the first successful result
func (r *DNSResolver) lookupSRV(service string, servers []string) ([]*net.SRV, error) {
	res, err := r.parallelLookup(servers, func(ctx context.Context, resolver *net.Resolver) (interface{}, error) {
		_, addrs, err := resolver.LookupSRV(ctx, "", "", service)
		if err != nil || len(addrs) == 0 {
			return nil, err
		}
		return addrs, nil
	})

	if err != nil {
		return nil, err
	}
	return res.([]*net.SRV), nil
}

// Run a parallel IP lookup against a list of servers and return the first successful result
func (r *DNSResolver) lookupIP(host string, servers []string) ([]net.IP, error) {
	res, err := r.parallelLookup(servers, func(ctx context.Context, resolver *net.Resolver) (interface{}, error) {
		ips, err := resolver.LookupIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, err
		}
		return ips, nil
	})

	if err != nil {
		return nil, err
	}
	return res.([]net.IP), nil
}

// Helper function to perform DNS lookup with fallback to system resolver
func (r *DNSResolver) performLookup(host string, customLookup func([]string) (interface{}, error), systemLookup func(context.Context, *net.Resolver) (interface{}, error)) (interface{}, error) {
	servers := r.getResolvers(host)

	if servers == nil {
		// Use system default resolver
		resolver := net.DefaultResolver
		ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
		defer cancel()

		return systemLookup(ctx, resolver)
	}

	// Use custom servers
	return customLookup(servers)
}

func (r *DNSResolver) LookupSRV(service string) ([]HostPort, error) {
	result, err := r.performLookup(service,
		// Custom lookup function
		func(servers []string) (interface{}, error) {
			srvAddrs, err := r.lookupSRV(service, servers)
			if err != nil {
				return nil, err
			}

			var hostPorts []HostPort
			for _, srvAddr := range srvAddrs {
				ips, err := r.lookupIP(srvAddr.Target, servers)
				if err == nil && len(ips) > 0 {
					for _, ip := range ips {
						hostPorts = append(hostPorts, HostPort{
							Host: ip.String(),
							Port: strconv.Itoa(int(srvAddr.Port)),
						})
					}
				}
			}
			return hostPorts, nil
		},
		// System lookup function
		func(ctx context.Context, resolver *net.Resolver) (interface{}, error) {
			_, srvAddrs, err := resolver.LookupSRV(ctx, "", "", service)
			if err != nil {
				return nil, err
			}

			var hostPorts []HostPort
			for _, srvAddr := range srvAddrs {
				ips, err := resolver.LookupIP(ctx, "ip", srvAddr.Target)
				if err == nil && len(ips) > 0 {
					for _, ip := range ips {
						hostPorts = append(hostPorts, HostPort{
							Host: ip.String(),
							Port: strconv.Itoa(int(srvAddr.Port)),
						})
					}
				}
			}
			return hostPorts, nil
		},
	)

	if err != nil {
		return nil, err
	}

	hostPorts := result.([]HostPort)
	if len(hostPorts) == 0 {
		return nil, errors.New("no such host")
	}

	return hostPorts, nil
}

func (r *DNSResolver) LookupIP(host string) ([]string, error) {
	result, err := r.performLookup(host,
		// Custom lookup function
		func(servers []string) (interface{}, error) {
			ips, err := r.lookupIP(host, servers)
			if err != nil {
				return nil, err
			}

			var hosts []string
			for _, ip := range ips {
				hosts = append(hosts, ip.String())
			}
			return hosts, nil
		},
		// System lookup function
		func(ctx context.Context, resolver *net.Resolver) (interface{}, error) {
			ips, err := resolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}

			var hosts []string
			for _, ip := range ips {
				hosts = append(hosts, ip.String())
			}
			return hosts, nil
		},
	)

	if err != nil {
		return nil, err
	}

	hosts := result.([]string)
	if len(hosts) == 0 {
		return nil, errors.New("no such host")
	}

	return hosts, nil
}

func (r *DNSResolver) ResolveSRVHttp(uri string) string {
	// If url starts with srv+ then remove it and resolve the actual url
	if strings.HasPrefix(uri, "srv+") {
		// Parse the url excluding the srv+ prefix
		u, err := url.Parse(uri[4:])
		if err != nil {
			return uri[4:]
		}

		hostPorts, err := r.LookupSRV(u.Host)
		if err != nil || len(hostPorts) == 0 {
			return uri[4:]
		}

		u.Host = hostPorts[0].Host + ":" + hostPorts[0].Port
		return u.String()
	}

	if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		return "https://" + uri
	}

	return uri
}

// Default global resolver instance
var defaultResolver = NewDNSResolver([]string{})

// Global convenience functions that use the default resolver
func UpdateResolverConfig(nameservers []string) {
	defaultResolver.UpdateConfig(nameservers)
}

func LookupSRV(service string) ([]HostPort, error) {
	return defaultResolver.LookupSRV(service)
}

func LookupIP(host string) ([]string, error) {
	return defaultResolver.LookupIP(host)
}

func ResolveSRVHttp(uri string) string {
	return defaultResolver.ResolveSRVHttp(uri)
}
