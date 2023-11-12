package driver_badgerdb

import (
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type BadgerDbDriver struct{
  connection *badger.DB
}

func (db *BadgerDbDriver) keyExists(key string) (bool, error) {
    var exists = false

    err := db.connection.View(func(txn *badger.Txn) error {
      item, err := txn.Get([]byte(key))
      if err == badger.ErrKeyNotFound {
        return nil
      }
      if err != nil {
        return err
      }

      exists = item != nil
      return nil
    })

    return exists, err
}

func (db *BadgerDbDriver) Connect() error {
  log.Debug().Msg("db: connecting to BadgerDB")

  var err error
  options := badger.DefaultOptions(viper.GetString("server.badgerdb.path"))
  options.Logger = badgerdbLogger()

  db.connection, err = badger.Open(options)
  if err == nil {

    // Start the garbage collector
    go func() {
      ticker := time.NewTicker(5 * time.Minute)
      defer ticker.Stop()
      for range ticker.C {
      again:
        fmt.Println("Running GC")
        err := db.connection.RunValueLogGC(0.7)
        if err == nil {
          goto again
        }
      }
    }()
  }

  return err
}
