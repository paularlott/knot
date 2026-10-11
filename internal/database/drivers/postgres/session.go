package driver_postgres

import (
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// Sessions are kept in PostgreSQL so every server in a zone sees the same
// sessions without Redis. Expired rows are hidden from reads and removed by
// the garbage collector.

func (db *PostgresDriver) SaveSession(session *model.Session) error {
	_, err := db.connection.Exec(`
		INSERT INTO sessions (session_id, ip, user_id, original_user_id, user_agent, expires_after, updated_at, is_deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET
			ip = EXCLUDED.ip,
			user_id = EXCLUDED.user_id,
			original_user_id = EXCLUDED.original_user_id,
			user_agent = EXCLUDED.user_agent,
			expires_after = EXCLUDED.expires_after,
			updated_at = EXCLUDED.updated_at,
			is_deleted = EXCLUDED.is_deleted`,
		session.Id, session.Ip, session.UserId, session.OriginalUserId, session.UserAgent, session.ExpiresAfter.UTC(), session.UpdatedAt, session.IsDeleted)
	return err
}

func (db *PostgresDriver) DeleteSession(session *model.Session) error {
	_, err := db.connection.Exec("DELETE FROM sessions WHERE session_id = ?", session.Id)
	return err
}

// GetSession returns nil, with no error, for a session that doesn't exist or
// has expired, as the other session stores do.
func (db *PostgresDriver) GetSession(id string) (*model.Session, error) {
	var sessions []*model.Session
	err := db.read("sessions", &sessions, nil, "session_id = ? AND expires_after > ?", id, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return sessions[0], nil
}

func (db *PostgresDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
	var sessions []*model.Session
	err := db.read("sessions", &sessions, nil, "user_id = ? AND expires_after > ?", userId, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (db *PostgresDriver) GetSessions() ([]*model.Session, error) {
	var sessions []*model.Session
	err := db.read("sessions", &sessions, nil, "expires_after > ?", time.Now().UTC())
	if err != nil {
		return nil, err
	}
	return sessions, nil
}
