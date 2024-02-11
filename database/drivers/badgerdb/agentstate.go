package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveAgentState(state *model.AgentState) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    // Calculate the expiration time
    state.ExpiresAfter = time.Now().UTC().Add(model.AGENT_STATE_TIMEOUT)

    data, err := json.Marshal(state)
    if err != nil {
      return err
    }

    e := badger.NewEntry([]byte(fmt.Sprintf("AgentState:%s", state.Id)), data).WithTTL(model.AGENT_STATE_TIMEOUT)
    if err = txn.SetEntry(e); err != nil {
      return err
    }

    return nil
  })

  return err
}

func (db *BadgerDbDriver) DeleteAgentState(state *model.AgentState) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    err := txn.Delete([]byte(fmt.Sprintf("AgentState:%s", state.Id)))
    if err != nil {
      return err
    }

    return nil
  })

  return err
}

func (db *BadgerDbDriver) GetAgentState(id string) (*model.AgentState, error) {
  var state = &model.AgentState{}

  err := db.connection.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte(fmt.Sprintf("AgentState:%s", id)))
    if err != nil {
      return err
    }

    return item.Value(func(val []byte) error {
      return json.Unmarshal(val, state)
    })
  })

  if err != nil {
    return nil, err
  }

  return state, err
}
