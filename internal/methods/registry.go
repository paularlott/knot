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
)

type Entry struct {
	MethodDefinition
	SpaceID   string
	SpaceName string
	OwnerID   string
	Owner     string
	Server    ServerConfig
	inFlight  int
}

type Registry struct {
	mu      sync.RWMutex
	entries map[string][]*Entry
	bySpace map[string][]*Entry
}

var defaultRegistry = NewRegistry()

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string][]*Entry),
		bySpace: make(map[string][]*Entry),
	}
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

	result := []MethodInfo{}
	seen := map[string]bool{}
	for _, entries := range r.entries {
		for _, entry := range entries {
			name, ok := visibleName(entry, user)
			if !ok || seen[name] {
				continue
			}
			info := entry.info()
			info.Name = name
			result = append(result, info)
			seen[name] = true
		}
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
			add(entry)
		}
	}

	if strings.HasPrefix(methodName, "user.") {
		// Scan all entries for a visibleName match. Handles bare canonical
		// names where visibleName produces "user.<owner>.<bare>".
		for _, entries := range r.entries {
			for _, entry := range entries {
				if name, ok := visibleName(entry, user); ok && name == methodName {
					add(entry)
				}
			}
		}

		// Strip "user.<owner>." and try the remainder as a canonical name.
		// Lets a caller use "user.paul.method.search" against a canonical
		// "method.search" — visibleName doesn't add the user.<owner>. prefix
		// to names that already contain a dot, so without this branch the
		// namespaced call form wouldn't route for dotted canonical names.
		// We pin the lookup to entries owned by the named user to avoid
		// accidentally matching the same dotted name registered by a
		// different owner.
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
					add(entry)
				}
			}
		}
	}

	if len(candidates) == 0 {
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
		// This avoids leaking method existence via a 404-vs-403 oracle while
		// still returning 404 for genuinely unknown methods.
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

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].inFlight < candidates[j].inFlight
	})

	candidates[0].inFlight++
	return candidates[0], candidates[0].LocalName, nil
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
		ParamsSchema: e.ParamsSchema,
		ResultSchema: e.ResultSchema,
		SpaceID:      e.SpaceID,
		SpaceName:    e.SpaceName,
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
