package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxBodyBytes = 256 << 10 // 256 KB, body order berisi builder state + items

type errorBody struct {
	Error errorBodyDetail `json:"error"`
}

type errorBodyDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorBodyDetail{Code: code, Message: message}})
}

// decodeJSON membatasi ukuran body; field JSON yang tidak dikenal diabaikan
// agar payload frontend boleh berkembang tanpa memecah API.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return fmt.Errorf("body terlalu besar (maks %d bytes)", maxErr.Limit)
		case errors.Is(err, io.EOF):
			return errors.New("body JSON wajib ada")
		default:
			return fmt.Errorf("body JSON tidak valid: %v", err)
		}
	}
	return nil
}
