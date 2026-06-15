package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
)

// A subset of fields returned by `nix path-info` that are required for exporting into a binary cache
type nixPathInfo struct {
	Path       string   `json:"path"`
	NarHash    string   `json:"narHash"`
	NarSize    uint64   `json:"narSize"`
	References []string `json:"references"`
}

// Probe the Nix implementation to figure out if we're using cppnix or lix..
// This probing has to be done because Nix & Lix has incompatible path-info output, and cppnix has additional flags not supported by Lix.
//
// What an awesome ecosystem...
type pathInfoFormat int

const (
	formatNixObject pathInfoFormat = iota // cppnix json format v1
	formatNixV2 // cppnix json format v2
	formatLixArray // lix json format
)

type pathInfoMode struct {
	flags  []string // path-info flags placed before the store paths
	format pathInfoFormat
}

var (
	pathInfoModeOnce sync.Once
	pathInfoModeVal  pathInfoMode
)

func nixPathInfoMode() pathInfoMode {
	pathInfoModeOnce.Do(func() { pathInfoModeVal = probePathInfoMode() })
	return pathInfoModeVal
}

func probePathInfoMode() pathInfoMode {
	if version, err := exec.Command("nix", "--version").Output(); err == nil && bytes.Contains(version, []byte("Lix")) {
		return pathInfoMode{flags: []string{"--json"}, format: formatLixArray}
	}

	help, err := exec.Command("nix", "--extra-experimental-features", "nix-command", "path-info", "--help").Output()
	if err == nil && bytes.Contains(help, []byte("--json-format")) {
		return pathInfoMode{flags: []string{"--json", "--json-format", "2"}, format: formatNixV2}
	}
	return pathInfoMode{flags: []string{"--json"}, format: formatNixObject}
}

func getPathInfos(storePaths []string) (map[string]*nixPathInfo, error) {
	if len(storePaths) == 0 {
		return map[string]*nixPathInfo{}, nil
	}

	mode := nixPathInfoMode()
	args := append([]string{"--extra-experimental-features", "nix-command", "path-info"}, mode.flags...)
	args = append(args, storePaths...)

	out, err := exec.Command("nix", args...).Output()
	if err != nil {
		return nil, err
	}

	return parsePathInfos(out, mode.format)
}

func parsePathInfos(out []byte, format pathInfoFormat) (map[string]*nixPathInfo, error) {
	pathInfos := make(map[string]*nixPathInfo)

	switch format {
	case formatLixArray:
		var arr []*nixPathInfo
		if err := json.Unmarshal(out, &arr); err != nil {
			return nil, err
		}
		for _, pathInfo := range arr {
			if pathInfo.Path == "" {
				return nil, fmt.Errorf("path-info entry without a path field")
			}
			pathInfos[pathInfo.Path] = pathInfo
		}

	case formatNixV2:
		var doc struct {
			Info     map[string]*nixPathInfo `json:"info"`
			StoreDir string                  `json:"storeDir"`
		}
		if err := json.Unmarshal(out, &doc); err != nil {
			return nil, err
		}
		for name, pathInfo := range doc.Info {
			// Version 2 keys entries by store-path base name.
			pathInfos[doc.StoreDir+"/"+name] = pathInfo
		}

	default: // formatNixObject
		if err := json.Unmarshal(out, &pathInfos); err != nil {
			return nil, err
		}
	}

	// References are stored as bare basenames on the wire (a no-op for the
	// formats that already use base names).
	for _, pathInfo := range pathInfos {
		for i, ref := range pathInfo.References {
			pathInfo.References[i] = filepath.Base(ref)
		}
	}

	return pathInfos, nil
}
