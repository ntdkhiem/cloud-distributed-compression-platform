package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
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
		log.Printf("ERROR: failed to unmarshal body from job message: %v", err)
		msg.Nack()
		return
	}

	log.Printf("INFO: received job %s", job.UID)

	// Download character frequency table from GCS
	ctx, cancel := context.WithTimeout(*app.CTX, time.Second*50)
	defer cancel()

	freqTableBytes, err := app.GCSClient.Bucket(app.Bucket).Object(job.FreqTablePath).NewReader(ctx)
	if err != nil {
		log.Printf("ERROR: failed to download character frequency table from job %s: %v", job.UID, err)
		msg.Nack()
		return
	}
	defer freqTableBytes.Close()

	var freqTable map[rune]uint64
	if err := json.NewDecoder(freqTableBytes).Decode(&freqTable); err != nil {
		log.Printf("ERROR: failed to decode character frequency table from job %s: %v", job.UID, err)
		msg.Nack()
		return
	}
	log.Printf("INFO: downloaded character frequency table from job %s", job.UID)

	huffmanTree, err := buildHuffmanTree(freqTable)
	if err != nil {
		log.Printf("ERROR: failed to HuffmanTree for job %s: %v", job.UID, err)
		msg.Nack()
		return
	}
	log.Printf("INFO: built Huffman Tree: %v", huffmanTree[0].value.char)

	// stream file content down and compress
	ogFileBytes, err := app.GCSClient.Bucket(app.Bucket).Object(job.OriginalFilePath).NewReader(ctx)
	if err != nil {
		log.Printf("ERROR: failed to download original file content from job %s: %v", job.UID, err)
		msg.Nack()
		return
	}
	defer ogFileBytes.Close()

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		compress(huffmanTree[0], bufio.NewReader(ogFileBytes), pw)
		log.Printf("DEBUG: ")
	}()

	compressedFilePath := fmt.Sprintf("%s/compressed.ranran", job.UID)
	wc := app.GCSClient.Bucket(app.Bucket).Object(compressedFilePath).NewWriter(ctx)
	if _, err := io.Copy(wc, pr); err != nil {
		log.Printf("ERROR: failed to stream compressed data to GCS: %v", err)
		msg.Nack()
		return
	}
	if err := wc.Close(); err != nil {
		log.Printf("ERROR: failed to close data stream to GCS: %v", err)
		msg.Nack()
		return
	}
	log.Printf("INFO: Uploaded compressed data from job %s to GCS", job.UID)

	msg.Ack()
	log.Printf("INFO: completed processing job %s", job.UID)
}

func main() {
	projectID := os.Getenv("GCP_PROJECT_ID")
	topicID := os.Getenv("PUBSUB_TOPIC_ID")
	subID := os.Getenv("PUBSUB_SUB_ID")
	bucket := os.Getenv("GCS_BUCKET")
	ctx := context.Background()

	GCSClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("ERROR: Cannot create new client for GCS: %v", err)
		return
	}
	defer GCSClient.Close()
	log.Printf("INFO: Initialized a GCS client.")

	PUBSUBClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Printf("ERROR: Cannot create new client for Pub/Sub: %v", err)
		return
	}
	defer PUBSUBClient.Close()
	log.Printf("INFO: Initialized a Pub/Sub client.")

	app := Application{
		GCSClient:    GCSClient,
		PUBSUBClient: PUBSUBClient,
		CTX:          &ctx,
		Bucket:       bucket,
		TopicID:      topicID,
	}

	sub := PUBSUBClient.Subscriber(subID)

	log.Printf("INFO: Listening for a new message...")
	// TODO: use context.WithCancel if fail to process during Receive
	err = sub.Receive(ctx, app.messageHandler)
	if err != nil {
		log.Printf("ERROR: cannot process job: %v", err)
		return
	}
}
