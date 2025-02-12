package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveToken(token *model.Token) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Calculate the expiration time as now + 1 week
	now := time.Now().UTC()
	token.ExpiresAfter = now.Add(time.Hour * 168)

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM tokens WHERE token_id=?)", token.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		_, err = tx.Exec("UPDATE tokens SET expires_after=?, name=?, session_id=? WHERE token_id=?", token.ExpiresAfter.UTC(), token.Name, token.SessionId, token.Id)
	} else {
		_, err = tx.Exec("INSERT INTO tokens (token_id, name, expires_after, user_id, session_id) VALUES (?, ?, ?, ?, ?)", token.Id, token.Name, token.ExpiresAfter.UTC(), token.UserId, token.SessionId)
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

func (db *MySQLDriver) getTokens(query string, args ...interface{}) ([]*model.Token, error) {
	var tokens []*model.Token

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var token = &model.Token{}
		var expiresAfter string

		err := rows.Scan(&token.Id, &token.Name, &expiresAfter, &token.UserId, &token.SessionId)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		token.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
	}

	return tokens, nil
}

func (db *MySQLDriver) GetToken(id string) (*model.Token, error) {
	tokens, err := db.getTokens("SELECT token_id, name, expires_after,user_id,session_id FROM tokens WHERE token_id = ?", id)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("token not found")
	}
	if err != nil {
		return nil, err
	}

	return tokens[0], nil
}

func (db *MySQLDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
	tokens, err := db.getTokens("SELECT token_id, name, expires_after,user_id,session_id FROM tokens WHERE user_id = ?", userId)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}
