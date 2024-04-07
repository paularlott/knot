package agent

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type dnsCacheEntry struct {
	Msg  *dns.Msg
	Time time.Time
}

var (
	dnsCache   = make(map[string]*dnsCacheEntry)
	cacheMutex sync.Mutex
)

// Handler to forward DNS requests to consul servers or DNS servers
func forwardDNSHandler(w dns.ResponseWriter, r *dns.Msg) {
	c := new(dns.Client)
	var in *dns.Msg = nil
	var err error

	log.Debug().Msgf("dns: received request for %s", r.Question[0].Name)

	// Check if the request is in the cache
	cacheMutex.Lock()
	if entry, ok := dnsCache[r.Question[0].Name]; ok {
		// If the cache entry is too old and remove it
		if time.Since(entry.Time) > 10*time.Second {
			delete(dnsCache, r.Question[0].Name)
		} else {
			log.Debug().Msgf("dns: cache hit for %s", r.Question[0].Name)
			in = entry.Msg
		}
	}
	cacheMutex.Unlock()

	if in == nil {
		if strings.HasSuffix(r.Question[0].Name, "consul.") {
			in, err = parallelExchange(c, r, viper.GetStringSlice("resolver.consul"), "8600")
		} else {
			in, err = parallelExchange(c, r, viper.GetStringSlice("resolver.nameservers"), "53")
		}

		// If error then log and return DNS name not found
		if err != nil || in == nil {
			log.Error().Msgf("dns: failed to forward request: %s\n", err.Error())

			in = new(dns.Msg)
			in.SetReply(r)
			in.SetRcode(r, dns.RcodeNameError)
		}

		// Cache the response
		cacheMutex.Lock()
		dnsCache[r.Question[0].Name] = &dnsCacheEntry{Msg: in, Time: time.Now()}
		cacheMutex.Unlock()
	}

	w.WriteMsg(in)
}

// Exchange a DNS request with multiple servers in parallel
func parallelExchange(c *dns.Client, r *dns.Msg, servers []string, defaultPort string) (*dns.Msg, error) {
	// If no servers are given then return an error
	if len(servers) == 0 {
		return nil, fmt.Errorf("no dns servers given")
	}

	responseChan := make(chan *dns.Msg, 1)
	errorChan := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	log.Debug().Msgf("dns: using Servers: %+v", servers)

	for _, server := range servers {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()

			// Split the server string into IP and port
			parts := strings.Split(s, ":")
			ip := parts[0]
			port := defaultPort
			if len(parts) > 1 {
				port = parts[1]
			}

			in, _, err := c.ExchangeContext(ctx, r, net.JoinHostPort(ip, port))
			if err != nil {
				select {
				case errorChan <- err:
				default:
				}
			} else {
				select {
				case responseChan <- in:
					cancel()
					log.Debug().Msgf("dns: response from: %s", net.JoinHostPort(ip, port))
				default:
				}
			}
		}(server)
	}

	go func() {
		wg.Wait()
		close(errorChan)
	}()

	select {
	case res := <-responseChan:
		return res, nil
	case <-ctx.Done():
		if err := <-errorChan; err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no response from any server")
	}
}

func ForwardDNS() {
	log.Info().Msgf("DNS forwarder listening on %s", viper.GetString("agent.dns_listen"))

	server := &dns.Server{Addr: viper.GetString("agent.dns_listen"), Net: "udp"}
	dns.HandleFunc(".", forwardDNSHandler)

	err := server.ListenAndServe()
	defer server.Shutdown()

	if err != nil {
		log.Fatal().Msgf("Failed to start DNS server: %s\n ", err.Error())
	}
}
