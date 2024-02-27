package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/database/model"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveDbService(service *model.DbService) error {
  var err error

  err = db.connection.Update(func(txn *badger.Txn) error {

    // Load the existing service
    existingService, _ := db.GetDbService(service.Id)

    // Check if the name is unique
    if existingService != nil {
      exists, err := db.keyExists(fmt.Sprintf("DbServiceByName:%s", strings.ToLower(service.Name)))
      if err != nil {
        return err
      } else if exists {
        return fmt.Errorf("duplicate database service name")
      }
    } else {
      service.Name = existingService.Name
    }

    data, err := json.Marshal(service)
    if err != nil {
      return err
    }

    // Save the new service
    err = txn.Set([]byte(fmt.Sprintf("DbService:%s", service.Id)), data)
    if err != nil {
      return err
    }

    return nil
  })

  return err
}

func (db *BadgerDbDriver) DeleteDbService(service *model.DbService) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    err := txn.Delete([]byte(fmt.Sprintf("DbService:%s", service.Id)))
    if err != nil {
      return err
    }

    err = txn.Delete([]byte(fmt.Sprintf("DbServiceByName:%s", service.Name)))
    if err != nil {
      return err
    }

    return nil
  })

  return err
}

func (db *BadgerDbDriver) GetDbService(id string) (*model.DbService, error) {
  var service = &model.DbService{}

  err := db.connection.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte(fmt.Sprintf("DbService:%s", id)))
    if err != nil {
      return err
    }

    return item.Value(func(val []byte) error {
      return json.Unmarshal(val, service)
    })
  })

  if err != nil {
    return nil, err
  }

  return service, err
}

func (db *BadgerDbDriver) GetDbServiceByName(name string) (*model.DbService, error) {
  var service = &model.DbService{}

  err := db.connection.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte(fmt.Sprintf("DbServiceByName:%s", name)))
    if err != nil {
      return err
    }

    return item.Value(func(val []byte) error {
      return json.Unmarshal(val, service)
    })
  })

  if err != nil {
    return nil, err
  }

  return service, err
}

func (db *BadgerDbDriver) GetDbServices() ([]*model.DbService, error) {
  var services []*model.DbService

  err := db.connection.View(func(txn *badger.Txn) error {
    it := txn.NewIterator(badger.DefaultIteratorOptions)
    defer it.Close()

    prefix := []byte("DbService:")
    for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
      item := it.Item()

      var service = &model.DbService{}
      err := item.Value(func(val []byte) error {
        return json.Unmarshal(val, service)
      })
      if err != nil {
        return err
      }

      services = append(services, service)
    }

    return nil
  })

  // Sort the by name
  sort.Slice(services, func(i, j int) bool {
    return services[i].Name < services[j].Name
  })

  return services, err
}
