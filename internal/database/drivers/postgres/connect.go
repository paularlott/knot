package driver_postgres

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/logger"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

const (
	healthCheckInterval = 10 * time.Second
	gcInterval          = 1 * time.Hour
	garbageMaxAge       = 3 * 24 * time.Hour
)

type PostgresDriver struct {
	connection *pgConn
	logger     logger.Logger
}

// Performs the real connection to the database, we use this to reconnect if the database moves to a new server etc.
func (db *PostgresDriver) realConnect() error {
	db.logger.Debug("connecting to PostgreSQL")

	cfg := config.GetServerConfig()
	host := cfg.Postgres.Host
	port := cfg.Postgres.Port

	// If the host starts with srv+ then lookup the SRV record
	if len(host) > 4 && host[:4] == "srv+" {
		for i := 0; i < 10; i++ {
			hostPort, err := dns.LookupSRV(host[4:])
			if err != nil {
				if i == 9 {
					db.logger.Fatal("failed to lookup SRV record for PostgreSQL database aborting after 10 attempts", "error", err)
				} else {
					db.logger.WithError(err).Error("failed to lookup SRV record for PostgreSQL database")
				}
				time.Sleep(3 * time.Second)
				continue
			}

			host = hostPort[0].IP.String()
			port = hostPort[0].Port

			break
		}
	}

	query := url.Values{}
	query.Set("sslmode", cfg.Postgres.SSLMode)
	query.Set("timezone", "UTC")
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.Postgres.User, cfg.Postgres.Password),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + cfg.Postgres.Database,
		RawQuery: query.Encode(),
	}

	conn, err := sql.Open("pgx", dsn.String())
	if err == nil {
		conn.SetConnMaxLifetime(time.Minute * time.Duration(cfg.Postgres.ConnectionMaxLifetime))
		conn.SetMaxOpenConns(cfg.Postgres.ConnectionMaxOpen)
		conn.SetMaxIdleConns(cfg.Postgres.ConnectionMaxIdle)
		db.connection = &pgConn{DB: conn}

		db.logger.Debug("connected to PostgreSQL")
	} else {
		db.logger.WithError(err).Error("failed to connect to PostgreSQL")
	}

	return err
}

func (db *PostgresDriver) Connect() error {
	db.logger = log.WithGroup("db")
	err := db.realConnect()
	if err == nil {
		err := db.initialize()
		if err != nil {
			db.logger.Fatal("failed to initialize PostgreSQL database", "error", err)
		}
	}

	// Start a go routine to monitor the database
	go func() {
		internal := time.NewTicker(healthCheckInterval)
		defer internal.Stop()

		for range internal.C {
			db.logger.Debug("testing PostgreSQL connection")

			// Ping the database
			err := db.connection.Ping()
			if err != nil {
				db.logger.WithError(err).Error("failed to ping PostgreSQL database")
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
			db.logger.Debug("running garbage collector")

			before := time.Now().UTC().Add(-garbageMaxAge)
			beforeHLC := hlc.FromTime(before).Uint64()

			for _, table := range []string{"groups", "roles", "spaces", "templates", "templatevars", "users", "tokens", "volumes", "scripts", "responses", "sessions"} {
				_, err := db.connection.Exec(fmt.Sprintf("DELETE FROM %s WHERE is_deleted AND updated_at < ?", quoteIdent(table)), beforeHLC)
				if err != nil {
					db.logger.WithError(err).Error("failed to delete old rows", "table", table)
				}
			}
		}
	}()

	return err
}
