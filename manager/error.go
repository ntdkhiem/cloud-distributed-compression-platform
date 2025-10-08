package main

import (
	"encoding/json"
	"net/http"
    "log"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, text string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := errorResponse{Error: text}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("ERROR: failed to write error JSON response: %v", err)
	}
}
