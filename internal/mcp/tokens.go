package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

type Token struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ExpiresAfter string `json:"expires_after"`
}

func listTokens(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	tokens, err := database.GetInstance().GetTokensForUser(user.Id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get tokens: %v", err)
	}

	var result []Token
	for _, token := range tokens {
		if token.IsDeleted {
			continue
		}

		result = append(result, Token{
			ID:           token.Id,
			Name:         token.Name,
			ExpiresAfter: token.ExpiresAfter.Format("2006-01-02T15:04:05Z"),
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func createToken(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	name := req.StringOr("name", "")
	if !validate.TokenName(name) {
		return nil, fmt.Errorf("Invalid token name")
	}

	token := model.NewToken(name, user.Id)
	err := database.GetInstance().SaveToken(token)
	if err != nil {
		return nil, fmt.Errorf("Failed to save token: %v", err)
	}

	service.GetTransport().GossipToken(token)

	result := map[string]interface{}{
		"status":   true,
		"token_id": token.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteToken(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	tokenId := req.StringOr("token_id", "")
	if !validate.Required(tokenId) {
		return nil, fmt.Errorf("Invalid token ID")
	}

	db := database.GetInstance()
	token, err := db.GetToken(tokenId)
	if err != nil || token.UserId != user.Id {
		return nil, fmt.Errorf("Token not found")
	}

	token.IsDeleted = true
	token.UpdatedAt = hlc.Now()
	err = db.SaveToken(token)
	if err != nil {
		return nil, fmt.Errorf("Failed to delete token: %v", err)
	}

	service.GetTransport().GossipToken(token)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}