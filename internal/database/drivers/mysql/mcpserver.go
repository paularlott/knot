package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveMCPServer(server *model.MCPServer, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM mcp_servers WHERE mcp_server_id=?)", server.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if doUpdate {
		err = db.update("mcp_servers", server, updateFields)
	} else {
		err = db.create("mcp_servers", server)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func (db *MySQLDriver) DeleteMCPServer(server *model.MCPServer) error {
	_, err := db.connection.Exec("DELETE FROM mcp_servers WHERE mcp_server_id = ?", server.Id)
	return err
}

func (db *MySQLDriver) GetMCPServer(id string) (*model.MCPServer, error) {
	var servers []*model.MCPServer

	err := db.read("mcp_servers", &servers, nil, "mcp_server_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("mcp server not found")
	}

	return servers[0], nil
}

func (db *MySQLDriver) GetMCPServers() ([]*model.MCPServer, error) {
	var servers []*model.MCPServer

	err := db.read("mcp_servers", &servers, nil, "1 ORDER BY namespace")
	return servers, err
}

func (db *MySQLDriver) GetMCPServersByUser(userId string) ([]*model.MCPServer, error) {
	var servers []*model.MCPServer

	err := db.read("mcp_servers", &servers, nil, "user_id = ? ORDER BY namespace", userId)
	return servers, err
}
