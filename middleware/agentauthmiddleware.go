package middleware

import (
	"fmt"
	"net/http"
)

var (
  AgentSpaceKey string
  ServerURL string
)

func AgentApiAuth(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

    // If have an Authorization header then we use that for authentication
    authorization := r.Header.Get("Authorization")
    if authorization != "" {

      // Get the auth token
      var token string
      fmt.Sscanf(authorization, "Bearer %s", &token)
      if token != AgentSpaceKey {
        returnUnauthorized(w)
        return
      }
    } else {
      returnUnauthorized(w)
    }

    // If authenticated, continue
    next.ServeHTTP(w, r)
  })
}
