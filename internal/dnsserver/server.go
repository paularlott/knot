package dnsserver

import (
	"net"
	"strings"
	"sync" // Add sync package for mutex
	"time" // Add time package for cache maintenance

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/util"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type dnsCacheEntry struct {
	ttl uint32
	rr  *dns.A
}

var (
	domain string
	ttl    uint32

	cacheMutex sync.RWMutex
	cache      map[string]*dnsCacheEntry
)

func ListenAndServe() {
	go func() {
		domain = viper.GetString("server.dns.domain")
		if domain == "" {
			log.Fatal().Msg("DNS: enabled but domain not set")
		}

		// Get the TTL and check it's valid
		ttl = viper.GetUint32("server.dns.ttl")
		if ttl < 1 {
			log.Fatal().Msg("DNS: enabled but TTL not set")
		}

		// Initialize the cache and start the maintenance goroutine
		cacheMutex = sync.RWMutex{}
		cache = make(map[string]*dnsCacheEntry)
		go maintainCache()

		// Start the DNS server
		dns.HandleFunc(domain, handleDNSRequest)
		server := &dns.Server{Addr: util.FixListenAddress(viper.GetString("server.dns.listen")), Net: "udp"}
		log.Printf("DNS: starting server on %s", server.Addr)

		// Run the server
		err := server.ListenAndServe()
		if err != nil {
			log.Fatal().Msgf("failed to start server: %s", err.Error())
		}

		log.Info().Msg("DNS: server stopped")
	}()
}

func maintainCache() {
	for {
		time.Sleep(1 * time.Second)

		cacheMutex.Lock()
		for k, v := range cache {
			if v.ttl == 0 {
				delete(cache, k)
			} else {
				v.ttl--
				if v.rr != nil {
					v.rr.Header().Ttl--
				}
			}
		}
		cacheMutex.Unlock()
	}
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		switch q.Qtype {
		case dns.TypeA:
			handleARecord(m, q)
		default:
			m.Rcode = dns.RcodeNameError
		}
	}

	w.WriteMsg(m)
}

func handleARecord(m *dns.Msg, q dns.Question) {
	cacheMutex.RLock()
	if d, found := cache[q.Name]; found {
		cacheMutex.RUnlock()

		if d.rr != nil {
			m.Answer = append(m.Answer, d.rr)
		} else {
			m.Rcode = dns.RcodeNameError
		}
		return
	}
	cacheMutex.RUnlock()

	var rr *dns.A = nil

	// Strip the domain from the query
	name := q.Name
	if len(name) > len(domain)+1 {
		name = name[:len(name)-len(domain)-2]

		// Split the name into parts on the 1st --
		parts := strings.Split(name, "--")
		if len(parts) == 2 {
			// Get the user and space
			db := database.GetInstance()
			user, err := db.GetUserByUsername(parts[0])
			if err == nil && user != nil {
				space, err := db.GetSpaceByName(user.Id, parts[1])
				if err == nil && space != nil {
					// Get the agent session for the space
					agentSession := agent_server.GetSession(space.Id)
					if agentSession != nil && agentSession.AgentIp != "" {

						// Create the A record
						rr = &dns.A{
							Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
							A:   net.ParseIP(agentSession.AgentIp),
						}

					}
				}
			}
		}
	}

	// Save the record in the cache
	cacheMutex.Lock()
	cache[q.Name] = &dnsCacheEntry{ttl: ttl, rr: rr}
	cacheMutex.Unlock()

	if rr != nil {
		m.Answer = append(m.Answer, rr)
	} else {
		m.Rcode = dns.RcodeNameError
	}
}
