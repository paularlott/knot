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

	"github.com/spf13/viper"
)

type HostPort struct {
	Host string
	Port string
}

// Run a parallel SRV query against a list of servers and return the first successful result
func lookupSRVWithFallback(service string, servers []string, defaultPort string) ([]*net.SRV, error) {
	result := make(chan []*net.SRV, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(servers))

	for _, server := range servers {
		go func(server string) {
			defer wg.Done()

			parts := strings.Split(server, ":")
			ip := parts[0]
			port := defaultPort
			if len(parts) > 1 {
				port = parts[1]
			}

			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: 5 * time.Second,
					}
					return dialer.DialContext(ctx, "udp", net.JoinHostPort(ip, port))
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
		}(server)
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
func lookupIPWithFallback(host string, servers []string, defaultPort string) (*[]net.IP, error) {
	result := make(chan *[]net.IP, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(servers))

	for _, server := range servers {
		go func(server string) {
			defer wg.Done()

			parts := strings.Split(server, ":")
			ip := parts[0]
			port := defaultPort
			if len(parts) > 1 {
				port = parts[1]
			}

			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: 5 * time.Second,
					}
					return dialer.DialContext(ctx, "udp", net.JoinHostPort(ip, port))
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
		}(server)
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
	var ns *[]string = nil
	var port string = "53"

	nameservers := viper.GetStringSlice("resolver.nameservers")
	consulServers := viper.GetStringSlice("resolver.consul")

	// If the service ends .consul then use the consul servers if they are given
	if (strings.HasSuffix(service, ".consul") || strings.HasSuffix(service, ".consul.")) && consulServers != nil && len(consulServers) > 0 {
		ns = &consulServers
		port = "8600"
	} else if nameservers != nil && len(nameservers) > 0 {
		ns = &nameservers
	}

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
		srvAddrs, err := lookupSRVWithFallback(service, *ns, port)
		if err == nil && len(srvAddrs) > 0 {
			for _, srvAddr := range srvAddrs {
				ips, err := lookupIPWithFallback(srvAddr.Target, *ns, port)
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
	var ns *[]string = nil
	var port string = "53"

	nameservers := viper.GetStringSlice("resolver.nameservers")
	consulServers := viper.GetStringSlice("resolver.consul")

	// If the service ends .consul then use the consul servers if they are given
	if (strings.HasSuffix(host, ".consul") || strings.HasSuffix(host, ".consul.")) && consulServers != nil && len(consulServers) > 0 {
		ns = &consulServers
		port = "8600"
	} else if nameservers != nil && len(nameservers) > 0 {
		ns = &nameservers
	}

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
		ips, err := lookupIPWithFallback(host, *ns, port)
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
