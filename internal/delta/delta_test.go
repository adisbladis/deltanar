package delta

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestPatchRejectsMalformed(t *testing.T) {
	if _, err := Patch([]byte("base"), []byte{0xff, 0xff, 0xff}, 4); err == nil {
		t.Fatal("expected error for malformed delta")
	}
}

func TestPatchRejectsResultSizeMismatch(t *testing.T) {
	base := []byte("hello world")
	d, err := Diff(base, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Patch(base, d, len(base)+1); err == nil {
		t.Fatal("expected result size mismatch error")
	}
}

func TestPatchRejectsOutOfBoundsCopy(t *testing.T) {
	// Handcraft a delta that copies past the end of the base.
	d, err := proto.Marshal(&Delta{
		Ops: []*Delta_Op{
			{Op: &Delta_Op_Copy{Copy: &Delta_Copy{Offset: 0, Length: 100}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Patch([]byte("short"), d, 100); err == nil {
		t.Fatal("expected copy out of bounds error")
	}
}
