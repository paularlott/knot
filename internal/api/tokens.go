package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetTokens(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	tokens, err := database.GetInstance().GetTokensForUser(user.Id)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	tokenData := []apiclient.TokenInfo{}
	for _, token := range tokens {
		if token.IsDeleted {
			continue
		}
		tokenData = append(tokenData, apiclient.TokenInfo{
			Id:           token.Id,
			Name:         token.Name,
			ExpiresAfter: token.ExpiresAfter,
			Scopes:       token.Scopes,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, tokenData)
}

func HandleDeleteToken(w http.ResponseWriter, r *http.Request) {
	tokenId := r.PathValue("token_id")

	if !validate.Required(tokenId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token ID"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	token, err := db.GetToken(tokenId)
	if err != nil || token.UserId != user.Id {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("token %s not found", tokenId)})
		return
	}

	token.IsDeleted = true
	token.UpdatedAt = hlc.Now()
	err = db.SaveToken(token)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipToken(token)
	sse.PublishTokensChanged("")

	w.WriteHeader(http.StatusOK)
}

func HandleCreateToken(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	request := apiclient.CreateTokenRequest{}

	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.TokenName(request.Name) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token name"})
		return
	}

	// Validate scopes (if provided). nil/empty = unrestricted (backward
	// compatible). Non-empty = each entry must be a known scope.
	scopes := request.Scopes
	if len(scopes) > 0 {
		for _, s := range scopes {
			if !model.IsKnownTokenScope(s) {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
					Error: fmt.Sprintf("Unknown token scope: %s", s),
				})
				return
			}
		}
	}

	token := model.NewToken(request.Name, user.Id)
	token.Scopes = scopes
	err = database.GetInstance().SaveToken(token)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipToken(token)
	sse.PublishTokensChanged("")

	rest.WriteResponse(http.StatusCreated, w, r, apiclient.CreateTokenResponse{
		Status:  true,
		TokenID: token.Id,
	})
}

// HandleUpdateToken allows changing a token's name and/or scopes without
// recreating it (the token ID stays the same, so callers don't need to
// re-credential).
func HandleUpdateToken(w http.ResponseWriter, r *http.Request) {
	tokenId := r.PathValue("token_id")
	if !validate.Required(tokenId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	token, err := db.GetToken(tokenId)
	if err != nil || token.UserId != user.Id {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("token %s not found", tokenId)})
		return
	}

	var request apiclient.UpdateTokenRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Name != nil {
		if !validate.TokenName(*request.Name) {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token name"})
			return
		}
		token.Name = *request.Name
	}

	if request.Scopes != nil {
		// Validate each scope. An empty slice (explicitly provided) clears
		// scopes → unrestricted. nil (omitted) preserves existing scopes.
		for _, s := range *request.Scopes {
			if !model.IsKnownTokenScope(s) {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{
					Error: fmt.Sprintf("Unknown token scope: %s", s),
				})
				return
			}
		}
		token.Scopes = *request.Scopes
	}

	token.UpdatedAt = hlc.Now()
	if err := db.SaveToken(token); err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipToken(token)
	sse.PublishTokensChanged("")

	w.WriteHeader(http.StatusOK)
}
