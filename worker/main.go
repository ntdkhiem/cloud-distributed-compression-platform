package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
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

// Must follow this schema to be accepted by Pub/Sub
type pubsubCompressMsgSchema struct {
	UID              string `json:"UID"`
	OriginalFilePath string `json:"OriginalFilePath"`
	FreqTablePath    string `json:"FreqTablePath"`
}

// Must follow this schema to be accepted by Pub/Sub
type pubsubDecompressMsgSchema struct {
	UID                string `json:"UID"`
	CompressedFilePath string `json:"CompressedFilePath"`
}

func (app *Application) compressMessageHandler(_ context.Context, msg *pubsub.Message) {
	var job pubsubCompressMsgSchema
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
		slog.Debug("Compressing file content", "job", job.UID)
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

func (app *Application) decompressMessageHandler(_ context.Context, msg *pubsub.Message) {
	var job pubsubDecompressMsgSchema
	if err := json.Unmarshal(msg.Data, &job); err != nil {
		slog.Error("Failed to unmarshal body from job message", "error", err)
		msg.Nack()
		return
	}

	slog.Info("Received job", "job", job.UID)

	//--- Download character frequency table from GCS
	ctx, cancel := context.WithTimeout(*app.CTX, time.Second*50)
	defer cancel()

	slog.Debug("Stream compressed file from GCS.", "job", job.UID)
	//--- stream file content down and decompress
	file, err := app.GCSClient.Bucket(app.Bucket).Object(job.CompressedFilePath).NewReader(ctx)
	if err != nil {
		slog.Error("Failed to download compressed file content", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	defer file.Close()

	//--- extract header
	headerLenBin := make([]byte, 2)
	_, err = file.Read(headerLenBin)
	if err != nil {
		slog.Error("failed to extract header's length", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	headerLen := binary.LittleEndian.Uint16(headerLenBin)

	headerBin := make([]byte, headerLen)
	_, err = file.Read(headerBin)
	if err != nil {
		slog.Error("failed to extract header's content", "job", job.UID, "error", err)
		msg.Nack()
		return
	}

	tree := buildHuffmanTreeFromBin(headerBin)

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		slog.Debug("Decompressing file content", "job", job.UID)
		err := decompress(tree, bufio.NewReader(file), pw)
		if err != nil {
			slog.Error("Failed to decompress", "job", job.UID, "error", err)
			msg.Nack()
			return
		}
	}()

	// TODO: create a metadata with original file name to be referenced here.
	resultFilePath := fmt.Sprintf("%s/file.txt", job.UID)
	wc := app.GCSClient.Bucket(app.Bucket).Object(resultFilePath).NewWriter(ctx)
	if _, err := io.Copy(wc, pr); err != nil {
		slog.Error("Failed to stream final data to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close data stream to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	slog.Debug("Uploaded final data to GCS", "job", job.UID)

	msg.Ack()
	slog.Info("Completed processing job", "job", job.UID)
}

func main() {
	methodFlag := flag.Bool("decompress", false, "flag to indicate this instance is for decompressing.")
	flag.Parse()

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

	// TODO: use context.WithCancel if fail to process during Receive
	if *methodFlag {
		slog.Info("Listening for a new decompressing message...")
		err = sub.Receive(ctx, app.decompressMessageHandler)
	} else {
		slog.Info("Listening for a new compressing message...")
		err = sub.Receive(ctx, app.compressMessageHandler)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Cannot process job", "error", err)
		return
	}
}
