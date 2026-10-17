package database

import (
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// dataMigrations rewrite stored records once, after each driver's own
// schema migrations, for changes no SQL statement or schema can make in
// every driver alike. Each runs once per database, recorded under its name
// in the cfg values. Never rename or remove an entry, only append.
var dataMigrations = []struct {
	name string
	run  func(db DbDriver) error
}{
	{"data_migration.drop_provider_tokens", dropProviderTokens},
}

func runDataMigrations(db DbDriver) {
	logger := log.WithGroup("db")
	for _, m := range dataMigrations {
		if done, err := db.GetCfgValue(m.name); err == nil && done != nil && done.Value != "" {
			continue
		}
		logger.Debug("applying data migration", "name", m.name)
		if err := m.run(db); err != nil {
			logger.Error("data migration failed, will retry at next start", "name", m.name, "error", err)
			continue
		}
		if err := db.SaveCfgValue(&model.CfgValue{Name: m.name, Value: "done"}); err != nil {
			logger.Error("failed to record data migration", "name", m.name, "error", err)
		}
	}
}

// dropProviderTokens rewrites every account with a linked sign-in provider,
// which drops the provider access and refresh tokens earlier versions kept
// with it: the model no longer has them, so they aren't written back.
func dropProviderTokens(db DbDriver) error {
	users, err := db.GetUsers()
	if err != nil {
		return err
	}
	for _, user := range users {
		if len(user.ExternalAuthProviders) == 0 {
			continue
		}
		if err := db.SaveUser(user, []string{"ExternalAuthProviders"}); err != nil {
			return err
		}
	}
	return nil
}
