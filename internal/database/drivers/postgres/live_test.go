package driver_postgres

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// newLiveDriver returns a driver on a fresh database, created on the server
// named by KNOT_POSTGRES_LIVE_DSN, e.g.
// postgres://postgres:secret@127.0.0.1:5432/postgres?sslmode=disable.
// Without it the live tests are skipped.
func newLiveDriver(t *testing.T) *PostgresDriver {
	t.Helper()
	dsn := os.Getenv("KNOT_POSTGRES_LIVE_DSN")
	if dsn == "" {
		t.Skip("KNOT_POSTGRES_LIVE_DSN not set")
	}

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { admin.Close() })

	name := fmt.Sprintf("knot_live_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, _ := url.Parse(dsn)
	u.Path = "/" + name
	conn, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
		admin.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`)
	})

	db := &PostgresDriver{connection: &pgConn{DB: conn}, logger: log.WithGroup("db")}
	if err := db.initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	// A second run must be a no-op, as on every server restart.
	if err := db.initialize(); err != nil {
		t.Fatalf("initialize again: %v", err)
	}
	return db
}

func TestLiveUserFlow(t *testing.T) {
	db := newLiveDriver(t)

	login := time.Date(2026, 5, 22, 22, 29, 56, 123456000, time.FixedZone("AWST", 8*60*60))
	user := &model.User{
		Id:       "u1",
		Username: "Alice",
		Email:    "Alice@Example.com",
		Roles:    []string{"r1"},
		Groups:   []string{"g1"},
		ExternalAuthProviders: map[string]model.ExternalProvider{
			"github": {ProviderUID: "42", Username: "alice"},
		},
		Active:      true,
		MaxSpaces:   4000000000,
		LastLoginAt: &login,
		UpdatedAt:   hlc.Now(),
		CreatedAt:   time.Now(),
	}
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	got, err := db.GetUserByEmail("alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.Id != "u1" || !got.Active || got.MaxSpaces != 4000000000 || len(got.Roles) != 1 || got.Groups[0] != "g1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.LastLoginAt == nil || !got.LastLoginAt.Equal(login) || got.LastLoginAt.Location() != time.UTC {
		t.Fatalf("last login: %v", got.LastLoginAt)
	}
	if got.UpdatedAt != user.UpdatedAt {
		t.Fatalf("updated at: %v != %v", got.UpdatedAt, user.UpdatedAt)
	}
	if _, err := db.GetUserByUsername("alice"); err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if p, err := db.GetUserByProviderUID("github", "42"); err != nil || p.Id != "u1" {
		t.Fatalf("GetUserByProviderUID: %v %v", p, err)
	}

	// Partial update, then a provider change re-points the index.
	user.Active = false
	user.LastLoginAt = nil
	user.ExternalAuthProviders = map[string]model.ExternalProvider{"github": {ProviderUID: "43"}}
	if err := db.SaveUser(user, []string{"Active", "LastLoginAt", "ExternalAuthProviders"}); err != nil {
		t.Fatalf("SaveUser update: %v", err)
	}
	got, _ = db.GetUser("u1")
	if got.Active || got.LastLoginAt != nil {
		t.Fatalf("update not applied: %+v", got)
	}
	if _, err := db.GetUserByProviderUID("github", "42"); err == nil {
		t.Fatal("stale provider index row remains")
	}
	if _, err := db.GetUserByProviderUID("github", "43"); err != nil {
		t.Fatalf("new provider index: %v", err)
	}

	if has, err := db.HasUsers(); err != nil || !has {
		t.Fatalf("HasUsers: %v %v", has, err)
	}
	if users, err := db.GetUsers(); err != nil || len(users) != 1 {
		t.Fatalf("GetUsers: %d %v", len(users), err)
	}
	if err := db.DeleteUser(user); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := db.GetUser("u1"); err == nil {
		t.Fatal("user not deleted")
	}
}

func TestLiveSpaceFlow(t *testing.T) {
	db := newLiveDriver(t)

	space := &model.Space{
		Id:        "s1",
		UserId:    "u1",
		Name:      "dev",
		Shares:    []string{"u2"},
		AltNames:  []model.AltNameEntry{{Name: "web", Port: 8080}},
		CreatedAt: time.Now(),
		UpdatedAt: hlc.Now(),
	}
	if err := db.SaveSpace(space, nil); err != nil {
		t.Fatalf("SaveSpace: %v", err)
	}

	got, err := db.GetSpace("s1")
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if len(got.AltNames) != 1 || got.AltNames[0].Port != 8080 {
		t.Fatalf("alt names: %+v", got.AltNames)
	}

	// The share makes the space visible to u2 by name and in their list.
	if s, err := db.GetSpaceByName("u2", "dev"); err != nil || s.Id != "s1" {
		t.Fatalf("GetSpaceByName shared: %v %v", s, err)
	}
	if spaces, err := db.GetSpacesForUser("u2"); err != nil || len(spaces) != 1 {
		t.Fatalf("GetSpacesForUser shared: %d %v", len(spaces), err)
	}
	if spaces, err := db.GetSpacesForUser("u3"); err != nil || len(spaces) != 0 {
		t.Fatalf("GetSpacesForUser other: %d %v", len(spaces), err)
	}

	// A second space with the same name for the same user is refused.
	dup := &model.Space{Id: "s2", UserId: "u1", Name: "dev", CreatedAt: time.Now()}
	if err := db.SaveSpace(dup, nil); err == nil {
		t.Fatal("duplicate space name accepted")
	}

	space.AltNames = []model.AltNameEntry{{Name: "api", Port: 9000}}
	space.IsDeployed = true
	if err := db.SaveSpace(space, []string{"AltNames", "IsDeployed"}); err != nil {
		t.Fatalf("SaveSpace update: %v", err)
	}
	got, _ = db.GetSpace("s1")
	if !got.IsDeployed || len(got.AltNames) != 1 || got.AltNames[0].Name != "api" {
		t.Fatalf("update not applied: %+v", got)
	}

	if spaces, err := db.GetSpaces(); err != nil || len(spaces) != 1 {
		t.Fatalf("GetSpaces: %d %v", len(spaces), err)
	}
	if err := db.DeleteSpace(space); err != nil {
		t.Fatalf("DeleteSpace: %v", err)
	}
	if spaces, _ := db.GetSpaces(); len(spaces) != 0 {
		t.Fatalf("space not deleted: %d", len(spaces))
	}
}

func TestLiveSessionFlow(t *testing.T) {
	db := newLiveDriver(t)

	session := &model.Session{Id: "sess-1", UserId: "u1", Ip: "127.0.0.1", ExpiresAfter: time.Now().Add(time.Hour), UpdatedAt: hlc.Now()}
	if err := db.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := db.GetSession("sess-1")
	if err != nil || got == nil || got.UserId != "u1" {
		t.Fatalf("GetSession: %v %v", got, err)
	}

	// A user switch moves the session to the new user.
	session.OriginalUserId = "u1"
	session.UserId = "u2"
	if err := db.SaveSession(session); err != nil {
		t.Fatalf("SaveSession switch: %v", err)
	}
	if s, _ := db.GetSessionsForUser("u1"); len(s) != 0 {
		t.Fatalf("session still listed under the old user: %d", len(s))
	}
	if s, _ := db.GetSessionsForUser("u2"); len(s) != 1 || s[0].OriginalUserId != "u1" {
		t.Fatalf("GetSessionsForUser: %+v", s)
	}

	expired := &model.Session{Id: "sess-2", UserId: "u1", ExpiresAfter: time.Now().Add(-time.Minute)}
	if err := db.SaveSession(expired); err != nil {
		t.Fatalf("SaveSession expired: %v", err)
	}
	if s, err := db.GetSession("sess-2"); err != nil || s != nil {
		t.Fatalf("expired session returned: %v %v", s, err)
	}
	if all, _ := db.GetSessions(); len(all) != 1 {
		t.Fatalf("GetSessions: %d", len(all))
	}

	if s, err := db.GetSession("missing"); err != nil || s != nil {
		t.Fatalf("missing session: %v %v", s, err)
	}
	if err := db.DeleteSession(session); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if s, _ := db.GetSession("sess-1"); s != nil {
		t.Fatal("session not deleted")
	}
}

func TestLiveMiscFlow(t *testing.T) {
	db := newLiveDriver(t)

	// Groups is a reserved word.
	if err := db.SaveGroup(&model.Group{Id: "g1", Name: "devs", MaxSpaces: 3}); err != nil {
		t.Fatalf("SaveGroup: %v", err)
	}
	if err := db.SaveGroup(&model.Group{Id: "g1", Name: "developers", MaxSpaces: 3}); err != nil {
		t.Fatalf("SaveGroup update: %v", err)
	}
	if groups, err := db.GetGroups(); err != nil || len(groups) != 1 || groups[0].Name != "developers" {
		t.Fatalf("GetGroups: %+v %v", groups, err)
	}

	if err := db.SaveCfgValue(&model.CfgValue{Name: "node_id", Value: "abc"}); err != nil {
		t.Fatalf("SaveCfgValue: %v", err)
	}
	if v, err := db.GetCfgValue("node_id"); err != nil || v.Value != "abc" {
		t.Fatalf("GetCfgValue: %v %v", v, err)
	}

	conv := &model.Conversation{Id: "c1", UserId: "u1", Title: "one", Data: "{}", CreatedAt: time.Now(), UpdatedAt: hlc.Now()}
	if err := db.SaveConversation(conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	conv.Title = "two"
	if err := db.SaveConversation(conv); err != nil {
		t.Fatalf("SaveConversation upsert: %v", err)
	}
	if c, err := db.GetConversation("u1", "c1"); err != nil || c.Title != "two" {
		t.Fatalf("GetConversation: %v %v", c, err)
	}

	sample := &model.SpaceUsageSample{Id: "su1", SpaceId: "s1", UserId: "u1", BucketKind: model.SpaceUsageBucketMinute, BucketStart: time.Now().Truncate(time.Minute), CPUPercent: 12.5, MemoryUsedBytes: 1 << 40, CreatedAt: time.Now()}
	for i := 0; i < 2; i++ {
		if err := db.SaveSpaceUsageSample(sample); err != nil {
			t.Fatalf("SaveSpaceUsageSample %d: %v", i, err)
		}
		sample.CPUPercent = 50
	}
	samples, err := db.GetSpaceUsageSamples("s1", model.SpaceUsageBucketMinute, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || len(samples) != 1 || samples[0].CPUPercent != 50 || samples[0].MemoryUsedBytes != 1<<40 {
		t.Fatalf("GetSpaceUsageSamples: %+v %v", samples, err)
	}

	for _, zones := range [][]string{nil, {"core"}} {
		skill := &model.Skill{Id: fmt.Sprintf("sk%d", len(zones)), Name: "review", Zones: zones, Active: true, CreatedAt: time.Now()}
		if err := db.SaveSkill(skill, nil); err != nil {
			t.Fatalf("SaveSkill: %v", err)
		}
	}
	if skills, err := db.GetSkillsByName("review"); err != nil || len(skills) != 2 || len(skills[0].Zones) != 1 {
		t.Fatalf("GetSkillsByName zone order: %+v %v", skills, err)
	}

	token := &model.Token{Id: "t1", UserId: "u1", Name: "ci", ExpiresAfter: time.Now().Add(time.Hour), Scopes: []string{"mcp"}}
	if err := db.SaveToken(token); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	if tk, err := db.GetToken("t1"); err != nil || len(tk.Scopes) != 1 {
		t.Fatalf("GetToken: %v %v", tk, err)
	}
}

func TestLivePoolFlow(t *testing.T) {
	db := newLiveDriver(t)

	// The migrations ran on the fresh database too, and only once.
	var applied int
	if err := db.connection.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil || applied != len(migrations) {
		t.Fatalf("schema_migrations rows = %d (%v), want %d", applied, err, len(migrations))
	}

	pool := model.NewPoolDefinition("agents", "t1", "", 3, "u1")
	pool.LeaseMaxTime = -1
	pool.LeaseMaxExtensions = -1
	pool.CustomFields = []model.SpaceCustomField{{Name: "anthropic_api_key", Value: "sk-test"}, {Name: "region", Value: "eu"}}
	if err := db.SavePoolDefinition(pool, nil); err != nil {
		t.Fatalf("SavePoolDefinition: %v", err)
	}
	got, err := db.GetPoolDefinition(pool.Id)
	if err != nil || len(got.CustomFields) != 2 || got.CustomFields[1].Value != "eu" || got.LeaseMaxTime != -1 {
		t.Fatalf("GetPoolDefinition: %+v %v", got, err)
	}

	// An update of just the custom fields, as SaveCustomFields does through
	// savePool (all fields), then clearing them.
	got.CustomFields = []model.SpaceCustomField{{Name: "region", Value: "us"}}
	if err := db.SavePoolDefinition(got, nil); err != nil {
		t.Fatalf("SavePoolDefinition update: %v", err)
	}
	byName, err := db.GetPoolDefinitionByName("u1", "agents")
	if err != nil || len(byName.CustomFields) != 1 || byName.CustomFields[0].Value != "us" {
		t.Fatalf("GetPoolDefinitionByName: %+v %v", byName, err)
	}
	byName.CustomFields = nil
	if err := db.SavePoolDefinition(byName, []string{"CustomFields"}); err != nil {
		t.Fatalf("SavePoolDefinition clear: %v", err)
	}
	pools, err := db.GetPoolDefinitions()
	if err != nil || len(pools) != 1 || len(pools[0].CustomFields) != 0 {
		t.Fatalf("GetPoolDefinitions: %+v %v", pools, err)
	}
}

// A deleted session stays deleted: a server that read it before a logout
// elsewhere saving a live copy afterwards must not revive it.
func TestLiveSessionKeepsDeletion(t *testing.T) {
	db := newLiveDriver(t)

	live := model.Session{Id: "sess-del", UserId: "u1", ExpiresAfter: time.Now().Add(time.Hour), UpdatedAt: hlc.Now()}
	if err := db.SaveSession(&live); err != nil {
		t.Fatal(err)
	}
	deleted := live
	deleted.IsDeleted = true
	deleted.UpdatedAt = hlc.Now()
	if err := db.SaveSession(&deleted); err != nil {
		t.Fatal(err)
	}
	late := live
	late.ExpiresAfter = time.Now().Add(2 * time.Hour)
	late.UpdatedAt = hlc.Now()
	if err := db.SaveSession(&late); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession("sess-del")
	if err != nil || got == nil || !got.IsDeleted {
		t.Fatalf("session after a late refresh = %+v, %v, want it still deleted", got, err)
	}
}

// The refresh_token column goes, and provider tokens earlier versions kept
// in a user's external_auth_providers go when the user is saved again, as
// the drop_provider_tokens data migration does.
func TestLiveDropProviderTokens(t *testing.T) {
	db := newLiveDriver(t)

	// A database from before: the column is back, with a token in it.
	if _, err := db.connection.Exec(`ALTER TABLE user_providers ADD COLUMN refresh_token TEXT`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.connection.Exec(migrations[5]); err != nil {
		t.Fatalf("migration 6: %v", err)
	}
	var cols int
	if err := db.connection.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'user_providers' AND column_name = 'refresh_token'`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("refresh_token column still there: %d, %v", cols, err)
	}

	user := &model.User{Id: "u-tok", Username: "owner", Email: "owner@example.com", Active: true, UpdatedAt: hlc.Now(),
		ExternalAuthProviders: map[string]model.ExternalProvider{"google": {ProviderUID: "g1", Username: "o"}}}
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.connection.Exec(`UPDATE users SET external_auth_providers = ? WHERE user_id = ?`,
		`{"google":{"provider_uid":"g1","username":"o","token":"enc-access","refresh_token":"enc-refresh"}}`, user.Id); err != nil {
		t.Fatal(err)
	}

	loaded, err := db.GetUser(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveUser(loaded, []string{"ExternalAuthProviders"}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.connection.QueryRow(`SELECT external_auth_providers::text FROM users WHERE user_id = ?`, user.Id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "enc-") || !strings.Contains(stored, `"g1"`) {
		t.Errorf("external_auth_providers after rewrite = %s", stored)
	}
	if u, err := db.GetUserByProviderUID("google", "g1"); err != nil || u == nil || u.Id != user.Id {
		t.Errorf("provider lookup after rewrite: %v, %v", u, err)
	}
}

func TestLiveSessionKeepsNewerProviderToken(t *testing.T) {
	db := newLiveDriver(t)

	read := model.Session{Id: "sess-grant", UserId: "u1", ExpiresAfter: time.Now().Add(time.Hour), UpdatedAt: hlc.Now(),
		AuthProvider: "corp", ProviderToken: "old", ProviderTokenAt: hlc.Now()}
	if err := db.SaveSession(&read); err != nil {
		t.Fatal(err)
	}
	replaced := read
	replaced.ProviderToken = "new"
	replaced.ProviderTokenAt = hlc.Now()
	if err := db.SaveSession(&replaced); err != nil {
		t.Fatal(err)
	}
	late := read
	late.ExpiresAfter = time.Now().Add(2 * time.Hour)
	late.UpdatedAt = hlc.Now()
	if err := db.SaveSession(&late); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSession("sess-grant")
	if err != nil || got == nil {
		t.Fatalf("session: %v, %v", got, err)
	}
	if got.ProviderToken != "new" || got.ProviderTokenAt != replaced.ProviderTokenAt || got.AuthProvider != "corp" {
		t.Errorf("session = %+v, want the newer grant kept", got)
	}
}
