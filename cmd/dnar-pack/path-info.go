package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

// A subset of fields returned by `nix path-info` that are required for exporting into a binary cache
type nixPathInfo struct {
	NarHash    string   `json:"narHash"`
	NarSize    uint64   `json:"narSize"`
	References []string `json:"references"`
}

func getPathInfo(storePath string) (*nixPathInfo, error) {
	cmd := exec.Command("nix", "--extra-experimental-features", "nix-command", "path-info", "--json", storePath)

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var pathinfos map[string]*nixPathInfo
	if err := json.Unmarshal(out, &pathinfos); err != nil {
		return nil, err
	}

	pathInfo, ok := pathinfos[storePath]
	if !ok {
		return nil, fmt.Errorf("store path '%s' not found in nix path-info output", storePath)
	}

	for i, ref := range pathInfo.References {
		pathInfo.References[i] = filepath.Base(ref)
	}

	return pathInfo, nil
}
