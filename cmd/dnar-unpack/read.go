package main

import (
	"bufio"
	"fmt"
	"io"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/adisbladis/deltanar/chunk_store"
	"github.com/adisbladis/deltanar/dnar"
)

func readNARs(reader *bufio.Reader) ([]*dnar.NAR, error) {
	narHeader := &dnar.StreamHeader{}
	if err := protodelim.UnmarshalFrom(reader, narHeader); err != nil {
		return nil, err
	}

	// Store nars for writing out once the stream is finished
	nars := make([]*dnar.NAR, narHeader.Length)
	{
		i := 0
		for i < int(narHeader.Length) {
			nar := &dnar.NAR{}
			if err := protodelim.UnmarshalFrom(reader, nar); err != nil {
				return nil, err
			}

			nars[i] = nar
			i++
		}
	}

	return nars, nil
}

func readChunks(reader *bufio.Reader, chunkStore *chunk_store.TempChunkStore) error {
	chunkHeader := &dnar.StreamHeader{}

	if err := protodelim.UnmarshalFrom(reader, chunkHeader); err != nil {
		return err
	}

	// Write chunks to temp store
	{
		i := 0
		length := int(chunkHeader.Length)

		for i < length {
			chunk := &dnar.CAChunk{}
			if err := protodelim.UnmarshalFrom(reader, chunk); err != nil {
				return err
			}

			if err := chunkStore.WriteChunk(chunk.Data); err != nil {
				return err
			}

			i++
		}

		// Trigger memory mapping
		if err := chunkStore.Map(); err != nil {
			return err
		}
	}

	return nil
}

func readInputStoreFiles(reader *bufio.Reader) ([]string, error) {
	pathTrailer := &dnar.PathTrailer{}
	if err := protodelim.UnmarshalFrom(reader, pathTrailer); err != nil {
		return nil, err
	}

	inputStoreFiles := make([]string, len(pathTrailer.Files))
	{
		for i, storeFile := range pathTrailer.Files {
			inputStoreFiles[i] = pathTrailer.Paths[storeFile.StorePath] + storeFile.Path
		}
	}

	return inputStoreFiles, nil
}

func readEOF(reader *bufio.Reader) error {
	if _, err := reader.ReadByte(); err != io.EOF {
		return fmt.Errorf("protocol violation: expected EOF")
	}

	return nil
}
