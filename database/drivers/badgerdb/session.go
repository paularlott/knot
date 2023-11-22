package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveSession(session *model.Session) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    // Calculate the expiration time as now + 2 hours
    session.ExpiresAfter = time.Now().UTC().Add(time.Hour * 2)

    data, err := json.Marshal(session)
    if err != nil {
      return err
    }

    e := badger.NewEntry([]byte(fmt.Sprintf("Sessions:%s", session.Id)), data).WithTTL(time.Hour * 2)
    if err = txn.SetEntry(e); err != nil {
      return err
    }

    e = badger.NewEntry([]byte(fmt.Sprintf("SessionsByUserId:%s:%s", session.UserId, session.Id)), []byte(session.Id)).WithTTL(time.Hour * 2)
    if err = txn.SetEntry(e); err != nil {
      return err
    }

    return nil
  })

  return err
}

func (db *BadgerDbDriver) DeleteSession(session *model.Session) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    err := txn.Delete([]byte(fmt.Sprintf("Sessions:%s", session.Id)))
    if err != nil {
      return err
    }

    err = txn.Delete([]byte(fmt.Sprintf("SessionsByUserId:%s:%s", session.UserId, session.Id)))
    if err != nil {
      return err
    }

    return nil
  })

  return err
}

func (db *BadgerDbDriver) GetSession(id string) (*model.Session, error) {
  var session = &model.Session{}

  err := db.connection.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte(fmt.Sprintf("Sessions:%s", id)))
    if err != nil {
      return err
    }

    return item.Value(func(val []byte) error {
      return json.Unmarshal(val, session)
    })
  })

  if err != nil {
    return nil, err
  }

  return session, err
}

func (db *BadgerDbDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
  var sessions []*model.Session

  err := db.connection.View(func(txn *badger.Txn) error {
    it := txn.NewIterator(badger.DefaultIteratorOptions)
    defer it.Close()

    prefix := []byte(fmt.Sprintf("SessionsByUserId:%s:", userId))
    for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
      item := it.Item()

      var sessionId string
      err := item.Value(func(val []byte) error {
        sessionId = string(val)
        return nil
      })
      if err != nil {
        return err
      }

      session, err := db.GetSession(sessionId)
      if err != nil {
        return err
      }

      sessions = append(sessions, session)
    }

    return nil
  })

  return sessions, err
}
