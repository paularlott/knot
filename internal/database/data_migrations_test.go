package database

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// fakeMigrationDB records the calls a data migration makes; the embedded
// interface is nil, so any other call fails the test with a panic.
type fakeMigrationDB struct {
	DbDriver
	cfg   map[string]string
	users []*model.User
	saved []string
}

func (f *fakeMigrationDB) GetCfgValue(name string) (*model.CfgValue, error) {
	if v, ok := f.cfg[name]; ok {
		return &model.CfgValue{Name: name, Value: v}, nil
	}
	return nil, nil
}

func (f *fakeMigrationDB) SaveCfgValue(v *model.CfgValue) error {
	f.cfg[v.Name] = v.Value
	return nil
}

func (f *fakeMigrationDB) GetUsers() ([]*model.User, error) { return f.users, nil }

func (f *fakeMigrationDB) SaveUser(u *model.User, fields []string) error {
	f.saved = append(f.saved, u.Id)
	return nil
}

func TestDropProviderTokensRunsOnce(t *testing.T) {
	db := &fakeMigrationDB{cfg: map[string]string{}, users: []*model.User{
		{Id: "linked", ExternalAuthProviders: map[string]model.ExternalProvider{"google": {ProviderUID: "g"}}},
		{Id: "password-only"},
	}}

	runDataMigrations(db)
	if len(db.saved) != 1 || db.saved[0] != "linked" {
		t.Fatalf("rewrote %v, want only the linked account", db.saved)
	}
	if db.cfg["data_migration.drop_provider_tokens"] == "" {
		t.Fatal("migration not recorded as done")
	}

	runDataMigrations(db)
	if len(db.saved) != 1 {
		t.Errorf("migration ran again: rewrote %v", db.saved)
	}
}
