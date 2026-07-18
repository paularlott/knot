package portforward

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// TestThrottledWriter_Latency verifies that a latency setting delays writes
// by approximately the configured amount.
func TestThrottledWriter_Latency(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(50, 0, 0, 0, false) // 50ms latency, no jitter, unlimited bandwidth, no timeout

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
	fwd.SetThrottle(0, 0, 1, 0, false) // 1 KB/s = 1024 bytes/sec

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
	fwd.SetThrottle(100, 50, 0, 0, false) // 100ms latency, 50ms jitter

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
	fwd.SetThrottle(0, 0, 10, 0, false) // 10 KB/s

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
	fwd.SetThrottle(50, 0, 0, 0, false) // 50ms latency

	writer := newThrottledWriter(&buf, fwd)

	// First write: ~50ms latency
	start := time.Now()
	writer.Write([]byte("a"))
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("First write should have ~50ms latency")
	}

	// Clear latency
	fwd.SetThrottle(0, 0, 0, 0, false)

	// Second write: fast (latency now 0)
	start = time.Now()
	writer.Write([]byte("b"))
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("Second write should be fast after clearing latency, elapsed=%v", elapsed)
	}
}

// TestThrottledWriter_Timeout verifies that a timeout setting kills the
// connection (returns error) after the specified duration.
func TestThrottledWriter_Timeout(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 0, 50, false) // 50ms timeout, no other limits

	writer := newThrottledWriter(&buf, fwd)

	// First write: should succeed (within 50ms)
	n, err := writer.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("First Write failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("Wrote %d, expected 5", n)
	}

	// Wait past the timeout
	time.Sleep(80 * time.Millisecond)

	// Second write: should fail with ErrClosedPipe
	_, err = writer.Write([]byte("world"))
	if err == nil {
		t.Fatal("Expected error after timeout, got nil")
	}
	if err != io.ErrClosedPipe {
		t.Fatalf("Expected io.ErrClosedPipe, got %v", err)
	}
}

// TestThrottledWriter_TimeoutFastRequest verifies that data written within
// the timeout window passes through successfully.
func TestThrottledWriter_TimeoutFastRequest(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 0, 1000, false) // 1s timeout

	writer := newThrottledWriter(&buf, fwd)

	// Write immediately — well within the 1s window
	data := []byte("quick request")
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed within timeout: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Wrote %d, expected %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("Buffer = %q, expected %q", buf.Bytes(), data)
	}
}

// TestForwardInfo_SetThrottle verifies Set/Get/Has throttle methods.
func TestForwardInfo_SetThrottle(t *testing.T) {
	fwd := &ForwardInfo{}

	lat, jit, bw, to, d := fwd.GetThrottle()
	if lat != 0 || jit != 0 || bw != 0 || to != 0 || d {
		t.Fatalf("Initial should be zero, got lat=%d jit=%d bw=%d to=%d d=%v", lat, jit, bw, to, d)
	}
	if fwd.HasThrottle() {
		t.Fatal("HasThrottle should be false initially")
	}

	fwd.SetThrottle(100, 20, 512, 0, false)
	lat, jit, bw, to, d = fwd.GetThrottle()
	if lat != 100 || jit != 20 || bw != 512 || to != 0 || d {
		t.Fatalf("After SetThrottle(100,20,512,0,false): got lat=%d jit=%d bw=%d to=%d d=%v", lat, jit, bw, to, d)
	}
	if !fwd.HasThrottle() {
		t.Fatal("HasThrottle should be true after setting values")
	}

	fwd.SetThrottle(0, 0, 0, 0, false)
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

// TestThrottledWriter_Down verifies that setting down=true blocks all writes.
func TestThrottledWriter_Down(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 0, 0, true) // down=true

	writer := newThrottledWriter(&buf, fwd)
	_, err := writer.Write([]byte("blocked"))
	if err == nil {
		t.Fatal("Expected error when down=true, got nil")
	}
	if err != io.ErrClosedPipe {
		t.Fatalf("Expected io.ErrClosedPipe, got %v", err)
	}
}

// TestThrottledWriter_DownThenUp verifies toggling down off restores traffic.
func TestThrottledWriter_DownThenUp(t *testing.T) {
	var buf bytes.Buffer
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 0, 0, true) // down

	writer := newThrottledWriter(&buf, fwd)
	_, err := writer.Write([]byte("blocked"))
	if err == nil {
		t.Fatal("Expected block when down=true")
	}

	// Toggle up
	fwd.SetThrottle(0, 0, 0, 0, false)

	n, err := writer.Write([]byte("restored"))
	if err != nil {
		t.Fatalf("Write failed after toggling up: %v", err)
	}
	if n != 8 {
		t.Fatalf("Wrote %d, expected 8", n)
	}
}
