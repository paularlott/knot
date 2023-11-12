package driver_mysql

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type MySQLDriver struct{
  connection *sql.DB
}

func (db *MySQLDriver) Connect() error {
  log.Debug().Msg("Connecting to MySQL")

  var err error
  db.connection, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
    viper.GetString("server.mysql.user"),
    viper.GetString("server.mysql.password"),
    viper.GetString("server.mysql.host"),
    viper.GetInt("server.mysql.port"),
    viper.GetString("server.mysql.database"),
  ))
  if err == nil {
    db.connection.SetConnMaxLifetime(time.Minute * time.Duration(viper.GetInt("server.mysql.connection_max_lifetime")))
    db.connection.SetMaxOpenConns(viper.GetInt("server.mysql.connection_max_open"))
    db.connection.SetMaxIdleConns(viper.GetInt("server.mysql.connection_max_idle"))

    err := db.initialize()
    if err != nil {
      log.Fatal().Err(err).Msg("Failed to initialize MySQL database")
    }
  }

  return err
}
