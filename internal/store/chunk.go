package store

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	"github.com/zeebo/blake3"
)

const DigestLen = 32

type StoreFileType int

const (
	TypeRegular = iota
	TypeDirectory
	TypeSymlink
)

// A single chunk from a file
type FileChunk struct {
	Offset int
	Len    int
	Digest []byte
}

type ChunkedFile struct {
	Chunks []*FileChunk
	Digest []byte
}

// A file inside a store path
type StoreFile struct {
	Path       string
	Type       StoreFileType
	Size       int64
	LinkTarget string
	Executable bool
	Chunks     []*FileChunk
	Digest     []byte
}

func ChunkFile(path string) (*ChunkedFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	fileSize := info.Size()

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening %s: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	chunker, err := chunkers.NewChunker("fastcdc", file, nil)
	if err != nil {
		return nil, fmt.Errorf("error initalizing chunker: %w", err)
	}

	// Store full file hash separately from chunk hashes
	fileHasher := blake3.New()
	chunkHasher := blake3.New()

	chunks := []*FileChunk{}

	bytesChunked := 0
	offset := 0
	for {
		chunk, err := chunker.Next()
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("error reading chunk from '%s': %w", path, err)
		} else if err == io.EOF {
			break
		}

		chunkHasher.Reset()
		if _, err := chunkHasher.Write(chunk); err != nil {
			return nil, fmt.Errorf("error hashing chunk: %w", err)
		}

		if _, err := fileHasher.Write(chunk); err != nil {
			return nil, fmt.Errorf("error hashing file chunk: %w", err)
		}

		digest := make([]byte, DigestLen)
		if _, err = chunkHasher.Digest().Read(digest); err != nil {
			return nil, err
		}

		chunkLen := len(chunk)
		bytesChunked += chunkLen
		chunks = append(chunks, &FileChunk{
			Len:    chunkLen,
			Offset: offset,
			Digest: digest,
		})

		offset += chunkLen
	}

	// The CDC library doesn't issue a chunk for the final bytes.
	// Read the rest in and treat as a single chunk.
	if bytesChunked < int(fileSize) {
		_, err = file.Seek(int64(offset), 0)
		if err != nil {
			return nil, err
		}

		chunkHasher.Reset()

		buf, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}

		_, err = chunkHasher.Write(buf)
		if err != nil {
			return nil, err
		}
		_, err = fileHasher.Write(buf)
		if err != nil {
			return nil, err
		}

		digest := make([]byte, DigestLen)
		if _, err = chunkHasher.Digest().Read(digest); err != nil {
			return nil, err
		}

		chunks = append(chunks, &FileChunk{
			Len:    len(buf),
			Offset: offset,
			Digest: digest,
		})
	}

	digest := make([]byte, DigestLen)
	if _, err = fileHasher.Digest().Read(digest); err != nil {
		return nil, err
	}

	return &ChunkedFile{
		Chunks: chunks,
		Digest: digest,
	}, nil
}

func ReadStorePath(ctx context.Context, storePath string) ([]*StoreFile, error) {
	var readPath func(string) ([]*StoreFile, error)
	readPath = func(path string) ([]*StoreFile, error) {
		files := []*StoreFile{}

		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}

		mode := info.Mode()

		var fileType StoreFileType
		var linkTarget string

		var chunks []*FileChunk
		var digest []byte
		var dirStoreFiles []*StoreFile

		switch mode.Type() {
		case fs.ModeSymlink:
			fileType = TypeSymlink
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return nil, fmt.Errorf("error reading symlink target: %w", err)
			}

		case fs.ModeDir:
			fileType = TypeDirectory

			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, err
			}

			for _, entry := range entries {
				ss, err := readPath(filepath.Join(path, entry.Name()))
				if err != nil {
					return nil, err
				}
				dirStoreFiles = append(dirStoreFiles, ss...)
			}

			// Compute a digest of the whole directory
			{
				h := blake3.New()

				for _, dirStoreFile := range dirStoreFiles {
					if _, err := h.Write([]byte(dirStoreFile.Path)); err != nil {
						return nil, err
					}

					if _, err := h.Write(dirStoreFile.Digest); err != nil {
						return nil, err
					}
				}

				d := make([]byte, DigestLen)
				if _, err = h.Digest().Read(d); err != nil {
					return nil, err
				}

				digest = d
			}

		default:
			if mode.Type().IsRegular() {
				fileType = TypeRegular
				chunked, err := ChunkFile(path)
				if err != nil {
					return nil, fmt.Errorf("error reading chunks: %w", err)
				}

				chunks = chunked.Chunks
				digest = chunked.Digest
			} else {
				return nil, fmt.Errorf("error determing file type for %s", path)
			}

		}

		files = append(files, &StoreFile{
			Path:       strings.TrimPrefix(path, storePath),
			Type:       fileType,
			Size:       info.Size(),
			LinkTarget: linkTarget,
			Executable: mode&0111 != 0,
			Chunks:     chunks,
			Digest:     digest,
		})
		files = append(files, dirStoreFiles...)

		return files, nil
	}

	return readPath(storePath)
}
