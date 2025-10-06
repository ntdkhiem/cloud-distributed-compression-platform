package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
)

const serverURL = "http://127.0.0.1:8080"

type errorResponse struct {
	Error string `json:"error"`
}

type huffmanRequest struct {
	UID            string         `json:"uid"`
	FrequencyTable map[string]int `json:"frequency_table"`
}

func main() {
	http.HandleFunc("/upload", uploadHandler)
	fmt.Println("Listening on localhost:8081...")
	http.ListenAndServe(":8081", nil)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: increase the limit through any means (the whole point of the program)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, "The uploaded file is too big. Please choose a file less than 10 MB", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("ERROR: failed to get file from form: %v", err)
		writeError(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	frequencyTable, err := calculateFrequencyFromStream(file)
	if err != nil {
		log.Printf("ERROR: failed to process file stream: %v", err)
		writeError(w, "Could not process file content", http.StatusInternalServerError)
		return
	}

	// w.Header().Set("Content-Type", "application/json")
	// w.WriteHeader(http.StatusOK)
	// if err := json.NewEncoder(w).Encode(frequencyTable); err != nil {
	// 	log.Printf("ERROR: failed to write frequency table JSON response: %v", err)
	// }

	newUID, err := uuid.NewRandom()
	if err != nil {
		log.Printf("ERROR: failed to generate UUID: %v", err)
		writeError(w, "Failed to generate unique identifier", http.StatusInternalServerError)
		return
	}

	ok = buildHuffmanTree(newUID.String(), frequencyTable)
	if !ok {
		writeError(w, "Failed to build Huffman Binary Tree", http.StatusInternalServerError)
		return
	}
}

func buildHuffmanTree(UID string, ft map[string]int) bool {
	request := huffmanRequest{
		UID:            UID,
		FrequencyTable: ft,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		log.Printf("ERROR: failed to marshal request: %v", err)
		return false
	}

	req, err := http.NewRequest(http.MethodPost, serverURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("ERROR: could not create HTTP request: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	return true
}

func calculateFrequencyFromStream(reader io.Reader) (map[string]int, error) {
	frequency := make(map[string]int)
	// Create a buffered reader to efficiently read rune by rune.
	bufReader := bufio.NewReader(reader)

	for {
		// ReadRune() reads a single UTF-8 encoded Unicode character (a rune).
		char, _, err := bufReader.ReadRune()
		if err != nil {
			// If we've reached the end of the file, we're done.
			if err == io.EOF {
				break
			}
			// Otherwise, it's a real error.
			return nil, err
		}
		// Increment the count for the character.
		frequency[string(char)]++
	}
	return frequency, nil
}

func processFile(path string) (map[rune]int, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	freqTable := make(map[rune]int)
	fileContent := []string{}

	fmt.Println("Building frequency table")
	scanner := bufio.NewReader(file)
	for {
		line, err := scanner.ReadString(byte('\n'))
		if err != nil && err != io.EOF {
			return nil, nil, err
		}
		fileContent = append(fileContent, line)
		for _, c := range line {
			freqTable[c]++
		}
		if err == io.EOF {
			break
		}
	}

	return freqTable, fileContent, nil
}

func requestUIDFromServer() (string, error) {
	return "1", nil
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
