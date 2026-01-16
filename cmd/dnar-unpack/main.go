package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/adisbladis/deltanar/dnar"
	"github.com/nix-community/go-nix/pkg/nar"
)

func binaryCacheMain(inputFile string, binaryCacheDir string) {
	var ioReader io.Reader
	if inputFile == "-" {
		ioReader = os.Stdin
	} else {
		fp, err := os.Open(inputFile)
		if err != nil {
			panic(err)
		}
		ioReader = fp
	}

	reader := bufio.NewReader(ioReader)

	narHeader := &dnar.StreamHeader{}
	if err := protodelim.UnmarshalFrom(reader, narHeader); err != nil {
		panic(err)
	}

	// Store nars for writing out once the stream is finished
	nars := make([]*dnar.NAR, narHeader.Length)
	{
		i := 0
		for i < int(narHeader.Length) {
			nar := &dnar.NAR{}
			if err := protodelim.UnmarshalFrom(reader, nar); err != nil {
				panic(err)
			}

			nars[i] = nar
			i++
		}
	}

	chunkHeader := &dnar.StreamHeader{}
	if err := protodelim.UnmarshalFrom(reader, chunkHeader); err != nil {
		panic(err)
	}

	// Create a temporary chunk store
	// This is because an input file may be larger than RAM
	// and needs to be written to persistent storage
	tempChunkStore, err := OpenTempChunkStore()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = tempChunkStore.Close()
	}()

	// Write chunks to temp store
	{
		i := 0
		length := int(chunkHeader.Length)

		for i < length {
			chunk := &dnar.CAChunk{}
			if err = protodelim.UnmarshalFrom(reader, chunk); err != nil {
				panic(err)
			}

			if err = tempChunkStore.WriteChunk(chunk.Data); err != nil {
				panic(err)
			}

			i++
		}

		// Trigger memory mapping
		if err = tempChunkStore.Map(); err != nil {
			panic(err)
		}
	}

	pathTrailer := &dnar.PathTrailer{}
	if err = protodelim.UnmarshalFrom(reader, pathTrailer); err != nil {
		panic(err)
	}

	// Assert EOF
	if _, err = reader.ReadByte(); err != io.EOF {
		panic(fmt.Errorf("protocol violation: expected EOF"))
	}

	inputStoreFiles := make([]string, len(pathTrailer.Files))
	{
		for i, storeFile := range pathTrailer.Files {
			inputStoreFiles[i] = pathTrailer.Paths[storeFile.StorePath] + storeFile.Path
		}
	}

	binaryCache := newLocalBinaryCache(binaryCacheDir)
	if err = binaryCache.Create(); err != nil {
		panic(err)
	}

	writeNAR := func(msgNar *dnar.NAR) error {
		fmt.Println(msgNar.Path)

		outf, err := binaryCache.CreateNAR(msgNar.Path)
		if err != nil {
			return err
		}

		nw, err := nar.NewWriter(outf)
		if err != nil {
			return err
		}
		defer func() {
			_ = nw.Close()
		}()

		for _, file := range msgNar.Files {
			switch file.FileType.(type) {
			case *dnar.NarFile_Regular:
				regularMeta := file.GetRegular()

				if err = nw.WriteHeader(&nar.Header{
					Path:       file.Path,
					Type:       nar.TypeRegular,
					Size:       int64(regularMeta.Size),
					Executable: regularMeta.Executable,
				}); err != nil {
					return err
				}

				// Store open files in a map for reuse
				inputFDs := make(map[string]*os.File)

				for _, chunk := range regularMeta.Chunks {
					switch chunk.ChunkType.(type) {
					case *dnar.NarFile_ChunkDescriptor_Ca:
						caMeta := chunk.GetCa()

						chunk, err := tempChunkStore.ReadChunk(int(caMeta.Index))
						if err != nil {
							return err
						}

						if _, err = nw.Write(chunk); err != nil {
							return err
						}

					case *dnar.NarFile_ChunkDescriptor_Fd:
						fdMeta := chunk.GetFd()
						inputFile := inputStoreFiles[fdMeta.Index]

						fp, ok := inputFDs[inputFile]
						if !ok {
							fp, err = os.Open(inputFile)
							if err != nil {
								return err
							}
							defer func() {
								_ = fp.Close()
							}()
							inputFDs[inputFile] = fp
						}

						if _, err = fp.Seek(int64(fdMeta.Offset), 0); err != nil {
							return err
						}

						buf := make([]byte, fdMeta.Size)
						if _, err = fp.Read(buf); err != nil {
							return err
						}

						if _, err = nw.Write(buf); err != nil {
							return err
						}
					case *dnar.NarFile_ChunkDescriptor_Inline:
						panic("Not implemented: Inline chunks")
					}
				}

			case *dnar.NarFile_Directory:
				if err = nw.WriteHeader(&nar.Header{
					Path: file.Path,
					Type: nar.TypeDirectory,
				}); err != nil {
					return err
				}

			case *dnar.NarFile_Symlink:
				symlinkMeta := file.GetSymlink()
				if err = nw.WriteHeader(&nar.Header{
					Path:       file.Path,
					Type:       nar.TypeSymlink,
					LinkTarget: symlinkMeta.Target,
					Size:       0,
				}); err != nil {
					return err
				}
			}
		}

		return binaryCache.WriteNARInfo(msgNar)
	}

	for _, msgNar := range nars {
		err := writeNAR(msgNar)
		if err != nil {
			panic(err)
		}
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  binary-cache    Unpack DNAR into a local binary cache\n")
		fmt.Fprintf(os.Stderr, "\nRun '%s <command> -help' for command-specific help.\n", os.Args[0])
	}

	bcCmd := flag.NewFlagSet("binary-cache", flag.ExitOnError)
	bcCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: binary-cache [options]\n\n")
		fmt.Fprintf(os.Stderr, "Unpack DNAR into a local binary cache.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		bcCmd.PrintDefaults()
	}

	var bcDir string
	bcCmd.StringVar(&bcDir, "cache", "", "binary cache directory")

	var inputFile string
	bcCmd.StringVar(&inputFile, "input", "delta.dnar", "input dnar file")

	flag.Parse()

	switch os.Args[1] {
	case "binary-cache":
		bcCmd.Parse(os.Args[2:])
		binaryCacheMain(inputFile, bcDir)
	default:
		panic("Invalid subcommand")
	}
}
