package toolapproval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
	mcpopenai "github.com/paularlott/mcp/ai/openai"
)

const approvalTimeout = 2 * time.Minute

// EventWriter is satisfied by rest.StreamWriter (and mcpopenai.SSEEventWriter).
type EventWriter interface {
	WriteEvent(eventType string, data any) error
}

// Manager tracks pending tool approvals in memory. Each approval is owned by
// the instance that started the tool call. When a response arrives on a
// different instance behind a load balancer it is forwarded via gossip.
type Manager struct {
	instanceID string
	mu         sync.Mutex
	pending    map[string]*pendingApproval
}

type pendingApproval struct {
	userID string
	result chan bool
}

// Event is the JSON payload sent as a tool_approval SSE comment.
type Event struct {
	RequestID  string         `json:"request_id"`
	InstanceID string         `json:"instance_id"`
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	ExpiresAt  int64          `json:"expires_at"`
}

// ResponseRequest is sent by the browser and forwarded between instances.
type ResponseRequest struct {
	RequestID  string `json:"request_id"`
	InstanceID string `json:"instance_id"`
	Approved   bool   `json:"approved"`
	UserID     string `json:"user_id,omitempty"`
}

// Forwarder forwards an approval response to the originating instance via gossip.
type Forwarder interface {
	ForwardToolApproval(req *ResponseRequest) (bool, error)
}

var defaultManager *Manager

func SetManager(m *Manager) { defaultManager = m }
func GetManager() *Manager  { return defaultManager }

func NewManager() *Manager {
	return &Manager{
		instanceID: loadInstanceID(),
		pending:    make(map[string]*pendingApproval),
	}
}

// Request writes a tool_approval SSE event and blocks until the user responds,
// the timeout expires, or the context is cancelled.
func (m *Manager) Request(ctx context.Context, writer EventWriter, userID, toolName string, args map[string]any) error {
	if writer == nil {
		return nil
	}

	requestID := randomID()
	pending := &pendingApproval{
		userID: userID,
		result: make(chan bool, 1),
	}

	m.mu.Lock()
	m.pending[requestID] = pending
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, requestID)
		m.mu.Unlock()
	}()

	event := Event{
		RequestID:  requestID,
		InstanceID: m.instanceID,
		ToolName:   toolName,
		Arguments:  args,
		ExpiresAt:  time.Now().Add(approvalTimeout).Unix(),
	}
	if err := writer.WriteEvent("tool_approval", event); err != nil {
		return fmt.Errorf("failed to request tool approval: %w", err)
	}

	timer := time.NewTimer(approvalTimeout)
	defer timer.Stop()

	select {
	case approved := <-pending.result:
		if !approved {
			return errors.New("user denied approval to run this tool. Do not retry.")
		}
		return nil
	case <-timer.C:
		return errors.New("timed out waiting for user approval to run this tool")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Respond delivers an approval/denial to the waiting Request. Returns false if
// no matching pending request exists or the user does not own it.
func (m *Manager) Respond(req *ResponseRequest) bool {
	m.mu.Lock()
	pending, ok := m.pending[req.RequestID]
	if ok && pending.userID == req.UserID {
		delete(m.pending, req.RequestID)
	}
	m.mu.Unlock()

	if !ok || pending.userID != req.UserID {
		return false
	}

	select {
	case pending.result <- req.Approved:
	default:
	}
	return true
}

// HandleResponse is the HTTP handler for POST /v1/chat/tool-approvals.
func (m *Manager) HandleResponse(w http.ResponseWriter, r *http.Request) {
	var req ResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid request body"})
		return
	}

	user, _ := r.Context().Value("user").(*model.User)
	if user == nil {
		rest.WriteResponse(http.StatusUnauthorized, w, r, map[string]string{"error": "not authenticated"})
		return
	}
	req.UserID = user.Id

	// Local instance owns this approval?
	if req.InstanceID == m.instanceID {
		if !m.Respond(&req) {
			rest.WriteResponse(http.StatusNotFound, w, r, map[string]string{"error": "approval request not found"})
			return
		}
		rest.WriteResponse(http.StatusOK, w, r, map[string]bool{"ok": true})
		return
	}

	// Forward to the originating instance via gossip.
	forwarder, ok := service.GetTransport().(Forwarder)
	if !ok || forwarder == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, map[string]string{"error": "approval request not found"})
		return
	}
	handled, err := forwarder.ForwardToolApproval(&req)
	if err != nil {
		rest.WriteResponse(http.StatusBadGateway, w, r, map[string]string{"error": err.Error()})
		return
	}
	if !handled {
		rest.WriteResponse(http.StatusNotFound, w, r, map[string]string{"error": "approval request not found"})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, map[string]bool{"ok": true})
}

// CheckToolApproval is called from the tool provider before executing a tool.
// If the context carries an SSE writer (web chat) and the tool requires
// approval, it blocks until the user responds. External MCP clients have no
// SSE writer and skip the check entirely.
func CheckToolApproval(ctx context.Context, userID, toolName string, args map[string]any) error {
	if defaultManager == nil {
		return nil
	}
	writer := mcpopenai.SSEEventWriterFromContext(ctx)
	if writer == nil {
		return nil
	}
	return defaultManager.Request(ctx, writer, userID, toolName, args)
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("approval_%d", time.Now().UnixNano())
	}
	return "approval_" + hex.EncodeToString(b[:])
}

func loadInstanceID() string {
	db := database.GetInstance()
	if db == nil {
		return randomID()
	}
	nodeID, err := db.GetCfgValue("node_id")
	if err != nil || nodeID == nil || nodeID.Value == "" {
		return randomID()
	}
	return nodeID.Value
}
