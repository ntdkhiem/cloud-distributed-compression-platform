package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/storage"
)

type Application struct {
	GCSClient    *storage.Client
	PUBSUBClient *pubsub.Client
	CTX          *context.Context
	Bucket       string
	TopicID      string
}

// Must follow this schema to be parsed from Pub/Sub
type pubsubMessageSchema struct {
	UID              string `json:"UID"`
	OriginalFilePath string `json:"OriginalFilePath"`
	FreqTablePath    string `json:"FreqTablePath"`
}

func (app *Application) messageHandler(_ context.Context, msg *pubsub.Message) {
	var job pubsubMessageSchema
	if err := json.Unmarshal(msg.Data, &job); err != nil {
		slog.Error("Failed to unmarshal body from job message", "error", err)
		msg.Nack()
		return
	}

	slog.Info("Received job", "job", job.UID)

	// Download character frequency table from GCS
	ctx, cancel := context.WithTimeout(*app.CTX, time.Second*50)
	defer cancel()

	freqTableBytes, err := app.GCSClient.Bucket(app.Bucket).Object(job.FreqTablePath).NewReader(ctx)
	if err != nil {
		slog.Error("Failed to download character frequency table", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	defer freqTableBytes.Close()

	var freqTable map[rune]uint64
	if err := json.NewDecoder(freqTableBytes).Decode(&freqTable); err != nil {
		slog.Error("Failed to decode character frequency table", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	slog.Debug("Downloaded character frequency table", "job", job.UID)

	huffmanTree, prefixTable, err := buildHuffmanTree(freqTable)
	if err != nil {
		slog.Error("Failed to HuffmanTree", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	slog.Debug("Built Huffman Tree", "job", job.UID)

	// stream file content down and compress
	ogFileBytes, err := app.GCSClient.Bucket(app.Bucket).Object(job.OriginalFilePath).NewReader(ctx)
	if err != nil {
		slog.Error("Failed to download original file content", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	defer ogFileBytes.Close()

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
        err := compress(huffmanTree[0], prefixTable, bufio.NewReader(ogFileBytes), pw)
        if err != nil {
            slog.Error("Failed to compress", "job", job.UID, "error", err)
            msg.Nack()
            return
        }
	}()

	compressedFilePath := fmt.Sprintf("%s/compressed.ranran", job.UID)
	wc := app.GCSClient.Bucket(app.Bucket).Object(compressedFilePath).NewWriter(ctx)
	if _, err := io.Copy(wc, pr); err != nil {
		slog.Error("Failed to stream compressed data to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close data stream to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	slog.Debug("Uploaded compressed data to GCS", "job", job.UID)

	msg.Ack()
	slog.Info("Completed processing job", "job", job.UID)
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
	subID := os.Getenv("PUBSUB_SUB_ID")
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

	sub := PUBSUBClient.Subscriber(subID)

	slog.Info("Listening for a new message...")
	// TODO: use context.WithCancel if fail to process during Receive
	err = sub.Receive(ctx, app.messageHandler)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Cannot process job", "error", err)
		return
	}
}
