package dnsproxy

import (
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (d *DNSProxy) maintainCache() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		// Channel for workers to listen on
		requestChan := make(chan *dnsCacheEntry, 100)

		// Start workers to refresh cache entries
		for i := 0; i < 5; i++ {
			go func() {
				for entry := range requestChan {
					log.Debug().Msgf("dnsproxy: worker refreshing cache entry %s IN %s", entry.request.Question[0].Name, dns.TypeToString[entry.request.Question[0].Qtype])
					d.queryAndCache(entry.request, entry)
				}
			}()
		}

		for range ticker.C {

			d.cacheMutex.Lock()
			for k, v := range d.cache {

				// If the TTL has reached the refresh threshold and entry has been used recently
				// then refresh the entry in the background
				if !v.refreshing && v.reply.Answer[0].Header().Ttl == BACKGROUND_REFRESH_AT && time.Since(v.lastAccess) < time.Duration(viper.GetUint16("dns.refresh_max_age"))*time.Second {
					log.Debug().Msgf("dnsproxy: refresh cache entry %s IN %s", v.request.Question[0].Name, dns.TypeToString[v.request.Question[0].Qtype])

					// Update the request Id
					v.request.Id++
					v.refreshing = true

					// Send the request to the worker pool without blocking
					select {
					case requestChan <- v:
					default:
						log.Warn().Msgf("dnsproxy: worker pool is full, dropping request for %s IN %s", v.request.Question[0].Name, dns.TypeToString[v.request.Question[0].Qtype])

						// Let the record expire
						v.refreshing = false
					}
				}

				// If the TTL is 0 then remove the entry unless it's being refreshed
				if v.reply.Answer[0].Header().Ttl == 0 {
					if !v.refreshing {
						log.Debug().Msgf("dnsproxy: removing cache entry %s IN %s", v.request.Question[0].Name, dns.TypeToString[v.request.Question[0].Qtype])
						delete(d.cache, k)
					}
				} else {
					// Reduce the TTL of the entry by 1 second
					for _, a := range v.reply.Answer {
						a.Header().Ttl--
					}

					// Reduce the TTL of any records in the Additional section by 1 second
					for _, a := range v.reply.Extra {
						a.Header().Ttl--
					}
				}
			}
			d.cacheMutex.Unlock()
		}
	}()
}
