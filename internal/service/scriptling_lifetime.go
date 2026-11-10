package service

import (
	"context"
	"sync"

	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs/fssecurity"
	scriptlingmercure "github.com/paularlott/scriptling/extlibs/mercure"
	"github.com/paularlott/scriptling/extlibs/netsecurity"
	"github.com/paularlott/scriptling/extlibs/provision/become"
)

// scriptLifetime bounds what a script leaves open in an environment: the
// Mercure clients and subscriptions it made. It ends when the environment is
// reset for its next use (the pools reset an env between uses) or cleaned up,
// so nothing one run opened outlives it or reaches the next user's run.
type scriptLifetime struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// newScriptLifetime starts env's first lifetime; every Reset of env ends it
// and starts the next.
func newScriptLifetime(env *scriptling.Scriptling) *scriptLifetime {
	l := &scriptLifetime{}
	l.renew()
	env.OnReset(l.renew)
	return l
}

// current is the lifetime a client made now belongs to.
func (l *scriptLifetime) current() context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ctx
}

// renew ends the current lifetime and starts the next.
func (l *scriptLifetime) renew() {
	ctx, cancel := context.WithCancel(context.Background())
	l.mu.Lock()
	old := l.cancel
	l.ctx, l.cancel = ctx, cancel
	l.mu.Unlock()
	if old != nil {
		old()
	}
}

// end ends the current lifetime: the env is being thrown away.
func (l *scriptLifetime) end() {
	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()
	cancel()
}

// registerMercure registers scriptling.mercure: connections follow the
// network policy, a CA bundle named with verify="path" must lie within paths,
// and clients close with the script's lifetime.
func registerMercure(env *scriptling.Scriptling, paths fssecurity.Config, policy *netsecurity.Config, life *scriptLifetime) {
	scriptlingmercure.RegisterConfigured(env, paths, policy, scriptlingmercure.WithLifetime(life.current))
}

// agentBecome runs agent scripts' become=True provisioning steps as root,
// through sudo. The agent runs as the space's user, inside their own space,
// so root there is theirs to use: sudo is asked without a password first, so
// a space with passwordless sudo is never prompted. One root helper serves
// every agent script; it is started on first use.
var agentBecome = sync.OnceValue(func() *become.Become {
	return become.New(become.Config{})
})
