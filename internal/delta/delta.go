// Opportunistic differential compression of file chunks.
//
// When a file being sent already shares some chunks with a file on the target,
// the chunks that are missing on the target can often be reconstructed cheaply
// from content the target already has in a similar "reference" file.
// Instead of sending such a chunk in full we send a binary delta against that reference file.
package delta

import (
	"encoding/binary"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

const (
	// Minimum match before encoding as COPY
	minMatch = 16

	// hashLen is the number of leading bytes folded into one index key.
	hashLen = 4

	// Width of hash value buefore folded to table size
	hashBits = hashLen * 8

	// Index table size
	minIndexBits = 12
	maxIndexBits = 23
)

type Index struct {
	base  []byte
	head  []int32 // head[h] = latest base position + 1 with hash bucket h (0 = empty)
	shift uint    // folds a hashBits-wide hash down to log2(len(head)) bits
}

func indexBits(n int) uint {
	bits := uint(minIndexBits)
	for (1<<bits) < n && bits < maxIndexBits {
		bits++
	}
	return bits
}

// Builds a match index over base
func NewIndex(base []byte) *Index {
	bits := indexBits(len(base))
	ix := &Index{
		base:  base,
		head:  make([]int32, 1<<bits),
		shift: hashBits - bits,
	}
	for i := 0; i+hashLen <= len(base); i++ {
		ix.head[ix.hash(base[i:])] = int32(i + 1)
	}
	return ix
}

func (ix *Index) hash(b []byte) uint32 {
	const knuthPrime = 2654435761
	return (binary.LittleEndian.Uint32(b) * knuthPrime) >> ix.shift
}

// Diff returns a serialised Delta that reconstructs target from the index's base.
func (ix *Index) Diff(target []byte) ([]byte, error) {
	base := ix.base
	d := &Delta{}

	litStart := 0
	emitLiteral := func(end int) {
		if end <= litStart {
			return
		}
		d.Ops = append(d.Ops, &Delta_Op{
			Op: &Delta_Op_Insert{Insert: target[litStart:end]},
		})
	}

	i := 0
	for i+hashLen <= len(target) {
		cand := int(ix.head[ix.hash(target[i:])]) - 1
		if cand >= 0 {
			fwd := matchLen(base, cand, target, i)
			if fwd >= minMatch {
				// Extend the match backwards into the pending literals so a copy
				// found mid-run still absorbs the bytes before its anchor
				bpos, tpos, length := cand, i, fwd
				for tpos > litStart && bpos > 0 && base[bpos-1] == target[tpos-1] {
					bpos--
					tpos--
					length++
				}
				emitLiteral(tpos)
				d.Ops = append(d.Ops, &Delta_Op{
					Op: &Delta_Op_Copy{Copy: &Delta_Copy{
						Offset: uint64(bpos),
						Length: uint64(length),
					}},
				})
				i = tpos + length
				litStart = i
				continue
			}
		}
		i++
	}
	emitLiteral(len(target))

	return proto.Marshal(d)
}

// Diff returns a serialised Delta that reconstructs target from base.
func Diff(base, target []byte) ([]byte, error) {
	return NewIndex(base).Diff(target)
}

// Patch reconstructs the original chunk by applying the serialised Delta to base.
func Patch(base, deltaBytes []byte, resultSize int) ([]byte, error) {
	if resultSize < 0 {
		return nil, errors.New("delta: negative result size")
	}

	var d Delta
	if err := proto.Unmarshal(deltaBytes, &d); err != nil {
		return nil, fmt.Errorf("delta: malformed delta: %w", err)
	}

	out := make([]byte, 0, resultSize)
	for _, op := range d.Ops {
		switch op := op.Op.(type) {
		case *Delta_Op_Insert:
			if len(out)+len(op.Insert) > resultSize {
				return nil, errors.New("delta: output exceeds result size")
			}
			out = append(out, op.Insert...)

		case *Delta_Op_Copy:
			length := int(op.Copy.Length)
			if length <= 0 {
				return nil, errors.New("delta: invalid copy length")
			}
			off := int(op.Copy.Offset)
			if off < 0 || off+length > len(base) {
				return nil, errors.New("delta: copy out of bounds")
			}
			if len(out)+length > resultSize {
				return nil, errors.New("delta: output exceeds result size")
			}
			out = append(out, base[off:off+length]...)

		default:
			return nil, errors.New("delta: unknown op")
		}
	}

	if len(out) != resultSize {
		return nil, errors.New("delta: result size mismatch")
	}

	return out, nil
}

func matchLen(a []byte, ai int, b []byte, bi int) int {
	n := 0
	for ai+n < len(a) && bi+n < len(b) && a[ai+n] == b[bi+n] {
		n++
	}
	return n
}
