package store

import (
	"os/exec"
	"strings"
)

func QueryRequisites(storePath ...string) ([]string, error) {
	args := append([]string{"--query", "--requisites"}, storePath...)
	cmd := exec.Command("nix-store", args...)

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(out)), nil
}
