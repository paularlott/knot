package driver_mysql

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/paularlott/knot/util"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type MySQLDriver struct {
	connection *sql.DB
}

// Performs the real connection to the database, we use this to reconnect if the database moves to a new server etc.
func (db *MySQLDriver) realConnect() error {
	log.Debug().Msg("db: connecting to MySQL")

	host := viper.GetString("server.mysql.host")
	port := viper.GetInt("server.mysql.port")

	// If the host starts with srv+ then lookup the SRV record
	if host[:4] == "srv+" {
		for i := 0; i < 10; i++ {
			hostPort, err := util.LookupSRV(host[4:])
			if err != nil {
				if i == 9 {
					log.Fatal().Err(err).Msg("db: failed to lookup SRV record for MySQL database aborting after 10 attempts")
				} else {
					log.Error().Err(err).Msg("db: failed to lookup SRV record for MySQL database")
				}
				time.Sleep(3 * time.Second)
				continue
			}

			host = (*hostPort)[0].Host
			port, err = strconv.Atoi((*hostPort)[0].Port)
			if err != nil {
				log.Fatal().Err(err).Msg("db: failed to convert MySQL port to integer")
			}

			break
		}
	}

	var err error
	db.connection, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		viper.GetString("server.mysql.user"),
		viper.GetString("server.mysql.password"),
		host,
		port,
		viper.GetString("server.mysql.database"),
	))
	if err == nil {
		db.connection.SetConnMaxLifetime(time.Minute * time.Duration(viper.GetInt("server.mysql.connection_max_lifetime")))
		db.connection.SetMaxOpenConns(viper.GetInt("server.mysql.connection_max_open"))
		db.connection.SetMaxIdleConns(viper.GetInt("server.mysql.connection_max_idle"))

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
		for {
			time.Sleep(10 * time.Second)

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

	return err
}
