package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const serverURL = "http://127.0.0.1:8080"

func main() {
	http.HandleFunc("/upload", uploadHandler)
	fmt.Println("Listening on localhost:8081...")
	http.ListenAndServe(":8081", nil)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// validate .txt extension
	if !strings.HasSuffix(header.Filename, ".txt") {
		http.Error(w, "Only .txt files are allowed right now.", http.StatusBadRequest)
		return
	}

	// create temp file to store uploaded file
	tmpFile, err := os.CreateTemp("", "*.txt")
	if err != nil {
		http.Error(w, "Temp file creation failed.", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// TODO: stream data to tmpFile while building frequency table of characters
	size, err := io.Copy(tmpFile, file)
	if err != nil {
		http.Error(w, "Failed to copy file:"+err.Error(), http.StatusInternalServerError)
		return
	}

	// build frequency table by processing tmpFile
	freqTable, fileContent, err := processFile(tmpFile.Name())
	if err != nil {
		http.Error(w, "Failed to process file:"+err.Error(), http.StatusInternalServerError)
		return
	}

	// request UID from server
	uid, err := requestUIDFromServer()
	if err != nil {
		http.Error(w, "Failed to request UID from server:"+err.Error(), http.StatusInternalServerError)
		return
	}

	// send table to server

	// send file chunks to given UID
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
