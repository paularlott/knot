package agent_client

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/logger"

	"github.com/paularlott/scriptling"
)

// pooledEnv pairs a scriptling env with its cleanup (plugin scope close).
type pooledEnv struct {
	env     *scriptling.Scriptling
	cleanup func()
}

// envPool is a bounded warm pool of scriptling environments. It holds at most
// maxIdle envs; when the pool is empty on Acquire, a fresh env is created
// (cold start). On Release, if the pool is full the env is discarded.
//
// One pool serves both the non-streaming (handleExecuteScript) and streaming
// (handleExecuteScriptStream) paths. Per-call I/O (output writer, input reader,
// argv, console stub) is swapped after Acquire and reset before Release.
//
// Startup scripts bypass the pool entirely — they use a different code path
// and never call Acquire/Release.
type envPool struct {
	mu      sync.Mutex
	idle    []*pooledEnv
	maxIdle int
}

var scriptPool = &envPool{maxIdle: 5}

// Acquire returns a warm env from the pool, or creates a fresh one when the
// pool is empty. The caller MUST call Release with the returned env and cleanup.
func (p *envPool) Acquire() (*scriptling.Scriptling, func(), error) {
	p.mu.Lock()
	if n := len(p.idle); n > 0 {
		pe := p.idle[n-1]
		p.idle = p.idle[:n-1]
		size := len(p.idle)
		p.mu.Unlock()
		log.Debug("script pool: acquired warm env", "pool_size", size)
		return pe.env, pe.cleanup, nil
	}
	p.mu.Unlock()

	env, cleanup, err := createPooledEnv()
	if err != nil {
		return nil, nil, err
	}
	log.Debug("script pool: created fresh env (pool was empty)")
	return env, cleanup, nil
}

// Release resets the env and returns it to the pool. If the pool is full the
// env is discarded (cleanup called). Safe to call from a defer.
func (p *envPool) Release(env *scriptling.Scriptling, cleanup func()) {
	// Disconnect any per-call I/O before reset so stray writes don't hit a
	// closed stream.
	env.SetOutputWriter(io.Discard)
	env.Reset()

	p.mu.Lock()
	if len(p.idle) < p.maxIdle {
		p.idle = append(p.idle, &pooledEnv{env: env, cleanup: cleanup})
		size := len(p.idle)
		p.mu.Unlock()
		log.Debug("script pool: returned env to pool", "pool_size", size)
		return
	}
	p.mu.Unlock()

	if cleanup != nil {
		cleanup()
	}
	log.Debug("script pool: discarded env (pool full)")
}

// createPooledEnv builds a fresh env with the superset of libraries (the
// streaming set, which includes everything the non-streaming path registers
// plus subprocess, runtime, sandbox, etc.). Output is set to io.Discard as a
// safe default; callers swap it per-call via SetOutputWriter / EnableOutputCapture.
func createPooledEnv() (*scriptling.Scriptling, func(), error) {
	var client *apiclient.ApiClient
	var userId string

	if agentClient != nil {
		server := agentClient.GetServerURL()
		token := agentClient.GetAgentToken()
		if server != "" && token != "" {
			c, err := apiclient.NewClient(server, token, true)
			if err == nil {
				c.SetTimeout(6 * time.Minute)
				client = c
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if user, err := client.WhoAmI(ctx); err == nil {
					userId = user.Id
				}
				cancel()
			}
		}
	}

	var customLogger logger.Logger
	if agentClient != nil {
		customLogger = NewAgentClientLogger(agentClient, "script")
	}

	// Use NewRemoteStreamingScriptlingEnv with Discard/nil I/O. It registers
	// the full library superset. Per-call argv is set via RegisterSysLibrary
	// after Acquire; per-call I/O is swapped via SetOutputWriter/SetInputReader.
	return service.NewRemoteStreamingScriptlingEnv(nil, client, userId, customLogger, io.Discard, nil)
}
