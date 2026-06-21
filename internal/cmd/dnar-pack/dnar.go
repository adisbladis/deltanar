package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/adisbladis/deltanar/internal/database"
	"github.com/adisbladis/deltanar/internal/delta"
	"github.com/adisbladis/deltanar/internal/dnar"
	"github.com/adisbladis/deltanar/internal/store"
)

func writeDNAR(ctx context.Context, writer io.Writer, queries *database.Queries, storePaths []string, localStorePaths []string) error {
	sendStreamHeader := func(length int) error {
		_, err := protodelim.MarshalTo(writer, &dnar.StreamHeader{
			Length: uint64(length),
		})

		return err
	}

	// Local content index
	li := newLocalIndex(queries, localStorePaths)

	// Store paths to send over the wire in header
	var inputStorePathIDs []int64
	addInputStorePathID := func(storeID int64) int {
		i := slices.Index(inputStorePathIDs, storeID)
		if i > -1 {
			return i
		}

		i = len(inputStorePathIDs)
		inputStorePathIDs = append(inputStorePathIDs, storeID)

		return i
	}

	// Store files to send over the wire in header
	var inputStoreFiles []*database.Storefile
	inputStoreFilesMap := make(map[int64]uint64) // Keep a map for quick lookup of already indexed store files
	addInputStoreFile := func(file *database.Storefile) uint64 {
		other, ok := inputStoreFilesMap[file.ID]
		if ok {
			return other
		}

		addInputStorePathID(file.StorePathID)

		i := len(inputStoreFiles)
		inputStoreFiles = append(inputStoreFiles, file)
		inputStoreFilesMap[file.ID] = uint64(i)

		return uint64(i)
	}

	// Bulk payloads (file contents), as either:
	// - Verbatim file-backed chunk (as CA data or existing file)
	// - Computed delta
	type bulkPayload struct {
		chunk *database.Chunk
		data  []byte
	}
	var chunkDeps []bulkPayload
	chunkDepsMap := make(map[string]uint64) // dedup of file-backed chunks by digest
	addChunkDep := func(chunk *database.Chunk) uint64 {
		other, ok := chunkDepsMap[string(chunk.Hash)]
		if ok {
			return other
		}

		i := len(chunkDeps)
		chunkDeps = append(chunkDeps, bulkPayload{chunk: chunk})
		chunkDepsMap[string(chunk.Hash)] = uint64(i)

		return uint64(i)
	}
	addDeltaPayload := func(data []byte) uint64 {
		i := len(chunkDeps)
		chunkDeps = append(chunkDeps, bulkPayload{data: data})
		return uint64(i)
	}

	// FDs for delta computation
	deltaFDs := make(map[string]*os.File)
	defer func() {
		for _, fd := range deltaFDs {
			_ = fd.Close()
		}
	}()
	getFD := func(path string) (*os.File, error) {
		if fd, ok := deltaFDs[path]; ok {
			return fd, nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		deltaFDs[path] = f
		return f, nil
	}

	// Cache of store path strings for resolving file paths & picking delta base chunks.
	storePathStrByID := make(map[int64]string)
	getStorePathStr := func(id int64) (string, error) {
		if s, ok := storePathStrByID[id]; ok {
			return s, nil
		}
		sp, err := queries.GetStorePathByID(ctx, id)
		if err != nil {
			return "", err
		}
		storePathStrByID[id] = sp.Path
		return sp.Path, nil
	}
	storeFileAbsPath := func(storePath, filePath string) string {
		if filePath == "/" {
			return storePath
		}
		return storePath + filePath
	}

	// Match index over the current reference file's contents, reused for every
	// missing chunk (see refIndexCache).
	refCache := newRefIndexCache()
	defer refCache.close()
	getRefIndex := func(file *database.Storefile) (*delta.Index, error) {
		storePath, err := getStorePathStr(file.StorePathID)
		if err != nil {
			return nil, err
		}
		return refCache.get(storeFileAbsPath(storePath, file.Path), file.ID)
	}

	// Fetch narinfo metadata for every store path up front in one nix
	// invocation, instead of spawning nix once per path inside the loop.
	pathInfos, err := getPathInfos(storePaths)
	if err != nil {
		return err
	}

	// Send NAR stream header
	if err := sendStreamHeader(len(storePaths)); err != nil {
		return err
	}

	// Iterate over new store paths while tracking inputs
	for _, storePath := range storePaths {
		storeFiles, err := queries.GetStoreFiles(ctx, storePath)
		if err != nil {
			return err
		}

		pathInfo, ok := pathInfos[storePath]
		if !ok {
			return fmt.Errorf("store path '%s' not found in nix path-info output", storePath)
		}

		nar := &dnar.NAR{
			Path:       storePath,
			Files:      []*dnar.NarFile{},
			NarHash:    pathInfo.NarHash,
			NarSize:    pathInfo.NarSize,
			References: pathInfo.References,
		}

		// If a directory was matched in-full we can skip writing the individual files
		//  from it to the delta.
		//
		// Keep a reference of the last directory that is already matched in-full so the files from it can be skipped.
		recursiveDir := ""

	STOREFILE_LOOP:
		for _, storeFile := range storeFiles {
			if recursiveDir != "" && strings.HasPrefix(storeFile.Path, recursiveDir) {
				continue STOREFILE_LOOP
			}

			file := &dnar.NarFile{
				Path: storeFile.Path,
			}

			nar.Files = append(nar.Files, file)

			switch storeFile.Type {
			case store.TypeRegular:
				meta := &dnar.NarFile_RegularFile{
					Size:       uint64(storeFile.Size),
					Executable: storeFile.Executable,
				}
				file.FileType = &dnar.NarFile_Regular{
					Regular: meta,
				}

				// Check if target host already has file by hash
				existingFile, err := li.fileByHash(ctx, storeFile.Hash)
				if err != nil {
					return err
				}
				if existingFile != nil { // WriteOp on the whole file byte range
					chunkDescriptor := &dnar.NarFile_ChunkDescriptor{
						ChunkType: &dnar.NarFile_ChunkDescriptor_Fd{
							Fd: &dnar.NarFile_ChunkDescriptor_FDChunk{
								Index:  addInputStoreFile(existingFile),
								Size:   uint64(existingFile.Size),
								Offset: 0,
								Digest: existingFile.Hash,
							},
						},
					}

					meta.Chunks = []*dnar.NarFile_ChunkDescriptor{chunkDescriptor}
				} else { // Write file chunk by chunk
					chunks, err := li.fileChunks(ctx, storeFile.ID)
					if err != nil {
						return err
					}

					meta.Chunks = make([]*dnar.NarFile_ChunkDescriptor, len(chunks))

					// Resolve the most "popular" file, referenced by the most chunks & use
					// that as the reference for computing delta.
					refFileVotes := make(map[int64]int)
					for _, m := range chunks {
						if m.matched {
							refFileVotes[m.localFileID]++
						}
					}

					// Use the most popular file for computing delta.
					var (
						refStoreFile *database.Storefile
						refFileIndex *delta.Index
					)
					if len(refFileVotes) > 0 {
						var refFileID int64
						bestVotes := -1
						for fileID, votes := range refFileVotes {
							if votes > bestVotes {
								bestVotes, refFileID = votes, fileID
							}
						}

						sizeCap := storeFile.Size * refSizeFactor
						if sizeCap < refSizeFloor {
							sizeCap = refSizeFloor
						}
						if f, err := li.storeFileByID(ctx, refFileID); err == nil && f.Size > 0 && f.Size <= sizeCap {
							if idx, err := getRefIndex(f); err == nil {
								refStoreFile, refFileIndex = f, idx
							}
						}
					}

					fileAbsPath := storeFileAbsPath(storePath, storeFile.Path)

					// Compute the delta for each missing chunk in parallel.
					deltaResults := make([][]byte, len(chunks))
					if refFileIndex != nil {
						fp, err := getFD(fileAbsPath)
						if err != nil {
							return err
						}

						eg := errgroup.Group{}
						eg.SetLimit(runtime.NumCPU())
						for i, m := range chunks {
							if m.matched || m.size == 0 {
								continue
							}
							eg.Go(func() error {
								chunkData := make([]byte, m.size)
								if _, err := fp.ReadAt(chunkData, m.offset); err != nil {
									return err
								}
								d, err := refFileIndex.Diff(chunkData)
								if err != nil {
									return err
								}
								if len(d) < len(chunkData) {
									deltaResults[i] = d // delta pays off
								}
								return nil
							})
						}
						if err := eg.Wait(); err != nil {
							return err
						}
					}

					// Assemble chunk descriptors in order.
					for i, m := range chunks {
						msgChunk := &dnar.NarFile_ChunkDescriptor{}
						meta.Chunks[i] = msgChunk

						switch {
						case m.matched: // WriteOp on the chunk range
							localStoreFile, err := li.storeFileByID(ctx, m.localFileID)
							if err != nil {
								return err
							}

							msgChunk.ChunkType = &dnar.NarFile_ChunkDescriptor_Fd{
								Fd: &dnar.NarFile_ChunkDescriptor_FDChunk{
									Index:  addInputStoreFile(localStoreFile),
									Size:   uint64(m.localSize),
									Offset: uint64(m.localOffset),
									Digest: m.hash,
								},
							}

						case deltaResults[i] != nil: // WriteOp reconstructed from a delta
							msgChunk.ChunkType = &dnar.NarFile_ChunkDescriptor_Delta{
								Delta: &dnar.NarFile_ChunkDescriptor_DeltaChunk{
									Index:      addInputStoreFile(refStoreFile),
									Size:       uint64(refStoreFile.Size),
									Offset:     0,
									Digest:     refStoreFile.Hash,
									DeltaIndex: addDeltaPayload(deltaResults[i]),
									ResultSize: uint64(m.size),
								},
							}

						default: // WriteOp on CA chunk
							msgChunk.ChunkType = &dnar.NarFile_ChunkDescriptor_Ca{
								Ca: &dnar.NarFile_ChunkDescriptor_CAChunk{
									Index: addChunkDep(&database.Chunk{
										FileID: storeFile.ID,
										Hash:   m.hash,
										Size:   m.size,
										Offset: m.offset,
									}),
								},
							}
						}
					}
				}
			case store.TypeDirectory: // WriteDirOp
				var from int64 = -1

				// Check for exact dir hash match
				existingDir, err := li.fileByHash(ctx, storeFile.Hash)
				if err != nil {
					return err
				}
				if existingDir != nil {
					recursiveDir = storeFile.Path
					from = int64(addInputStoreFile(existingDir))
				}

				file.FileType = &dnar.NarFile_Directory{
					Directory: &dnar.NarFile_DirectoryFile{
						From: from,
					},
				}

			case store.TypeSymlink: // WriteSymlinkOp
				file.FileType = &dnar.NarFile_Symlink{
					Symlink: &dnar.NarFile_SymlinkFile{
						Target: storeFile.LinkTarget.String,
					},
				}
			default:
				panic("Unknown file type") // Invalid state
			}
		}

		if _, err = protodelim.MarshalTo(writer, nar); err != nil {
			return err
		}
	}

	// Send NAR stream header
	if err := sendStreamHeader(len(chunkDeps)); err != nil {
		return err
	}

	// Write the chunk stream (verbatim CA chunks + delta payloads)
	// Note: Wrapped in a func so the fds map can be cleaned up with defer as early as possible
	err = func() error {
		fds := make(map[int64]*os.File)

		for _, payload := range chunkDeps {
			// Precomputed payload (delta)
			if payload.chunk == nil {
				if _, err := protodelim.MarshalTo(writer, &dnar.CAChunk{Data: payload.data}); err != nil {
					return err
				}
				continue
			}

			chunk := payload.chunk
			fd, ok := fds[chunk.FileID]
			if !ok {
				dbFile, err := queries.GetStoreFileByID(ctx, chunk.FileID)
				if err != nil {
					return err
				}

				dbStorePath, err := queries.GetStorePathByID(ctx, dbFile.StorePathID)
				if err != nil {
					return err
				}

				filePath := dbStorePath.Path
				if dbFile.Path != "/" {
					filePath += dbFile.Path
				}

				fd, err = os.Open(filePath)
				if err != nil {
					return err
				}
				defer func() {
					if err := fd.Close(); err != nil {
						panic(err)
					}
				}()

				fds[chunk.FileID] = fd
			}

			data := make([]byte, chunk.Size)
			if _, err := fd.Seek(chunk.Offset, 0); err != nil {
				return err
			}
			if _, err := fd.Read(data); err != nil {
				return err
			}

			caChunk := &dnar.CAChunk{
				Data: data,
			}

			if _, err := protodelim.MarshalTo(writer, caChunk); err != nil {
				return err
			}
		}

		return nil
	}()
	if err != nil {
		return err
	}

	// Write Path trailer
	{
		trailer := &dnar.PathTrailer{
			Paths: make([]string, len(inputStorePathIDs)),
			Files: make([]*dnar.FileDescriptor, len(inputStoreFiles)),
		}

		// Write out store paths
		for i, dbStorePathID := range inputStorePathIDs {
			dbStorePath, err := queries.GetStorePathByID(ctx, dbStorePathID)
			if err != nil {
				return err
			}

			trailer.Paths[i] = dbStorePath.Path
		}

		// Write out store files
		for i, inputStoreFile := range inputStoreFiles {
			path := inputStoreFile.Path
			if path == "/" {
				path = ""
			}

			trailer.Files[i] = &dnar.FileDescriptor{
				StorePath: uint32(slices.Index(inputStorePathIDs, inputStoreFile.StorePathID)),
				Path:      path,
			}
		}

		if _, err := protodelim.MarshalTo(writer, trailer); err != nil {
			return err
		}
	}

	return nil
}
