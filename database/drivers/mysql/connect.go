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

type MySQLDriver struct{
  connection *sql.DB
}

func (db *MySQLDriver) Connect() error {
  log.Debug().Msg("db: connecting to MySQL")

  host := viper.GetString("server.mysql.host")
  port := viper.GetInt("server.mysql.port")

  // If the host starts with srv+ then lookup the SRV record
  if host[:4] == "srv+" {
    hostSrv, portSrv, err := util.GetTargetFromSRV(host[4:], viper.GetString("server.nameserver"))
    if err != nil {
      log.Fatal().Err(err).Msg("db: failed to lookup SRV record for MySQL database")
    }

    host = hostSrv
    port, err = strconv.Atoi(portSrv)
    if err != nil {
      log.Fatal().Err(err).Msg("db: failed to convert MySQL port to integer")
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

    err := db.initialize()
    if err != nil {
      log.Fatal().Err(err).Msg("db: failed to initialize MySQL database")
    }
  }

  return err
}
