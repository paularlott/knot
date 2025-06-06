package driver_memory

import (
	"sync"
	"time"

	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
)

type MemoryDbDriver struct {
	sessionMutex     *sync.RWMutex
	sessions         map[string]*model.Session
	sessionsByUserId map[string][]*model.Session
}

func (db *MemoryDbDriver) Connect() {
	log.Debug().Msg("db: starting memory driver")

	// Initialize the mutexes and maps
	db.sessionMutex = &sync.RWMutex{}
	db.sessions = make(map[string]*model.Session)
	db.sessionsByUserId = make(map[string][]*model.Session)

	// Add a task to clean up expired sessions
	log.Debug().Msg("db: starting session GC")
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			log.Debug().Msg("db: running GC")
			now := time.Now().UTC()

			// Sweep sessions
			db.sessionMutex.Lock()
			for id, session := range db.sessions {
				if session.ExpiresAfter.Before(now) {
					log.Debug().Msg("db: removing session " + session.Id)

					delete(db.sessions, id)
					for i, s := range db.sessionsByUserId[session.UserId] {
						if s.Id == session.Id {
							db.sessionsByUserId[session.UserId] = append(db.sessionsByUserId[session.UserId][:i], db.sessionsByUserId[session.UserId][i+1:]...)
							break
						}
					}
				}
			}
			db.sessionMutex.Unlock()
		}
	}()
}
