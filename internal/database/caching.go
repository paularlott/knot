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
