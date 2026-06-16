package api

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

func TestIsSpaceRenameBlocked(t *testing.T) {
	tests := []struct {
		name    string
		existing *model.Space
		request  *apiclient.SpaceRequest
		want     bool
	}{
		{
			name:     "stacked space renamed while staying in stack is blocked",
			existing: &model.Space{Name: "myapp-web", Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Name: "myapp-frontend", Stack: "myapp"},
			want:     true,
		},
		{
			name:     "stacked space, same name, staying in stack is allowed",
			existing: &model.Space{Name: "myapp-web", Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Name: "myapp-web", Stack: "myapp"},
			want:     false,
		},
		{
			name:     "stacked space cleared from stack and renamed is allowed",
			existing: &model.Space{Name: "myapp-web", Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Name: "renamed", Stack: ""},
			want:     false,
		},
		{
			name:     "stacked space moved to a different stack and renamed is blocked",
			existing: &model.Space{Name: "myapp-web", Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Name: "renamed", Stack: "other"},
			want:     true,
		},
		{
			name:     "standalone space (no prefix) renamed is allowed",
			existing: &model.Space{Name: "myspace", Stack: "", StackPrefix: ""},
			request:  &apiclient.SpaceRequest{Name: "renamed", Stack: ""},
			want:     false,
		},
		{
			name:     "space with only a stack label (no prefix) renamed is allowed",
			existing: &model.Space{Name: "myspace", Stack: "sometgroup", StackPrefix: ""},
			request:  &apiclient.SpaceRequest{Name: "renamed", Stack: "sometgroup"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSpaceRenameBlocked(tt.existing, tt.request)
			if got != tt.want {
				t.Fatalf("isSpaceRenameBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolvedStackPrefix(t *testing.T) {
	tests := []struct {
		name     string
		existing *model.Space
		request  *apiclient.SpaceRequest
		want     string
	}{
		{
			name:     "staying in stack preserves the existing prefix",
			existing: &model.Space{Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Stack: "myapp"},
			want:     "myapp",
		},
		{
			name:     "clearing the stack name clears the prefix (detach)",
			existing: &model.Space{Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Stack: ""},
			want:     "",
		},
		{
			name:     "moving to another non-empty stack keeps the existing prefix",
			existing: &model.Space{Stack: "myapp", StackPrefix: "myapp"},
			request:  &apiclient.SpaceRequest{Stack: "other"},
			want:     "myapp",
		},
		{
			name:     "standalone space joining a stack has no prefix to carry over",
			existing: &model.Space{Stack: "", StackPrefix: ""},
			request:  &apiclient.SpaceRequest{Stack: "group"},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvedStackPrefix(tt.existing, tt.request)
			if got != tt.want {
				t.Fatalf("resolvedStackPrefix() = %q, want %q", got, tt.want)
			}
		})
	}
}
