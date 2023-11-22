package apiv1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type CreateTokenRequest struct {
  Name string `json:"name"`
}

func HandleGetTokens(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)
  tokens, err := database.GetInstance().GetTokensForUser(user.Id)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of token data to return to the client
  tokenData := make([]struct {
    Id string `json:"token_id"`
    Name string `json:"name"`
    ExpiresAfter time.Time `json:"expires_at"`
  }, len(tokens))

  for i, token := range tokens {
    tokenData[i].Id = token.Id
    tokenData[i].Name = token.Name
    tokenData[i].ExpiresAfter = token.ExpiresAfter
  }

  rest.SendJSON(http.StatusOK, w, tokenData)
}

func HandleDeleteToken(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  // Load the token if not found or doesn't belong to the user then treat both as not found
  token, err := database.GetInstance().GetToken(chi.URLParam(r, "token_id"))
  if err != nil || token.UserId != user.Id {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: fmt.Sprintf("token %s not found", chi.URLParam(r, "token_id"))})
    return
  }

  // Delete the token
  err = database.GetInstance().DeleteToken(token)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateToken(w http.ResponseWriter, r *http.Request) {
  request := CreateTokenRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if(!validate.TokenName(request.Name)) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid token name"})
    return
  }

  // Create the token
  user := r.Context().Value("user").(*model.User)
  token := model.NewToken(request.Name, user.Id)
  err = database.GetInstance().SaveToken(token)

  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the Token ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    TokenID string `json:"token_id"`
  }{
    Status: true,
    TokenID: token.Id,
  })
}
