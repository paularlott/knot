package driver_mysql

import (
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
	now := time.Now().UTC()
	session.ExpiresAfter = now.Add(time.Hour * 2)

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE session_id=?)", session.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		err = db.update("sessions", session, []string{"ExpiresAfter"})
	} else {
		err = db.create("sessions", session)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteSession(session *model.Session) error {
	_, err := db.connection.Exec("DELETE FROM sessions WHERE session_id = ?", session.Id)
	return err
}

func (db *MySQLDriver) GetSession(id string) (*model.Session, error) {
	var sessions []*model.Session

	err := db.read("sessions", &sessions, nil, "session_id = ?", id)
	if err != nil || len(sessions) == 0 {
		return nil, err
	}

	return sessions[0], nil
}

func (db *MySQLDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
	var sessions []*model.Session

	err := db.read("sessions", &sessions, nil, "user_id = ?", userId)
	if err != nil {
		return nil, err
	}

	return sessions, nil
}

func (db *MySQLDriver) GetSessions() ([]*model.Session, error) {
	var sessions []*model.Session

	err := db.read("sessions", &sessions, nil, "1")
	if err != nil {
		return nil, err
	}

	return sessions, nil
}
