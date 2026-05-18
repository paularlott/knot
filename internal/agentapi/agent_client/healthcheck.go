package agent_client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
	"github.com/paularlott/scriptling/object"
)

const healthCheckTickInterval = 5 * time.Second

type spaceHealthState struct {
	nextCheck           time.Time
	consecutiveFailures uint32
	wasUnhealthy        bool
}

var (
	healthStateMu sync.Mutex
	healthState   = make(map[string]*spaceHealthState)
)

// RunHealthChecks starts the health check loop for the agent.
// It runs once per agent process (not per server connection).
func (c *AgentClient) RunHealthChecks() {
	logger := log.WithGroup("healthcheck")
	logger.Info("starting health check runner")

	ticker := time.NewTicker(healthCheckTickInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.runHealthCheckTick(logger)
	}
}

func (c *AgentClient) runHealthCheckTick(logger logger.Logger) {
	c.healthCheckMu.RLock()
	hcType := c.healthCheckType
	hcConfig := c.healthCheckConfig
	hcSkipSSL := c.healthCheckSkipSSLVerify
	hcTimeout := c.healthCheckTimeout
	hcInterval := c.healthCheckInterval
	hcMaxFailures := c.healthCheckMaxFailures
	hcAutoRestart := c.healthCheckAutoRestart
	c.healthCheckMu.RUnlock()

	if hcType == model.HealthCheckNone || hcType == "" {
		return
	}

	healthStateMu.Lock()
	state, ok := healthState[c.spaceId]
	if !ok {
		state = &spaceHealthState{nextCheck: time.Now()}
		healthState[c.spaceId] = state
	}
	if time.Now().Before(state.nextCheck) {
		healthStateMu.Unlock()
		return
	}
	interval := time.Duration(hcInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	state.nextCheck = time.Now().Add(interval)
	healthStateMu.Unlock()

	script := buildHealthCheckScript(hcType, hcConfig, hcSkipSSL, hcTimeout)
	if script == "" {
		return
	}

	logger.Debug("running health check", "space_id", c.spaceId, "type", hcType)

	result := runHealthCheckScript(script)

	healthStateMu.Lock()
	unhealthy := result != nil && !result.Healthy
	if unhealthy {
		state.consecutiveFailures++
	}
	failures := state.consecutiveFailures
	wasUnhealthy := state.wasUnhealthy

	if unhealthy {
		state.wasUnhealthy = true
	} else if wasUnhealthy {
		state.wasUnhealthy = false
		state.consecutiveFailures = 0
	} else {
		state.consecutiveFailures = 0
	}
	healthStateMu.Unlock()

	if hcMaxFailures == 0 {
		hcMaxFailures = 3
	}

	if unhealthy {
		if failures >= hcMaxFailures {
			logger.Error("health check failing", "space_id", c.spaceId, "failures", failures)
		} else {
			logger.Warn("health check failed", "space_id", c.spaceId, "failures", failures)
		}
	} else if wasUnhealthy {
		logger.Info("health check recovered", "space_id", c.spaceId)
	} else {
		logger.Debug("health check passed", "space_id", c.spaceId)
	}

	// Store result — reportState picks it up on the next tick
	c.healthMu.Lock()
	c.healthy = !unhealthy
	c.healthMu.Unlock()

	if hcAutoRestart && unhealthy && failures >= hcMaxFailures {
		logger.Error("health check threshold reached, requesting restart", "space_id", c.spaceId, "failures", failures)
		healthStateMu.Lock()
		state.consecutiveFailures = 0
		state.wasUnhealthy = false
		healthStateMu.Unlock()
		if err := c.SendSpaceRestart(); err != nil {
			logger.Error("failed to send restart", "space_id", c.spaceId, "error", err)
		}
	}
}

func buildHealthCheckScript(hcType, hcConfig string, skipSSL bool, timeout uint32) string {
	if timeout == 0 {
		timeout = 10
	}
	switch hcType {
	case model.HealthCheckHTTP:
		ssl := "False"
		if skipSSL {
			ssl = "True"
		}
		return fmt.Sprintf("import knot.healthcheck as hc\nhc.check_result(hc.http_head(%q, skip_ssl_verify=%s, timeout=%d))", hcConfig, ssl, timeout)
	case model.HealthCheckTCP:
		return fmt.Sprintf("import knot.healthcheck as hc\nhc.check_result(hc.tcp_port(%s, timeout=%d))", hcConfig, timeout)
	case model.HealthCheckProgram:
		return fmt.Sprintf("import knot.healthcheck as hc\nhc.check_result(hc.program(%q, timeout=%d))", hcConfig, timeout)
	case model.HealthCheckCustom:
		return hcConfig
	}
	return ""
}

func runHealthCheckScript(script string) *knotscriptling.HealthCheckResult {
	env, err := service.NewHealthCheckScriptlingEnv()
	if err != nil {
		return &knotscriptling.HealthCheckResult{Healthy: false}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, evalErr := env.EvalWithContext(ctx, script)

	if ex, ok := object.AsException(result); ok && ex.IsSystemExit() {
		if hcResult, ok := knotscriptling.ParseHealthCheckResult(ex.Message); ok {
			return hcResult
		}
	}

	if evalErr != nil {
		if hcResult, ok := knotscriptling.ParseHealthCheckResult(evalErr.Error()); ok {
			return hcResult
		}
		return &knotscriptling.HealthCheckResult{Healthy: false}
	}

	return &knotscriptling.HealthCheckResult{Healthy: true}
}
