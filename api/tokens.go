package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

func HandleGetTokens(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	tokens, err := database.GetInstance().GetTokensForUser(user.Id)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of token data to return to the client
	tokenData := make([]apiclient.TokenInfo, len(tokens))

	for i, token := range tokens {
		tokenData[i].Id = token.Id
		tokenData[i].Name = token.Name
		tokenData[i].ExpiresAfter = token.ExpiresAfter
	}

	rest.SendJSON(http.StatusOK, w, r, tokenData)
}

func HandleDeleteToken(w http.ResponseWriter, r *http.Request) {
	tokenId := r.PathValue("token_id")

	if !validate.Required(tokenId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token ID"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	token, err := db.GetToken(tokenId)
	if err != nil || token.UserId != user.Id {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("token %s not found", tokenId)})
		return
	}

	// Delete the token
	err = db.DeleteToken(token)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleCreateToken(w http.ResponseWriter, r *http.Request) {
	var token *model.Token

	user := r.Context().Value("user").(*model.User)
	request := apiclient.CreateTokenRequest{}

	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Validate
	if !validate.TokenName(request.Name) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid token name"})
		return
	}

	// Create the token
	token = model.NewToken(request.Name, user.Id)

	err = database.GetInstance().SaveToken(token)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Return the Token ID
	rest.SendJSON(http.StatusCreated, w, r, apiclient.CreateTokenResponse{
		Status:  true,
		TokenID: token.Id,
	})
}
