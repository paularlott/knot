package agent_client

import (
	"context"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
)

// cliLockTTL is how long the agent trusts its last look at whether the
// space's command line is restricted.
const cliLockTTL = 3 * time.Second

var cliLock struct {
	sync.Mutex
	locked    bool
	checkedAt time.Time
}

// CliLocked reports whether the space has locked its processes out of knot
// (knot agent restrict --no-cli). The agent then refuses their requests,
// other than restarting or shutting the space down. It asks the server,
// with the agent's own token, at most every few seconds; if the server
// can't be asked, the last answer stands.
func (c *AgentClient) CliLocked() bool {
	cliLock.Lock()
	defer cliLock.Unlock()
	if time.Since(cliLock.checkedAt) < cliLockTTL {
		return cliLock.locked
	}

	server, token := c.GetServerURL(), c.GetAgentToken()
	if server == "" || token == "" {
		return cliLock.locked
	}
	client, err := apiclient.NewClient(server, token, config.GetAgentConfig().TLS.SkipVerify)
	if err != nil {
		return cliLock.locked
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, _, err := client.GetSpaceRestrictions(ctx, c.GetSpaceId())
	if err != nil {
		log.WithError(err).Warn("couldn't check whether the space's command line is restricted")
		return cliLock.locked
	}
	cliLock.locked = res.Restricted && res.NoCli
	cliLock.checkedAt = time.Now()
	return cliLock.locked
}
