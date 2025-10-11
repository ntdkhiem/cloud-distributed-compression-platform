package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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
		slog.Error("Failed to get file from form", "error", err)
		writeError(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

    slog.Info("Processing a request for compressing")

	jobID := uuid.New().String()
	slog.Debug("Creating new job", "job", jobID, "file", header.Filename)

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
				slog.Error("Failed to read file to build freq. table", "job", jobID, "error", err)
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
		slog.Error("Failed to stream data to GCS", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close data stream to GCS", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug(fmt.Sprintf("Uploaded %s to GCS", header.Filename), "job", jobID)

	freqTableBytes, err := json.Marshal(freqTable)
	if err != nil {
		slog.Error("Failed to marshal frequency table", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	freqTablePath := fmt.Sprintf("%s/frequency_table.json", jobID)
	wc = app.GCSClient.Bucket(app.Bucket).Object(freqTablePath).NewWriter(ctx)
	if _, err := io.Copy(wc, bytes.NewReader(freqTableBytes)); err != nil {
		slog.Error("Failed to stream frequency table to GCS", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close frequency table data stream to GCS", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug("Uploaded frequency table to GCS", "job", jobID)

	// initialize new pushlisher everytime to avoid sending messages in batch.
	publisher := app.PUBSUBClient.Publisher(app.TopicID)
	message := pubsubMessageSchema{
		UID:              jobID,
		OriginalFilePath: originalFilePath,
		FreqTablePath:    freqTablePath,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.Error("Failed to marshal MQ message", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	result := publisher.Publish(*app.CTX, &pubsub.Message{
		Data: messageBytes,
	})
	returnedMessageID, err := result.Get(*app.CTX) // blocks until Pub/Sub returns server-generated ID or error
	if err != nil {
		slog.Error("Failed to send MQ message", "job", jobID, "error", err)
		writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug("Sent message to Pub/Sub ", "job", jobID, "server_generated_message_id", returnedMessageID)

	// Send 202 Accepted Code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

func main() {
	// initialize logging system
	var programLevel = new(slog.LevelVar) // Info by default
	developmentMode := os.Getenv("DEVELOPMENT_MODE")
	isDev, err := strconv.ParseBool(developmentMode)
	if err == nil && isDev {
		programLevel.Set(slog.LevelDebug)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel}))
	slog.SetDefault(logger)

	// initialize GCP services
	projectID := os.Getenv("GCP_PROJECT_ID")
	topicID := os.Getenv("PUBSUB_TOPIC_ID")
	bucket := os.Getenv("GCS_BUCKET")
	ctx := context.Background()

	GCSClient, err := storage.NewClient(ctx)
	if err != nil {
		slog.Error("Cannot create new client for GCS", "error", err)
		return
	}
	defer GCSClient.Close()
	slog.Debug("Initialized a GCS client.")

	PUBSUBClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		slog.Error("Cannot create new client for Pub/Sub", "error", err)
		return
	}
	defer PUBSUBClient.Close()
	slog.Debug("Initialized a Pub/Sub client.")

	app := Application{
		GCSClient:    GCSClient,
		PUBSUBClient: PUBSUBClient,
		CTX:          &ctx,
		Bucket:       bucket,
		TopicID:      topicID,
	}

	http.HandleFunc("/upload", app.uploadHandler)
	slog.Info("Listening on localhost:8081...")
	http.ListenAndServe(":8081", nil)
}
