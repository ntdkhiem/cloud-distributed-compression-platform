package compression

import (
	"os"
	"testing"
)

func TestCompress_ValidFile(t *testing.T) {
	// Step 1: Create a temporary file
	tmpFile, err := os.CreateTemp("", "test_input_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	// Step 2: Write some content to it
	content := "this is a test for compression"
	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Step 3: Call Compress
	result, err := Compress(tmpFile.Name())
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if result == nil || result.Len() == 0 {
		t.Errorf("expected non-empty compressed result")
	}
}
