package database

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

// cacheTTL bounds how long an entry can survive without a write-driven
// invalidation. Invalidation on Save/Delete is the primary coherence
// mechanism (both local API writes and gossip-applied writes land in the
// same methods); the TTL only self-heels staleness from out-of-band
// changes (direct database edits, a missed gossip message).
const cacheTTL = 5 * time.Minute

// User cache lifetimes: a sliding idle window (refreshed on every read, so
// actively requested users stay cached and quiet ones drop out) with an
// absolute cap that bounds how long an out-of-band edit — one that bypasses
// the driver seam — can go unnoticed on a permanently-busy user.
const (
	userIdleTTL     = 5 * time.Minute
	userMaxLifetime = time.Hour
)

// tokenTTL bounds how long a token change that bypassed this server's cache
// (another server writing a shared database, or an offline admin command)
// can go unseen. Changes made here or gossiped here invalidate at once.
const tokenTTL = 30 * time.Second

type tokenCacheEntry struct {
	tokens  []*model.Token
	expires time.Time
}

type groupCacheEntry struct {
	groups  []*model.Group
	expires time.Time
}

type userCacheEntry struct {
	user        *model.User
	expires     time.Time // sliding: refreshed on read, drops quiet users
	hardExpires time.Time // absolute: set once at fill
}

type templateCacheEntry struct {
	template *model.Template
	expires  time.Time
}

type poolCacheEntry struct {
	spaces  []*model.Space
	expires time.Time
}

type poolDefCacheEntry struct {
	pool    *model.PoolDefinition
	expires time.Time
}

// cachingDriver decorates the main DbDriver with in-memory caches for the
// hot read paths: templates (read per connection by port-forward
// authorization, every minute by the scheduler, and whenever port URLs are
// built) and pool membership (a full space-table scan before pools got a
// per-pool read). Everything else passes through untouched.
//
// Coherence comes from the driver seam itself: every write path on a node —
// API handlers and gossip-applied replication both — calls the same
// Save/Delete methods, so evicting there covers all writers without a
// separate gossip hook. Cached objects are handed out as copies because
// callers mutate what they read (template updates, pool member filtering).
type cachingDriver struct {
	DbDriver

	sessions SessionStorage

	mu sync.Mutex
	// templates by id; nameToID and the list are derived and cleared
	// wholesale on any template write (template writes are rare admin
	// operations, so a full clear beats tracking renames).
	templates    map[string]*templateCacheEntry
	nameToID     map[string]string
	templateList *templateCacheListEntry
	// pool member lists by pool id, plus the reverse index used to
	// invalidate the right pool when a space is saved or deleted — the
	// space object in a partial SaveSpace may not carry PoolId at all.
	poolMembers map[string]*poolCacheEntry
	poolBySpace map[string]string
	// pool definitions by "userId/name" — routing resolves the pool before
	// picking a member, so this read is on the per-connection path too.
	poolDefs map[string]*poolDefCacheEntry
	// users by id, plus the username index (cleared wholesale on any user
	// write — a save may rename). Users sit on the hottest path in the
	// system: request authentication and cross-user forward resolution.
	users      map[string]*userCacheEntry
	userByName map[string]string

	// API tokens by user, read on every S3 request, and the group list,
	// read for every quota check.
	userTokens map[string]*tokenCacheEntry
	groups     *groupCacheEntry

	now func() time.Time // overridable in tests
}

type templateCacheListEntry struct {
	templates []*model.Template
	expires   time.Time
}

func newCachingDriver(inner DbDriver) *cachingDriver {
	d := &cachingDriver{
		DbDriver:    inner,
		templates:   make(map[string]*templateCacheEntry),
		nameToID:    make(map[string]string),
		poolMembers: make(map[string]*poolCacheEntry),
		poolBySpace: make(map[string]string),
		poolDefs:    make(map[string]*poolDefCacheEntry),
		users:       make(map[string]*userCacheEntry),
		userByName:  make(map[string]string),
		userTokens:  make(map[string]*tokenCacheEntry),
		now:         time.Now,
	}
	if sessions, ok := inner.(SessionStorage); ok {
		d.sessions = sessions
	}
	return d
}

func (d *cachingDriver) expired(expires time.Time) bool {
	return !d.now().Before(expires)
}

// copyTemplate deep-copies via JSON — cheap next to a database round trip,
// and safe: callers legitimately mutate the templates they read.
func copyTemplate(t *model.Template) *model.Template {
	data, err := json.Marshal(t)
	if err != nil {
		return t
	}
	var copy model.Template
	if json.Unmarshal(data, &copy) != nil {
		return t
	}
	return &copy
}

func copySpaces(spaces []*model.Space) []*model.Space {
	out := make([]*model.Space, len(spaces))
	for i, s := range spaces {
		spaceCopy := *s
		out[i] = &spaceCopy
	}
	return out
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

func (d *cachingDriver) GetTemplate(id string) (*model.Template, error) {
	d.mu.Lock()
	entry, ok := d.templates[id]
	if ok && d.expired(entry.expires) {
		delete(d.templates, id)
		ok = false
	}
	d.mu.Unlock()

	if ok {
		return copyTemplate(entry.template), nil
	}

	template, err := d.DbDriver.GetTemplate(id)
	if err != nil || template == nil {
		return template, err
	}

	stored := copyTemplate(template)

	d.mu.Lock()
	d.templates[id] = &templateCacheEntry{template: stored, expires: d.now().Add(cacheTTL)}
	d.nameToID[template.Name] = id
	d.mu.Unlock()

	return copyTemplate(template), nil
}

func (d *cachingDriver) GetTemplateByName(name string) (*model.Template, error) {
	d.mu.Lock()
	id, ok := d.nameToID[name]
	var entry *templateCacheEntry
	if ok {
		entry = d.templates[id]
		if entry != nil && d.expired(entry.expires) {
			delete(d.templates, id)
			delete(d.nameToID, name)
			entry = nil
		}
	}
	d.mu.Unlock()

	if entry != nil {
		return copyTemplate(entry.template), nil
	}

	template, err := d.DbDriver.GetTemplateByName(name)
	if err != nil || template == nil {
		return template, err
	}

	stored := copyTemplate(template)

	d.mu.Lock()
	d.templates[template.Id] = &templateCacheEntry{template: stored, expires: d.now().Add(cacheTTL)}
	d.nameToID[template.Name] = template.Id
	d.mu.Unlock()

	return copyTemplate(template), nil
}

func (d *cachingDriver) GetTemplates() ([]*model.Template, error) {
	d.mu.Lock()
	list := d.templateList
	if list != nil && d.expired(list.expires) {
		d.templateList = nil
		list = nil
	}
	d.mu.Unlock()

	if list != nil {
		out := make([]*model.Template, len(list.templates))
		for i, t := range list.templates {
			out[i] = copyTemplate(t)
		}
		return out, nil
	}

	templates, err := d.DbDriver.GetTemplates()
	if err != nil {
		return nil, err
	}

	storedList := make([]*model.Template, len(templates))
	d.mu.Lock()
	for i, t := range templates {
		storedList[i] = copyTemplate(t)
		d.templates[t.Id] = &templateCacheEntry{template: storedList[i], expires: d.now().Add(cacheTTL)}
		d.nameToID[t.Name] = t.Id
	}
	d.templateList = &templateCacheListEntry{templates: storedList, expires: d.now().Add(cacheTTL)}
	d.mu.Unlock()

	out := make([]*model.Template, len(templates))
	for i, t := range templates {
		out[i] = copyTemplate(t)
	}
	return out, nil
}

func (d *cachingDriver) SaveTemplate(template *model.Template, updateFields []string) error {
	err := d.DbDriver.SaveTemplate(template, updateFields)
	d.mu.Lock()
	delete(d.templates, template.Id)
	d.nameToID = make(map[string]string)
	d.templateList = nil
	d.mu.Unlock()
	return err
}

func (d *cachingDriver) DeleteTemplate(template *model.Template) error {
	err := d.DbDriver.DeleteTemplate(template)
	d.mu.Lock()
	delete(d.templates, template.Id)
	d.nameToID = make(map[string]string)
	d.templateList = nil
	d.mu.Unlock()
	return err
}

// ---------------------------------------------------------------------------
// Pool membership
// ---------------------------------------------------------------------------

func (d *cachingDriver) GetSpacesByPoolId(poolId string) ([]*model.Space, error) {
	d.mu.Lock()
	entry, ok := d.poolMembers[poolId]
	if ok && d.expired(entry.expires) {
		delete(d.poolMembers, poolId)
		ok = false
	}
	d.mu.Unlock()

	if ok {
		return copySpaces(entry.spaces), nil
	}

	spaces, err := d.DbDriver.GetSpacesByPoolId(poolId)
	if err != nil {
		return nil, err
	}

	stored := copySpaces(spaces)

	d.mu.Lock()
	d.poolMembers[poolId] = &poolCacheEntry{spaces: stored, expires: d.now().Add(cacheTTL)}
	for _, s := range stored {
		d.poolBySpace[s.Id] = poolId
	}
	d.mu.Unlock()

	return copySpaces(spaces), nil
}

// invalidateSpacePool evicts every pool list the space could appear in. A
// partial SaveSpace may not carry PoolId, so the reverse index (filled when
// a pool list was cached) is authoritative for where the space used to
// live; together the two cover moves between pools and removal from one.
func (d *cachingDriver) invalidateSpacePool(space *model.Space) {
	poolIds := make(map[string]bool)
	if space.PoolId != "" {
		poolIds[space.PoolId] = true
	}
	if id, ok := d.poolBySpace[space.Id]; ok {
		poolIds[id] = true
		delete(d.poolBySpace, space.Id)
	}
	for poolId := range poolIds {
		delete(d.poolMembers, poolId)
	}
}

func (d *cachingDriver) SaveSpace(space *model.Space, updateFields []string) error {
	err := d.DbDriver.SaveSpace(space, updateFields)
	d.mu.Lock()
	d.invalidateSpacePool(space)
	d.mu.Unlock()
	return err
}

func (d *cachingDriver) DeleteSpace(space *model.Space) error {
	err := d.DbDriver.DeleteSpace(space)
	d.mu.Lock()
	d.invalidateSpacePool(space)
	d.mu.Unlock()
	return err
}

// ---------------------------------------------------------------------------
// Pool definitions
// ---------------------------------------------------------------------------

func poolDefKey(userId, name string) string {
	return fmt.Sprintf("%s/%s", userId, name)
}

func (d *cachingDriver) GetPoolDefinitionByName(userId, name string) (*model.PoolDefinition, error) {
	key := poolDefKey(userId, name)

	d.mu.Lock()
	entry, ok := d.poolDefs[key]
	if ok && d.expired(entry.expires) {
		delete(d.poolDefs, key)
		ok = false
	}
	d.mu.Unlock()

	if ok {
		poolCopy := *entry.pool
		return &poolCopy, nil
	}

	pool, err := d.DbDriver.GetPoolDefinitionByName(userId, name)
	if err != nil || pool == nil {
		return pool, err
	}

	stored := *pool

	d.mu.Lock()
	d.poolDefs[key] = &poolDefCacheEntry{pool: &stored, expires: d.now().Add(cacheTTL)}
	d.mu.Unlock()

	poolCopy := *pool
	return &poolCopy, nil
}

func (d *cachingDriver) SavePoolDefinition(pool *model.PoolDefinition, updateFields []string) error {
	err := d.DbDriver.SavePoolDefinition(pool, updateFields)
	d.mu.Lock()
	d.poolDefs = make(map[string]*poolDefCacheEntry)
	if pool != nil {
		delete(d.poolMembers, pool.Id)
	}
	d.mu.Unlock()
	return err
}

func (d *cachingDriver) DeletePoolDefinition(pool *model.PoolDefinition) error {
	err := d.DbDriver.DeletePoolDefinition(pool)
	d.mu.Lock()
	d.poolDefs = make(map[string]*poolDefCacheEntry)
	delete(d.poolMembers, pool.Id)
	d.mu.Unlock()
	return err
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

// copyUser deep-copies via JSON — cheap next to a database round trip, and
// necessary in both directions: handlers mutate the users they read (profile
// and role updates), and a driver handing out shared pointers must not
// poison the cache.
func copyUser(u *model.User) *model.User {
	data, err := json.Marshal(u)
	if err != nil {
		return u
	}
	var copy model.User
	if json.Unmarshal(data, &copy) != nil {
		return u
	}
	return &copy
}

// getUserEntry returns a live cache entry for the id, refreshing the sliding
// idle window. nil when absent, idle-expired, or past the absolute cap.
func (d *cachingDriver) getUserEntry(id string) *userCacheEntry {
	entry, ok := d.users[id]
	if !ok {
		return nil
	}
	now := d.now()
	if !now.Before(entry.hardExpires) || !now.Before(entry.expires) {
		delete(d.users, id)
		return nil
	}
	entry.expires = now.Add(userIdleTTL)
	return entry
}

func (d *cachingDriver) GetUser(id string) (*model.User, error) {
	d.mu.Lock()
	entry := d.getUserEntry(id)
	d.mu.Unlock()

	if entry != nil {
		return copyUser(entry.user), nil
	}

	user, err := d.DbDriver.GetUser(id)
	if err != nil || user == nil {
		return user, err
	}

	stored := copyUser(user)
	now := d.now()

	d.mu.Lock()
	d.users[id] = &userCacheEntry{user: stored, expires: now.Add(userIdleTTL), hardExpires: now.Add(userMaxLifetime)}
	if user.Username != "" {
		d.userByName[user.Username] = id
	}
	d.mu.Unlock()

	return copyUser(user), nil
}

func (d *cachingDriver) GetUserByUsername(username string) (*model.User, error) {
	d.mu.Lock()
	id, ok := d.userByName[username]
	var entry *userCacheEntry
	if ok {
		entry = d.getUserEntry(id)
		if entry == nil {
			delete(d.userByName, username)
		}
	}
	d.mu.Unlock()

	if entry != nil {
		return copyUser(entry.user), nil
	}

	user, err := d.DbDriver.GetUserByUsername(username)
	if err != nil || user == nil {
		return user, err
	}

	stored := copyUser(user)
	now := d.now()

	d.mu.Lock()
	d.users[user.Id] = &userCacheEntry{user: stored, expires: now.Add(userIdleTTL), hardExpires: now.Add(userMaxLifetime)}
	if user.Username != "" {
		d.userByName[user.Username] = user.Id
	}
	d.mu.Unlock()

	return copyUser(user), nil
}

func (d *cachingDriver) invalidateUser(userId string) {
	delete(d.users, userId)
	d.userByName = make(map[string]string)
}

func (d *cachingDriver) SaveUser(user *model.User, updateFields []string) error {
	err := d.DbDriver.SaveUser(user, updateFields)
	d.mu.Lock()
	if user != nil {
		d.invalidateUser(user.Id)
	}
	d.mu.Unlock()
	return err
}

func (d *cachingDriver) DeleteUser(user *model.User) error {
	err := d.DbDriver.DeleteUser(user)
	d.mu.Lock()
	if user != nil {
		d.invalidateUser(user.Id)
	}
	d.mu.Unlock()
	return err
}

// ---------------------------------------------------------------------------
// Tokens and groups
// ---------------------------------------------------------------------------

func copyTokens(tokens []*model.Token) []*model.Token {
	out := make([]*model.Token, len(tokens))
	for i, t := range tokens {
		c := *t
		c.Scopes = append([]string(nil), t.Scopes...)
		out[i] = &c
	}
	return out
}

func copyGroups(groups []*model.Group) []*model.Group {
	out := make([]*model.Group, len(groups))
	for i, g := range groups {
		c := *g
		out[i] = &c
	}
	return out
}

func (d *cachingDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
	d.mu.Lock()
	entry := d.userTokens[userId]
	if entry != nil && d.expired(entry.expires) {
		delete(d.userTokens, userId)
		entry = nil
	}
	d.mu.Unlock()
	if entry != nil {
		return copyTokens(entry.tokens), nil
	}

	tokens, err := d.DbDriver.GetTokensForUser(userId)
	if err != nil {
		return tokens, err
	}
	d.mu.Lock()
	d.userTokens[userId] = &tokenCacheEntry{tokens: copyTokens(tokens), expires: d.now().Add(tokenTTL)}
	d.mu.Unlock()
	return tokens, nil
}

func (d *cachingDriver) SaveToken(token *model.Token) error {
	err := d.DbDriver.SaveToken(token)
	if token != nil {
		d.InvalidateTokens(token.UserId)
	}
	return err
}

func (d *cachingDriver) DeleteToken(token *model.Token) error {
	err := d.DbDriver.DeleteToken(token)
	if token != nil {
		d.InvalidateTokens(token.UserId)
	}
	return err
}

// InvalidateTokens drops a user's cached tokens.
func (d *cachingDriver) InvalidateTokens(userId string) {
	d.mu.Lock()
	delete(d.userTokens, userId)
	d.mu.Unlock()
}

func (d *cachingDriver) GetGroups() ([]*model.Group, error) {
	d.mu.Lock()
	entry := d.groups
	if entry != nil && d.expired(entry.expires) {
		d.groups, entry = nil, nil
	}
	d.mu.Unlock()
	if entry != nil {
		return copyGroups(entry.groups), nil
	}

	groups, err := d.DbDriver.GetGroups()
	if err != nil {
		return groups, err
	}
	d.mu.Lock()
	d.groups = &groupCacheEntry{groups: copyGroups(groups), expires: d.now().Add(cacheTTL)}
	d.mu.Unlock()
	return groups, nil
}

func (d *cachingDriver) SaveGroup(group *model.Group) error {
	err := d.DbDriver.SaveGroup(group)
	d.InvalidateGroups()
	return err
}

func (d *cachingDriver) DeleteGroup(group *model.Group) error {
	err := d.DbDriver.DeleteGroup(group)
	d.InvalidateGroups()
	return err
}

// InvalidateGroups drops the cached group list.
func (d *cachingDriver) InvalidateGroups() {
	d.mu.Lock()
	d.groups = nil
	d.mu.Unlock()
}

// TokensChanged drops a user's cached tokens. A server sharing its database
// with others calls it for each token change gossiped to it, as the change
// may already be in the database and so never saved, and so never seen,
// here.
func TokensChanged(userId string) {
	if d, ok := GetInstance().(*cachingDriver); ok {
		d.InvalidateTokens(userId)
	}
}

// GroupsChanged drops the cached group list, for the same reason.
func GroupsChanged() {
	if d, ok := GetInstance().(*cachingDriver); ok {
		d.InvalidateGroups()
	}
}

// ---------------------------------------------------------------------------
// Session storage passthrough (preserves the SessionStorage assertion the
// driver init performs — the inner driver keeps serving sessions).
// ---------------------------------------------------------------------------

func (d *cachingDriver) SaveSession(session *model.Session) error {
	if d.sessions == nil {
		return fmt.Errorf("session storage not supported by driver")
	}
	return d.sessions.SaveSession(session)
}

func (d *cachingDriver) DeleteSession(session *model.Session) error {
	if d.sessions == nil {
		return fmt.Errorf("session storage not supported by driver")
	}
	return d.sessions.DeleteSession(session)
}

func (d *cachingDriver) GetSession(id string) (*model.Session, error) {
	if d.sessions == nil {
		return nil, fmt.Errorf("session storage not supported by driver")
	}
	return d.sessions.GetSession(id)
}

func (d *cachingDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
	if d.sessions == nil {
		return nil, fmt.Errorf("session storage not supported by driver")
	}
	return d.sessions.GetSessionsForUser(userId)
}

func (d *cachingDriver) GetSessions() ([]*model.Session, error) {
	if d.sessions == nil {
		return nil, fmt.Errorf("session storage not supported by driver")
	}
	return d.sessions.GetSessions()
}
