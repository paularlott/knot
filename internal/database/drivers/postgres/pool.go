package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *PostgresDriver) SavePoolDefinition(pool *model.PoolDefinition, updateFields []string) error {
	var doUpdate bool
	err := db.connection.QueryRow("SELECT EXISTS(SELECT 1 FROM pools WHERE pool_id=?)", pool.Id).Scan(&doUpdate)
	if err != nil {
		return err
	}

	if doUpdate {
		return db.update("pools", pool, updateFields)
	}
	return db.create("pools", pool)
}

func (db *PostgresDriver) DeletePoolDefinition(pool *model.PoolDefinition) error {
	_, err := db.connection.Exec("DELETE FROM pools WHERE pool_id = ?", pool.Id)
	return err
}

func (db *PostgresDriver) GetPoolDefinition(id string) (*model.PoolDefinition, error) {
	var pools []*model.PoolDefinition
	err := db.read("pools", &pools, nil, "pool_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("pool not found")
	}
	return pools[0], nil
}

func (db *PostgresDriver) GetPoolDefinitionByName(userId, name string) (*model.PoolDefinition, error) {
	var pools []*model.PoolDefinition
	err := db.read("pools", &pools, nil, "created_user_id = ? AND name = ? AND is_deleted = FALSE", userId, name)
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("pool not found")
	}
	return pools[0], nil
}

func (db *PostgresDriver) GetPoolDefinitions() ([]*model.PoolDefinition, error) {
	var pools []*model.PoolDefinition
	err := db.read("pools", &pools, nil, "1 = 1 ORDER BY name")
	return pools, err
}

// GetPoolDefinitionsByUser returns the live pools owned by a user. Uses the
// leading column of the created_user_name unique index, so no separate index
// is required.
func (db *PostgresDriver) GetPoolDefinitionsByUser(userId string) ([]*model.PoolDefinition, error) {
	var pools []*model.PoolDefinition
	err := db.read("pools", &pools, nil, "created_user_id = ? AND is_deleted = FALSE ORDER BY name", userId)
	return pools, err
}
