package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/nix-community/go-nix/pkg/storepath"

	"github.com/adisbladis/deltanar/internal/chunk_store"
)

// Magic bytes for Nix --export format
const exportMagic uint64 = 0x4558494e

func writeExportTrailer(w io.Writer, storePath string, refs []string, deriver string) error {
	if err := writeUint64LE(w, exportMagic); err != nil {
		return err
	}
	if err := writeNixString(w, storePath); err != nil {
		return err
	}
	if err := writeUint64LE(w, uint64(len(refs))); err != nil {
		return err
	}
	for _, ref := range refs {
		if err := writeNixString(w, ref); err != nil {
			return err
		}
	}
	if err := writeNixString(w, deriver); err != nil {
		return err
	}
	return writeUint64LE(w, 0) // Registry flag is always 0
}

func nixStoreExportMain(inputFile string, outputFile string) {
	ioReader, err := openInput(inputFile)
	if err != nil {
		panic(err)
	}
	defer ioReader.Close()

	ioWriter, err := openOutput(outputFile)
	if err != nil {
		panic(err)
	}
	defer ioWriter.Close()

	bw := bufio.NewWriter(ioWriter)
	defer func() {
		if err := bw.Flush(); err != nil {
			panic(err)
		}
	}()

	reader := bufio.NewReader(ioReader)

	nars, err := readNARs(reader)
	if err != nil {
		panic(err)
	}

	tempChunkStore, err := chunk_store.OpenTempChunkStore()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = tempChunkStore.Close()
	}()

	if err = readChunks(reader, tempChunkStore); err != nil {
		panic(err)
	}

	inputStoreFiles, err := readInputStoreFiles(reader)
	if err != nil {
		panic(err)
	}

	if err = readEOF(reader); err != nil {
		panic(err)
	}

	for _, msgNar := range nars {
		fmt.Fprintln(os.Stderr, msgNar.Path)

		// Each store path is preceded by uint64(1), named count in cppnix
		// I guess this was originally supposed to be a "this many store paths to follow" counter
		// but it's always a 1 if any paths are to follow, and a 0 on end of stream
		if err = writeUint64LE(bw, 1); err != nil {
			panic(err)
		}

		err := writeNAR(bw, msgNar, tempChunkStore, inputStoreFiles)
		if err != nil {
			panic(err)
		}

		// The DNAR only stores store path reference basenames.
		// Prepend store path to every ref.
		refs := make([]string, len(msgNar.References))
		for i, ref := range msgNar.References {
			refs[i] = storepath.StoreDir + "/" + ref
		}

		// Note: Hard coded empty deriver field.
		// It's not required and would just take up extra wire space for no reason.
		deriver := ""
		if err = writeExportTrailer(bw, msgNar.Path, refs, deriver); err != nil {
			panic(err)
		}
	}

	// End of stream
	if err = writeUint64LE(bw, 0); err != nil {
		panic(err)
	}
}
