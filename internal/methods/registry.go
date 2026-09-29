package methods

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/paularlott/knot/internal/database/model"
)

var (
	ErrMethodNotFound = errors.New("method not found")
	ErrPermission     = errors.New("method not visible to caller")
	ErrMethodDraining = errors.New("method temporarily unavailable")
	ErrMethodLeased   = errors.New("method exclusively leased")
)

type Entry struct {
	MethodDefinition
	SpaceID   string
	SpaceName string
	OwnerID   string
	Owner     string
	Server    ServerConfig
	inFlight  int
	draining  bool
}

type Registry struct {
	mu           sync.RWMutex
	entries      map[string][]*Entry
	bySpace      map[string][]*Entry
	rrCursor     map[string]uint64
	drainChecker func(spaceID string) bool
	leaseChecker func(spaceID string) bool
}

var defaultRegistry = NewRegistry()

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func NewRegistry() *Registry {
	return &Registry{
		entries:  make(map[string][]*Entry),
		bySpace:  make(map[string][]*Entry),
		rrCursor: make(map[string]uint64),
	}
}

// SetDrainChecker installs a callback that Register consults to determine
// whether a space should be draining even if no prior entries exist (e.g.
// after an unregister/register churn cycle). The PoolService sets this
// so drain state survives method server restarts.
func (r *Registry) SetDrainChecker(fn func(spaceID string) bool) {
	r.mu.Lock()
	r.drainChecker = fn
	r.mu.Unlock()
}

// SetLeaseChecker installs a callback that Pick and List consult to
// determine whether a space is exclusively leased (a pool member checked
// out via a lease). Leased entries are excluded from shared routing for
// everyone; the lease holder reaches its space through PickForSpace, which
// deliberately bypasses the check. The PoolService sets this.
func (r *Registry) SetLeaseChecker(fn func(spaceID string) bool) {
	r.mu.Lock()
	r.leaseChecker = fn
	r.mu.Unlock()
}

func (r *Registry) Register(space *model.Space, owner *model.User, reg *Registration) error {
	if space == nil || owner == nil || reg == nil {
		return fmt.Errorf("space, owner and registration are required")
	}

	newEntries := make([]*Entry, 0, len(reg.Methods))
	for _, method := range reg.Methods {
		newEntries = append(newEntries, &Entry{
			MethodDefinition: method,
			SpaceID:          space.Id,
			SpaceName:        space.Name,
			OwnerID:          owner.Id,
			Owner:            owner.Username,
			Server:           reg.Server,
		})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	oldEntries := r.bySpace[space.Id]
	wasDraining := false
	for _, old := range oldEntries {
		if old.draining {
			wasDraining = true
			break
		}
	}
	// Also consult the external drain checker — covers the case where
	// UnregisterSpace removed all entries (e.g. method server restart)
	// but the space is still being drained by the pool sweep.
	if !wasDraining && r.drainChecker != nil && r.drainChecker(space.Id) {
		wasDraining = true
	}

	for _, newEntry := range newEntries {
		for existingName, entries := range r.entries {
			for _, existing := range entries {
				if existing.SpaceID == space.Id {
					continue
				}
				if existingName == newEntry.Name {
					if !sameVisibleRegistration(existing, newEntry) {
						return fmt.Errorf("method %q already registered with different definition", newEntry.Name)
					}
					continue
				}
				if existing.MCPTool && newEntry.MCPTool && MCPToolName(existingName) == MCPToolName(newEntry.Name) {
					return fmt.Errorf("method %q produces the same MCP tool name as %q", newEntry.Name, existingName)
				}
			}
		}
	}

	for _, oldEntry := range oldEntries {
		r.removeEntryLocked(oldEntry)
	}
	delete(r.bySpace, space.Id)

	for _, newEntry := range newEntries {
		if wasDraining {
			newEntry.draining = true
		}
		r.entries[newEntry.Name] = append(r.entries[newEntry.Name], newEntry)
		r.bySpace[space.Id] = append(r.bySpace[space.Id], newEntry)
	}

	return nil
}

func (r *Registry) UnregisterSpace(spaceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.bySpace[spaceID] {
		r.removeEntryLocked(entry)
	}
	delete(r.bySpace, spaceID)
}

func (r *Registry) Drain(spaceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.bySpace[spaceID] {
		entry.draining = true
	}
}

func (r *Registry) Undrain(spaceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.bySpace[spaceID] {
		entry.draining = false
	}
}

func (r *Registry) InFlightForSpace(spaceID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, entry := range r.bySpace[spaceID] {
		total += entry.inFlight
	}
	return total
}

func (r *Registry) removeEntryLocked(entry *Entry) {
	entries := r.entries[entry.Name]
	for i, existing := range entries {
		if existing == entry {
			entries = append(entries[:i], entries[i+1:]...)
			break
		}
	}
	if len(entries) == 0 {
		delete(r.entries, entry.Name)
	} else {
		r.entries[entry.Name] = entries
	}
}

// Count returns the total number of registered method entries across all
// spaces and names. Each duplicate exact-match registration counts once. Useful
// for debug/diagnostic logging; not part of the discovery surface.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, entries := range r.entries {
		total += len(entries)
	}
	return total
}

func (r *Registry) List(user *model.User) []MethodInfo {
	if user == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// First pass: group visible entries by their visible name so we can
	// count providers.
	type visibleGroup struct {
		first    *Entry
		count    int
		draining int
		leased   int
	}
	groups := map[string]*visibleGroup{}
	var order []string

	for _, entries := range r.entries {
		for _, entry := range entries {
			name, ok := visibleName(entry, user)
			if !ok {
				continue
			}
			g, exists := groups[name]
			if !exists {
				g = &visibleGroup{first: entry, count: 0}
				groups[name] = g
				order = append(order, name)
			}
			g.count++
			if r.isLeasedLocked(entry.SpaceID) {
				g.leased++
			} else if entry.draining {
				g.draining++
			}
		}
	}

	result := make([]MethodInfo, 0, len(order))
	for _, name := range order {
		g := groups[name]
		liveCount := g.count - g.draining - g.leased
		if liveCount <= 0 {
			continue
		}
		info := g.first.info()
		info.Name = name
		info.ProviderCount = liveCount
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (r *Registry) Pick(methodName string, user *model.User) (*Entry, string, error) {
	if user == nil {
		return nil, "", ErrPermission
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var candidates []*Entry
	drainedVisible := 0
	leasedVisible := 0
	seen := map[*Entry]bool{}
	add := func(e *Entry) {
		if !seen[e] {
			candidates = append(candidates, e)
			seen[e] = true
		}
	}

	// Direct canonical-name lookup. Matches when the caller uses the canonical
	// name as visibleName produces it (the owner's bare/dotted name, or a
	// non-owner's user.<owner>.<bare> form for bare canonical names).
	for _, entry := range r.entries[methodName] {
		if name, ok := visibleName(entry, user); ok && name == methodName {
			if r.isLeasedLocked(entry.SpaceID) {
				leasedVisible++
				continue
			}
			if entry.draining {
				drainedVisible++
				continue
			}
			add(entry)
		}
	}

	if strings.HasPrefix(methodName, "user.") {
		// Scan all entries for a visibleName match. Handles bare canonical
		// names where visibleName produces "user.<owner>.<bare>".
		for _, entries := range r.entries {
			for _, entry := range entries {
				if name, ok := visibleName(entry, user); ok && name == methodName {
					if r.isLeasedLocked(entry.SpaceID) {
						if !seen[entry] {
							leasedVisible++
						}
						continue
					}
					if entry.draining {
						if !seen[entry] {
							drainedVisible++
						}
						continue
					}
					add(entry)
				}
			}
		}

		// Strip "user.<owner>." and try the remainder as a canonical name.
		if parts := strings.SplitN(methodName, ".", 3); len(parts) == 3 {
			ownerName, rest := parts[1], parts[2]
			if rest != "" {
				for _, entry := range r.entries[rest] {
					if entry.Owner != ownerName {
						continue
					}
					if _, ok := visibleName(entry, user); !ok {
						continue
					}
					if r.isLeasedLocked(entry.SpaceID) {
						if !seen[entry] {
							leasedVisible++
						}
						continue
					}
					if entry.draining {
						if !seen[entry] {
							drainedVisible++
						}
						continue
					}
					add(entry)
				}
			}
		}
	}

	if len(candidates) == 0 {
		// All visible providers are exclusively leased — the method exists
		// but is checked out to a lease holder.
		if leasedVisible > 0 {
			return nil, "", ErrMethodLeased
		}
		// All visible providers are draining — method exists but temporarily
		// unavailable.
		if drainedVisible > 0 {
			return nil, "", ErrMethodDraining
		}
		// Direct form: caller used a canonical name that exists in the
		// registry but they failed scope/permission/group filtering.
		if _, exists := r.entries[methodName]; exists {
			return nil, "", ErrPermission
		}
		// Namespaced form (user.<owner>.<canonical>): the caller's input
		// is never a key in r.entries (those are canonicals), so the direct
		// check above can't tell permission-denied from not-found. Strip the
		// prefix and check whether any entry has that canonical AND is owned
		// by the named user. If so, the method exists but the caller failed
		// filtering — return ErrPermission (403), not ErrMethodNotFound (404).
		if strings.HasPrefix(methodName, "user.") {
			if parts := strings.SplitN(methodName, ".", 3); len(parts) == 3 {
				ownerName, rest := parts[1], parts[2]
				if rest != "" {
					for _, entry := range r.entries[rest] {
						if entry.Owner == ownerName {
							return nil, "", ErrPermission
						}
					}
				}
			}
		}
		return nil, "", ErrMethodNotFound
	}

	selected := r.pickCandidateLocked(methodName, candidates)
	selected.inFlight++
	return selected, selected.LocalName, nil
}

// PickForSpace selects the entry for methodName registered by the given
// space, visibility-checked for the caller. It bypasses lease and drain
// exclusion: the caller explicitly targets this space, which is how the
// holder of an exclusive pool lease reaches its member. At most one entry
// can match, so no load balancing is involved.
func (r *Registry) PickForSpace(methodName, spaceID string, user *model.User) (*Entry, string, error) {
	if user == nil {
		return nil, "", ErrPermission
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Accept the name in whatever visible form the caller uses: the bare
	// canonical (owner), or the namespaced user.<owner>.<canonical> form
	// (anyone the method is shared with).
	for _, entry := range r.bySpace[spaceID] {
		if name, ok := visibleName(entry, user); ok && name == methodName {
			entry.inFlight++
			return entry, entry.LocalName, nil
		}
	}
	if strings.HasPrefix(methodName, "user.") {
		if parts := strings.SplitN(methodName, ".", 3); len(parts) == 3 && parts[2] != "" {
			for _, entry := range r.bySpace[spaceID] {
				if entry.Name != parts[2] {
					continue
				}
				if _, ok := visibleName(entry, user); !ok {
					return nil, "", ErrPermission
				}
				entry.inFlight++
				return entry, entry.LocalName, nil
			}
		}
	}

	// The method exists on the space but the caller can't see it.
	for _, entry := range r.bySpace[spaceID] {
		if entry.Name == methodName {
			return nil, "", ErrPermission
		}
	}
	return nil, "", ErrMethodNotFound
}

func (r *Registry) isLeasedLocked(spaceID string) bool {
	return r.leaseChecker != nil && r.leaseChecker(spaceID)
}

func (r *Registry) pickCandidateLocked(routeName string, candidates []*Entry) *Entry {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].inFlight != candidates[j].inFlight {
			return candidates[i].inFlight < candidates[j].inFlight
		}
		if candidates[i].SpaceID != candidates[j].SpaceID {
			return candidates[i].SpaceID < candidates[j].SpaceID
		}
		if candidates[i].OwnerID != candidates[j].OwnerID {
			return candidates[i].OwnerID < candidates[j].OwnerID
		}
		return candidates[i].LocalName < candidates[j].LocalName
	})

	minInFlight := candidates[0].inFlight
	tied := candidates[:0]
	for _, candidate := range candidates {
		if candidate.inFlight != minInFlight {
			break
		}
		tied = append(tied, candidate)
	}

	cursor := r.rrCursor[routeName]
	selected := tied[int(cursor%uint64(len(tied)))]
	r.rrCursor[routeName] = cursor + 1
	return selected
}

func (r *Registry) Done(entry *Entry) {
	if entry == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry.inFlight > 0 {
		entry.inFlight--
	}
}

func visibleName(entry *Entry, user *model.User) (string, bool) {
	// Owner always sees the bare canonical name. Critical: a user must never
	// see their own methods under the user.<owner>. namespace — that's for
	// other users only.
	if entry.OwnerID == user.Id {
		return entry.Name, true
	}
	if entry.Scope != ScopeShared {
		return "", false
	}
	if !user.HasPermission(model.PermissionUseMethods) {
		return "", false
	}
	if len(entry.Groups) > 0 && !user.HasAnyGroup(&entry.Groups) {
		return "", false
	}
	// Non-owners always see shared methods under the user.<owner>. namespace,
	// regardless of whether the canonical name contains a dot. This keeps the
	// display unambiguous (the caller can tell whose method it is) and makes
	// the calling convention uniform across bare and dotted canonical names.
	return "user." + entry.Owner + "." + entry.Name, true
}

func (e *Entry) info() MethodInfo {
	return MethodInfo{
		Name:         e.Name,
		LocalName:    e.LocalName,
		Description:  e.Description,
		Keywords:     e.Keywords,
		Scope:        e.Scope,
		Groups:       e.Groups,
		MCPTool:      e.MCPTool,
		Events:       e.Events,
		EventSinks:   e.EventSinks,
		ParamsSchema: e.ParamsSchema,
		ResultSchema: e.ResultSchema,
		OwnerID:      e.OwnerID,
		Owner:        e.Owner,
	}
}

func sameVisibleRegistration(a, b *Entry) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name &&
		a.LocalName == b.LocalName &&
		a.Description == b.Description &&
		a.Scope == b.Scope &&
		a.MCPTool == b.MCPTool &&
		reflect.DeepEqual(a.Keywords, b.Keywords) &&
		reflect.DeepEqual(a.Groups, b.Groups) &&
		reflect.DeepEqual(a.ParamsSchema, b.ParamsSchema) &&
		reflect.DeepEqual(a.ResultSchema, b.ResultSchema)
}
