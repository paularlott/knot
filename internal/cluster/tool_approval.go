package cluster

import (
	"fmt"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/toolapproval"
)

type toolApprovalForwardResponse struct {
	Handled bool   `json:"handled"`
	Error   string `json:"error,omitempty"`
}

// ForwardToolApproval sends an approval response to the instance that owns the
// pending request via gossip.
func (c *Cluster) ForwardToolApproval(req *toolapproval.ResponseRequest) (bool, error) {
	if c.gossipCluster == nil {
		return false, nil
	}

	node := c.GetNodeByIDString(req.InstanceID)
	if node == nil {
		return false, nil
	}

	response := &toolApprovalForwardResponse{}
	if err := c.gossipCluster.SendToWithResponse(node, ToolApprovalResponseMsg, req, response); err != nil {
		return false, err
	}
	if response.Error != "" {
		return false, fmt.Errorf("%s", response.Error)
	}

	return response.Handled, nil
}

// handleToolApprovalResponse is called on the originating instance when a
// forwarded approval response arrives via gossip.
func (c *Cluster) handleToolApprovalResponse(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	req := &toolapproval.ResponseRequest{}
	if err := packet.Unmarshal(req); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal tool approval response")
		return nil, err
	}

	manager := toolapproval.GetManager()
	if manager == nil {
		return &toolApprovalForwardResponse{
			Handled: false,
			Error:   "tool approval manager is not available on target server",
		}, nil
	}

	return &toolApprovalForwardResponse{
		Handled: manager.Respond(req),
	}, nil
}
