package authratelimit

import (
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
)

func withRateConfig(t *testing.T, attempts, windowSec, blockSec int) {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		AuthRateLimitAttempts: attempts,
		AuthRateLimitWindow:   windowSec,
		AuthRateLimitBlock:    blockSec,
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
}

func resetLimiters() {
	ipMutex.Lock()
	ipLimiters = make(map[string]*loginAttempts)
	ipMutex.Unlock()
	emailMutex.Lock()
	emailLimiters = make(map[string]*loginAttempts)
	emailMutex.Unlock()
}

func TestRateLimitBlocksAfterThreshold(t *testing.T) {
	withRateConfig(t, 3, 60, 300)
	resetLimiters()

	for i := 0; i < 2; i++ {
		RecordFailure("10.0.0.1", "alice@example.com")
		if Blocked("10.0.0.1", "alice@example.com") {
			t.Fatalf("blocked too early after %d failures", i+1)
		}
	}

	RecordFailure("10.0.0.1", "alice@example.com")
	if !Blocked("10.0.0.1", "alice@example.com") {
		t.Fatal("should be blocked at the threshold")
	}
	if Blocked("10.0.0.2", "bob@example.com") {
		t.Error("other keys must not be affected")
	}
}

func TestRateLimitBlockExpires(t *testing.T) {
	withRateConfig(t, 1, 60, 300)
	resetLimiters()

	RecordFailure("10.0.0.1", "a@example.com")
	if !Blocked("10.0.0.1", "a@example.com") {
		t.Fatal("should be blocked immediately")
	}

	expireBlocks("10.0.0.1", "a@example.com")

	if Blocked("10.0.0.1", "a@example.com") {
		t.Error("block should expire")
	}
}

func TestRateLimitWindowPrunesOldFailures(t *testing.T) {
	withRateConfig(t, 3, 1, 300)
	resetLimiters()

	RecordFailure("10.0.0.1", "a@example.com")
	RecordFailure("10.0.0.1", "a@example.com")

	ageFailures("10.0.0.1", "a@example.com")

	RecordFailure("10.0.0.1", "a@example.com")
	if Blocked("10.0.0.1", "a@example.com") {
		t.Error("old failures should not count towards the threshold")
	}
}

func TestSuccessfulLoginClearsLimiters(t *testing.T) {
	withRateConfig(t, 3, 60, 300)
	resetLimiters()

	RecordFailure("10.0.0.1", "alice@example.com")
	RecordFailure("10.0.0.1", "alice@example.com")
	Clear("10.0.0.1", "alice@example.com")

	RecordFailure("10.0.0.1", "alice@example.com")
	RecordFailure("10.0.0.1", "alice@example.com")
	if Blocked("10.0.0.1", "alice@example.com") {
		t.Error("successful login should have reset the counters")
	}
}

func TestRateLimitDefaults(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	attempts, window, block := rateLimitConfig()
	if attempts != 10 || window != 60*time.Second || block != 300*time.Second {
		t.Errorf("unexpected defaults: attempts=%d window=%s block=%s", attempts, window, block)
	}
}

// Cluster propagation: a failure recorded on one server and gossiped as an
// Event must count towards the budget on the receiving server; a tripped
// block must block the receiver; a clear must reset it.
func TestGossipPropagationBetweenServers(t *testing.T) {
	withRateConfig(t, 3, 60, 300)

	// "Server A": two failures below threshold, gossiped as plain events.
	resetLimiters()
	for i := 0; i < 2; i++ {
		_, _ = RecordFailure("10.9.9.9", "carol@example.com")
	}

	// "Server B": fresh state — apply the two gossiped failures.
	emailMutex.Lock()
	emailA := append([]time.Time{}, emailLimiters["carol@example.com"].failures...)
	ipMutex.Lock()
	ipA := append([]time.Time{}, ipLimiters["10.9.9.9"].failures...)
	ipMutex.Unlock()
	emailMutex.Unlock()

	resetLimiters()
	for _, at := range ipA {
		ApplyEvent(&Event{IP: "10.9.9.9", Email: "carol@example.com", At: at})
	}
	_ = emailA

	if Blocked("10.9.9.9", "carol@example.com") {
		t.Fatal("two gossiped failures should not block yet")
	}

	// One more local failure on server B trips the shared budget.
	RecordFailure("10.9.9.9", "carol@example.com")
	if !Blocked("10.9.9.9", "carol@example.com") {
		t.Fatal("gossiped failures must count towards the shared threshold")
	}

	// A tripped block gossiped from server A blocks server B immediately.
	resetLimiters()
	until := time.Now().Add(5 * time.Minute)
	ApplyEvent(&Event{IP: "10.9.9.9", Email: "carol@example.com", At: time.Now(), BlockUntil: until})
	if !Blocked("10.9.9.9", "carol@example.com") {
		t.Fatal("a gossiped block must apply on the receiver")
	}

	// A successful login on server A clears server B too.
	ApplyEvent(&Event{IP: "10.9.9.9", Email: "carol@example.com", At: time.Now(), Clear: true})
	if Blocked("10.9.9.9", "carol@example.com") {
		t.Fatal("a gossiped clear must reset the receiver")
	}
}

// expireBlocks back-dates the block deadline for both keys.
func expireBlocks(ip, email string) {
	ipMutex.Lock()
	if e, ok := ipLimiters[ip]; ok {
		e.blockedUntil = time.Now().Add(-time.Second)
	}
	ipMutex.Unlock()
	emailMutex.Lock()
	if e, ok := emailLimiters[email]; ok {
		e.blockedUntil = time.Now().Add(-time.Second)
	}
	emailMutex.Unlock()
}

// ageFailures back-dates recorded failures for both keys so they fall
// outside a short window.
func ageFailures(ip, email string) {
	old := time.Now().Add(-2 * time.Second)
	ipMutex.Lock()
	if e, ok := ipLimiters[ip]; ok {
		for i := range e.failures {
			e.failures[i] = old
		}
	}
	ipMutex.Unlock()
	emailMutex.Lock()
	if e, ok := emailLimiters[email]; ok {
		for i := range e.failures {
			e.failures[i] = old
		}
	}
	emailMutex.Unlock()
}
