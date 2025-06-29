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
	DefaultServers []string
	DomainServers  map[string][]string
	Timeout        time.Duration
}

type DNSResolver struct {
	config ResolverConfig
	mu     sync.RWMutex
}

// NewDNSResolver creates a new DNS resolver with the given configuration
func NewDNSResolver(config *ResolverConfig) *DNSResolver {
	resolver := &DNSResolver{
		config: ResolverConfig{
			DefaultServers: make([]string, 0, len(config.DefaultServers)),
			DomainServers:  make(map[string][]string),
			Timeout:        config.Timeout,
		},
	}

	resolver.UpdateConfig(config)

	return resolver
}

// Add this method to DNSResolver
func (r *DNSResolver) UpdateConfig(config *ResolverConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing configuration
	r.config.DefaultServers = make([]string, 0, len(config.DefaultServers))
	r.config.DomainServers = make(map[string][]string)
	r.config.Timeout = config.Timeout

	// Set default timeout if not provided
	if r.config.Timeout == 0 {
		r.config.Timeout = 2 * time.Second
	}

	// Process default servers
	for _, s := range config.DefaultServers {
		if _, _, err := net.SplitHostPort(s); err != nil {
			r.config.DefaultServers = append(r.config.DefaultServers, net.JoinHostPort(s, "53"))
		} else {
			r.config.DefaultServers = append(r.config.DefaultServers, s)
		}
	}

	// Process domain-specific servers
	for domain, servers := range config.DomainServers {
		if !strings.HasSuffix(domain, ".") {
			domain = domain + "."
		}

		processedServers := make([]string, 0, len(servers))
		for _, s := range servers {
			if _, _, err := net.SplitHostPort(s); err != nil {
				processedServers = append(processedServers, net.JoinHostPort(s, "53"))
			} else {
				processedServers = append(processedServers, s)
			}
		}
		r.config.DomainServers[domain] = processedServers
	}

	log.Trace().Msgf("dns: updated default servers: %+v", r.config.DefaultServers)
	log.Trace().Msgf("dns: updated domain servers: %+v", r.config.DomainServers)
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
	if len(r.config.DefaultServers) == 0 {
		log.Trace().Msg("Using system default nameservers")
		return nil
	} else {
		log.Trace().Msgf("Using Default Servers: %+v", r.config.DefaultServers)
		return r.config.DefaultServers
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
var DefaultResolver = NewDNSResolver(&ResolverConfig{})

// Global convenience functions that use the default resolver
func UpdateResolverConfig(config *ResolverConfig) {
	DefaultResolver.UpdateConfig(config)
}

func LookupSRV(service string) ([]HostPort, error) {
	return DefaultResolver.LookupSRV(service)
}

func LookupIP(host string) ([]string, error) {
	return DefaultResolver.LookupIP(host)
}

func ResolveSRVHttp(uri string) string {
	return DefaultResolver.ResolveSRVHttp(uri)
}
