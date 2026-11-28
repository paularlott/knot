// Package mcpstatekey keeps the key that signs MCP question state.
//
// When a tool asks the person something (MCP elicitation), a 2026-07-28
// client, and knot's web chat, answer by calling the tool again with the
// answers and a signed state. The call may land on any node of a cluster,
// and after a restart, so every node must sign with the same key.
//
// The key is generated once, stored in the database, and shared through the
// cluster: nodes exchange it when they join and in the periodic gossip, and
// every node keeps the oldest key it has seen (ties go to the smaller
// value), so nodes that started at the same time agree on one. Each change
// is applied to the MCP servers registered with Apply.
package mcpstatekey

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/mcp"
)

// cfgName is the config value the key is stored under.
const cfgName = "mcp_request_state_key"

// Key is the shared key and when it was first made.
type Key struct {
	Value     string    `json:"key" msgpack:"key"` // 32 random bytes, hex
	CreatedAt time.Time `json:"created_at" msgpack:"created_at"`
}

// valid reports whether k holds a usable key.
func (k Key) valid() bool {
	b, err := hex.DecodeString(k.Value)
	return err == nil && len(b) >= 32 && !k.CreatedAt.IsZero()
}

// wins reports whether k should replace current: the older key wins, and
// between keys made at the same moment, the smaller value.
func (k Key) wins(current Key) bool {
	if !current.valid() {
		return true
	}
	if !k.CreatedAt.Equal(current.CreatedAt) {
		return k.CreatedAt.Before(current.CreatedAt)
	}
	return k.Value < current.Value
}

var (
	mu      sync.Mutex
	current Key
	servers []*mcp.Server
)

// Current returns the key this node uses, loading or creating it on first
// use.
func Current() (Key, error) {
	mu.Lock()
	defer mu.Unlock()
	if current.valid() {
		return current, nil
	}
	k, err := loadOrCreate()
	if err != nil {
		return Key{}, err
	}
	current = k
	return current, nil
}

// loadOrCreate reads the stored key, or makes and stores a new one.
func loadOrCreate() (Key, error) {
	db := database.GetInstance()
	if v, err := db.GetCfgValue(cfgName); err == nil && v != nil && v.Value != "" {
		var k Key
		if json.Unmarshal([]byte(v.Value), &k) == nil && k.valid() {
			return k, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Key{}, err
	}
	k := Key{Value: hex.EncodeToString(raw), CreatedAt: time.Now().UTC()}
	if err := save(k); err != nil {
		return Key{}, err
	}
	return k, nil
}

func save(k Key) error {
	b, err := json.Marshal(k)
	if err != nil {
		return err
	}
	return database.GetInstance().SaveCfgValue(&model.CfgValue{Name: cfgName, Value: string(b)})
}

// Merge takes a key another node holds. When it wins over this node's, it
// is stored and applied, and Merge reports true.
func Merge(incoming Key) (bool, error) {
	if !incoming.valid() {
		return false, errors.New("mcpstatekey: invalid key")
	}
	if _, err := Current(); err != nil {
		return false, err
	}
	mu.Lock()
	defer mu.Unlock()
	if !incoming.wins(current) {
		return false, nil
	}
	if err := save(incoming); err != nil {
		return false, err
	}
	current = incoming
	applyLocked()
	return true, nil
}

// Apply signs the question state of each server with the shared key, now
// and whenever the key changes.
func Apply(s ...*mcp.Server) error {
	if _, err := Current(); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	servers = append(servers, s...)
	applyLocked()
	return nil
}

func applyLocked() {
	raw, err := hex.DecodeString(current.Value)
	if err != nil {
		return
	}
	for _, s := range servers {
		if s != nil {
			s.SetRequestStateKey(raw)
		}
	}
}

// reset forgets the loaded key and servers (tests).
func reset() {
	mu.Lock()
	defer mu.Unlock()
	current = Key{}
	servers = nil
}
