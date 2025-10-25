package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/ntdkhiem/cloud-distributed-compression-platform/internal/common"
)

// --- Mocks ---

// mockGCSClient satisfies the GCSClientInterface
type mockGCSClient struct {
	mu    sync.Mutex
	files map[string]*bytes.Buffer // Stores uploaded files in memory
	// failRead tells NewGCSObjectReader to return an error
	failRead bool
	// failWrite tells NewGCSObjectWriter to return a writer that fails on Close
	// failWrite bool
}

// NewObjectWriter creates an in-memory writer
func (c *mockGCSClient) NewObjectWriter(ctx context.Context, bucket, object string) common.GCSObjectWriterInterface {
	return &mockGCSWriter{
		objectPath: object,
		buffer:     new(bytes.Buffer),
		client:     c,
	}
}

func (c *mockGCSClient) NewObjectReader(ctx context.Context, bucket, object string) (common.GCSObjectReaderInterface, error) {
	if c.failRead {
		return nil, errors.New("mock gcs read error")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.files[object]
	if !ok {
		return nil, errors.New("storage: object doesn't exist")
	}
	// Create a new reader from a copy of the bytes
	return &mockGCSObjectReader{bytes.NewReader(data.Bytes())}, nil
}

// Helper to pre-populate files
func (c *mockGCSClient) SetObject(object string, content []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[object] = bytes.NewBuffer(content)
}

// Helper to get file content from the mock
func (c *mockGCSClient) GetObjectContent(object string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf, ok := c.files[object]
	if !ok {
		return nil, false
	}
	return buf.Bytes(), true
}

// mockGCSWriter satisfies io.WriteCloser
type mockGCSWriter struct {
	objectPath string
	buffer     *bytes.Buffer
	client     *mockGCSClient
}

// Write adds data to the in-memory buffer
func (w *mockGCSWriter) Write(p []byte) (n int, err error) {
	return w.buffer.Write(p)
}

// Close "commits" the buffer to the mock client's file map
func (w *mockGCSWriter) Close() error {
	w.client.mu.Lock()
	defer w.client.mu.Unlock()
	w.client.files[w.objectPath] = w.buffer
	return nil
}

type mockGCSObjectReader struct {
	*bytes.Reader
}

func (r *mockGCSObjectReader) Close() error { return nil } // No-op

// mockMessage satisfies MessageInterface
type mockMessage struct {
	data       []byte
	ackCalled  bool
	nackCalled bool
}

func (m *mockMessage) Ack()            { m.ackCalled = true }
func (m *mockMessage) Nack()           { m.nackCalled = true }
func (m *mockMessage) GetData() []byte { return m.data }

// --- Dummy Huffman Functions (for testing) ---
// These MUST be defined for the test to compile.
// We add error flags to control their behavior.

// type huffmanNode struct{}
// type prefixTable map[rune]string
//
// var (
// 	failBuildHuffman bool
// 	failCompress     bool
// 	failDecompress   bool
// )
//
// func buildHuffmanTree(freqTable map[rune]uint64) ([]*huffmanNode, prefixTable, error) {
// 	if failBuildHuffman {
// 		return nil, nil, errors.New("mock buildHuffmanTree error")
// 	}
// 	if len(freqTable) == 0 {
// 		return nil, nil, errors.New("empty frequency table")
// 	}
// 	return []*huffmanNode{{}}, prefixTable{'a': "01"}, nil
// }
//
// func compress(root *huffmanNode, table prefixTable, reader *bufio.Reader) (*bytes.Buffer, error) {
// 	if failCompress {
// 		return nil, errors.New("mock compress error")
// 	}
// 	// Read the content to simulate work
// 	_, _ = io.ReadAll(reader)
// 	return bytes.NewBufferString("mock_compressed_data"), nil
// }
//
// func decompress(reader *bytes.Buffer, writer io.WriteCloser) error {
// 	if failDecompress {
// 		return errors.New("mock decompress error")
// 	}
// 	// Read and write to simulate work
// 	_, _ = io.ReadAll(reader)
// 	_, err := writer.Write([]byte("mock_decompressed_data"))
// 	return err
// }

// --- Test Setup ---

const testBucket = "test-bucket"

// setupTestApp initializes a new Application with mock clients.
func setupTestApp(t *testing.T) (*Application, *mockGCSClient) {
	t.Helper()

	ctx := context.Background()

	mockGCS := &mockGCSClient{
		files: make(map[string]*bytes.Buffer),
	}

	app := &Application{
		GCSClient: mockGCS,
		CTX:       &ctx,
		Bucket:    testBucket,
	}

	return app, mockGCS
}

// --- Tests ---

func TestCompressMessageHandler(t *testing.T) {
	app, mockGCS := setupTestApp(t)
	jobID := uuid.New().String()

	// --- Test: Success ---
	t.Run("success", func(t *testing.T) {
		// 1. Setup
		app, mockGCS = setupTestApp(t) // Reset mocks
		freqTablePath := fmt.Sprintf("%s/frequency_table.json", jobID)
		originalFilePath := fmt.Sprintf("%s/original.txt", jobID)

		// CompressedMsgSchema (check)
		jobMsg := common.CompressedMsgSchema{
			UID:              jobID, // correct job ID (check)
			OriginalFilePath: originalFilePath,
			FreqTablePath:    freqTablePath,
		}
		msgBytes, _ := json.Marshal(jobMsg)
		mockMsg := &mockMessage{data: msgBytes}

		// freqTable exists (check)
		freqTable := map[string]uint8{
			"10": 1,
			"13": 1,
			"97": 1,
			"98": 2,
		}
		freqTableBytes, _ := json.Marshal(freqTable)
		mockGCS.SetObject(freqTablePath, freqTableBytes)
		// file streaming (check)
		testContentReader, err := os.ReadFile("./test_data/test_data.txt")
		if err != nil {
			t.Error("Could not find test content to test compression")
			return
		}
		mockGCS.SetObject(originalFilePath, bytes.NewBuffer(testContentReader).Bytes())

		// 2. Execute
		app.compressMessageHandler(context.Background(), mockMsg)

		// 3. Verify
		// buildHuffmanTree works (check) / compress works (check)
		// (These are verified by the lack of an error and the presence of the output file)

		// compressed.ranran exists (check)
		expectedCompressedPath := fmt.Sprintf("%s/compressed.ranran", jobID)
		content, ok := mockGCS.GetObjectContent(expectedCompressedPath)
		if !ok {
			t.Errorf("Expected compressed file %q to exist, but it doesn't", expectedCompressedPath)
		}
		actualContentReader, err := os.ReadFile("./test_data/compressed.ranran")
		if err != nil {
			t.Error("Could not find actual content to verify against compressed data")
			return
		}

		actualContentBuff := bytes.NewBuffer(actualContentReader)
		// verify the size
		if len(content) != actualContentBuff.Len() {
			t.Errorf("Expected compressed content to have length of %d bytes but got %d bytes", actualContentBuff.Len(), len(content))
		}

		// TODO: must have a better check for the content here.
		// if !reflect.DeepEqual(content, actualContentBuff) {
		// 	t.Errorf("Expected compressed content\n'%08b'\ngot\n%08b", actualContentBuff.Bytes(), content)
		// }

		// msg is ack-ed (check)
		if !mockMsg.ackCalled {
			t.Error("Expected message to be Ack-ed, but it wasn't")
		}
		if mockMsg.nackCalled {
			t.Error("Expected message to not be Nack-ed, but it was")
		}
	})

	testCases := []struct {
		name  string
		setup func(t *testing.T) (*Application, *mockGCSClient, common.MessageInterface)
	}{
		{
			name: "bad Pub/Sub message",
			setup: func(t *testing.T) (*Application, *mockGCSClient, common.MessageInterface) {
				app, mockGCS := setupTestApp(t)
				mockMsg := &mockMessage{data: []byte("not json")}
				return app, mockGCS, mockMsg
			},
		},
		{
			name: "character frequency table does not exist",
			setup: func(t *testing.T) (*Application, *mockGCSClient, common.MessageInterface) {
				app, mockGCS := setupTestApp(t)
				jobMsg := common.CompressedMsgSchema{UID: jobID, FreqTablePath: "missing.json"}
				msgBytes, _ := json.Marshal(jobMsg)
				return app, mockGCS, &mockMessage{data: msgBytes}
			},
		},
		// TODO: buildHuffmanTree fails, compress fails, gcs write close fails
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app, _, mockMsg := tc.setup(t)
			app.compressMessageHandler(context.Background(), mockMsg)

			if !mockMsg.(*mockMessage).nackCalled {
				t.Error("Expected message to be Nack-ed, but it wasn't")
			}
			if mockMsg.(*mockMessage).ackCalled {
				t.Error("Expected message to not be Ack-ed, but it was")
			}
		})

	}
}

func TestDecompressMessageHandler(t *testing.T) {
	jobID := uuid.New().String()

	t.Run("success", func(t *testing.T) {
		// 1. Setup
		app, mockGCS := setupTestApp(t)
		compressedPath := fmt.Sprintf("%s/compressed.ranran", jobID)

		// pubsubDecompressMsgSchema (check)
		jobMsg := common.DecompressedMsgSchema{
			UID:                jobID,          // correct job ID (check)
			CompressedFilePath: compressedPath, // path is correct (check)
		}
		msgBytes, _ := json.Marshal(jobMsg)
		mockMsg := &mockMessage{data: msgBytes}

		// compressfile exists (check)
		compressedTestDataReader, err := os.ReadFile("./test_data/compressed.ranran")
		if err != nil {
			t.Error("Could not find compressed content to test decompression")
			return
		}
		mockGCS.SetObject(compressedPath, bytes.NewBuffer(compressedTestDataReader).Bytes())

		// 2. Execute
		app.decompressMessageHandler(context.Background(), mockMsg)

		// 3. Verify
		expectedFinalPath := fmt.Sprintf("%s/file.txt", jobID)
		content, ok := mockGCS.GetObjectContent(expectedFinalPath)
		if !ok {
			t.Errorf("Expected compressed file %q to exist, but it doesn't", expectedFinalPath)
		}
		actualContentReader, err := os.ReadFile("./test_data/test_data.txt")
		if err != nil {
			t.Error("Could not find actual content to verify against decompressed data")
			return
		}

		actualContentBuff := bytes.NewBuffer(actualContentReader)
		// verify the size
		if len(content) != actualContentBuff.Len() {
			t.Errorf("Expected compressed content to have length of %d bytes but got %d bytes", actualContentBuff.Len(), len(content))
		}

		if string(content)+"\n" == actualContentBuff.String() {
			t.Errorf("Expected decompressed content to be '%s', but got %s", actualContentBuff.String(), string(content))
		}

		// msg is ack-ed (check)
		if !mockMsg.ackCalled {
			t.Error("Expected message to be Ack-ed, but it wasn't")
		}
		if mockMsg.nackCalled {
			t.Error("Expected message to not be Nack-ed, but it was")
		}
	})

	// --- Test: Failure Cases ---
	testCases := []struct {
		name  string
		setup func(t *testing.T) (*Application, common.MessageInterface)
	}{
		// ... (bad pubsub, file missing are the same) ...
		{
			"bad pubsub message",
			func(t *testing.T) (*Application, common.MessageInterface) {
				app, _ := setupTestApp(t)
				return app, &mockMessage{data: []byte("not json")}
			},
		},
		{
			"compressed file does not exist",
			func(t *testing.T) (*Application, common.MessageInterface) {
				app, _ := setupTestApp(t)
				jobMsg := common.DecompressedMsgSchema{UID: jobID, CompressedFilePath: "missing.ranran"}
				msgBytes, _ := json.Marshal(jobMsg)
				return app, &mockMessage{data: msgBytes}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app, mockMsg := tc.setup(t)
			app.decompressMessageHandler(context.Background(), mockMsg)

			if !mockMsg.(*mockMessage).nackCalled {
				t.Error("Expected message to be Nack-ed, but it wasn't")
			}
			if mockMsg.(*mockMessage).ackCalled {
				t.Error("Expected message to not be Ack-ed, but it was")
			}
		})
	}
}
