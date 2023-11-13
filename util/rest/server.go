package rest

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Bind JSON from request body to a struct
func BindJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
  if r.Header.Get("Content-Type") != "" && r.Header.Get("Content-Type") != "application/json" {
    w.WriteHeader(http.StatusUnsupportedMediaType)
    return errors.New("Content-Type header is not application/json")
  }

  decoder := json.NewDecoder(r.Body)
  decoder.DisallowUnknownFields()

  err := decoder.Decode(v)
  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    return err
  }

  return nil
}

// Send JSON response
func SendJSON(status int, w http.ResponseWriter, v interface{}) error {
  w.Header().Set("Content-Type", "application/json; charset=utf-8")
  w.WriteHeader(status)
  return json.NewEncoder(w).Encode(v)
}
