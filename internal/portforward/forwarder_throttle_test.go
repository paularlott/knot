package portforward

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestThrottledWriter_Latency verifies that a latency setting delays writes
// by approximately the configured amount.
func TestThrottledWriter_Latency(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(50, 0, 0) // 50ms latency, no jitter, unlimited bandwidth

	writer := newThrottledWriter(&buf, fwd)
	data := []byte("hello world")

	start := time.Now()
	n, err := writer.Write(data)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Wrote %d bytes, expected %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("Buffer = %q, expected %q", buf.Bytes(), data)
	}
	if elapsed < 45*time.Millisecond {
		t.Fatalf("Latency not applied: elapsed=%v, expected >=45ms", elapsed)
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("Latency too high: elapsed=%v, expected <150ms", elapsed)
	}
}

// TestThrottledWriter_NoThrottle verifies that a ForwardInfo with no throttle
// settings returns the original writer (passthrough).
func TestThrottledWriter_NoThrottle(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{} // no throttle set

	writer := newThrottledWriter(&buf, fwd)
	if _, ok := writer.(*throttledWriter); ok {
		t.Fatal("Expected original writer when no throttle set, got throttledWriter")
	}
}

// TestThrottledWriter_NilForward verifies nil ForwardInfo returns original writer.
func TestThrottledWriter_NilForward(t *testing.T) {
	var buf bytes.Buffer
	writer := NewThrottledWriter(&buf, nil)
	if _, ok := writer.(*throttledWriter); ok {
		t.Fatal("Should not wrap when ForwardInfo is nil")
	}
}

// TestThrottledWriter_Bandwidth verifies that bandwidth limiting slows down
// writes proportional to data size.
func TestThrottledWriter_Bandwidth(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 1) // 1 KB/s = 1024 bytes/sec

	writer := newThrottledWriter(&buf, fwd)
	data := make([]byte, 512)
	for i := range data {
		data[i] = 'x'
	}

	start := time.Now()
	n, err := writer.Write(data)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Wrote %d bytes, expected %d", n, len(data))
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("Bandwidth not limited: elapsed=%v, expected >=400ms for 512 bytes at 1KB/s", elapsed)
	}
}

// TestThrottledWriter_Jitter verifies jitter adds variance within bounds.
func TestThrottledWriter_Jitter(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(100, 50, 0) // 100ms latency, 50ms jitter

	for i := 0; i < 5; i++ {
		buf.Reset()
		writer := newThrottledWriter(&buf, fwd)

		start := time.Now()
		_, err := writer.Write([]byte("x"))
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
		// Expected: 100ms ± 50ms = [50ms, 150ms]
		if elapsed < 40*time.Millisecond {
			t.Fatalf("Write %d: too fast, elapsed=%v, expected >=40ms", i, elapsed)
		}
		if elapsed > 250*time.Millisecond {
			t.Fatalf("Write %d: too slow, elapsed=%v, expected <=250ms", i, elapsed)
		}
	}
}

// TestThrottledWriter_LargeWrite verifies bandwidth with larger payloads.
func TestThrottledWriter_LargeWrite(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 10) // 10 KB/s

	writer := newThrottledWriter(&buf, fwd)
	data := make([]byte, 1024)

	start := time.Now()
	n, err := writer.Write(data)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 1024 {
		t.Fatalf("Wrote %d bytes, expected 1024", n)
	}
	// 1KB at 10KB/s should take ~100ms
	if elapsed < 80*time.Millisecond {
		t.Fatalf("Bandwidth not limited: elapsed=%v, expected >=80ms", elapsed)
	}
}

// TestThrottledWriter_DynamicUpdate verifies throttle settings can change
// on a running forward (throttledWriter reads from ForwardInfo per Write).
func TestThrottledWriter_DynamicUpdate(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(50, 0, 0) // 50ms latency

	writer := newThrottledWriter(&buf, fwd)

	// First write: ~50ms latency
	start := time.Now()
	writer.Write([]byte("a"))
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("First write should have ~50ms latency")
	}

	// Clear latency
	fwd.SetThrottle(0, 0, 0)

	// Second write: fast (latency now 0)
	start = time.Now()
	writer.Write([]byte("b"))
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("Second write should be fast after clearing latency, elapsed=%v", elapsed)
	}
}

// TestForwardInfo_SetThrottle verifies Set/Get/Has throttle methods.
func TestForwardInfo_SetThrottle(t *testing.T) {
	fwd := &ForwardInfo{}

	lat, jit, bw := fwd.GetThrottle()
	if lat != 0 || jit != 0 || bw != 0 {
		t.Fatalf("Initial should be zero, got lat=%d jit=%d bw=%d", lat, jit, bw)
	}
	if fwd.HasThrottle() {
		t.Fatal("HasThrottle should be false initially")
	}

	fwd.SetThrottle(100, 20, 512)
	lat, jit, bw = fwd.GetThrottle()
	if lat != 100 || jit != 20 || bw != 512 {
		t.Fatalf("After SetThrottle(100,20,512): got lat=%d jit=%d bw=%d", lat, jit, bw)
	}
	if !fwd.HasThrottle() {
		t.Fatal("HasThrottle should be true after setting values")
	}

	fwd.SetThrottle(0, 0, 0)
	if fwd.HasThrottle() {
		t.Fatal("HasThrottle should be false after reset")
	}
}

// TestFindForwardBySpace verifies lookup by space name.
func TestFindForwardBySpace(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartForward(19999, 8080, "test-target", cancel)
	defer StopForward(19999)

	found := FindForwardBySpace("test-target")
	if found == nil {
		t.Fatal("FindForwardBySpace returned nil")
	}
	if found.LocalPort != 19999 {
		t.Fatalf("LocalPort=%d, expected 19999", found.LocalPort)
	}

	if FindForwardBySpace("no-such-space") != nil {
		t.Fatal("Should return nil for non-existent space")
	}
}
