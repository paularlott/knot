package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *PostgresDriver) SaveUser(user *model.User, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE user_id=?)", user.Id).Scan(&doUpdate)
	if err != nil {
		return err
	}

	// Determine whether ExternalAuthProviders is changing
	updatingProviders := len(updateFields) == 0 || util.InArray(updateFields, "ExternalAuthProviders")

	var oldProviders map[string]model.ExternalProvider
	if doUpdate && updatingProviders {
		// Load old providers so we can diff the index
		var rows []model.User
		if e := db.read("users", &rows, []string{"ExternalAuthProviders"}, "user_id=?", user.Id); e == nil && len(rows) > 0 {
			oldProviders = rows[0].ExternalAuthProviders
		}
	}

	if doUpdate {
		err = db.update("users", user, updateFields)
	} else {
		err = db.create("users", user)
	}
	if err != nil {
		return err
	}

	// Maintain provider index
	if updatingProviders {
		// Remove stale index rows
		for providerID, ep := range oldProviders {
			if newEp, ok := user.ExternalAuthProviders[providerID]; !ok || newEp.ProviderUID != ep.ProviderUID {
				_, err = tx.Exec("DELETE FROM user_providers WHERE provider_id=? AND provider_uid=?", providerID, ep.ProviderUID)
				if err != nil {
					return err
				}
			}
		}
		// Upsert current index rows
		for providerID, ep := range user.ExternalAuthProviders {
			_, err = tx.Exec(
				"INSERT INTO user_providers (provider_id, provider_uid, user_id) VALUES (?,?,?) ON CONFLICT (provider_id, provider_uid) DO UPDATE SET user_id=EXCLUDED.user_id",
				providerID, ep.ProviderUID, user.Id,
			)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (db *PostgresDriver) DeleteUser(user *model.User) error {
	if _, err := db.connection.Exec("DELETE FROM scripts WHERE user_id = ?", user.Id); err != nil {
		return err
	}
	if _, err := db.connection.Exec("DELETE FROM user_providers WHERE user_id = ?", user.Id); err != nil {
		return err
	}
	_, err := db.connection.Exec("DELETE FROM users WHERE user_id = ?", user.Id)
	return err
}

func (db *PostgresDriver) GetUser(id string) (*model.User, error) {
	var users []model.User
	err := db.read("users", &users, nil, "user_id=?", id)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (db *PostgresDriver) GetUserByEmail(email string) (*model.User, error) {
	var users []model.User
	err := db.read("users", &users, nil, "lower(email) = lower(?)", email)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (db *PostgresDriver) GetUserByUsername(name string) (*model.User, error) {
	var users []model.User
	err := db.read("users", &users, nil, "lower(username) = lower(?)", name)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (db *PostgresDriver) GetUserByProviderUID(providerID, providerUID string) (*model.User, error) {
	var userId string
	err := db.connection.QueryRow(
		"SELECT user_id FROM user_providers WHERE provider_id=? AND provider_uid=?",
		providerID, providerUID,
	).Scan(&userId)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	return db.GetUser(userId)
}

func (db *PostgresDriver) GetUsers() ([]*model.User, error) {
	var users []*model.User
	err := db.read("users", &users, nil, "TRUE")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return users, nil
}

func (db *PostgresDriver) HasUsers() (bool, error) {
	var count int

	row := db.connection.QueryRow("SELECT COUNT(*) FROM users")
	if row == nil {
		return false, fmt.Errorf("failed to get user count")
	}

	err := row.Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
