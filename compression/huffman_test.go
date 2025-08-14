package compression

import (
	"os"
	"strings"
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

func TestCompress_InvalidFile(t *testing.T) {
	_, err := Compress("non_existent_file.txt")
	if err == nil {
		t.Errorf("expected error for nonexistent file, got nil")
	}
}

func TestDecompress_ValidFile(t *testing.T) {
	// Create a compressed file using Compress()
	originalContent := "decompression test"
	tmpFile, err := os.CreateTemp("", "compressed_*.kn")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	buf, err := Compress(writeTempFile(t, originalContent))
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}

	err = os.WriteFile(tmpFile.Name(), buf.Bytes(), 0644)
	if err != nil {
		t.Fatalf("writing compressed file failed: %v", err)
	}

	builder, err := Decompress(tmpFile.Name())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if builder.String() != originalContent {
		t.Errorf("expected '%s', got '%s'", originalContent, builder.String())
	}
}

func TestDecompress_InvalidFile(t *testing.T) {
	_, err := Decompress("nonexistent.kn")
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}

func TestCompressDecompress_Basic(t *testing.T) {
	text := "simple roundtrip"
	roundTripCheck(t, text)
}

func TestCompressDecompress_Empty(t *testing.T) {
	text := ""
	roundTripCheck(t, text)
}

func TestCompressDecompress_Repetitive(t *testing.T) {
	text := strings.Repeat("ab", 1000)
	roundTripCheck(t, text)
}

func TestCompressDecompress_Large(t *testing.T) {
	text := strings.Repeat("abcde", 2000) // ~10 KB
	roundTripCheck(t, text)
}

func TestCompressDecompress_Larger(t *testing.T) {
	text := strings.Repeat("abcdefg1234567", 75000) // ~1 MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_Largest(t *testing.T) {
	text := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 170000) // ~10 MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_100MB(t *testing.T) {
	t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat("abc1234567890xyz", 7_000_000) // ~100MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_150MB(t *testing.T) {
	t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat("abcdefgh12345678", 9_800_000) // ~150MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_200MB(t *testing.T) {
	t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat("GoLangCompressionStressTest!", 7_800_000) // ~200MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_200MB_U(t *testing.T) {
	sampleChunk := `
# Title: 多言語テスト 🧪
This	is	a	test	line	with	tabs	and	foreign	chars.	中文行
Another line with emoji 🚀 and Cyrillic: Пример строки.
----------------------------------------
`
	// t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat(sampleChunk, 1_000_000) // ~200MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_500MB(t *testing.T) {
	t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat("GoLangCompressionStressTest!", 17_800_000) // ~500MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_500MB_U(t *testing.T) {
	sampleChunk := `
# Title: 多言語テスト 🧪
This	is	a	test	line	with	tabs	and	foreign	chars.	中文行
Another line with emoji 🚀 and Cyrillic: Пример строки.
----------------------------------------
`
	// t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat(sampleChunk, 2_500_000) // ~500MB
	roundTripCheck(t, text)
}

func TestCompressDecompress_1GB_U(t *testing.T) {
	sampleChunk := `
# Title: 多言語テスト 🧪
This	is	a	test	line	with	tabs	and	foreign	chars.	中文行
Another line with emoji 🚀 and Cyrillic: Пример строки.
----------------------------------------
`
	// t.Skip("Skip by default. Run manually when needed.")
	text := strings.Repeat(sampleChunk, 5_500_000) // ~500MB
	roundTripCheck(t, text)
}

func writeTempFile(t *testing.T, content string) string {
	tmp, err := os.CreateTemp("", "original_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer tmp.Close()
	_, err = tmp.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	return tmp.Name()
}

func roundTripCheck(t *testing.T, input string) {
	t.Helper()
	inputPath := writeTempFile(t, input)
	defer os.Remove(inputPath)
	compressed, err := Compress(inputPath)
	if err != nil {
		t.Fatalf("compress failed: %v", err)
	}
	compressedPath := inputPath + ".kn"
	err = os.WriteFile(compressedPath, compressed.Bytes(), 0644)
	if err != nil {
		t.Fatalf("writing compressed file failed: %v", err)
	}
	defer os.Remove(compressedPath)
	output, err := Decompress(compressedPath)
	if err != nil {
		t.Fatalf("decompress failed: %v", err)
	}
	if output.String() != input {
		t.Errorf("expected '%s', got '%s'", input, output.String())
	}
}
