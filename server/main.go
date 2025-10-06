package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type huffmanRequest struct {
	UID            string         `json:"uid"`
	FrequencyTable map[string]int `json:"frequency_table"`
}

type treeResponse struct {
	Message   string `json:"message"`
	UID       string `json:"uid"`
	Timestamp string `json:"timestamp"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	http.HandleFunc("/tree", buildTreeHandler)
	fmt.Println("Listening on localhost:8080...")
	http.ListenAndServe(":8080", nil)
}

func buildTreeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var requestPayload huffmanRequest
	if err := json.NewDecoder(r.Body).Decode(&requestPayload); err != nil {
		writeError(w, "Invalid or malformed JSON body", http.StatusBadRequest)
		return
	}

	if requestPayload.UID == "" {
		writeError(w, "Missing required field: uid", http.StatusBadRequest)
		return
	}

	if len(requestPayload.FrequencyTable) == 0 {
		writeError(w, "Missing or empty required field: frequency_table", http.StatusBadRequest)
		return
	}

	log.Printf("INFO: Received valid request to build tree for UID: %s", requestPayload.UID)
	log.Printf("INFO: Frequency table has %d unique characters.", len(requestPayload.FrequencyTable))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	response := treeResponse{
		Message:   "Request accepted. Huffman tree building process initiated.",
		UID:       requestPayload.UID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("ERROR: failed to write success response for /tree: %v", err)
	}
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
