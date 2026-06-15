package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
)

// A subset of fields returned by `nix path-info` that are required for exporting into a binary cache
type nixPathInfo struct {
	NarHash    string   `json:"narHash"`
	NarSize    uint64   `json:"narSize"`
	References []string `json:"references"`
}

func getPathInfos(storePaths []string) (map[string]*nixPathInfo, error) {
	if len(storePaths) == 0 {
		return map[string]*nixPathInfo{}, nil
	}

	args := append([]string{"--extra-experimental-features", "nix-command", "path-info", "--json"}, storePaths...)
	out, err := exec.Command("nix", args...).Output()
	if err != nil {
		return nil, err
	}

	var pathinfos map[string]*nixPathInfo
	if err := json.Unmarshal(out, &pathinfos); err != nil {
		return nil, err
	}

	for _, pathInfo := range pathinfos {
		for i, ref := range pathInfo.References {
			pathInfo.References[i] = filepath.Base(ref)
		}
	}

	return pathinfos, nil
}
