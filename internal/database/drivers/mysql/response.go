package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveResponse(response *model.Response) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM responses WHERE response_id=?)", response.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update or create
	if doUpdate {
		err = db.update("responses", response, nil)
	} else {
		err = db.create("responses", response)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteResponse(response *model.Response) error {
	_, err := db.connection.Exec("DELETE FROM responses WHERE response_id = ?", response.Id)
	return err
}

func (db *MySQLDriver) GetResponse(id string) (*model.Response, error) {
	var responses []*model.Response

	err := db.read("responses", &responses, nil, "response_id = ?", id)
	if err != nil {
		return nil, err
	}

	if len(responses) == 0 {
		return nil, fmt.Errorf("response not found")
	}

	return responses[0], nil
}

func (db *MySQLDriver) GetResponses() ([]*model.Response, error) {
	var responses []*model.Response

	err := db.read("responses", &responses, nil, "1 ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}

	return responses, nil
}

func (db *MySQLDriver) GetResponsesByUser(userId string) ([]*model.Response, error) {
	var responses []*model.Response

	err := db.read("responses", &responses, nil, "user_id = ? ORDER BY created_at DESC", userId)
	if err != nil {
		return nil, err
	}

	return responses, nil
}

func (db *MySQLDriver) GetResponsesByStatus(status model.ResponseStatus) ([]*model.Response, error) {
	var responses []*model.Response

	err := db.read("responses", &responses, nil, "status = ? ORDER BY created_at ASC", status)
	if err != nil {
		return nil, err
	}

	return responses, nil
}
