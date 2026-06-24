package agent_client

import (
	"testing"
	"time"
)

// TestServerIsRediscoverableAfterGiveUp demonstrates a regression
// introduced in commit 3043d2f3 ("Handle duplicate addresses").
//
// Symptom in production: agent reports "connected" (it has sessions
// with other servers in the zone), but one server shows the space as
// not connected. The agent never tries to reconnect to that server.
//
// Root cause: the agent tracks every endpoint it has ever seen in
// knownServerAddresses (populated at register time from the server's
// response.AgentEndpoint, and when adding new servers). When the
// agent gives up on a server after maxConnectionAttempts
// (agent_server.go:67-86), it removes the entry from serverList but
// leaves it in knownServerAddresses. The next reportState() loop
// (report_state.go:180) skips any endpoint present in
// knownServerAddresses, so the agent never re-attempts the discarded
// server even when other servers keep advertising it.
//
// Before 3043d2f3, the filter was against serverList, which meant a
// discarded server would be rediscovered on the next state ping. The
// change to filter against knownServerAddresses fixed duplicate
// dialling when a server advertises itself by both IP and hostname,
// but broke recovery after give-up.
//
// This test drives the real give-up cleanup (agentServer.abandonLocked,
// called from the maxConnectionAttempts branch of ConnectAndServe) and
// asserts that afterwards the peer is no longer in knownServerAddresses,
// so the discovery filter re-adds it on the next state ping.
func TestServerIsRediscoverableAfterGiveUp(t *testing.T) {
	c := NewAgentClient("default.example.com:443", "space-1")

	// The default server is still "connected" so the agent stays alive
	// and keeps calling reportState().
	c.serverListMutex.Lock()
	defaultConn := NewAgentServer("default.example.com:443", "space-1", c)
	c.serverList["default.example.com:443"] = defaultConn
	c.knownServerAddresses["default.example.com:443"] = true
	c.serverListMutex.Unlock()
	defaultConn.cancel() // prevent the background ConnectAndServe goroutine from dialling

	// The agent previously discovered a peer server (added to both
	// serverList and knownServerAddresses) and learned its canonical
	// endpoint at registration (an alias).
	peer := NewAgentServer("peer.example.com:443", "space-1", c)
	peer.cancel()
	c.serverListMutex.Lock()
	c.serverList["peer.example.com:443"] = peer
	c.knownServerAddresses["peer.example.com:443"] = true
	c.knownServerAddresses["10.0.0.9:443"] = true // alias reported by peer
	peer.aliases["10.0.0.9:443"] = true
	c.serverListMutex.Unlock()

	// The peer has a blip and the agent exhausts its connection attempts.
	// This is exactly what the give-up branch of ConnectAndServe runs.
	c.serverListMutex.Lock()
	peer.abandonLocked()
	c.serverListMutex.Unlock()

	// After give-up neither the peer's dial address nor its alias may
	// remain in knownServerAddresses, otherwise the discovery filter
	// blocks recovery forever (the 3043d2f3 regression).
	c.serverListMutex.RLock()
	if c.knownServerAddresses["peer.example.com:443"] {
		c.serverListMutex.RUnlock()
		t.Fatal("peer dial address still in knownServerAddresses after give-up; recovery is blocked")
	}
	if c.knownServerAddresses["10.0.0.9:443"] {
		c.serverListMutex.RUnlock()
		t.Fatal("peer alias still in knownServerAddresses after give-up; recovery is blocked")
	}
	c.serverListMutex.RUnlock()

	// On the next state ping the default server re-advertises the peer
	// (alive again). Immediately after give-up the peer is held in the
	// rediscovery cooldown, so the discovery filter must NOT re-add it yet
	// (otherwise a genuinely-down server is hammered every state ping).
	endpoints := []string{"default.example.com:443", "peer.example.com:443"}

	c.serverListMutex.RLock()
	duringCooldown := c.discoverNewServersLocked(endpoints)
	c.serverListMutex.RUnlock()
	if stringInSlice("peer.example.com:443", duringCooldown) {
		t.Fatalf("peer.example.com:443 was rediscovered during the cooldown window; "+
			"a down server would be re-dialled every state ping; got %v", duringCooldown)
	}

	// Once the cooldown elapses the peer must be rediscovered. Simulate the
	// passage of time by ageing the cooldown entries past rediscoverCooldown.
	c.serverListMutex.Lock()
	expired := time.Now().Add(-2 * rediscoverCooldown)
	c.recentlyGaveUp["peer.example.com:443"] = expired
	c.recentlyGaveUp["10.0.0.9:443"] = expired
	afterCooldown := c.discoverNewServersLocked(endpoints)
	c.serverListMutex.Unlock()

	if !stringInSlice("peer.example.com:443", afterCooldown) {
		t.Fatalf("peer.example.com:443 was not rediscovered after the cooldown elapsed; "+
			"knownServerAddresses is blocking recovery (regression from 3043d2f3); got %v", afterCooldown)
	}
}

// TestKnownServerAddressesSurfacesServerAliases documents why the
// 3043d2f3 change was made: a single physical server can be
// reachable via multiple addresses (IP and hostname, or several IPs),
// and without knownServerAddresses the agent would open parallel
// connections to the same server. This is the legitimate use case
// that the give-up regression is colliding with — any fix must
// preserve it.
func TestKnownServerAddressesSurfacesServerAliases(t *testing.T) {
	c := NewAgentClient("default.example.com:443", "space-1")

	// Agent is connected to the default server by hostname, and the
	// server has told the agent its canonical endpoint.
	c.serverListMutex.Lock()
	defaultConn := NewAgentServer("default.example.com:443", "space-1", c)
	c.serverList["default.example.com:443"] = defaultConn
	c.knownServerAddresses["default.example.com:443"] = true
	c.knownServerAddresses["10.0.0.5:443"] = true // reported by server via response.AgentEndpoint
	c.serverListMutex.Unlock()
	defaultConn.cancel()

	// Another server reports both the hostname AND the IP. The agent
	// must NOT dial either one again.
	endpoints := []string{"default.example.com:443", "10.0.0.5:443"}

	var newServers []string
	for _, reportedServer := range endpoints {
		if !c.knownServerAddresses[reportedServer] {
			if !stringInSlice(reportedServer, newServers) {
				newServers = append(newServers, reportedServer)
			}
		}
	}

	if len(newServers) != 0 {
		t.Fatalf("alias addresses were not filtered out, got %v", newServers)
	}
}
