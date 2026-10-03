package database

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// tokenDriver counts token and group reads.
type tokenDriver struct {
	DbDriver

	tokens map[string][]*model.Token // by user id
	groups []*model.Group

	tokenReads, groupReads int
}

func (d *tokenDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
	d.tokenReads++
	out := make([]*model.Token, 0, len(d.tokens[userId]))
	for _, t := range d.tokens[userId] {
		c := *t
		out = append(out, &c)
	}
	return out, nil
}

func (d *tokenDriver) SaveToken(t *model.Token) error {
	list := d.tokens[t.UserId]
	for i, x := range list {
		if x.Id == t.Id {
			c := *t
			list[i] = &c
			return nil
		}
	}
	c := *t
	d.tokens[t.UserId] = append(list, &c)
	return nil
}

func (d *tokenDriver) DeleteToken(t *model.Token) error {
	list := d.tokens[t.UserId]
	for i, x := range list {
		if x.Id == t.Id {
			d.tokens[t.UserId] = append(list[:i], list[i+1:]...)
			break
		}
	}
	return nil
}

func (d *tokenDriver) GetGroups() ([]*model.Group, error) {
	d.groupReads++
	out := make([]*model.Group, len(d.groups))
	for i, g := range d.groups {
		c := *g
		out[i] = &c
	}
	return out, nil
}

func (d *tokenDriver) SaveGroup(g *model.Group) error {
	for i, x := range d.groups {
		if x.Id == g.Id {
			c := *g
			d.groups[i] = &c
			return nil
		}
	}
	c := *g
	d.groups = append(d.groups, &c)
	return nil
}

func (d *tokenDriver) DeleteGroup(g *model.Group) error {
	for i, x := range d.groups {
		if x.Id == g.Id {
			d.groups = append(d.groups[:i], d.groups[i+1:]...)
			break
		}
	}
	return nil
}

func newTokenCache(t *testing.T) (*cachingDriver, *tokenDriver, *time.Time) {
	t.Helper()
	inner := &tokenDriver{tokens: map[string][]*model.Token{}}
	d := newCachingDriver(inner)
	current := time.Now()
	d.now = func() time.Time { return current }
	return d, inner, &current
}

func TestCachingTokensReadThrough(t *testing.T) {
	d, inner, _ := newTokenCache(t)
	inner.tokens["u1"] = []*model.Token{{Id: "tk_a", UserId: "u1", Name: "a"}}

	for range 100 {
		got, err := d.GetTokensForUser("u1")
		if err != nil || len(got) != 1 || got[0].Id != "tk_a" {
			t.Fatalf("tokens %v %v", got, err)
		}
	}
	if inner.tokenReads != 1 {
		t.Errorf("100 reads made %d database reads, want 1", inner.tokenReads)
	}
}

func TestCachingTokensInvalidatedOnWrite(t *testing.T) {
	d, inner, _ := newTokenCache(t)
	inner.tokens["u1"] = []*model.Token{{Id: "tk_a", UserId: "u1"}}
	d.GetTokensForUser("u1")

	// A new token is seen at once.
	if err := d.SaveToken(&model.Token{Id: "tk_b", UserId: "u1"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.GetTokensForUser("u1"); len(got) != 2 {
		t.Errorf("after save: %d tokens, want 2", len(got))
	}

	// Revoking, by marking deleted as the API and gossip do, is seen at once.
	if err := d.SaveToken(&model.Token{Id: "tk_a", UserId: "u1", IsDeleted: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetTokensForUser("u1")
	for _, tk := range got {
		if tk.Id == "tk_a" && !tk.IsDeleted {
			t.Error("revoked token still live in the cache")
		}
	}

	// As is a hard delete.
	if err := d.DeleteToken(&model.Token{Id: "tk_b", UserId: "u1"}); err != nil {
		t.Fatal(err)
	}
	for _, tk := range mustTokens(t, d, "u1") {
		if tk.Id == "tk_b" {
			t.Error("deleted token still in the cache")
		}
	}

	// Another user's cache is left alone.
	inner.tokens["u2"] = []*model.Token{{Id: "tk_c", UserId: "u2"}}
	d.GetTokensForUser("u2")
	reads := inner.tokenReads
	d.SaveToken(&model.Token{Id: "tk_d", UserId: "u1"})
	d.GetTokensForUser("u2")
	if inner.tokenReads != reads {
		t.Error("a write for one user dropped another user's cached tokens")
	}
}

func mustTokens(t *testing.T, d *cachingDriver, userId string) []*model.Token {
	t.Helper()
	got, err := d.GetTokensForUser(userId)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCachingTokensExpire(t *testing.T) {
	d, inner, now := newTokenCache(t)
	inner.tokens["u1"] = []*model.Token{{Id: "tk_a", UserId: "u1"}}
	d.GetTokensForUser("u1")

	// A change written straight to the database, bypassing the cache, is
	// seen once the entry expires.
	inner.tokens["u1"] = nil
	*now = now.Add(tokenTTL - time.Second)
	if len(mustTokens(t, d, "u1")) != 1 {
		t.Error("entry expired early")
	}
	*now = now.Add(2 * time.Second)
	if len(mustTokens(t, d, "u1")) != 0 {
		t.Error("entry outlived its TTL")
	}
}

func TestCachingTokensInvalidate(t *testing.T) {
	d, inner, _ := newTokenCache(t)
	inner.tokens["u1"] = []*model.Token{{Id: "tk_a", UserId: "u1"}}
	d.GetTokensForUser("u1")
	inner.tokens["u1"] = nil // changed in a shared database by another server
	d.InvalidateTokens("u1")
	if len(mustTokens(t, d, "u1")) != 0 {
		t.Error("invalidate kept the cached tokens")
	}
}

func TestCachingTokensCopies(t *testing.T) {
	d, inner, _ := newTokenCache(t)
	inner.tokens["u1"] = []*model.Token{{Id: "tk_a", UserId: "u1", Scopes: []string{"files"}}}
	got := mustTokens(t, d, "u1")
	got[0].ExpiresAfter = time.Now().Add(time.Hour)
	got[0].Scopes[0] = "mcp"
	again := mustTokens(t, d, "u1")
	if !again[0].ExpiresAfter.IsZero() || again[0].Scopes[0] != "files" {
		t.Error("a caller's change leaked into the cache")
	}
}

func TestCachingGroups(t *testing.T) {
	d, inner, now := newTokenCache(t)
	inner.groups = []*model.Group{{Id: "g1", Name: "devs", FileStorageMB: 10}}
	for range 50 {
		d.GetGroups()
	}
	if inner.groupReads != 1 {
		t.Errorf("50 reads made %d database reads", inner.groupReads)
	}

	d.SaveGroup(&model.Group{Id: "g1", Name: "devs", FileStorageMB: 20})
	if g, _ := d.GetGroups(); g[0].FileStorageMB != 20 {
		t.Error("group change not seen after save")
	}
	d.DeleteGroup(&model.Group{Id: "g1"})
	if g, _ := d.GetGroups(); len(g) != 0 {
		t.Error("group delete not seen")
	}

	inner.groups = []*model.Group{{Id: "g2"}}
	d.InvalidateGroups()
	if g, _ := d.GetGroups(); len(g) != 1 || g[0].Id != "g2" {
		t.Error("invalidate kept the cached groups")
	}

	inner.groups = nil
	*now = now.Add(cacheTTL + time.Second)
	if g, _ := d.GetGroups(); len(g) != 0 {
		t.Error("group list outlived its TTL")
	}

	g, _ := d.GetGroups()
	inner.groups = []*model.Group{{Id: "g3", MaxBuckets: 1}}
	d.InvalidateGroups()
	g, _ = d.GetGroups()
	g[0].MaxBuckets = 99
	if again, _ := d.GetGroups(); again[0].MaxBuckets != 1 {
		t.Error("a caller's change leaked into the group cache")
	}
}
