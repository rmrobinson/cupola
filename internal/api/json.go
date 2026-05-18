package api

import (
	"encoding/json"
	"net/http"
)

const maxJSONBodyBytes = 2 << 20

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}
