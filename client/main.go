package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

const serverURL = "http://127.0.0.1:8080"

type Application struct {
	GCSClient *storage.Client
	CTX       *context.Context
	Bucket    string
}

type huffmanRequest struct {
	UID            string         `json:"uid"`
	FrequencyTable map[string]int `json:"frequency_table"`
}

func (app *Application) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	// set request body size limit
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	// // TODO: increase the limit through any means (the whole point of the program)
	// if err := r.ParseMultipartForm(10 << 20); err != nil {
	// 	writeError(w, "The uploaded file is too big. Please choose a file less than 10 MB", http.StatusBadRequest)
	// 	return
	// }

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("ERROR: failed to get file from form: %v", err)
		writeError(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	jobID := uuid.New().String()
	log.Printf("Processing new job %s for file %s", jobID, header.Filename)

	// create a pipe to simultaneously building char. req. table while streaming content to GCS
	// pr, pw := io.Pipe()

	ctx, cancel := context.WithTimeout(*app.CTX, time.Second*50)
	defer cancel()

	originalFilePath := fmt.Sprintf("%s/original_%s", jobID, header.Filename)
	wc := app.GCSClient.Bucket(app.Bucket).Object(originalFilePath).NewWriter(ctx)
	if _, err := io.Copy(wc, file); err != nil {
		log.Printf("ERROR: failed to stream data to GCS: %v", err)
		writeError(w, "Failed to upload data to server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		log.Printf("ERROR: failed to close data stream to GCS: %v", err)
		writeError(w, "Failed to upload data to server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Uploaded %s to GCS", header.Filename)

	// frequencyTable, err := calculateFrequencyFromStream(file)
	// if err != nil {
	// 	log.Printf("ERROR: failed to process file stream: %v", err)
	// 	writeError(w, "Could not process file content", http.StatusInternalServerError)
	// 	return
	// }

	// w.Header().Set("Content-Type", "application/json")
	// w.WriteHeader(http.StatusOK)
	// if err := json.NewEncoder(w).Encode(frequencyTable); err != nil {
	// 	log.Printf("ERROR: failed to write frequency table JSON response: %v", err)
	// }

	// ok = buildHuffmanTree(newUID.String(), frequencyTable)
	// if !ok {
	// 	writeError(w, "Failed to build Huffman Binary Tree", http.StatusInternalServerError)
	// 	return
	// }
}

// func buildHuffmanTree(UID string, ft map[string]int) bool {
// 	request := huffmanRequest{
// 		UID:            UID,
// 		FrequencyTable: ft,
// 	}
//
// 	jsonData, err := json.Marshal(request)
// 	if err != nil {
// 		log.Printf("ERROR: failed to marshal request: %v", err)
// 		return false
// 	}
//
// 	req, err := http.NewRequest(http.MethodPost, serverURL, bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		log.Printf("ERROR: could not create HTTP request: %v", err)
// 		return false
// 	}
// 	req.Header.Set("Content-Type", "application/json")
// 	return true
// }

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

// func processFile(path string) (map[rune]int, []string, error) {
// 	file, err := os.Open(path)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	defer file.Close()
//
// 	freqTable := make(map[rune]int)
// 	fileContent := []string{}
//
// 	fmt.Println("Building frequency table")
// 	scanner := bufio.NewReader(file)
// 	for {
// 		line, err := scanner.ReadString(byte('\n'))
// 		if err != nil && err != io.EOF {
// 			return nil, nil, err
// 		}
// 		fileContent = append(fileContent, line)
// 		for _, c := range line {
// 			freqTable[c]++
// 		}
// 		if err == io.EOF {
// 			break
// 		}
// 	}
//
// 	return freqTable, fileContent, nil
// }

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("ERROR: Cannot create new client for GCS: %v", err)
		return
	}
	defer client.Close()

	app := Application{
		GCSClient: client,
		CTX:       &ctx,
		Bucket:    os.Getenv("GCS_BUCKET"),
	}

	http.HandleFunc("/upload", app.uploadHandler)
	fmt.Println("Listening on localhost:8081...")
	http.ListenAndServe(":8081", nil)
}
