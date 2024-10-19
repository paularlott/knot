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
	"github.com/spf13/viper"
)

type HostPort struct {
	Host string
	Port string
}

var (
	configLoaded   = false
	defaultServers = []string{}
	domainServers  = map[string][]string{}
)

func getResolvers(record string) *[]string {
	// If load loaded then load the nameserver config
	if !configLoaded {
		configLoaded = true

		// Load the default nameservers
		for _, s := range viper.GetStringSlice("resolver.nameservers") {
			// If the nameserver doesn't have a port, add the default port 53
			if _, _, err := net.SplitHostPort(s); err != nil {
				defaultServers = append(defaultServers, net.JoinHostPort(s, "53"))
			} else {
				defaultServers = append(defaultServers, s)
			}
		}

		log.Debug().Msgf("dns: default servers: %+v", defaultServers)

		// Load the domain specific nameservers
		domains := viper.GetStringMap("resolver.domains")
		for domain, servers := range domains {
			// Append a . to the domain if it doesn't have one
			if !strings.HasSuffix(domain, ".") {
				domain = domain + "."
			}

			log.Debug().Msgf("dns: domain: %s, servers: %+v", domain, servers)

			domainServers[domain] = make([]string, len(servers.([]interface{})))
			for _, s := range servers.([]interface{}) {
				// If the nameserver doesn't have a port, add the default port 53
				if _, _, err := net.SplitHostPort(s.(string)); err != nil {
					domainServers[domain] = append(domainServers[domain], net.JoinHostPort(s.(string), "53"))
				} else {
					domainServers[domain] = append(domainServers[domain], s.(string))
				}
			}
		}
	}

	if !strings.HasSuffix(record, ".") {
		record = record + "."
	}

	var servers *[]string = nil

	// Look through the domains map to see if we have a specific servers for this domain
	for domain, ns := range domainServers {
		if strings.HasSuffix(record, domain) {
			log.Debug().Msgf("Using Servers for %s: %+v", domain, ns)
			servers = &ns
			break
		}
	}

	// If no specific servers are found, use the default servers
	if servers == nil {
		if len(defaultServers) == 0 {
			log.Debug().Msg("Using system default nameservers")
			return nil
		} else {
			log.Debug().Msgf("Using Default Servers: %+v", defaultServers)
			servers = &defaultServers
		}
	}

	return servers
}

// Run a parallel SRV query against a list of servers and return the first successful result
func lookupSRV(service string, servers []string) ([]*net.SRV, error) {
	result := make(chan []*net.SRV, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(servers))

	for _, server := range servers {
		go func() {
			defer wg.Done()

			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: 5 * time.Second,
					}
					return dialer.DialContext(ctx, "udp", server)
				},
			}

			_, addrs, err := resolver.LookupSRV(ctx, "", "", service)
			if err == nil && len(addrs) > 0 {
				select {
				case result <- addrs:
					cancel()
				default:
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(result)
	}()

	r, ok := <-result
	if !ok {
		return nil, errors.New("no successful SRV lookup")
	}
	return r, nil
}

// Run a parallel IP against a list of servers and return the first successful result
func lookupIP(host string, servers []string) (*[]net.IP, error) {
	result := make(chan *[]net.IP, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(servers))

	for _, server := range servers {
		go func() {
			defer wg.Done()

			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: 5 * time.Second,
					}
					return dialer.DialContext(ctx, "udp", server)
				},
			}

			ips, err := resolver.LookupIP(ctx, "ip", host)
			if err == nil && len(ips) > 0 {
				select {
				case result <- &ips:
					cancel()
				default:
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(result)
	}()

	r, ok := <-result
	if !ok {
		return nil, errors.New("no successful SRV lookup")
	}
	return r, nil
}

// Look up the address of a service via a DNS SRV lookup against either consul servers or nameservers
func LookupSRV(service string) (*[]HostPort, error) {
	var hostPorts []HostPort = make([]HostPort, 0)

	ns := getResolvers(service)
	if ns == nil {
		// Not using custom nameservers then use the default
		resolver := net.DefaultResolver
		_, srvAddrs, err := resolver.LookupSRV(context.Background(), "", "", service)
		if err == nil && len(srvAddrs) > 0 {
			for _, srvAddr := range srvAddrs {
				ips, err := resolver.LookupIP(context.Background(), "ip", srvAddr.Target)
				if err == nil && len(ips) > 0 {
					for _, ip := range ips {
						hostPorts = append(hostPorts, HostPort{Host: ip.String(), Port: strconv.Itoa(int(srvAddr.Port))})
					}
				}
			}
		}
	} else {
		srvAddrs, err := lookupSRV(service, *ns)
		if err == nil && len(srvAddrs) > 0 {
			for _, srvAddr := range srvAddrs {
				ips, err := lookupIP(srvAddr.Target, *ns)
				if err == nil && len(*ips) > 0 {
					for _, ip := range *ips {
						hostPorts = append(hostPorts, HostPort{Host: ip.String(), Port: strconv.Itoa(int(srvAddr.Port))})
					}
				}
			}
		}
	}

	if len(hostPorts) == 0 {
		return nil, errors.New("no such host")
	} else {
		return &hostPorts, nil
	}
}

func LookupIP(host string) (*[]string, error) {
	var hosts []string = make([]string, 0)

	ns := getResolvers(host)
	if ns == nil {
		// Not using custom nameservers then use the default
		resolver := net.DefaultResolver
		ips, err := resolver.LookupIP(context.Background(), "ip", host)
		if err == nil && len(ips) > 0 {
			for _, ip := range ips {
				hosts = append(hosts, ip.String())
			}
		}
	} else {
		ips, err := lookupIP(host, *ns)
		if err == nil && len(*ips) > 0 {
			for _, ip := range *ips {
				hosts = append(hosts, ip.String())
			}
		}
	}

	if len(hosts) == 0 {
		return nil, errors.New("no such host")
	} else {
		return &hosts, nil
	}
}

func ResolveSRVHttp(uri string) string {
	// If url starts with srv+ then remove it and resolve the actual url
	if len(uri) > 4 && uri[0:4] == "srv+" {

		// Parse the url excluding the srv+ prefix
		u, err := url.Parse(uri[4:])
		if err != nil {
			return uri[4:]
		}

		hostPorts, err := LookupSRV(u.Host)
		if err != nil {
			return uri[4:]
		}

		u.Host = (*hostPorts)[0].Host + ":" + (*hostPorts)[0].Port
		uri = u.String()
	} else if !strings.HasPrefix(uri, "http://") && !strings.HasPrefix(uri, "https://") {
		uri = "https://" + uri
	}

	return uri
}
