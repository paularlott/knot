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
  token.ExpiresAfter = time.Now().UTC().Add(time.Hour * 168)

  // Assume update
  result, err := tx.Exec("UPDATE tokens SET expires_after=?, name=? WHERE token_id=?", token.ExpiresAfter.UTC(), token.Name, token.Id)
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO tokens (token_id, name, expires_after, user_id) VALUES (?, ?, ?, ?)", token.Id, token.Name, token.ExpiresAfter.UTC(), token.UserId)
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteToken(token *model.Token) error {
  _, err := db.connection.Exec("DELETE FROM tokens WHERE token_id = ?", token.Id)
  return err
}

func (db *MySQLDriver) GetToken(id string) (*model.Token, error) {
  var token model.Token
  var expiresAfter string

  row := db.connection.QueryRow("SELECT token_id, name, expires_after,user_id FROM tokens WHERE token_id = ?", id)
  if row == nil {
    return nil, fmt.Errorf("token not found")
  }

  err := row.Scan(&token.Id, &token.Name, &expiresAfter, &token.UserId)
  if err != nil {
    return nil, err
  }

  // Parse the dates
  token.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
  if err != nil {
    return nil, err
  }

  return &token, nil
}

func (db *MySQLDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
  var tokens []*model.Token

  rows, err := db.connection.Query("SELECT token_id, name, expires_after,user_id FROM tokens WHERE user_id = ?", userId)
  if err != nil {
    return nil, err
  }

  for rows.Next() {
    var token model.Token
    var expiresAfter string

    err := rows.Scan(&token.Id, &token.Name, &expiresAfter, &token.UserId)
    if err != nil {
      return nil, err
    }

    // Parse the dates
    token.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
    if err != nil {
      return nil, err
    }

    tokens = append(tokens, &token)
  }

  return tokens, nil
}
