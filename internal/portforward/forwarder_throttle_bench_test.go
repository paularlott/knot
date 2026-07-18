package portforward

import (
	"bytes"
	"io"
	"testing"
)

// BenchmarkRawCopy is the baseline: io.Copy with no throttle wrapper.
func BenchmarkRawCopy(b *testing.B) {
	src := bytes.NewReader(make([]byte, 1024*1024)) // 1MB
	var dst bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.Reset(make([]byte, 1024*1024))
		dst.Reset()
		io.Copy(&dst, src)
	}
}

// BenchmarkThrottledCopy_NoThrottle measures the overhead when throttle
// is configured but set to zero (the normal case). newThrottledWriter
// should return the original writer, so this should match BenchmarkRawCopy.
func BenchmarkThrottledCopy_NoThrottle(b *testing.B) {
	fwd := &ForwardInfo{}
	fwd.SetThrottle(0, 0, 0, 0, false) // no throttle

	src := bytes.NewReader(make([]byte, 1024*1024))
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.Reset(make([]byte, 1024*1024))
		buf.Reset()
		// This is what the forwarder does per connection
		writer := newThrottledWriter(&buf, fwd)
		io.Copy(writer, src)
	}
}

// BenchmarkThrottledCopy_WithLatency measures throughput with 1ms latency
// applied (to show the throttle mechanism works but isn't the focus here).
func BenchmarkThrottledCopy_WithLatency(b *testing.B) {
	fwd := &ForwardInfo{}
	fwd.SetThrottle(1, 0, 0, 0, false) // 1ms latency

	data := make([]byte, 1024) // small writes to see latency effect
	src := bytes.NewReader(data)
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.Reset(data)
		buf.Reset()
		writer := &throttledWriter{dest: &buf, fwd: fwd, rng: nil}
		io.Copy(writer, src)
	}
}
