package api_utils

import (
	"testing"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database/model"
)

func TestNewApiUtilsUsers(t *testing.T) {
	utils := NewApiUtilsUsers()
	if utils == nil {
		t.Fatal("NewApiUtilsUsers returned nil")
	}
}

func TestShouldSendAgentSSHUpdates(t *testing.T) {
	tests := []struct {
		name       string
		template   *model.Template
		agentState *agent_server.Session
		want       bool
	}{
		{
			name:       "nil template",
			template:   nil,
			agentState: agent_server.NewSession("space-ssh-update", "0.0.0"),
			want:       false,
		},
		{
			name:       "nil agent",
			template:   &model.Template{WithSSH: true},
			agentState: nil,
			want:       false,
		},
		{
			name:       "template ssh before port report",
			template:   &model.Template{WithSSH: true},
			agentState: agent_server.NewSession("space-ssh-update", "0.0.0"),
			want:       true,
		},
		{
			name:     "reported ssh port without template flag",
			template: &model.Template{},
			agentState: func() *agent_server.Session {
				session := agent_server.NewSession("space-ssh-update", "0.0.0")
				session.SSHPort = 22
				return session
			}(),
			want: true,
		},
		{
			name:       "no ssh capability",
			template:   &model.Template{},
			agentState: agent_server.NewSession("space-ssh-update", "0.0.0"),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSendAgentSSHUpdates(tt.template, tt.agentState); got != tt.want {
				t.Fatalf("shouldSendAgentSSHUpdates() = %v, want %v", got, tt.want)
			}
		})
	}
}
