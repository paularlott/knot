package driver_redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
)

// newLiveDriver returns a driver backed by miniredis, or a real server when
// KNOT_REDIS_LIVE_ADDR is set (e.g. KNOT_REDIS_LIVE_ADDR=127.0.0.1:6379).
func newLiveDriver(t *testing.T) *RedisDbDriver {
	t.Helper()
	addr := os.Getenv("KNOT_REDIS_LIVE_ADDR")
	if addr == "" {
		server := miniredis.RunT(t)
		addr = server.Addr()
	}
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{addr},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(client.Close)
	return &RedisDbDriver{prefix: "livetest:", connection: client}
}

func TestLiveSetGetDelete(t *testing.T) {
	db := newLiveDriver(t)
	ctx := context.Background()

	if err := db.set(ctx, "k1", []byte(`{"a":1}`), 0); err != nil {
		t.Fatalf("set []byte: %v", err)
	}
	v, err := db.get(ctx, "k1")
	if err != nil || v != `{"a":1}` {
		t.Fatalf("get: %q %v", v, err)
	}

	if err := db.set(ctx, "k2", "plain", 0); err != nil {
		t.Fatalf("set string: %v", err)
	}
	if v, err = db.get(ctx, "k2"); err != nil || v != "plain" {
		t.Fatalf("get string: %q %v", v, err)
	}

	// missing key surfaces valkey.Nil which convertRedisError maps to nil
	_, err = db.get(ctx, "missing")
	if convertRedisError(err) != nil {
		t.Fatalf("expected nil for missing key, got %v", err)
	}

	if err := db.del(ctx, "k1", "k2"); err != nil {
		t.Fatalf("del: %v", err)
	}
}

func TestLiveSetTTL(t *testing.T) {
	db := newLiveDriver(t)
	ctx := context.Background()

	if err := db.set(ctx, "ttl", "v", 500*time.Millisecond); err != nil {
		t.Fatalf("set with ttl: %v", err)
	}
	if v, err := db.get(ctx, "ttl"); err != nil || v != "v" {
		t.Fatalf("get before expiry: %q %v", v, err)
	}
	time.Sleep(700 * time.Millisecond)
	if _, err := db.get(ctx, "ttl"); convertRedisError(err) != nil {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestLiveSetNX(t *testing.T) {
	db := newLiveDriver(t)
	ctx := context.Background()

	ok, err := db.setNX(ctx, "nx1", "1", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("first setNX: %v %v", ok, err)
	}
	ok, err = db.setNX(ctx, "nx1", "2", 10*time.Second)
	if err != nil || ok {
		t.Fatalf("second setNX should be refused: %v %v", ok, err)
	}
	db.del(ctx, "nx1")
}

func TestLiveScanAndKeys(t *testing.T) {
	db := newLiveDriver(t)
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		if err := db.set(ctx, "scan:"+string(rune('a'+i%26))+string(rune('a'+i/26)), "v", 0); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	found := 0
	iter := db.scan(ctx, "scan:*")
	for iter.Next(ctx) {
		found++
	}
	if err := iter.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if found != 50 {
		t.Fatalf("scan found %d keys, want 50", found)
	}

	keys, err := db.keys(ctx, "scan:a*")
	if err != nil || len(keys) != 2 { // "scan:aa" and "scan:ab"
		t.Fatalf("keys: %d %v", len(keys), err)
	}

	// cleanup everything this test wrote
	iter = db.scan(ctx, "scan:*")
	var all []string
	for iter.Next(ctx) {
		all = append(all, iter.Val())
	}
	if len(all) > 0 {
		if err := db.del(ctx, all...); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}
}
