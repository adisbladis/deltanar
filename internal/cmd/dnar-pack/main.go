package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"slices"

	"github.com/adisbladis/deltanar/internal/database"
	"github.com/adisbladis/deltanar/internal/gcroots"
	"github.com/adisbladis/deltanar/internal/store"
)

func main() {
	ctx := context.Background()

    f, _ := os.Create("cpu.prof")
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()

	var dbPath string
	flag.StringVar(&dbPath, "db", "dnar.sqlite3", "path to sqlite database (created on first use)")

	var gcRootsDir string
	flag.StringVar(&gcRootsDir, "gcroots", "gcroots", "path to gcroots directory")

	var deployHost string
	flag.StringVar(&deployHost, "host", "", "name of host to deploy to")

	var storePath string
	flag.StringVar(&storePath, "path", "", "nix store path to deploy")

	var outputFile string
	flag.StringVar(&outputFile, "out", "delta.dnar", "output dnar file")

	flag.Parse()

	if deployHost == "" {
		panic("Missing host flag")
	}
	if storePath == "" {
		panic("Missing path flag")
	}

	db, err := sql.Open(SQL_DIALECT, fmt.Sprintf("file:%s", dbPath))
	if err != nil {
		panic(err)
	}

	if err = migrateDB(db, database.SchemaFS); err != nil {
		panic(err)
	}

	// Figure out which store paths are already on host
	localStorePaths, err := gcroots.ReadDirectory(gcRootsDir, deployHost)
	if err != nil {
		panic(err)
	}

	// List store contents of new closure
	deployStorePaths, err := store.QueryRequisites(storePath)
	if err != nil {
		panic(err)
	}

	// Index paths into db
	var allStorePaths []string
	{
		// Unique list of all known store paths
		allStorePaths = append(localStorePaths, deployStorePaths...)
		slices.Sort(allStorePaths)
		allStorePaths = slices.Compact(allStorePaths)

		if err = indexPaths(ctx, db, allStorePaths); err != nil {
			panic(err)
		}
	}

	// Compute store path level diff
	var newStorePaths []string
	{
		existingStorePaths := slices.Clone(localStorePaths)
		slices.Sort(existingStorePaths)

		for _, storePath := range deployStorePaths {
			_, found := slices.BinarySearch(existingStorePaths, storePath)
			if !found {
				newStorePaths = append(newStorePaths, storePath)
			}
		}
	}

	// DB transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	queries := database.New(tx)

	// Known files on the deployment target
	var localStoreFiles []*database.Storefile
	{
		for _, localPath := range localStorePaths {
			storeFiles, err := queries.GetStoreFiles(ctx, localPath)
			if err != nil {
				panic(err)
			}

			for _, storeFile := range storeFiles {
				localStoreFiles = append(localStoreFiles, &storeFile)
			}
		}
	}

	// Known chunks on the deployment target
	var localStoreChunks []*database.Chunk
	{
		chunks, err := queries.GetStoreChunksByPaths(ctx, localStorePaths)
		if err != nil {
			panic(err)
		}
		for _, chunk := range chunks {
			localStoreChunks = append(localStoreChunks, &chunk)
		}
	}

	var writer io.Writer
	if outputFile == "-" {
		writer = os.Stdout
	} else {
		outf, err := os.Create(outputFile)
		if err != nil {
			panic(err)
		}
		writer = outf
	}

	// Write DNAR archive
	err = writeDNAR(ctx, writer, queries, newStorePaths, localStoreFiles, localStoreChunks)
	if err != nil {
		panic(err)
	}
}
