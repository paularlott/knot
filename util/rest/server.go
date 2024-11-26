package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

// Bind JSON from request body to a struct
func BindJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	contentType := r.Header.Get("Content-Type")

	if contentType == "application/json" {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		err := decoder.Decode(v)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return err
		}
	} else if contentType == "application/msgpack" {
		decoder := msgpack.NewDecoder(r.Body)
		decoder.DisallowUnknownFields(true)

		err := decoder.Decode(v)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return err
		}
	} else {
		return errors.New("Content-Type header is not application/json or application/msgpack")
	}

	return nil
}

// Send JSON response
func SendJSON(status int, w http.ResponseWriter, r *http.Request, v interface{}) error {
	if strings.Contains(r.Header.Get("Accept"), "application/msgpack") {
		w.Header().Set("Content-Type", "application/msgpack")
		w.WriteHeader(status)
		return msgpack.NewEncoder(w).Encode(v)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		return json.NewEncoder(w).Encode(v)
	}
}
