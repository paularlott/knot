package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveToken(token *model.Token) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=?)", token.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		err = db.update("tokens", token, nil)
	} else {
		err = db.create("tokens", token)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteToken(token *model.Token) error {
	_, err := db.connection.Exec("DELETE FROM tokens WHERE token_id = ?", token.Id)
	return err
}

func (db *MySQLDriver) GetToken(id string) (*model.Token, error) {
	var tokens []*model.Token

	err := db.read("tokens", &tokens, nil, "token_id = ?", id)
	if err != nil || len(tokens) == 0 {
		return nil, fmt.Errorf("token not found")
	}
	if err != nil {
		return nil, err
	}

	return tokens[0], nil
}

func (db *MySQLDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
	var tokens []*model.Token

	err := db.read("tokens", &tokens, nil, "user_id = ?", userId)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}

func (db *MySQLDriver) GetTokens() ([]*model.Token, error) {
	var tokens []*model.Token

	err := db.read("tokens", &tokens, nil, "1")
	if err != nil {
		return nil, err
	}

	return tokens, nil
}
