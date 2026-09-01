package driver_redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"

	"github.com/paularlott/knot/internal/database/model"
)

// newTestDriver returns a driver backed by miniredis, plus the server so tests
// can advance time (FastForward) to prove TTL behaviour.
func newTestDriver(t *testing.T) (*RedisDbDriver, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{server.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(client.Close)
	return &RedisDbDriver{prefix: "test:", connection: client}, server
}

func TestDriverSessionFlow(t *testing.T) {
	db, server := newTestDriver(t)

	session := &model.Session{Id: "sess-1", UserId: "u1"}
	if err := db.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := db.GetSession("sess-1")
	if err != nil || got == nil {
		t.Fatalf("GetSession: %v %v", got, err)
	}
	if got.Id != "sess-1" || got.UserId != "u1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	forUser, err := db.GetSessionsForUser("u1")
	if err != nil || len(forUser) != 1 {
		t.Fatalf("GetSessionsForUser: %d %v", len(forUser), err)
	}

	all, err := db.GetSessions()
	if err != nil || len(all) != 1 {
		t.Fatalf("GetSessions: %d %v", len(all), err)
	}

	if err := db.DeleteSession(session); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if got, err = db.GetSession("sess-1"); got != nil || err != nil {
		t.Fatalf("deleted session should be gone: %v %v", got, err)
	}
	if forUser, err = db.GetSessionsForUser("u1"); err != nil || len(forUser) != 0 {
		t.Fatalf("user index should be gone: %d %v", len(forUser), err)
	}

	// Sessions are the optional session store: prove the TTL rides through
	// SaveSession by expiring past model.SessionExpiryDuration.
	if err := db.SaveSession(session); err != nil {
		t.Fatalf("SaveSession (2nd): %v", err)
	}
	server.FastForward(model.SessionExpiryDuration + time.Minute)
	if got, err = db.GetSession("sess-1"); got != nil || err != nil {
		t.Fatalf("session should have expired: %v %v", got, err)
	}
}

func TestDriverUserFlow(t *testing.T) {
	db, _ := newTestDriver(t)

	user := &model.User{Id: "u1", Username: "Paul", Email: "paul@example.com"}
	if err := db.SaveUser(user, []string{}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	got, err := db.GetUser("u1")
	if err != nil || got == nil || got.Email != "paul@example.com" {
		t.Fatalf("GetUser: %+v %v", got, err)
	}

	if got, err = db.GetUserByEmail("paul@example.com"); err != nil || got == nil || got.Id != "u1" {
		t.Fatalf("GetUserByEmail: %+v %v", got, err)
	}

	// Username index is stored lowercased
	if got, err = db.GetUserByUsername("paul"); err != nil || got == nil || got.Id != "u1" {
		t.Fatalf("GetUserByUsername: %+v %v", got, err)
	}

	if _, err = db.GetUser("missing"); err == nil {
		t.Fatalf("GetUser on missing id should error")
	}
	if _, err = db.GetUserByEmail("missing@example.com"); err == nil {
		t.Fatalf("GetUserByEmail on missing email should error")
	}

	if err := db.DeleteUser(user); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err = db.GetUser("u1"); err == nil {
		t.Fatalf("deleted user should not be found")
	}
	if _, err = db.GetUserByEmail("paul@example.com"); err == nil {
		t.Fatalf("email index should be gone")
	}
}

func TestDriverCfgValueFlow(t *testing.T) {
	db, _ := newTestDriver(t)

	if err := db.SaveCfgValue(&model.CfgValue{Name: "node_id", Value: "abc123"}); err != nil {
		t.Fatalf("SaveCfgValue: %v", err)
	}

	got, err := db.GetCfgValue("node_id")
	if err != nil || got == nil || got.Value != "abc123" {
		t.Fatalf("GetCfgValue: %+v %v", got, err)
	}

	// Missing keys must surface an error: node_id bootstrap at startup
	// depends on err != nil to know it has to generate one.
	if v, err := db.GetCfgValue("missing"); err == nil || v != nil {
		t.Fatalf("GetCfgValue on missing key should error, got %+v %v", v, err)
	}

	values, err := db.GetCfgValues()
	if err != nil || len(values) != 1 || values[0].Name != "node_id" || values[0].Value != "abc123" {
		t.Fatalf("GetCfgValues: %+v %v", values, err)
	}
}

func TestDriverSpaceFlow(t *testing.T) {
	db, _ := newTestDriver(t)

	space := &model.Space{Id: "space-1", Name: "dev", UserId: "u1"}
	if err := db.SaveSpace(space, []string{}); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	got, err := db.GetSpace("space-1")
	if err != nil || got == nil || got.Name != "dev" {
		t.Fatalf("GetSpace: %+v %v", got, err)
	}

	forUser, err := db.GetSpacesForUser("u1")
	if err != nil || len(forUser) != 1 || forUser[0].Id != "space-1" {
		t.Fatalf("GetSpacesForUser: %+v %v", forUser, err)
	}

	byName, err := db.GetSpaceByName("u1", "dev")
	if err != nil || byName == nil || byName.Id != "space-1" {
		t.Fatalf("GetSpaceByName: %+v %v", byName, err)
	}

	// Duplicate name for the same user is refused (exercises keyExists)
	dup := &model.Space{Id: "space-2", Name: "dev", UserId: "u1"}
	if err := db.SaveSpace(dup, []string{}); err == nil {
		t.Fatalf("duplicate space name should be rejected")
	}

	if err := db.DeleteSpace(space); err != nil {
		t.Fatalf("DeleteSpace: %v", err)
	}
	if _, err = db.GetSpace("space-1"); err == nil {
		t.Fatalf("deleted space should report not found")
	}
	if forUser, err = db.GetSpacesForUser("u1"); err != nil || len(forUser) != 0 {
		t.Fatalf("space index should be gone: %d %v", len(forUser), err)
	}
}

func TestDriverTokenFlow(t *testing.T) {
	db, _ := newTestDriver(t)

	token := &model.Token{Id: "tok-1", UserId: "u1", Name: "ci"}
	if err := db.SaveToken(token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	got, err := db.GetToken("tok-1")
	if err != nil || got == nil || got.Name != "ci" {
		t.Fatalf("GetToken: %+v %v", got, err)
	}

	// Short (non-UUID) user ids must list cleanly, not panic
	tokens, err := db.GetTokensForUser("u1")
	if err != nil || len(tokens) != 1 || tokens[0].Id != "tok-1" {
		t.Fatalf("GetTokensForUser: %+v %v", tokens, err)
	}

	if err := db.DeleteToken(token); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if got, err = db.GetToken("tok-1"); got != nil || err != nil {
		t.Fatalf("deleted token should be gone: %v %v", got, err)
	}
}

func TestDriverSkillFlow(t *testing.T) {
	db, _ := newTestDriver(t)

	skill := &model.Skill{Id: "skill-1", UserId: "u1", Name: "deploy", Description: "d"}
	if err := db.SaveSkill(skill, []string{}); err != nil {
		t.Fatalf("SaveSkill: %v", err)
	}

	got, err := db.GetSkill("skill-1")
	if err != nil || got == nil || got.Name != "deploy" {
		t.Fatalf("GetSkill: %+v %v", got, err)
	}

	all, err := db.GetSkills()
	if err != nil || len(all) != 1 {
		t.Fatalf("GetSkills: %d %v", len(all), err)
	}

	byName, err := db.GetSkillsByNameAndUser("deploy", "u1")
	if err != nil || len(byName) != 1 || byName[0].Id != "skill-1" {
		t.Fatalf("GetSkillsByNameAndUser: %+v %v", byName, err)
	}

	// Rename must move the name index; the old name must no longer resolve
	skill.Name = "ship"
	if err := db.SaveSkill(skill, []string{}); err != nil {
		t.Fatalf("SaveSkill (rename): %v", err)
	}
	if _, err = db.GetSkillsByNameAndUser("deploy", "u1"); err == nil {
		t.Fatalf("old name should no longer resolve")
	}
	if byName, err = db.GetSkillsByNameAndUser("ship", "u1"); err != nil || len(byName) != 1 {
		t.Fatalf("new name index should resolve: %+v %v", byName, err)
	}

	if err := db.DeleteSkill(skill); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}
	if got, err = db.GetSkill("skill-1"); got != nil || err != nil {
		t.Fatalf("deleted skill should be gone: %v %v", got, err)
	}
}

func TestDriverConversationFlow(t *testing.T) {
	db, _ := newTestDriver(t)

	conv := &model.Conversation{Id: "conv-1", UserId: "u1", Title: "hello"}
	if err := db.SaveConversation(conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	got, err := db.GetConversation("u1", "conv-1")
	if err != nil || got == nil || got.Title != "hello" {
		t.Fatalf("GetConversation: %+v %v", got, err)
	}

	byUser, err := db.GetConversationsByUser("u1")
	if err != nil || len(byUser) != 1 || byUser[0].Id != "conv-1" {
		t.Fatalf("GetConversationsByUser: %+v %v", byUser, err)
	}

	all, err := db.GetConversations()
	if err != nil || len(all) != 1 {
		t.Fatalf("GetConversations: %d %v", len(all), err)
	}

	if err := db.DeleteConversation(conv); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if got, err = db.GetConversation("u1", "conv-1"); got != nil || err != nil {
		t.Fatalf("deleted conversation should be gone: %v %v", got, err)
	}
}
