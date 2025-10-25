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
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"

	"github.com/ntdkhiem/cloud-distributed-compression-platform/internal/common"
)

type Application struct {
	GCSClient         common.GCSClientInterface
	PUBSUBClient      common.PubSubClientInterface
	CTX               *context.Context
	Bucket            string
	CompressTopicID   string
	DecompressTopicID string
	MaxUploadSize     int64
	GCSTimeout        time.Duration
}

func (app *Application) compressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	// set request body size limit (1GB for now)
	// TODO: increase the limit through any means (the whole point of the program)
	r.Body = http.MaxBytesReader(w, r.Body, app.MaxUploadSize)

	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Error("Failed to get file from form", "error", err)
		// This error is triggered when MaxBytesReader limit is exceeded
		if strings.Contains(err.Error(), "request body too large") {
			common.WriteError(w, "File exceeds size limit", http.StatusRequestEntityTooLarge)
			return
		}
		common.WriteError(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
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
				slog.Error("Failed to read file to build freq. table", "job", jobID, "error", err)
				pw.CloseWithError(err)
				return
			}
			// Increment the count for the character.
			freqTable[char]++
		}
	}()

	ctx, cancel := context.WithTimeout(*app.CTX, app.GCSTimeout)
	defer cancel()

	originalFilePath := fmt.Sprintf("%s/original_%s", jobID, header.Filename)
	wc := app.GCSClient.NewObjectWriter(ctx, app.Bucket, originalFilePath)
	if _, err := io.Copy(wc, pr); err != nil {
		slog.Error("Failed to stream data to GCS", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close data stream to GCS", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug(fmt.Sprintf("Uploaded %s to GCS", header.Filename), "job", jobID)

	freqTableBytes, err := json.Marshal(freqTable)
	if err != nil {
		slog.Error("Failed to marshal frequency table", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	freqTablePath := fmt.Sprintf("%s/frequency_table.json", jobID)
	wc = app.GCSClient.NewObjectWriter(ctx, app.Bucket, freqTablePath)
	if _, err := io.Copy(wc, bytes.NewReader(freqTableBytes)); err != nil {
		slog.Error("Failed to stream frequency table to GCS", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close frequency table data stream to GCS", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug("Uploaded frequency table to GCS", "job", jobID)

	// initialize new publisher everytime to avoid sending messages in batch.
	// TODO: make this more tolerable to message delivery failures.
	message := common.CompressedMsgSchema{
		UID:              jobID,
		OriginalFilePath: originalFilePath,
		FreqTablePath:    freqTablePath,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.Error("Failed to marshal MQ message", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	returnedMessageID, err := app.PUBSUBClient.PublishMessage(*app.CTX, app.CompressTopicID, &pubsub.Message{
		Data: messageBytes,
	})
	if err != nil {
		slog.Error("Failed to send MQ message", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug("Sent message to Pub/Sub ", "job", jobID, "server_generated_message_id", returnedMessageID)

	// Send 202 Accepted Code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

func (app *Application) decompressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		common.WriteError(w, "Only POST method allowed", http.StatusMethodNotAllowed)
		return
	}

	// set request body size limit (1GB for now)
	// TODO: increase the limit through any means (the whole point of the program)
	r.Body = http.MaxBytesReader(w, r.Body, app.MaxUploadSize)

	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Error("Failed to get file from form", "error", err)
		// This error is triggered when MaxBytesReader limit is exceeded
		if strings.Contains(err.Error(), "request body too large") {
			common.WriteError(w, "File exceeds size limit", http.StatusRequestEntityTooLarge)
			return
		}
		common.WriteError(w, "Failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(header.Filename, ".ranran") {
		common.WriteError(w, "Wrong file format", http.StatusBadRequest)
		return
	}

	slog.Info("Processing a request for decompressing")

	jobID := uuid.New().String()
	slog.Debug("Creating new job", "job", jobID, "file", header.Filename)

	ctx, cancel := context.WithTimeout(*app.CTX, time.Second*50)
	defer cancel()

	compressedFilePath := fmt.Sprintf("%s/%s", jobID, header.Filename)
	wc := app.GCSClient.NewObjectWriter(ctx, app.Bucket, compressedFilePath)
	if _, err := io.Copy(wc, file); err != nil {
		slog.Error("Failed to stream compressed data to GCS", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close data stream to GCS", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Debug(fmt.Sprintf("Uploaded %s to GCS", header.Filename), "job", jobID)

	// initialize new publisher everytime to avoid sending messages in batch.
	// TODO: make this more tolerable to message delivery failures.
	message := common.DecompressedMsgSchema{
		UID:                jobID,
		CompressedFilePath: compressedFilePath,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		slog.Error("Failed to marshal MQ message", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	returnedMessageID, err := app.PUBSUBClient.PublishMessage(*app.CTX, app.DecompressTopicID, &pubsub.Message{
		Data: messageBytes,
	})
	if err != nil {
		slog.Error("Failed to send MQ message", "job", jobID, "error", err)
		common.WriteError(w, "Internal server error", http.StatusInternalServerError)
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
	compressTopicID := os.Getenv("PUBSUB_COMPRESS_TOPIC_ID")
	decompressTopicID := os.Getenv("PUBSUB_DECOMPRESS_TOPIC_ID")
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

	realGCS := &common.RealGCSClient{Client: GCSClient}
	realPubSub := &common.RealPubSubClient{Client: PUBSUBClient}

	app := Application{
		GCSClient:         realGCS,
		PUBSUBClient:      realPubSub,
		CTX:               &ctx,
		Bucket:            bucket,
		CompressTopicID:   compressTopicID,
		DecompressTopicID: decompressTopicID,
		MaxUploadSize:     1 << 30, // 1GB
		GCSTimeout:        50 * time.Second,
	}

	http.HandleFunc("/compress", app.compressHandler)
	http.HandleFunc("/decompress", app.decompressHandler)
	slog.Info("Listening on localhost:8081...")
	http.ListenAndServe(":8081", nil)
}
