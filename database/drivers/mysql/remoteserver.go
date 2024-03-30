package driver_mysql

import (
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveRemoteServer(server *model.RemoteServer) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Work out the expire time, now + AGENT_STATE_TIMEOUT
	server.ExpiresAfter = time.Now().UTC().Add(model.REMOTE_SERVER_TIMEOUT)

	// Assume update
	result, err := tx.Exec("UPDATE remoteservers SET url=?, expires_after=? WHERE server_id=?",
		server.Url, server.ExpiresAfter, server.Id,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	// If no rows were updated then do an insert
	if rows, _ := result.RowsAffected(); rows == 0 {
		_, err = tx.Exec("INSERT INTO remoteservers (server_id, url, expires_after) VALUES (?, ?, ?)",
			server.Id, server.Url, server.ExpiresAfter,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteRemoteServer(server *model.RemoteServer) error {
	_, err := db.connection.Exec("DELETE FROM remoteservers WHERE server_id = ?", server.Id)
	return err
}

func (db *MySQLDriver) getRemoteServers(query string, args ...interface{}) ([]*model.RemoteServer, error) {
	var servers []*model.RemoteServer

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var server = &model.RemoteServer{}
		var expiresAfter string

		err := rows.Scan(&server.Id, &server.Url, &expiresAfter)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		server.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
		if err != nil {
			return nil, err
		}

		servers = append(servers, server)
	}

	return servers, nil
}

func (db *MySQLDriver) GetRemoteServer(id string) (*model.RemoteServer, error) {
	servers, err := db.getRemoteServers("SELECT server_id, url, expires_after FROM remoteservers WHERE server_id = ?", id)
	if err != nil || len(servers) == 0 {
		return nil, err
	}
	return servers[0], nil
}

func (db *MySQLDriver) GetRemoteServers() ([]*model.RemoteServer, error) {
	return db.getRemoteServers("SELECT server_id, url, expires_after FROM remoteservers")
}
