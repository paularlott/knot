package dnsproxy

import (
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (d *DNSProxy) maintainCache() {
	go func() {
		for {
			time.Sleep(1 * time.Second)

			d.cacheMutex.Lock()
			for k, v := range d.cache {

				// If the TTL is 0 then remove the entry unless it's being refreshed
				if v.reply.Answer[0].Header().Ttl == 0 {
					if v.refreshing {
						log.Debug().Msgf("Cache entry %s IN %s is being refreshed keeping current record", v.request.Question[0].Name, dns.TypeToString[v.request.Question[0].Qtype])
						continue
					}

					log.Debug().Msgf("Removing cache entry %s IN %s", v.request.Question[0].Name, dns.TypeToString[v.request.Question[0].Qtype])
					delete(d.cache, k)
				} else {
					// Reduce the TTL of the entry by 1 second
					for _, a := range v.reply.Answer {
						a.Header().Ttl--
					}
				}

				// If the TTL has reached the refresh threshold and entry has been used recently
				// then refresh the entry in the background
				if v.reply.Answer[0].Header().Ttl == BACKGROUND_REFRESH_AT && time.Since(v.lastAccess) < time.Duration(viper.GetUint16("dns.refresh_max_age"))*time.Second {
					log.Debug().Msgf("Refresh cache entry %s IN %s", v.request.Question[0].Name, dns.TypeToString[v.request.Question[0].Qtype])

					// Update the request Id
					v.request.Id++
					v.refreshing = true
					go d.queryAndCache(v.request, v)
				}
			}
			d.cacheMutex.Unlock()
		}
	}()
}
