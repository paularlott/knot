package driver_redis

import (
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/paularlott/logger"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

// withRedisConfig points the global config at the given Redis settings for
// the duration of the test, restoring whatever was there before.
func withRedisConfig(t *testing.T, redis config.RedisConfig) {
	t.Helper()
	if prev := config.GetServerConfig(); prev != nil {
		saved := prev.Redis
		prev.Redis = redis
		t.Cleanup(func() { prev.Redis = saved })
		return
	}
	fresh := &config.ServerConfig{Redis: redis}
	config.SetServerConfig(fresh)
	t.Cleanup(func() { config.SetServerConfig(nil) })
}

// TestDriverConnect drives the real Connect() path (config -> client options
// -> prefix normalization -> migration) against miniredis.
func TestDriverConnect(t *testing.T) {
	server := miniredis.RunT(t)

	withRedisConfig(t, config.RedisConfig{
		Enabled: true,
		Hosts:   []string{server.Addr()},
		// no trailing colon: Connect must normalize to "pre:"
		KeyPrefix: "pre",
	})

	db := &RedisDbDriver{}
	if err := db.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.connection.Close()

	if db.prefix != "pre:" {
		t.Fatalf("prefix should be normalized to %q, got %q", "pre:", db.prefix)
	}

	// The connected driver must serve real traffic through the same key
	// layout the rest of the driver uses.
	if err := db.SaveCfgValue(&model.CfgValue{Name: "node_id", Value: "n1"}); err != nil {
		t.Fatalf("SaveCfgValue: %v", err)
	}
	v, err := db.GetCfgValue("node_id")
	if err != nil || v == nil || v.Value != "n1" {
		t.Fatalf("GetCfgValue: %+v %v", v, err)
	}
	if !server.Exists("pre:Configs:node_id") {
		t.Fatalf("key should live under the normalized prefix")
	}
}

// TestRealConnectRetriesWhenServerDown proves a dead Redis doesn't kill the
// process: realConnect keeps retrying, and recovers once the server appears.
func TestRealConnectRetriesWhenServerDown(t *testing.T) {
	// Reserve a port then free it, giving an address nothing listens on.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	oldDelay := connectRetryDelay
	connectRetryDelay = 5 * time.Millisecond
	prevCfg := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{Redis: config.RedisConfig{
		Enabled: true,
		Hosts:   []string{addr},
	}})
	// Restore shared state only after the goroutine below has exited.
	defer func() {
		connectRetryDelay = oldDelay
		config.SetServerConfig(prevCfg)
	}()

	db := &RedisDbDriver{logger: logger.NewNullLogger()}
	done := make(chan struct{})
	go func() {
		db.realConnect()
		close(done)
	}()

	// After many retry periods realConnect must still be trying: had it
	// Fatal'ed the test process would be gone; had it returned, done closes.
	select {
	case <-done:
		t.Fatal("realConnect gave up instead of retrying")
	case <-time.After(60 * time.Millisecond):
	}

	// Bring a server up on the same address: the next retry must recover.
	server := miniredis.NewMiniRedis()
	if err := server.StartAddr(addr); err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("realConnect did not recover once the server came up")
	}
}
