package main

import (
	"github.com/adisbladis/deltanar/internal/delta"
	"github.com/adisbladis/deltanar/internal/mmapfile"
)

// When calculating chunk delta don't load a whole reference file if the file is much smaller than the chunk.
const (
	refSizeFactor = 4
	refSizeFloor  = 16 << 20 // 16 MiB
)

// refIndexCache memory-maps the current delta reference file and indexes it.
// Only one reference is held at a time, so peak memory is bounded by the largest
// reference used; the mapping is kept off the Go heap and released when a
// different reference is loaded or on close.
type refIndexCache struct {
	fileID int64
	index  *delta.Index
	unmap  func() error
}

func newRefIndexCache() *refIndexCache {
	return &refIndexCache{fileID: -1}
}

// get returns a match index over the whole reference file at absPath, reusing
// the cached index while fileID is unchanged.
func (c *refIndexCache) get(absPath string, fileID int64) (*delta.Index, error) {
	if c.index != nil && c.fileID == fileID {
		return c.index, nil
	}
	data, unmap, err := mmapfile.Open(absPath)
	if err != nil {
		return nil, err
	}
	c.close()
	c.unmap, c.fileID, c.index = unmap, fileID, delta.NewIndex(data)
	return c.index, nil
}

func (c *refIndexCache) close() {
	if c.unmap != nil {
		_ = c.unmap()
		c.unmap = nil
	}
	c.index, c.fileID = nil, -1
}
