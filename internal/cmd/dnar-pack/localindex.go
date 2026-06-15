package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/adisbladis/deltanar/internal/database"
)

// localIndex exists to check whether some content already exists on the deployment target
type localIndex struct {
	queries    *database.Queries
	localPaths []string

	// Resolved store files cache
	fileCache map[int64]*database.Storefile
}

type matchedChunk struct {
	size   int64
	offset int64
	hash   []byte

	matched     bool
	localFileID int64
	localSize   int64
	localOffset int64
}

func newLocalIndex(queries *database.Queries, localPaths []string) *localIndex {
	return &localIndex{
		queries:    queries,
		localPaths: localPaths,
		fileCache:  make(map[int64]*database.Storefile),
	}
}

func (li *localIndex) fileChunks(ctx context.Context, fileID int64) ([]matchedChunk, error) {
	rows, err := li.queries.GetStoreChunksWithLocalMatch(ctx, database.GetStoreChunksWithLocalMatchParams{
		FileID:     fileID,
		LocalPaths: li.localPaths,
	})
	if err != nil {
		return nil, err
	}

	out := make([]matchedChunk, len(rows))
	for i, row := range rows {
		out[i] = matchedChunk{
			size:        row.Size,
			offset:      row.Offset,
			hash:        row.Hash,
			matched:     row.LocalFileID != 0,
			localFileID: row.LocalFileID,
			localSize:   row.LocalSize,
			localOffset: row.LocalOffset,
		}
	}
	return out, nil
}

func (li *localIndex) fileByHash(ctx context.Context, hash []byte) (*database.Storefile, error) {
	f, err := li.queries.GetLocalFileByHash(ctx, database.GetLocalFileByHashParams{
		Hash:       hash,
		LocalPaths: li.localPaths,
	})
	switch {
	case err == nil:
		return &f, nil
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	default:
		return nil, err
	}
}

func (li *localIndex) storeFileByID(ctx context.Context, id int64) (*database.Storefile, error) {
	if f, ok := li.fileCache[id]; ok {
		return f, nil
	}
	f, err := li.queries.GetStoreFileByID(ctx, id)
	if err != nil {
		return nil, err
	}
	li.fileCache[id] = &f
	return &f, nil
}
