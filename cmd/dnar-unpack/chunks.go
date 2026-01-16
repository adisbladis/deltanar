package main

import (
	"fmt"
	"os"
	"syscall"
)

type chunkStoreEntry struct {
	size   int
	offset int
}

type TempChunkStore struct {
	file *os.File
	buf  []byte

	// Current offset into file while writing
	size int

	chunks []*chunkStoreEntry
}

func OpenTempChunkStore() (*TempChunkStore, error) {
	tmpfile, err := os.CreateTemp("", "dnar-chunks")
	if err != nil {
		return nil, err
	}

	return &TempChunkStore{
		file: tmpfile,
		buf:  nil,
	}, nil
}

func (c *TempChunkStore) WriteChunk(data []byte) error {
	entry := &chunkStoreEntry{
		size:   len(data),
		offset: c.size,
	}

	// Update offset for next write
	c.size = c.size + entry.size

	c.chunks = append(c.chunks, entry)

	_, err := c.file.Write(data)
	return err
}

func (c *TempChunkStore) Map() error {
	if c.size == 0 {
		c.buf = []byte{}
		return nil
	}

	buf, err := syscall.Mmap(
		int(c.file.Fd()),
		0,
		c.size,
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)

	c.buf = buf

	return err
}

func (c *TempChunkStore) ReadChunk(i int) ([]byte, error) {
	if len(c.chunks) <= i {
		return nil, fmt.Errorf("index %d out of bounds", i)
	}
	entry := c.chunks[i]
	return c.buf[entry.offset : entry.offset+entry.size], nil
}

func (c *TempChunkStore) Close() error {
	var err error

	if c.buf != nil {
		if err = syscall.Munmap(c.buf); err != nil {
			_ = c.file.Close()
			return err
		}
	}

	if c.file != nil {
		if err = c.file.Close(); err != nil {
			return err
		}
	}

	return nil
}
