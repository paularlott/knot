package driver_mysql

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveSession(session *model.Session) error {

  tx, err := db.connection.Begin()
  if err != nil {
    return err
  }

  // Calculate the expiration time as now + 2 hours
  session.ExpiresAfter = time.Now().UTC().Add(time.Hour * 2)

  values, err := json.Marshal(session.Values)
  if err != nil {
      return err
  }

  // Assume update
  result, err := tx.Exec("UPDATE sessions SET data=?, expires_after=? WHERE session_id=?", string(values), session.ExpiresAfter.UTC(), session.Id)
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO sessions (session_id, data, expires_after, ip, user_id, user_agent) VALUES (?, ?, ?, ?, ?, ?)", session.Id, string(values), session.ExpiresAfter.UTC(), session.Ip, session.UserId, session.UserAgent)
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteSession(session *model.Session) error {
  _, err := db.connection.Exec("DELETE FROM sessions WHERE session_id = ?", session.Id)
  return err
}

func (db *MySQLDriver) GetSession(id string) (*model.Session, error) {
  var session model.Session
  var expiresAfter string
  var values string

  row := db.connection.QueryRow("SELECT session_id, data, expires_after, ip, user_id, user_agent FROM sessions WHERE session_id = ?", id)
  if row == nil {
    return nil, fmt.Errorf("user not found")
  }

  err := row.Scan(&session.Id, &values, &expiresAfter, &session.Ip, &session.UserId, &session.UserAgent)
  if err != nil {
    return nil, err
  }

  // Parse the values
  err = json.Unmarshal([]byte(values), &session.Values)
  if err != nil {
    return nil, err
  }

  // Parse the dates
  session.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
  if err != nil {
    return nil, err
  }

  return &session, nil
}

func (db *MySQLDriver) GetSessions(userId string) ([]*model.Session, error) {
  var sessions []*model.Session

  rows, err := db.connection.Query("SELECT session_id, data, expires_after, ip, user_id, user_agent FROM sessions WHERE user_id = ?", userId)
  if err != nil {
    return nil, err
  }

  for rows.Next() {
    var session model.Session
    var expiresAfter string
    var values string

    err := rows.Scan(&session.Id, &values, &expiresAfter, &session.Ip, &session.UserId, &session.UserAgent)
    if err != nil {
      return nil, err
    }

    // Parse the values
    err = json.Unmarshal([]byte(values), &session.Values)
    if err != nil {
      return nil, err
    }

    // Parse the dates
    session.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
    if err != nil {
      return nil, err
    }

    sessions = append(sessions, &session)
  }

  return sessions, nil
}
