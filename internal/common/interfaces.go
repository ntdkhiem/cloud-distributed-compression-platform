package common

import (
	"context"
	"io"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/storage"
)

type GCSObjectReaderInterface interface {
	io.ReadCloser
}
type GCSObjectWriterInterface interface {
	io.WriteCloser
}

type GCSClientInterface interface {
	NewObjectWriter(ctx context.Context, bucket, object string) GCSObjectWriterInterface
	NewObjectReader(ctx context.Context, bucket, object string) (GCSObjectReaderInterface, error)
}

type PubSubClientInterface interface {
	PublishMessage(ctx context.Context, topicID string, msg *pubsub.Message) (string, error)
}

// MessageInterface abstracts the Pub/Sub message for testing.
type MessageInterface interface {
	Ack()
	Nack()
	GetData() []byte
}

type RealGCSClient struct {
	Client *storage.Client
}

func (c *RealGCSClient) NewObjectWriter(ctx context.Context, bucket, object string) GCSObjectWriterInterface {
	return c.Client.Bucket(bucket).Object(object).NewWriter(ctx)
}

func (c *RealGCSClient) NewObjectReader(ctx context.Context, bucket, object string) (GCSObjectReaderInterface, error) {
	return c.Client.Bucket(bucket).Object(object).NewReader(ctx)
}

type RealPubSubClient struct {
	Client *pubsub.Client
}

func (c *RealPubSubClient) PublishMessage(ctx context.Context, topicID string, msg *pubsub.Message) (string, error) {
	publisher := c.Client.Publisher(topicID)
	result := publisher.Publish(ctx, msg)
	return result.Get(ctx)
}

// realMessage wraps the concrete pubsub.Message.
type RealMessage struct {
	Msg *pubsub.Message
}

func (r *RealMessage) Ack() {
	r.Msg.Ack()
}

func (r *RealMessage) Nack() {
	r.Msg.Nack()
}

func (r *RealMessage) GetData() []byte {
	return r.Msg.Data
}

// Must follow this schema to be accepted by Pub/Sub
type CompressedMsgSchema struct {
	UID              string `json:"UID"`
	OriginalFilePath string `json:"OriginalFilePath"`
	FreqTablePath    string `json:"FreqTablePath"`
}

// Must follow this schema to be accepted by Pub/Sub
type DecompressedMsgSchema struct {
	UID                string `json:"UID"`
	CompressedFilePath string `json:"CompressedFilePath"`
}
