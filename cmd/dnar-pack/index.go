package main

import (
	"context"
	"database/sql"
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
	_ "modernc.org/sqlite"

	"github.com/adisbladis/deltanar/database"
	"github.com/adisbladis/deltanar/store"
)

func indexPaths(ctx context.Context, db *sql.DB, paths []string) error {
	requisites, err := store.QueryRequisites(paths...)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	queries := database.New(tx)

	var newRequisities []string
	for _, path := range requisites {
		_, err := queries.GetStorePath(ctx, path)
		if err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return err
		}
		newRequisities = append(newRequisities, path)
	}

	storeFiles := make([][]*store.StoreFile, len(newRequisities))
	{
		eg := errgroup.Group{}
		eg.SetLimit(runtime.NumCPU())

		var mux sync.Mutex
		for i, path := range newRequisities {
			eg.Go(func() error {
				files, err := store.ReadStorePath(ctx, path)
				if err != nil {
					return err
				}

				mux.Lock()
				storeFiles[i] = files
				mux.Unlock()

				return nil
			})
		}

		if err = eg.Wait(); err != nil {
			return err
		}
	}

	for i, path := range newRequisities {
		storePathEntry, err := queries.CreateStorePath(ctx, path)
		if err != nil {
			return err
		}

		for _, file := range storeFiles[i] {
			filePath := file.Path
			if filePath == "" {
				filePath = "/"
			}

			fileEntry, err := queries.CreateStoreFile(ctx, database.CreateStoreFileParams{
				StorePathID: storePathEntry.ID,
				Path:        filePath,
				Size:        file.Size,
				Type:        int64(file.Type),
				LinkTarget:  sql.NullString{String: file.LinkTarget, Valid: file.Type == store.TypeSymlink},
				Executable:  file.Executable,
				Hash:        file.Digest,
			})
			if err != nil {
				return err
			}

			for _, c := range file.Chunks {
				if err = queries.InsertChunk(ctx, database.InsertChunkParams{
					FileID: fileEntry.ID,
					Hash:   c.Digest,
					Size:   int64(c.Len),
					Offset: int64(c.Offset),
				}); err != nil {
					return err
				}
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}
