package mcpstatekey

import (
	"context"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/mcp"
)

func useTempDB(t *testing.T) {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()}})
	t.Cleanup(func() { config.SetServerConfig(prev); reset() })
	reset()
}

func key(b byte, at time.Time) Key {
	v := make([]byte, 64)
	for i := range v {
		v[i] = "0123456789abcdef"[int(b)%16]
	}
	return Key{Value: string(v), CreatedAt: at}
}

// asker is a server with one tool that asks a question.
func asker() *mcp.Server {
	s := mcp.NewServer("t", "1")
	s.RegisterTool(mcp.NewTool("ask", "asks"), func(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
		res, err := mcp.Elicit(ctx, mcp.ElicitForm{Key: "ok", Message: "Go?", Schema: map[string]any{
			"type": "object", "properties": map[string]any{"approve": map[string]any{"type": "boolean"}}}})
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResponseText(string(res.Action)), nil
	})
	return s
}

// answeredOn reports whether b accepts the answer to a question a asked.
func answeredOn(t *testing.T, a, b *mcp.Server) bool {
	t.Helper()
	_, need, err := a.CallToolWithInput(context.Background(), "ask", nil, nil)
	if err != nil || need == nil {
		t.Fatal(need, err)
	}
	_, _, err = b.CallToolWithInput(context.Background(), "ask", nil, &mcp.ToolInput{
		Responses: map[string]*mcp.ElicitResult{"ok": {Action: mcp.ElicitDecline}},
		State:     need.State,
	})
	return err == nil
}

func TestKeyIsCreatedOnceAndKept(t *testing.T) {
	useTempDB(t)
	first, err := Current()
	if err != nil || !first.valid() {
		t.Fatalf("first: %+v %v", first, err)
	}
	reset() // as after a restart: read back from the database
	again, err := Current()
	if err != nil || again.Value != first.Value || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("after restart: %+v, want %+v (%v)", again, first, err)
	}
}

func TestMergeKeepsTheOldestKey(t *testing.T) {
	useTempDB(t)
	mine, err := Current()
	if err != nil {
		t.Fatal(err)
	}

	if changed, err := Merge(key(1, mine.CreatedAt.Add(time.Hour))); err != nil || changed {
		t.Fatalf("a newer key replaced ours: %v %v", changed, err)
	}
	older := key(2, mine.CreatedAt.Add(-time.Hour))
	if changed, err := Merge(older); err != nil || !changed {
		t.Fatalf("an older key was not taken: %v %v", changed, err)
	}
	if got, _ := Current(); got.Value != older.Value {
		t.Fatalf("current = %+v", got)
	}
	reset()
	if got, _ := Current(); got.Value != older.Value {
		t.Fatalf("the merged key was not stored: %+v", got)
	}

	// Made at the same moment: the smaller value wins, so both sides agree.
	tie := key(1, older.CreatedAt)
	if changed, _ := Merge(tie); !changed {
		t.Fatal("the smaller of two keys made at the same time was not taken")
	}
	if changed, _ := Merge(older); changed {
		t.Fatal("the larger of two keys made at the same time replaced the smaller")
	}

	if _, err := Merge(Key{Value: "short", CreatedAt: time.Now()}); err == nil {
		t.Fatal("an invalid key was accepted")
	}
}

// Servers sharing the key accept each other's question state, also after the
// key changes under them (another node's older key arrived).
func TestApplySharesTheKeyAcrossServers(t *testing.T) {
	useTempDB(t)
	a, b, other := asker(), asker(), asker()
	if err := Apply(a, b); err != nil {
		t.Fatal(err)
	}
	if !answeredOn(t, a, b) {
		t.Fatal("servers sharing the key refused each other's state")
	}
	if answeredOn(t, a, other) {
		t.Fatal("a server without the key accepted the state")
	}

	mine, _ := Current()
	if changed, err := Merge(key(3, mine.CreatedAt.Add(-time.Minute))); err != nil || !changed {
		t.Fatal(changed, err)
	}
	if !answeredOn(t, a, b) {
		t.Fatal("after the key changed, the servers no longer agree")
	}
}
