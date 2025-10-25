package main

import (
	"bufio"
	"bytes"
	"context"
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

	"github.com/ntdkhiem/cloud-distributed-compression-platform/internal/common"
)

type Application struct {
	GCSClient  common.GCSClientInterface
	CTX        *context.Context
	Bucket     string
	GCSTimeout time.Duration
}

func (app *Application) compressMessageHandler(_ context.Context, msg common.MessageInterface) {
	var job common.CompressedMsgSchema
	if err := json.Unmarshal(msg.GetData(), &job); err != nil {
		slog.Error("Failed to unmarshal body from job message", "error", err)
		msg.Nack()
		return
	}

	slog.Info("Received job", "job", job.UID)

	// Download character frequency table from GCS
	ctx, cancel := context.WithTimeout(*app.CTX, app.GCSTimeout)
	defer cancel()

	freqTableReader, err := app.GCSClient.NewObjectReader(ctx, app.Bucket, job.FreqTablePath)
	if err != nil {
		slog.Error("Failed to download character frequency table", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	defer freqTableReader.Close()

	var freqTable map[rune]uint64
	if err := json.NewDecoder(freqTableReader).Decode(&freqTable); err != nil {
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
	ogFileReader, err := app.GCSClient.NewObjectReader(ctx, app.Bucket, job.OriginalFilePath)
	if err != nil {
		slog.Error("Failed to locate original file content", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	defer ogFileReader.Close()

	ogFileBytes, err := io.ReadAll(ogFileReader)
	if err != nil {
		slog.Error("Failed to download data to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	slog.Debug("Downloaded text data", "job", job.UID)

	// reset the file cursor back to the beginning
	fileReader := bufio.NewReader(bytes.NewReader(ogFileBytes))

	compFileBuf, err := compress(huffmanTree[0], prefixTable, fileReader)
	if err != nil {
		slog.Error("Failed to compress data", "job", job.UID, "error", err)
		msg.Nack()
		return
	}

	compressedFilePath := fmt.Sprintf("%s/compressed.ranran", job.UID)
	wc := app.GCSClient.NewObjectWriter(ctx, app.Bucket, compressedFilePath)
	if _, err := io.Copy(wc, compFileBuf); err != nil {
		slog.Error("Failed to upload compressed data to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	if err := wc.Close(); err != nil {
		slog.Error("Failed to close data compressing stream to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	slog.Debug("Uploaded compressed data to GCS", "job", job.UID)

	msg.Ack()
	slog.Info("Completed processing job", "job", job.UID)
}

func (app *Application) decompressMessageHandler(_ context.Context, msg common.MessageInterface) {
	var job common.DecompressedMsgSchema
	if err := json.Unmarshal(msg.GetData(), &job); err != nil {
		slog.Error("Failed to unmarshal body from job message", "error", err)
		msg.Nack()
		return
	}

	slog.Info("Received job", "job", job.UID)

	ctx, cancel := context.WithTimeout(*app.CTX, app.GCSTimeout)
	defer cancel()

	compFile, err := app.GCSClient.NewObjectReader(ctx, app.Bucket, job.CompressedFilePath)
	if err != nil {
		slog.Error("Failed to locate compressed file content", "job", job.UID, "error", err)
		msg.Nack()
		return
	}
	defer compFile.Close()

	fileBytes, err := io.ReadAll(compFile)
	if err != nil {
		slog.Error("Failed to download data to GCS", "job", job.UID, "error", err)
		msg.Nack()
		return
	}

	slog.Debug("Downloaded compressed file from GCS.", "job", job.UID)

	resultFilePath := fmt.Sprintf("%s/file.txt", job.UID)
	wc := app.GCSClient.NewObjectWriter(ctx, app.Bucket, resultFilePath)

	err = decompress(bytes.NewBuffer(fileBytes), wc)
	if err != nil {
		slog.Error("failed to decompress data", "job", job.UID, "error", err)
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

	realGCS := &common.RealGCSClient{Client: GCSClient}

	app := Application{
		GCSClient:  realGCS,
		CTX:        &ctx,
		Bucket:     bucket,
		GCSTimeout: 50 * time.Second,
	}

	sub := PUBSUBClient.Subscriber(subID)
	receiveFunc := func(ctx context.Context, msg *pubsub.Message) {
		wrappedMsg := &common.RealMessage{Msg: msg}
		if *methodFlag {
			app.decompressMessageHandler(ctx, wrappedMsg)
		} else {
			app.compressMessageHandler(ctx, wrappedMsg)
		}
	}

	if *methodFlag {
		slog.Info("Listening for a new decompressing message...")
	} else {
		slog.Info("Listening for a new compressing message...")
	}
	err = sub.Receive(ctx, receiveFunc)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Cannot process job", "error", err)
		return
	}
}
