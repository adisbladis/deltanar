package main

import (
	"reflect"
	"testing"
)

// Sample `nix path-info --json hello-2.12.3` output from the two implementations.
const nixPathInfoSample = `{
  "/nix/store/zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3": {
    "ca": null,
    "deriver": "/nix/store/67mdzby3g0maqqp93xj03rc99nnrpdp9-hello-2.12.3.drv",
    "narHash": "sha256-vFV572J30ggnxdytFswEAwEVHoR3dIqbh6azYLI2lW8=",
    "narSize": 279624,
    "references": [
      "/nix/store/57iz36553175g3178pvxjij8z5rcsd4n-glibc-2.42-61",
      "/nix/store/zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3"
    ],
    "registrationTime": 1780542434,
    "signatures": ["cache.nixos.org-1:DwOHEUMyxq4aUrwzcZJrPPqqlImdA7042VJ+HWOnyjZeBMaKkSQxFS2vArnR7Okej9R8tRzEP9dgTEglZQN8Aw=="],
    "storeDir": "/nix/store",
    "ultimate": false,
    "version": 1
  }
}`

const lixPathInfoSample = `[
  {
    "deriver": "/nix/store/67mdzby3g0maqqp93xj03rc99nnrpdp9-hello-2.12.3.drv",
    "narHash": "sha256-vFV572J30ggnxdytFswEAwEVHoR3dIqbh6azYLI2lW8=",
    "narSize": 279624,
    "path": "/nix/store/zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3",
    "references": [
      "/nix/store/57iz36553175g3178pvxjij8z5rcsd4n-glibc-2.42-61",
      "/nix/store/zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3"
    ],
    "registrationTime": 1780542434,
    "signatures": ["cache.nixos.org-1:DwOHEUMyxq4aUrwzcZJrPPqqlImdA7042VJ+HWOnyjZeBMaKkSQxFS2vArnR7Okej9R8tRzEP9dgTEglZQN8Aw=="],
    "valid": true
  }
]`

// Nix --json-format 2: entries are keyed by base name under "info", references
// are base names, and the store dir is carried separately.
const v2PathInfoSample = `{
  "info": {
    "zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3": {
      "ca": null,
      "deriver": "67mdzby3g0maqqp93xj03rc99nnrpdp9-hello-2.12.3.drv",
      "narHash": "sha256-vFV572J30ggnxdytFswEAwEVHoR3dIqbh6azYLI2lW8=",
      "narSize": 279624,
      "references": [
        "57iz36553175g3178pvxjij8z5rcsd4n-glibc-2.42-61",
        "zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3"
      ],
      "registrationTime": 1780542434,
      "signatures": ["cache.nixos.org-1:DwOHEUMyxq4aUrwzcZJrPPqqlImdA7042VJ+HWOnyjZeBMaKkSQxFS2vArnR7Okej9R8tRzEP9dgTEglZQN8Aw=="],
      "ultimate": false,
      "version": 2
    }
  },
  "storeDir": "/nix/store",
  "version": 2
}`

func TestParsePathInfosNixAndLix(t *testing.T) {
	const path = "/nix/store/zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3"
	wantRefs := []string{
		"57iz36553175g3178pvxjij8z5rcsd4n-glibc-2.42-61",
		"zi2bj2hlavv8q743li2s9diqbcpmrf9b-hello-2.12.3",
	}

	cases := []struct {
		name   string
		sample string
		format pathInfoFormat
	}{
		{"nix-object", nixPathInfoSample, formatNixObject},
		{"nix-v2", v2PathInfoSample, formatNixV2},
		{"lix-array", lixPathInfoSample, formatLixArray},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePathInfos([]byte(tc.sample), tc.format)
			if err != nil {
				t.Fatalf("parsePathInfos: %v", err)
			}
			pi, ok := got[path]
			if !ok {
				t.Fatalf("store path %q not found in %v", path, got)
			}
			if pi.NarHash != "sha256-vFV572J30ggnxdytFswEAwEVHoR3dIqbh6azYLI2lW8=" {
				t.Errorf("narHash = %q", pi.NarHash)
			}
			if pi.NarSize != 279624 {
				t.Errorf("narSize = %d, want 279624", pi.NarSize)
			}
			if !reflect.DeepEqual(pi.References, wantRefs) {
				t.Errorf("references = %v, want %v", pi.References, wantRefs)
			}
		})
	}
}

func TestParsePathInfosEmpty(t *testing.T) {
	cases := []struct {
		name   string
		sample string
		format pathInfoFormat
	}{
		{"nix-object", "{}", formatNixObject},
		{"nix-v2", `{"info":{},"storeDir":"/nix/store"}`, formatNixV2},
		{"lix-array", "[]", formatLixArray},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePathInfos([]byte(tc.sample), tc.format)
			if err != nil {
				t.Fatalf("parsePathInfos: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("expected empty result, got %v", got)
			}
		})
	}
}
