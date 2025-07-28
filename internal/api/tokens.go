package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
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

	// Build a json array of token data to return to the client
	tokenData := []apiclient.TokenInfo{}

	for _, token := range tokens {
		if token.IsDeleted {
			continue
		}

		tokenData = append(tokenData, apiclient.TokenInfo{
			Id:           token.Id,
			Name:         token.Name,
			ExpiresAfter: token.ExpiresAfter,
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

	// Delete the token
	token.IsDeleted = true
	token.UpdatedAt = hlc.Now()
	err = db.SaveToken(token)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipToken(token)

	w.WriteHeader(http.StatusOK)
}

func HandleCreateToken(w http.ResponseWriter, r *http.Request) {
	var token *model.Token

	user := r.Context().Value("user").(*model.User)
	request := apiclient.CreateTokenRequest{}

	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if !validate.TokenName(request.Name) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token name"})
		return
	}

	// Create the token
	token = model.NewToken(request.Name, user.Id)
	err = database.GetInstance().SaveToken(token)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipToken(token)

	// Return the Token ID
	rest.WriteResponse(http.StatusCreated, w, r, apiclient.CreateTokenResponse{
		Status:  true,
		TokenID: token.Id,
	})
}
