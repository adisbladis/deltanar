package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nix-community/go-nix/pkg/nar"

	"github.com/adisbladis/deltanar/internal/chunk_store"
	"github.com/adisbladis/deltanar/internal/dnar"
)

func writeUint64LE(w io.Writer, n uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], n)
	_, err := w.Write(buf[:])
	return err
}

func writeNixString(w io.Writer, s string) error {
	b := []byte(s)
	if err := writeUint64LE(w, uint64(len(b))); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	// Pad to 8-byte boundary.
	if pad := (8 - len(b)%8) % 8; pad > 0 {
		var zeros [8]byte
		if _, err := w.Write(zeros[:pad]); err != nil {
			return err
		}
	}
	return nil
}

func writeNAR(
	w io.Writer,
	msgNar *dnar.NAR,
	tempChunkStore *chunk_store.TempChunkStore,
	inputStoreFiles []string,
) error {
	nw, err := nar.NewWriter(w)
	if err != nil {
		return err
	}

	// Re-use the same file-descriptor map across the whole NAR so we do not
	// open the same source file repeatedly.
	inputFDs := make(map[string]*os.File)
	defer func() {
		for _, fp := range inputFDs {
			_ = fp.Close()
		}
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

			for _, chunk := range regularMeta.Chunks {
				switch chunk.ChunkType.(type) {

				case *dnar.NarFile_ChunkDescriptor_Ca:
					caMeta := chunk.GetCa()
					chunkData, err := tempChunkStore.ReadChunk(int(caMeta.Index))
					if err != nil {
						return err
					}
					if _, err = nw.Write(chunkData); err != nil {
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
						inputFDs[inputFile] = fp
					}

					if _, err = fp.Seek(int64(fdMeta.Offset), io.SeekStart); err != nil {
						return err
					}
					chunkBuf := make([]byte, fdMeta.Size)
					if _, err = io.ReadFull(fp, chunkBuf); err != nil {
						return err
					}
					if _, err = nw.Write(chunkBuf); err != nil {
						return err
					}

				case *dnar.NarFile_ChunkDescriptor_Inline:
					inlineMeta := chunk.GetInline()
					if _, err = nw.Write(inlineMeta.Data); err != nil {
						return err
					}
				}
			}

		case *dnar.NarFile_Directory:
			dir := file.GetDirectory()

			if dir.From > -1 {
				// The entire directory already exists on the sender; copy it
				// recursively from the local Nix store.
				inputDir := inputStoreFiles[dir.From]

				if err = filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					info, err := d.Info()
					if err != nil {
						return err
					}

					mode := info.Mode()
					relPath := file.Path + strings.TrimPrefix(path, inputDir)

					switch {
					case mode&os.ModeSymlink != 0:
						target, err := os.Readlink(path)
						if err != nil {
							return fmt.Errorf("could not read symlink: %w", err)
						}
						return nw.WriteHeader(&nar.Header{
							Path:       relPath,
							Type:       nar.TypeSymlink,
							LinkTarget: target,
						})

					case mode.IsDir():
						return nw.WriteHeader(&nar.Header{
							Path: relPath,
							Type: nar.TypeDirectory,
						})

					case mode.IsRegular():
						if err = nw.WriteHeader(&nar.Header{
							Path:       relPath,
							Type:       nar.TypeRegular,
							Size:       info.Size(),
							Executable: mode&0o111 != 0,
						}); err != nil {
							return err
						}
						fp, err := os.Open(path)
						if err != nil {
							return err
						}
						defer fp.Close()
						_, err = io.Copy(nw, fp)
						return err

					default:
						return fmt.Errorf("unsupported file type at %s: %v", path, mode)
					}
				}); err != nil {
					return err
				}
			} else {
				// Directory header only; files follow as subsequent NarFile messages.
				if err = nw.WriteHeader(&nar.Header{
					Path: file.Path,
					Type: nar.TypeDirectory,
				}); err != nil {
					return err
				}
			}

		case *dnar.NarFile_Symlink:
			symlinkMeta := file.GetSymlink()
			if err = nw.WriteHeader(&nar.Header{
				Path:       file.Path,
				Type:       nar.TypeSymlink,
				LinkTarget: symlinkMeta.Target,
			}); err != nil {
				return err
			}
		}
	}

	if err = nw.Close(); err != nil {
		return err
	}

	return nil
}
