package common

import (
	"encoding/json"
	"log"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

func WriteError(w http.ResponseWriter, text string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := errorResponse{Error: text}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("ERROR: failed to write error JSON response: %v", err)
	}
}
