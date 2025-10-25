package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/google/uuid"
)

// --- Mocks ---

// mockGCSClient satisfies the GCSClientInterface
type mockGCSClient struct {
	mu    sync.Mutex
	files map[string]*bytes.Buffer // Stores uploaded files in memory
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

// NewObjectWriter creates an in-memory writer
func (c *mockGCSClient) NewObjectWriter(ctx context.Context, bucket, object string) io.WriteCloser {
	// Note: We don't need to check the bucket for this mock
	return &mockGCSWriter{
		objectPath: object,
		buffer:     new(bytes.Buffer),
		client:     c,
	}
}

// Helper to get file content from the mock
func (c *mockGCSClient) GetObjectContent(object string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf, ok := c.files[object]
	if !ok {
		return "", false
	}
	return buf.String(), true
}

// mockPubSubClient satisfies the PubSubClientInterface
type mockPubSubClient struct {
	mu       sync.Mutex
	messages map[string][]*pubsub.Message // Stores published messages in memory
}

// PublishMessage adds the message to the in-memory map and returns a mock ID
func (c *mockPubSubClient) PublishMessage(ctx context.Context, topicID string, msg *pubsub.Message) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.messages == nil {
		c.messages = make(map[string][]*pubsub.Message)
	}
	c.messages[topicID] = append(c.messages[topicID], msg)
	return "mock-message-id-" + uuid.NewString(), nil
}

// Helper to get messages from the mock
func (c *mockPubSubClient) GetMessages(topicID string) []*pubsub.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.messages[topicID]
}

// --- Test Setup ---

const (
	testBucket          = "test-bucket"
	testCompressTopic   = "compress-topic"
	testDecompressTopic = "decompress-topic"
	testSmallUploadSize = 1024 // 1KB for testing size limits
)

// setupTestApp initializes a new Application with mock clients.
func setupTestApp(t *testing.T) (*Application, *mockGCSClient, *mockPubSubClient) {
	t.Helper()

	ctx := context.Background()

	mockGCS := &mockGCSClient{
		files: make(map[string]*bytes.Buffer),
	}

	mockPubSub := &mockPubSubClient{
		messages: make(map[string][]*pubsub.Message),
	}

	app := &Application{
		GCSClient:         mockGCS,
		PUBSUBClient:      mockPubSub,
		CTX:               &ctx,
		Bucket:            testBucket,
		CompressTopicID:   testCompressTopic,
		DecompressTopicID: testDecompressTopic,
		MaxUploadSize:     testSmallUploadSize, // Set a small limit for testing
		GCSTimeout:        5 * time.Second,
	}

	return app, mockGCS, mockPubSub
}

// createTestMultipartRequest is a helper to build a file upload request.
func createTestMultipartRequest(t *testing.T, fieldName, fileName, fileContent string) *http.Request {
	t.Helper()

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	_, err = io.Copy(part, strings.NewReader(fileContent))
	if err != nil {
		t.Fatalf("Failed to write file content to form: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// getJobIDFromResponse is a helper to parse the {"job_id": "..."} response.
func getJobIDFromResponse(t *testing.T, body *bytes.Buffer) string {
	t.Helper()
	var resp map[string]string
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}
	jobID, ok := resp["job_id"]
	if !ok {
		t.Fatal("Response body missing 'job_id'")
	}
	if _, err := uuid.Parse(jobID); err != nil {
		t.Fatalf("job_id '%s' is not a valid UUID", jobID)
	}
	return jobID
}

// --- Tests ---

func TestCompressHandler(t *testing.T) {
	app, mockGCS, mockPubSub := setupTestApp(t)

	testCases := []struct {
		name             string
		fileContent      string
		fileName         string
		expectedStatus   int
		expectedFreqJSON string
		expectedErr      string
	}{
		{
			name:             "success",
			fileContent:      "hello world 👋",
			fileName:         "test.txt",
			expectedStatus:   http.StatusAccepted,
			expectedFreqJSON: `{"32":2,"100":1,"101":1,"104":1,"108":3,"111":2,"114":1,"119":1,"128075":1}`,
		},
		{
			name:           "file size limit",
			fileContent:    strings.Repeat("a", int(testSmallUploadSize)+1), // 1 byte over limit
			fileName:       "large.txt",
			expectedStatus: http.StatusRequestEntityTooLarge,
			expectedErr:    "File exceeds size limit",
		},
		{
			name:           "no file",
			fileContent:    "", // Will cause a different multipart error
			fileName:       "",
			expectedStatus: http.StatusBadRequest,
			expectedErr:    "Failed to read file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mocks for each sub-test
			mockGCS.files = make(map[string]*bytes.Buffer)
			mockPubSub.messages = make(map[string][]*pubsub.Message)

			var req *http.Request
			if tc.name == "no file" {
				// Send an empty body to trigger r.FormFile error
				req = httptest.NewRequest(http.MethodPost, "/compress", new(bytes.Buffer))
				req.Header.Set("Content-Type", "multipart/form-data; boundary=--boundary")
			} else {
				req = createTestMultipartRequest(t, "file", tc.fileName, tc.fileContent)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(app.compressHandler)
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, tc.expectedStatus)
			}

			if tc.expectedErr != "" {
				if !strings.Contains(rr.Body.String(), tc.expectedErr) {
					t.Errorf("handler returned wrong error: got %q want to contain %q", rr.Body.String(), tc.expectedErr)
				}
				return // Don't check side effects on failure
			}

			// --- Check Side Effects (on success) ---
			jobID := getJobIDFromResponse(t, rr.Body)

			// Check: file streaming
			originalFilePath := fmt.Sprintf("%s/original_%s", jobID, tc.fileName)
			content, ok := mockGCS.GetObjectContent(originalFilePath)
			if !ok {
				t.Errorf("GCS file %q was not created", originalFilePath)
			}
			if content != tc.fileContent {
				t.Errorf("GCS file content mismatch: got %q want %q", content, tc.fileContent)
			}

			// Check: character frequency table & table streaming
			freqTablePath := fmt.Sprintf("%s/frequency_table.json", jobID)
			freqContent, ok := mockGCS.GetObjectContent(freqTablePath)
			if !ok {
				t.Errorf("GCS file %q was not created", freqTablePath)
			}

			// Unmarshal both to maps for a robust comparison
			var gotMap, wantMap map[string]uint64
			if err := json.Unmarshal([]byte(freqContent), &gotMap); err != nil {
				t.Fatalf("Failed to unmarshal actual freq table: %v", err)
			}
			if err := json.Unmarshal([]byte(tc.expectedFreqJSON), &wantMap); err != nil {
				t.Fatalf("Failed to unmarshal expected freq table: %v", err)
			}
			if !reflect.DeepEqual(gotMap, wantMap) {
				t.Errorf("GCS freq table mismatch:\ngot  %v\nwant %v", gotMap, wantMap)
			}

			// Check: messages pubs
			messages := mockPubSub.GetMessages(app.CompressTopicID)
			if len(messages) != 1 {
				t.Fatalf("Expected 1 Pub/Sub message, got %d", len(messages))
			}
			var pubsubMsg pubsubCompressMsgSchema
			if err := json.Unmarshal(messages[0].Data, &pubsubMsg); err != nil {
				t.Fatalf("Failed to unmarshal Pub/Sub message: %v", err)
			}

			if pubsubMsg.UID != jobID {
				t.Errorf("Pub/Sub message UID mismatch: got %q want %q", pubsubMsg.UID, jobID)
			}
			if pubsubMsg.OriginalFilePath != originalFilePath {
				t.Errorf("Pub/Sub OriginalFilePath mismatch: got %q want %q", pubsubMsg.OriginalFilePath, originalFilePath)
			}
			if pubsubMsg.FreqTablePath != freqTablePath {
				t.Errorf("Pub/Sub FreqTablePath mismatch: got %q want %q", pubsubMsg.FreqTablePath, freqTablePath)
			}
		})

	}

	// Test: wrong http method
	t.Run("wrong http method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/compress", nil)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(app.compressHandler)
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusMethodNotAllowed)
		}
	})
}

// TestDecompressHandler covers all requested test points for /decompress
func TestDecompressHandler(t *testing.T) {
	app, mockGCS, mockPubSub := setupTestApp(t)

	testCases := []struct {
		name           string
		fileContent    string
		fileName       string
		expectedStatus int
		expectedErr    string
	}{
		{
			name:           "success",
			fileContent:    "compressed_data_bytes",
			fileName:       "archive.ranran",
			expectedStatus: http.StatusAccepted,
		},
        {
			name:           "file extension (wrong extension)",
			fileContent:    "some_data",
			fileName:       "archive.zip",
			expectedStatus: http.StatusBadRequest,
			expectedErr:    "Wrong file format",
		},
		{
			name:           "file size limit",
			fileContent:    strings.Repeat("a", int(testSmallUploadSize)+1), // 1 byte over limit
			fileName:       "large.ranran",
			expectedStatus: http.StatusRequestEntityTooLarge,
			expectedErr:    "File exceeds size limit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mocks
			mockGCS.files = make(map[string]*bytes.Buffer)
			mockPubSub.messages = make(map[string][]*pubsub.Message)

			req := createTestMultipartRequest(t, "file", tc.fileName, tc.fileContent)
			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(app.decompressHandler)
			handler.ServeHTTP(rr, req)

			// --- Check Status Code ---
			if rr.Code != tc.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, tc.expectedStatus)
			}

			if tc.expectedErr != "" {
				if !strings.Contains(rr.Body.String(), tc.expectedErr) {
					t.Errorf("handler returned wrong error: got %q want to contain %q", rr.Body.String(), tc.expectedErr)
				}
				return // Don't check side effects on failure
			}

			// --- Check Side Effects (on success) ---
			jobID := getJobIDFromResponse(t, rr.Body)

			// Check: file streaming
			compressedFilePath := fmt.Sprintf("%s/%s", jobID, tc.fileName)
			content, ok := mockGCS.GetObjectContent(compressedFilePath)
			if !ok {
				t.Errorf("GCS file %q was not created", compressedFilePath)
			}
			if content != tc.fileContent {
				t.Errorf("GCS file content mismatch: got %q want %q", content, tc.fileContent)
			}

			// Check: messages pubs
			messages := mockPubSub.GetMessages(app.DecompressTopicID)
			if len(messages) != 1 {
				t.Fatalf("Expected 1 Pub/Sub message, got %d", len(messages))
			}
			var pubsubMsg pubsubDecompressMsgSchema
			if err := json.Unmarshal(messages[0].Data, &pubsubMsg); err != nil {
				t.Fatalf("Failed to unmarshal Pub/Sub message: %v", err)
			}

			if pubsubMsg.UID != jobID {
				t.Errorf("Pub/Sub message UID mismatch: got %q want %q", pubsubMsg.UID, jobID)
			}
			if pubsubMsg.CompressedFilePath != compressedFilePath {
				t.Errorf("Pub/Sub CompressedFilePath mismatch: got %q want %q", pubsubMsg.CompressedFilePath, compressedFilePath)
			}
		})
	}
}
