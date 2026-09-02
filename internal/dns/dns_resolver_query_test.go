package dns

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// startTestDNSServer starts a UDP DNS server on a random loopback port using the
// given handler and returns its "host:port" address plus a cleanup function.
func startTestDNSServer(t *testing.T, handler dns.Handler) (string, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create UDP listener: %v", err)
	}
	server := &dns.Server{PacketConn: pc, Handler: handler}
	started := make(chan struct{})
	server.NotifyStartedFunc = func() { close(started) }
	go func() { _ = server.ActivateAndServe() }()
	<-started
	return pc.LocalAddr().String(), func() { _ = server.Shutdown() }
}

// fixedAnswerHandler answers A queries for the configured names, optionally
// after a delay, and counts the queries it receives.
type fixedAnswerHandler struct {
	records map[string][]net.IP
	ttl     uint32
	delay   time.Duration
	mu      sync.Mutex
	queries int
}

func (h *fixedAnswerHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	h.mu.Lock()
	h.queries++
	h.mu.Unlock()
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	m := new(dns.Msg)
	m.SetReply(r)
	if len(r.Question) > 0 {
		name := r.Question[0].Name
		if ips, ok := h.records[name]; ok && r.Question[0].Qtype == dns.TypeA {
			for _, ip := range ips {
				m.Answer = append(m.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: h.ttl},
					A:   ip,
				})
			}
		}
	}
	_ = w.WriteMsg(m)
}

func (h *fixedAnswerHandler) QueryCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.queries
}

// nxdomainAnswerHandler always replies with NXDOMAIN, optionally after a delay.
type nxdomainAnswerHandler struct {
	delay time.Duration
}

func (h *nxdomainAnswerHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	m := new(dns.Msg)
	m.SetRcode(r, dns.RcodeNameError)
	_ = w.WriteMsg(m)
}

// droppingAnswerHandler never responds (simulates an unreachable server).
type droppingAnswerHandler struct{}

func (h *droppingAnswerHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {}

func newResolverWithServers(t *testing.T, servers []string, timeout time.Duration) *DNSResolver {
	t.Helper()
	r := NewDNSResolver(ResolverConfig{QueryTimeout: timeout})
	t.Cleanup(r.Stop)
	// Set servers directly (they already include host:port from the test servers).
	r.mu.Lock()
	r.nameservers = append([]string(nil), servers...)
	r.mu.Unlock()
	return r
}

func TestQueryUpstream_SingleServerSuccess(t *testing.T) {
	h := &fixedAnswerHandler{
		records: map[string][]net.IP{"svc.example.com.": {net.ParseIP("1.2.3.4")}},
		ttl:     300,
	}
	addr, cleanup := startTestDNSServer(t, h)
	defer cleanup()

	r := newResolverWithServers(t, []string{addr}, 2*time.Second)

	records, err := r.QueryUpstream("svc.example.com", "A")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(records) != 1 || records[0].Target != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %+v", records)
	}
}

// TestQueryUpstream_ValidAnswerWinsOverErroringServer ensures a transport error
// from one server does not prevent returning a valid answer from another, and
// is not surfaced as an overall failure.
func TestQueryUpstream_ValidAnswerWinsOverErroringServer(t *testing.T) {
	valid := &fixedAnswerHandler{
		records: map[string][]net.IP{"svc.example.com.": {net.ParseIP("10.0.0.9")}},
		ttl:     300,
	}
	validAddr, validCleanup := startTestDNSServer(t, valid)
	defer validCleanup()

	// "127.0.0.1:1" has nothing listening -> transport error.
	r := newResolverWithServers(t, []string{"127.0.0.1:1", validAddr}, 2*time.Second)

	records, err := r.QueryUpstream("svc.example.com", "A")
	if err != nil {
		t.Fatalf("expected valid answer despite one erroring server, got: %v", err)
	}
	if len(records) != 1 || records[0].Target != "10.0.0.9" {
		t.Fatalf("expected 10.0.0.9, got %+v", records)
	}
}

// TestQueryUpstream_ValidAnswerWinsOverSlowNXDOMAIN ensures a valid answer is
// returned even when another server (slowly) returns NXDOMAIN.
func TestQueryUpstream_ValidAnswerWinsOverSlowNXDOMAIN(t *testing.T) {
	valid := &fixedAnswerHandler{
		records: map[string][]net.IP{"here.example.com.": {net.ParseIP("10.1.1.1")}},
		ttl:     300,
	}
	validAddr, validCleanup := startTestDNSServer(t, valid)
	defer validCleanup()

	nx := &nxdomainAnswerHandler{delay: 150 * time.Millisecond}
	nxAddr, nxCleanup := startTestDNSServer(t, nx)
	defer nxCleanup()

	r := newResolverWithServers(t, []string{nxAddr, validAddr}, 2*time.Second)

	records, err := r.QueryUpstream("here.example.com", "A")
	if err != nil {
		t.Fatalf("expected valid answer, got: %v", err)
	}
	if len(records) != 1 || records[0].Target != "10.1.1.1" {
		t.Fatalf("expected 10.1.1.1, got %+v", records)
	}
}

// TestQueryUpstream_AllServersFailReturnsError verifies that when every server
// fails at the transport level, an error is returned rather than a nil record
// set with no error.
func TestQueryUpstream_AllServersFailReturnsError(t *testing.T) {
	r := newResolverWithServers(t, []string{"127.0.0.1:1", "127.0.0.1:2"}, 500*time.Millisecond)

	records, err := r.QueryUpstream("dead.example.com", "A")
	if err == nil {
		t.Fatalf("expected an error when all servers fail, got records: %+v", records)
	}
}

// TestQueryUpstream_NoSpuriousCancellationFailure repeatedly runs a fast-vs-slow
// race to shake out the previous bug where cancelling the losing sibling caused
// a spurious "all nameservers failed" error even though a valid answer was
// available. Before the fix this failed intermittently.
func TestQueryUpstream_NoSpuriousCancellationFailure(t *testing.T) {
	fast := &fixedAnswerHandler{
		records: map[string][]net.IP{"race.example.com.": {net.ParseIP("10.2.2.2")}},
		ttl:     300,
	}
	fastAddr, fastCleanup := startTestDNSServer(t, fast)
	defer fastCleanup()

	slow := &fixedAnswerHandler{
		records: map[string][]net.IP{"race.example.com.": {net.ParseIP("10.3.3.3")}},
		ttl:     300,
		delay:   40 * time.Millisecond,
	}
	slowAddr, slowCleanup := startTestDNSServer(t, slow)
	defer slowCleanup()

	r := newResolverWithServers(t, []string{fastAddr, slowAddr}, 2*time.Second)

	for i := 0; i < 50; i++ {
		r.ClearCache()
		records, err := r.QueryUpstream("race.example.com", "A")
		if err != nil {
			t.Fatalf("iteration %d: unexpected error (spurious cancellation?): %v", i, err)
		}
		if len(records) == 0 {
			t.Fatalf("iteration %d: expected a valid answer, got none", i)
		}
	}
}

// TestQueryUpstream_AllNXDOMAINReturnsError verifies that when every server
// returns NXDOMAIN (no valid answer records), QueryUpstream reports failure
// rather than hanging or returning empty success.
func TestQueryUpstream_AllNXDOMAINReturnsError(t *testing.T) {
	nx1 := &nxdomainAnswerHandler{}
	nx2 := &nxdomainAnswerHandler{}
	addr1, cleanup1 := startTestDNSServer(t, nx1)
	defer cleanup1()
	addr2, cleanup2 := startTestDNSServer(t, nx2)
	defer cleanup2()

	r := newResolverWithServers(t, []string{addr1, addr2}, 2*time.Second)

	records, err := r.QueryUpstream("nope.example.com", "A")
	if err == nil {
		t.Fatalf("expected error for all-NXDOMAIN, got records: %+v", records)
	}
}

// TestQueryUpstream_TimeoutWithDroppingServer verifies the ctx.Done() path:
// a server that never responds should cause a timeout error, not a hang.
func TestQueryUpstream_TimeoutWithDroppingServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout-dependent test in -short mode")
	}
	dropAddr, dropCleanup := startTestDNSServer(t, &droppingAnswerHandler{})
	defer dropCleanup()

	r := newResolverWithServers(t, []string{dropAddr}, 400*time.Millisecond)

	start := time.Now()
	_, err := r.QueryUpstream("slow.example.com", "A")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error from a dropping server")
	}
	if elapsed > 3*time.Second {
		t.Errorf("took too long to time out: %v", elapsed)
	}
}
