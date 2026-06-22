package service

import (
	"fmt"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func newPoolTestUser(id string) *model.User {
	return &model.User{
		Id:    id,
		Roles: []string{model.RoleAdminUUID},
	}
}

func newPoolTestTemplate(t *testing.T, name string) *model.Template {
	t.Helper()
	template := model.NewTemplate(
		name,
		"test template",
		"job",
		"",
		"user-admin",
		nil,
		model.PlatformManual,
		false,
		false,
		false,
		false,
		false,
		false,
		"",
		"",
		0,
		0,
		false,
		nil,
		nil,
		false,
		true,
		0,
		"",
		"",
		nil,
	)
	if err := database.GetInstance().SaveTemplate(template, nil); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	return template
}

func setupPoolTestDB(t *testing.T) {
	t.Helper()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	model.SetRoleCache(nil)
}

func TestPoolNamesAreScopedPerUser(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-scoped")

	userA := newPoolTestUser("pool-user-a")
	userB := newPoolTestUser("pool-user-b")

	poolA := model.NewPoolDefinition("shared-pool-name", template.Id, "", 1, userA.Id)
	poolA.Active = false
	if err := GetPoolService().Create(poolA, userA); err != nil {
		t.Fatalf("Create poolA: %v", err)
	}

	poolB := model.NewPoolDefinition("shared-pool-name", template.Id, "", 1, userB.Id)
	poolB.Active = false
	if err := GetPoolService().Create(poolB, userB); err != nil {
		t.Fatalf("Create poolB with same name for different user: %v", err)
	}

	if _, err := database.GetInstance().GetPoolDefinitionByName(userA.Id, poolA.Name); err != nil {
		t.Fatalf("GetPoolDefinitionByName userA: %v", err)
	}
	if _, err := database.GetInstance().GetPoolDefinitionByName(userB.Id, poolB.Name); err != nil {
		t.Fatalf("GetPoolDefinitionByName userB: %v", err)
	}
}

func TestPoolSetSizeAdjustsDesiredCount(t *testing.T) {
	setupPoolTestDB(t)
	template := newPoolTestTemplate(t, "pool-test-template-update")
	user := newPoolTestUser("pool-user-update")

	pool := model.NewPoolDefinition(fmt.Sprintf("pool-update-%s", template.Id[:8]), template.Id, "", 1, user.Id)
	pool.Active = false
	if err := GetPoolService().Create(pool, user); err != nil {
		t.Fatalf("Create pool: %v", err)
	}

	if err := GetPoolService().SetSize(pool, 3, user); err != nil {
		t.Fatalf("SetSize pool: %v", err)
	}

	after, err := database.GetInstance().GetPoolDefinition(pool.Id)
	if err != nil {
		t.Fatalf("GetPoolDefinition: %v", err)
	}
	if after.Name != pool.Name {
		t.Fatalf("Name = %q, want %q", after.Name, pool.Name)
	}
	if after.TemplateId != template.Id {
		t.Fatalf("TemplateId = %q, want %q", after.TemplateId, template.Id)
	}
	if after.DesiredCount != 3 {
		t.Fatalf("DesiredCount = %d, want 3", after.DesiredCount)
	}
}
