package dnsproxy

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

const (
	MIN_TTL               = 1
	QUERY_TIMEOUT         = 3 * time.Second
	BACKGROUND_REFRESH_AT = 1
)

type dnsCacheEntry struct {
	lastAccess time.Time
	request    *dns.Msg
	reply      *dns.Msg
	refreshing bool
}

type DNSProxy struct {
	client         *dns.Client
	defaultServers []string
	domains        map[string][]string // map of domain to servers
	cache          map[string]*dnsCacheEntry
	cacheMutex     sync.RWMutex
}

func NewDNSProxy() *DNSProxy {
	dnsProxy := &DNSProxy{
		client:         &dns.Client{},
		defaultServers: make([]string, 0),
		domains:        make(map[string][]string),
		cache:          make(map[string]*dnsCacheEntry),
		cacheMutex:     sync.RWMutex{},
	}

	// Load the default nameservers
	for _, s := range viper.GetStringSlice("resolver.nameservers") {
		// If the nameserver doesn't have a port, add the default port 53
		if _, _, err := net.SplitHostPort(s); err != nil {
			dnsProxy.defaultServers = append(dnsProxy.defaultServers, net.JoinHostPort(s, "53"))
		} else {
			dnsProxy.defaultServers = append(dnsProxy.defaultServers, s)
		}
	}

	log.Debug().Msgf("dns: default servers: %+v", dnsProxy.defaultServers)

	// Load the domain specific nameservers
	domains := viper.GetStringMap("resolver.domains")
	for domain, servers := range domains {
		// Append a . to the domain if it doesn't have one
		if !strings.HasSuffix(domain, ".") {
			domain = domain + "."
		}

		log.Debug().Msgf("dns: domain: %s, servers: %+v", domain, servers)

		dnsProxy.domains[domain] = make([]string, len(servers.([]interface{})))
		for _, s := range servers.([]interface{}) {
			// If the nameserver doesn't have a port, add the default port 53
			if _, _, err := net.SplitHostPort(s.(string)); err != nil {
				dnsProxy.domains[domain] = append(dnsProxy.domains[domain], net.JoinHostPort(s.(string), "53"))
			} else {
				dnsProxy.domains[domain] = append(dnsProxy.domains[domain], s.(string))
			}
		}
	}

	return dnsProxy
}

// Forward a DNS request to the appropriate server and cache the results
func (d *DNSProxy) queryAndCache(r *dns.Msg, dnsCacheItem *dnsCacheEntry) (*dns.Msg, error) {
	in, err := d.parallelExchange(r)
	if err != nil {
		log.Error().Msgf("Failed to query %s IN %s: %s", r.Question[0].Name, dns.TypeToString[r.Question[0].Qtype], err.Error())

		// Create a NXDOMAIN response
		in = new(dns.Msg)
		in.SetReply(r)
		in.SetRcode(r, dns.RcodeNameError)
		in.Answer = []dns.RR{&dns.NS{Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 0}}}
	}

	if len(in.Answer) > 0 {

		// Cache the response
		d.cacheMutex.Lock()
		defer d.cacheMutex.Unlock()

		// If updating an existing cache item
		if dnsCacheItem != nil {
			dnsCacheItem.reply = in
			dnsCacheItem.refreshing = false
		} else {

			// New cache item
			var recordType = dns.TypeToString[r.Question[0].Qtype]
			var cacheKey = r.Question[0].Name + ":" + recordType

			dnsCacheItem = &dnsCacheEntry{
				lastAccess: time.Now(),
				request:    r,
				reply:      in,
				refreshing: false,
			}
			d.cache[cacheKey] = dnsCacheItem
		}
	}

	return in, nil
}

// Exchange a DNS request with multiple servers in parallel
func (d *DNSProxy) parallelExchange(r *dns.Msg) (*dns.Msg, error) {
	var servers *[]string = nil

	// Look through the domains map to see if we have a specific servers for this domain
	for domain, ns := range d.domains {
		if strings.HasSuffix(r.Question[0].Name, domain) {
			log.Debug().Msgf("Using Servers for %s: %+v", domain, ns)
			servers = &ns
			break
		}
	}

	// If no specific servers are found, use the default servers
	if servers == nil {
		log.Debug().Msgf("Using Default Servers: %+v", d.defaultServers)
		servers = &d.defaultServers
	}

	// If no servers are given then return an error
	if len(*servers) == 0 {
		return nil, fmt.Errorf("no dns servers given")
	}

	responseChan := make(chan *dns.Msg, 1)
	errorChan := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	log.Debug().Msgf("dns: using Servers: %+v", servers)

	for _, server := range *servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			in, _, err := d.client.ExchangeContext(ctx, r, server)
			if err != nil {
				select {
				case errorChan <- err:
				default:
				}
			} else {
				select {
				case responseChan <- in:
					cancel()
					log.Debug().Msgf("dns: response from: %s", server)
				default:
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(errorChan)
	}()

	select {
	case res := <-responseChan:

		// Unify the TTL of all answers to match the 1st
		if len(res.Answer) > 1 {
			ttl := res.Answer[0].Header().Ttl
			for i := 1; i < len(res.Answer); i++ {
				res.Answer[i].Header().Ttl = max(ttl, MIN_TTL)
			}
		}

		return res, nil
	case <-ctx.Done():
		if err := <-errorChan; err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no response from any server")
	}
}
