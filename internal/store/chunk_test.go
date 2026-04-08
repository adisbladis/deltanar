package store

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
)

func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "chunkfile-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return f.Name()
}

func blake3Digest(data []byte) []byte {
	h := blake3.New()
	_, _ = h.Write(data)
	d := make([]byte, DigestLen)
	_, _ = h.Digest().Read(d)
	return d
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return buf
}

func reassemble(t *testing.T, cf *ChunkedFile, original []byte) {
	t.Helper()
	var reassembled []byte
	for i, c := range cf.Chunks {
		segment := original[c.Offset : c.Offset+c.Len]
		reassembled = append(reassembled, segment...)

		want := blake3Digest(segment)
		if !bytes.Equal(c.Digest, want) {
			t.Errorf("chunk %d digest mismatch", i)
		}
	}
	if !bytes.Equal(reassembled, original) {
		t.Errorf("reassembled content differs from original")
	}
}

func TestChunkFile_EmptyFile(t *testing.T) {
	path := writeTempFile(t, []byte{})

	cf, err := ChunkFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Chunks) != 0 {
		t.Errorf("expected 0 chunks for empty file, got %d", len(cf.Chunks))
	}

	want := blake3Digest([]byte{})
	if !bytes.Equal(cf.Digest, want) {
		t.Errorf("digest mismatch for empty file")
	}
}

func TestChunkFile_SmallFile(t *testing.T) {
	data := []byte("hello, world")
	path := writeTempFile(t, data)

	cf, err := ChunkFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// Chunks must cover the whole file with no gaps.
	total := 0
	for _, c := range cf.Chunks {
		total += c.Len
	}
	if total != len(data) {
		t.Errorf("total chunk bytes %d != file size %d", total, len(data))
	}

	// File digest must equal blake3(full content).
	want := blake3Digest(data)
	if !bytes.Equal(cf.Digest, want) {
		t.Errorf("file digest mismatch")
	}

	reassemble(t, cf, data)
}

func TestChunkFile_LargeFile(t *testing.T) {
	const size = 4 * 1024 * 1024 // 4 MiB
	data := randomBytes(t, size)
	path := writeTempFile(t, data)

	cf, err := ChunkFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Chunks) < 2 {
		t.Errorf("expected multiple chunks for 4 MiB file, got %d", len(cf.Chunks))
	}

	total := 0
	for _, c := range cf.Chunks {
		total += c.Len
	}
	if total != size {
		t.Errorf("total chunk bytes %d != file size %d", total, size)
	}

	want := blake3Digest(data)
	if !bytes.Equal(cf.Digest, want) {
		t.Errorf("file digest mismatch")
	}

	reassemble(t, cf, data)
}

func TestChunkFile_DigestLength(t *testing.T) {
	data := randomBytes(t, 512*1024)
	path := writeTempFile(t, data)

	cf, err := ChunkFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Digest) != DigestLen {
		t.Errorf("file digest length %d, want %d", len(cf.Digest), DigestLen)
	}
	for i, c := range cf.Chunks {
		if len(c.Digest) != DigestLen {
			t.Errorf("chunk %d digest length %d, want %d", i, len(c.Digest), DigestLen)
		}
	}
}

func TestChunkFile_NonExistentFile(t *testing.T) {
	_, err := ChunkFile(filepath.Join(t.TempDir(), "does-not-exist.bin"))
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestChunkFile_Directory(t *testing.T) {
	_, err := ChunkFile(t.TempDir())
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
}

func TestChunkFile_SingleByteFile(t *testing.T) {
	data := []byte{0x42}
	path := writeTempFile(t, data)

	cf, err := ChunkFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cf.Chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	total := 0
	for _, c := range cf.Chunks {
		total += c.Len
	}
	if total != 1 {
		t.Errorf("total chunk bytes %d, want 1", total)
	}

	want := blake3Digest(data)
	if !bytes.Equal(cf.Digest, want) {
		t.Error("file digest mismatch for single-byte file")
	}

	reassemble(t, cf, data)
}
