package driver_mysql

import (
	"encoding/json"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveAgentState(state *model.AgentState) error {

  tx, err := db.connection.Begin()
  if err != nil {
    return err
  }

  // Work out the expire time, now + AGENT_STATE_TIMEOUT
  state.ExpiresAfter = time.Now().UTC().Add(model.AGENT_STATE_TIMEOUT)

  // JSON encode tcp & http ports
  tcpPorts, _ := json.Marshal(state.TcpPorts)
  httpPorts, _ := json.Marshal(state.HttpPorts)

  // Assume update
  result, err := tx.Exec("UPDATE agentstate SET access_token=?, has_code_server=?, ssh_port=?, vnc_http_port=?, has_terminal=?, tcp_ports=?, http_ports=?, expires_after=? WHERE space_id=?",
    state.AccessToken, state.HasCodeServer, state.SSHPort, state.VNCHttpPort, state.HasTerminal, tcpPorts, httpPorts, state.ExpiresAfter, state.Id,
  )
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO agentstate (space_id, access_token, has_code_server, ssh_port, vnc_http_port, has_terminal, tcp_ports, http_ports, expires_after) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
      state.Id, state.AccessToken, state.HasCodeServer, state.SSHPort, state.VNCHttpPort, state.HasTerminal, tcpPorts, httpPorts, state.ExpiresAfter,
    )
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteAgentState(state *model.AgentState) error {
  _, err := db.connection.Exec("DELETE FROM agentstate WHERE space_id = ?", state.Id)
  return err
}

func (db *MySQLDriver) getAgentState(query string, args ...interface{}) ([]*model.AgentState, error) {
  var states []*model.AgentState

  rows, err := db.connection.Query(query, args...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  for rows.Next() {
    var state = &model.AgentState{}
    var expiresAfter string
    var tcpPorts string
    var httpPorts string

    err := rows.Scan(&state.Id, &state.AccessToken, &state.HasCodeServer, &state.SSHPort, &state.VNCHttpPort, &state.HasTerminal, &tcpPorts, &httpPorts, &expiresAfter)
    if err != nil {
      return nil, err
    }

    // Parse the dates
    state.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
    if err != nil {
      return nil, err
    }

    // Parse the poers
    err = json.Unmarshal([]byte(tcpPorts), &state.TcpPorts)
    if err != nil {
      return nil, err
    }
    err = json.Unmarshal([]byte(httpPorts), &state.HttpPorts)
    if err != nil {
      return nil, err
    }

    states = append(states, state)
  }

  return states, nil
}

func (db *MySQLDriver) GetAgentState(id string) (*model.AgentState, error) {
  states, err := db.getAgentState("SELECT space_id, access_token, has_code_server, ssh_port, vnc_http_port, has_terminal, tcp_ports, http_ports, expires_after FROM agentstate WHERE space_id = ?", id)
  if err != nil || len(states) == 0 {
    return nil, err
  }
  return states[0], nil
}
