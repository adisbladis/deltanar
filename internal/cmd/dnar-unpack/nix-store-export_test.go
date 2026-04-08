package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWriteUint64LE(t *testing.T) {
	var buf bytes.Buffer
	if err := writeUint64LE(&buf, 0x4558494e); err != nil {
		t.Fatal(err)
	}
	got := binary.LittleEndian.Uint64(buf.Bytes())
	if got != exportMagic {
		t.Fatalf("want 0x%x, got 0x%x", exportMagic, got)
	}
}

func TestWriteNixString(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantLen int // expected total bytes written (8 for length + padded content)
	}{
		{"empty", "", 8},          // 8-byte length field only (0 bytes content, no padding needed)
		{"three", "foo", 16},      // 8 (len) + 3 (data) + 5 (pad) = 16
		{"eight", "12345678", 16}, // 8 (len) + 8 (data, already aligned) = 16
		{"nine", "123456789", 24}, // 8 (len) + 9 (data) + 7 (pad) = 24
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeNixString(&buf, tc.input); err != nil {
				t.Fatal(err)
			}
			if buf.Len() != tc.wantLen {
				t.Fatalf("writeNixString(%q): want %d bytes, got %d", tc.input, tc.wantLen, buf.Len())
			}

			gotLen := binary.LittleEndian.Uint64(buf.Bytes()[:8])
			if gotLen != uint64(len(tc.input)) {
				t.Fatalf("length field: want %d, got %d", len(tc.input), gotLen)
			}

			if string(buf.Bytes()[8:8+len(tc.input)]) != tc.input {
				t.Fatalf("content mismatch")
			}

			for i, b := range buf.Bytes()[8+len(tc.input):] {
				if b != 0 {
					t.Fatalf("padding byte %d is 0x%x, want 0x00", i, b)
				}
			}
		})
	}
}

func TestWriteExportTrailer(t *testing.T) {
	const storePath = "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa0-hello-2.12"
	refs := []string{
		"/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1-glibc-2.38",
	}

	var buf bytes.Buffer
	if err := writeExportTrailer(&buf, storePath, refs, ""); err != nil {
		t.Fatal(err)
	}

	r := bytes.NewReader(buf.Bytes())

	readU64 := func() uint64 {
		var b [8]byte
		if _, err := r.Read(b[:]); err != nil {
			t.Fatal(err)
		}
		return binary.LittleEndian.Uint64(b[:])
	}

	if got := readU64(); got != exportMagic {
		t.Fatalf("magic: want 0x%x, got 0x%x", exportMagic, got)
	}

	pathLen := readU64()
	if pathLen != uint64(len(storePath)) {
		t.Fatalf("path length: want %d, got %d", len(storePath), pathLen)
	}
	padded := (8 - pathLen%8) % 8
	pathBuf := make([]byte, pathLen+padded)
	if _, err := r.Read(pathBuf); err != nil {
		t.Fatal(err)
	}
	if string(pathBuf[:pathLen]) != storePath {
		t.Fatalf("store path mismatch")
	}

	numRefs := readU64()
	if numRefs != uint64(len(refs)) {
		t.Fatalf("numRefs: want %d, got %d", len(refs), numRefs)
	}
	for _, ref := range refs {
		refLen := readU64()
		if refLen != uint64(len(ref)) {
			t.Fatalf("ref length: want %d, got %d", len(ref), refLen)
		}
		refPad := (8 - refLen%8) % 8
		refBuf := make([]byte, refLen+refPad)
		if _, err := r.Read(refBuf); err != nil {
			t.Fatal(err)
		}
		if string(refBuf[:refLen]) != ref {
			t.Fatalf("ref mismatch: want %q, got %q", ref, string(refBuf[:refLen]))
		}
	}

	deriverLen := readU64()
	if deriverLen != 0 {
		t.Fatalf("deriver length: want 0, got %d", deriverLen)
	}

	if flag := readU64(); flag != 0 {
		t.Fatalf("registry flag: want 0, got %d", flag)
	}

	if r.Len() != 0 {
		t.Fatalf("unexpected trailing bytes: %d", r.Len())
	}
}
