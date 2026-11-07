package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func TestTokenInfoFromRequest(t *testing.T) {
	if info := tokenInfoFromRequest(context.Background()); info != nil {
		t.Fatalf("no user: got %+v, want nil", info)
	}

	user := &model.User{Id: "u1", Username: "alice"}
	ctx := context.WithValue(context.Background(), "user", user)
	info := tokenInfoFromRequest(ctx)
	if info == nil || info.Subject != "u1" || info.Claims["username"] != "alice" || len(info.Scopes) != 0 {
		t.Fatalf("session user: got %+v", info)
	}

	expiry := time.Now().Add(time.Hour).UTC()
	token := &model.Token{Scopes: []string{model.ScopeMCP}, ExpiresAfter: expiry}
	info = tokenInfoFromRequest(context.WithValue(ctx, "access_token", token))
	if info == nil || !info.HasScope(model.ScopeMCP) || !info.Expiry.Equal(expiry) {
		t.Fatalf("token user: got %+v", info)
	}
}
