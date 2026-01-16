package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adisbladis/deltanar/dnar"
	"github.com/nix-community/go-nix/pkg/storepath"
)

// Manages a local binary cache directory
type localBinaryCache struct {
	dir string
}

func newLocalBinaryCache(rootDir string) *localBinaryCache {
	return &localBinaryCache{
		dir: rootDir,
	}
}

// Create the skeleton of a binary cache
func (bc *localBinaryCache) Create() error {
	err := os.MkdirAll(filepath.Join(bc.dir, "nar"), 0750)
	if err != nil {
		return err
	}

	cacheInfoPath := filepath.Join(bc.dir, "nix-cache-info")

	if _, err := os.Stat(cacheInfoPath); os.IsNotExist(err) {
		err = os.WriteFile(cacheInfoPath, fmt.Appendf(nil, "StoreDir: %s\n", storepath.StoreDir), 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

// Create a single NAR file inside the binary cache, returning it's writable file descriptor
func (bc *localBinaryCache) CreateNAR(storePath string) (*os.File, error) {
	base := filepath.Base(storePath)
	narPath := filepath.Join(bc.dir, "nar", base+".nar")
	return os.Create(narPath)
}

// Write narinfo file for NAR
func (bc *localBinaryCache) WriteNARInfo(msgNar *dnar.NAR) error {
	storePath := msgNar.Path

	base := filepath.Base(storePath)
	storeHash := base[:32]
	narInfoPath := filepath.Join(bc.dir, storeHash+".narinfo")
	narPath := filepath.Join("nar", base+".nar")

	outf, err := os.Create(narInfoPath)
	if err != nil {
		return err
	}

	if _, err = fmt.Fprintf(outf, "StorePath: %s\n", storePath); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(outf, "URL: %s\n", narPath); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(outf, "Compression: none\n"); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(outf, "NarHash: %s\n", msgNar.NarHash); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(outf, "NarSize: %d\n", msgNar.NarSize); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(outf, "References: %s\n", strings.Join(msgNar.References, " ")); err != nil {
		return err
	}

	return nil
}
