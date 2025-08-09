package driver_mysql

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"
)

const (
	healthCheckInterval = 10 * time.Second
	gcInterval          = 1 * time.Hour
	garbageMaxAge       = 3 * 24 * time.Hour
)

type MySQLDriver struct {
	connection *sql.DB
}

// Performs the real connection to the database, we use this to reconnect if the database moves to a new server etc.
func (db *MySQLDriver) realConnect() error {
	log.Debug().Msg("db: connecting to MySQL")

	cfg := config.GetServerConfig()
	host := cfg.MySQL.Host
	port := cfg.MySQL.Port

	// If the host starts with srv+ then lookup the SRV record
	if host[:4] == "srv+" {
		for i := 0; i < 10; i++ {
			hostPort, err := dns.LookupSRV(host[4:])
			if err != nil {
				if i == 9 {
					log.Fatal().Err(err).Msg("db: failed to lookup SRV record for MySQL database aborting after 10 attempts")
				} else {
					log.Error().Err(err).Msg("db: failed to lookup SRV record for MySQL database")
				}
				time.Sleep(3 * time.Second)
				continue
			}

			host = hostPort[0].IP.String()
			port = hostPort[0].Port
			break
		}
	}

	var err error
	db.connection, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		cfg.MySQL.User,
		cfg.MySQL.Password,
		host,
		port,
		cfg.MySQL.Database,
	))
	if err == nil {
		db.connection.SetConnMaxLifetime(time.Minute * time.Duration(cfg.MySQL.ConnectionMaxLifetime))
		db.connection.SetMaxOpenConns(cfg.MySQL.ConnectionMaxOpen)
		db.connection.SetMaxIdleConns(cfg.MySQL.ConnectionMaxIdle)

		log.Debug().Msg("db: connected to MySQL")
	} else {
		log.Error().Err(err).Msg("db: failed to connect to MySQL")
	}

	return err
}

func (db *MySQLDriver) Connect() error {
	err := db.realConnect()
	if err == nil {
		err := db.initialize()
		if err != nil {
			log.Fatal().Err(err).Msg("db: failed to initialize MySQL database")
		}
	}

	// Start a go routine to monitor the database
	go func() {
		internal := time.NewTicker(healthCheckInterval)
		defer internal.Stop()

		for range internal.C {
			log.Debug().Msg("db: testing MySQL connection")

			// Ping the database
			err := db.connection.Ping()
			if err != nil {
				log.Error().Err(err).Msg("db: failed to ping MySQL database")
				db.connection.Close()

				// Attempt to reconnect
				db.realConnect()
			}
		}
	}()

	// Start a go routine to clear deleted items from the database
	go func() {
		intervalTimer := time.NewTicker(gcInterval)
		defer intervalTimer.Stop()

		for range intervalTimer.C {
			log.Debug().Msg("db: running garbage collector")

			before := time.Now().UTC()
			before = before.Add(-garbageMaxAge)

			// Remove old groups
			_, err := db.connection.Exec("DELETE FROM groups WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old groups")
			}

			// Remove old roles
			_, err = db.connection.Exec("DELETE FROM roles WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old roles")
			}

			// Remove old spaces
			_, err = db.connection.Exec("DELETE FROM spaces WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old spaces")
			}

			// Remove old templates
			_, err = db.connection.Exec("DELETE FROM templates WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old templates")
			}

			// Remove old template vars
			_, err = db.connection.Exec("DELETE FROM templatevars WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old template vars")
			}

			// Remove old users
			_, err = db.connection.Exec("DELETE FROM users WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old users")
			}

			// Remove old tokens
			_, err = db.connection.Exec("DELETE FROM tokens WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old tokens")
			}

			// Remove old volumes
			_, err = db.connection.Exec("DELETE FROM volumes WHERE is_deleted > 0 AND updated_at < ?", before)
			if err != nil {
				log.Error().Err(err).Msg("db: failed to delete old volumes")
			}
		}
	}()

	return err
}
