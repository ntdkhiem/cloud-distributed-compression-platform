package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

type Application struct {
	GCSClient    *storage.Client
	PUBSUBClient *pubsub.Client
	CTX          *context.Context
	Bucket       string
	TopicID      string
}

// Must follow this schema to be accepted by Pub/Sub
type pubsubMessageSchema struct {
	UID              string `json:"UID"`
	OriginalFilePath string `json:"OriginalFilePath"`
	FreqTablePath    string `json:"FreqTablePath"`
}

func (app *Application) uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	// set request body size limit (1GB for now)
	// TODO: increase the limit through any means (the whole point of the program)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("ERROR: failed to get file from form: %v", err)
		writeError(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	jobID := uuid.New().String()
	log.Printf("INFO: Processing new job %s for file %s", jobID, header.Filename)

	// create a pipe to simultaneously building char. req. table while streaming content to GCS
	pr, pw := io.Pipe()

	freqTable := make(map[rune]uint64)
	teeReader := io.TeeReader(file, pw)

	go func() {
		defer pw.Close()
		bufReader := bufio.NewReader(teeReader)
		for {
			// ReadRune() reads a single UTF-8 encoded Unicode character (a rune).
			char, _, err := bufReader.ReadRune()
			if err != nil {
				// If we've reached the end of the file, we're done.
				if err == io.EOF {
					break
				}
				// Otherwise, it's a real error.
				// TODO: how do I return this error as internal error from go routine?
				log.Printf("ERROR: failed to read file to build freq. table: %v", err)
				return
			}
			// Increment the count for the character.
			freqTable[char]++
		}
	}()

	ctx, cancel := context.WithTimeout(*app.CTX, time.Second*50)
	defer cancel()

	originalFilePath := fmt.Sprintf("%s/original_%s", jobID, header.Filename)
	wc := app.GCSClient.Bucket(app.Bucket).Object(originalFilePath).NewWriter(ctx)
	if _, err := io.Copy(wc, pr); err != nil {
		log.Printf("ERROR: failed to stream data to GCS: %v", err)
		writeError(w, "Failed to upload data to server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		log.Printf("ERROR: failed to close data stream to GCS: %v", err)
		writeError(w, "Failed to upload data to server: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Uploaded %s to GCS", header.Filename)

	freqTableBytes, err := json.Marshal(freqTable)
	if err != nil {
		log.Printf("ERROR: failed to marshal frequency table for job %s: %v", jobID, err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	freqTablePath := fmt.Sprintf("%s/frequency_table.json", jobID)
	wc = app.GCSClient.Bucket(app.Bucket).Object(freqTablePath).NewWriter(ctx)
	if _, err := io.Copy(wc, bytes.NewReader(freqTableBytes)); err != nil {
		log.Printf("ERROR: failed to stream frequency table to GCS: %v", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		log.Printf("ERROR: failed to close frequency table data stream to GCS: %v", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Uploaded frequency table for job %s to GCS", jobID)

	// initialize new pushlisher everytime to avoid sending messages in batch.
	publisher := app.PUBSUBClient.Publisher(app.TopicID)
	message := pubsubMessageSchema{
		UID:              jobID,
		OriginalFilePath: originalFilePath,
		FreqTablePath:    freqTablePath,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("ERROR: failed to marshal MQ message for job %s: %v", jobID, err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	result := publisher.Publish(*app.CTX, &pubsub.Message{
		Data: messageBytes,
	})
	returnedMessageID, err := result.Get(*app.CTX) // blocks until Pub/Sub returns server-generated ID or error
	if err != nil {
		log.Printf("ERROR: failed to send MQ message for job %s: %v", jobID, err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("INFO: Sent message to Pub/Sub for job %s: %s", jobID, returnedMessageID)

	// Send 202 Accepted Code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

func main() {
	projectID := os.Getenv("GCP_PROJECT_ID")
	topicID := os.Getenv("PUBSUB_TOPIC_ID")
	bucket := os.Getenv("GCS_BUCKET")
	ctx := context.Background()

	GCSClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("ERROR: Cannot create new client for GCS: %v", err)
		return
	}
	defer GCSClient.Close()
	log.Printf("Initialized a GCS client.")

	PUBSUBClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Printf("ERROR: Cannot create new client for Pub/Sub: %v", err)
		return
	}
	defer PUBSUBClient.Close()
	log.Printf("Initialized a Pub/Sub client.")

	app := Application{
		GCSClient:    GCSClient,
		PUBSUBClient: PUBSUBClient,
		CTX:          &ctx,
		Bucket:       bucket,
		TopicID:      topicID,
	}

	http.HandleFunc("/upload", app.uploadHandler)
	fmt.Println("Listening on localhost:8081...")
	http.ListenAndServe(":8081", nil)
}
