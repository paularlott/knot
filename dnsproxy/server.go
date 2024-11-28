package dnsproxy

import (
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Handler to forward DNS requests to upstream servers
func (d *DNSProxy) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	var in *dns.Msg = nil
	var err error
	var recordType = dns.TypeToString[r.Question[0].Qtype]

	log.Debug().Msgf("Received request for %s IN %s", r.Question[0].Name, recordType)

	// Check the cache for the name + type
	var cacheKey = r.Question[0].Name + ":" + recordType
	var cacheItem *dnsCacheEntry = nil
	var ok bool

	d.cacheMutex.RLock()
	if cacheItem, ok = d.cache[cacheKey]; ok {
		log.Debug().Msgf("Cache Hit for %s IN %s", r.Question[0].Name, recordType)

		// Look through the list of Answers and find the first A record, then move it to the end of the list
		if len(cacheItem.reply.Answer) > 1 {
			for i, a := range cacheItem.reply.Answer {
				if a.Header().Rrtype == dns.TypeA {
					answer := append(cacheItem.reply.Answer[:i], cacheItem.reply.Answer[i+1:]...)
					answer = append(answer, a)
					cacheItem.reply.Answer = answer
					break
				}
			}
		}

		// Update the last access time & use the cached response
		cacheItem.lastAccess = time.Now()
		in = cacheItem.reply
	}
	d.cacheMutex.RUnlock()

	// If not found in the cache then we query for it
	if in == nil {
		in, err = d.queryAndCache(r, nil)
		if err != nil {
			log.Warn().Msgf("Failed to forward request: %s\n", err.Error())
		}
	}

	// Copy the response and set the response ID to match the request ID
	var out *dns.Msg = in.Copy()
	out.Id = r.Id

	w.WriteMsg(out)
}

func (d *DNSProxy) RunServer() {
	log.Info().Msgf("DNS proxy listening on %s", viper.GetString("dns.listen"))

	// Start the background task to maintain the cache
	d.maintainCache()

	server := &dns.Server{Addr: viper.GetString("dns.listen"), Net: "udp"}
	dns.Handle(".", d)

	err := server.ListenAndServe()
	defer server.Shutdown()

	if err != nil {
		log.Fatal().Msgf("Failed to start DNS server: %s\n ", err.Error())
	}
}
