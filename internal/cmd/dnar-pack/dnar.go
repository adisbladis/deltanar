package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/adisbladis/deltanar/internal/database"
	"github.com/adisbladis/deltanar/internal/dnar"
	"github.com/adisbladis/deltanar/internal/store"
)

func writeDNAR(ctx context.Context, writer io.Writer, queries *database.Queries, storePaths []string, localStoreFiles []*database.Storefile, localStoreChunks []*database.Chunk) error {
	sendStreamHeader := func(length int) error {
		_, err := protodelim.MarshalTo(writer, &dnar.StreamHeader{
			Length: uint64(length),
		})

		return err
	}

	// Group local store files by hash for lookup
	localStoreFilesByDigest := make(map[string][]*database.Storefile)
	for _, storeFile := range localStoreFiles {
		digest := string(storeFile.Hash)
		localStoreFilesByDigest[digest] = append(localStoreFilesByDigest[digest], storeFile)
	}

	// Group local store chunks by hash for lookup
	localStoreChunksByDigest := make(map[string][]*database.Chunk)
	for _, chunk := range localStoreChunks {
		digest := string(chunk.Hash)
		localStoreChunksByDigest[digest] = append(localStoreChunksByDigest[digest], chunk)
	}

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
	addInputStoreFile := func(file *database.Storefile) uint64 {
		i := slices.IndexFunc(inputStoreFiles, func(other *database.Storefile) bool {
			return file.ID == other.ID
		})
		if i > -1 {
			return uint64(i)
		}

		addInputStorePathID(file.StorePathID)

		i = len(inputStoreFiles)
		inputStoreFiles = append(inputStoreFiles, file)

		return uint64(i)
	}

	// Store which chunks to send over by their digest
	var chunkDeps []*database.Chunk
	addChunkDep := func(chunk *database.Chunk) uint64 {
		i := slices.IndexFunc(chunkDeps, func(other *database.Chunk) bool {
			return bytes.Equal(chunk.Hash, other.Hash)
		})
		if i > -1 {
			return uint64(i)
		}

		i = len(chunkDeps)
		chunkDeps = append(chunkDeps, chunk)

		return uint64(i)
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

		pathInfo, err := getPathInfo(storePath)
		if err != nil {
			return err
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
		// Maintain a list of directories which are already copied in-full so the files from it can be skipped.
		recursiveDirs := []string{}

	STOREFILE_LOOP:
		for _, storeFile := range storeFiles {
			for _, recursiveDir := range recursiveDirs {
				if strings.HasPrefix(storeFile.Path, recursiveDir) {
					continue STOREFILE_LOOP
				}
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

				// TODO: Create inline data for very short writes

				// Check if target host already has file by hash
				existingFiles, ok := localStoreFilesByDigest[string(storeFile.Hash)]
				if ok { // WriteOp on the whole file byte range
					existingFile := existingFiles[0]
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
					chunks, err := queries.GetStoreChunks(ctx, storeFile.ID)
					if err != nil {
						return err
					}

					meta.Chunks = make([]*dnar.NarFile_ChunkDescriptor, len(chunks))

					for i, chunk := range chunks {
						msgChunk := &dnar.NarFile_ChunkDescriptor{}
						meta.Chunks[i] = msgChunk

						localChunks, ok := localStoreChunksByDigest[string(chunk.Hash)]
						if ok { // WriteOp on the chunk range
							localChunk := localChunks[0]
							localStoreFile := localStoreFiles[slices.IndexFunc(localStoreFiles, func(other *database.Storefile) bool {
								return localChunk.FileID == other.ID
							})]

							msgChunk.ChunkType = &dnar.NarFile_ChunkDescriptor_Fd{
								Fd: &dnar.NarFile_ChunkDescriptor_FDChunk{
									Index:  addInputStoreFile(localStoreFile),
									Size:   uint64(localChunk.Size),
									Offset: uint64(localChunk.Offset),
									Digest: chunk.Hash,
								},
							}
						} else { // WriteOp on CA chunk
							msgChunk.ChunkType = &dnar.NarFile_ChunkDescriptor_Ca{
								Ca: &dnar.NarFile_ChunkDescriptor_CAChunk{
									Index: addChunkDep(&chunk),
								},
							}
						}
					}
				}
			case store.TypeDirectory: // WriteDirOp
				var from int64 = -1

				// Check for exact dir hash match
				existingDirs, ok := localStoreFilesByDigest[string(storeFile.Hash)]
				if ok {
					recursiveDirs = append(recursiveDirs, storeFile.Path)
					existingDir := existingDirs[0]
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

	// Write CA chunks
	{
		fds := make(map[int64]*os.File)

		for _, chunk := range chunkDeps {
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
			trailer.Files[i] = &dnar.FileDescriptor{
				StorePath: uint32(slices.Index(inputStorePathIDs, inputStoreFile.StorePathID)),
				Path:      inputStoreFile.Path,
			}
		}

		if _, err := protodelim.MarshalTo(writer, trailer); err != nil {
			return err
		}
	}

	return nil
}
