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

// A delta is a stream of tokens. Each token is a uvarint whose lowest bit
// selects the operation and whose remaining bits carry a length:
//
//	COPY:   length<<opBits | opCopy     followed by a uvarint offset into the base
//	INSERT: length<<opBits | opInsert   followed by `length` literal bytes
const (
	opBits   = 1             // width of the operation tag
	opMask   = 1<<opBits - 1 // selects the operation tag
	opCopy   = 0
	opInsert = 1
)

func copyToken(length int) uint64 {
	return uint64(length)<<opBits | opCopy
}

func insertToken(length int) uint64 {
	return uint64(length)<<opBits | opInsert
}

func tokenOp(tok uint64) uint64 {
	return tok & opMask
}

func tokenLen(tok uint64) int {
	return int(tok >> opBits)
}

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

// Return delta from target
func (ix *Index) Diff(target []byte) []byte {
	base := ix.base
	// Optimistic capacity
	// delta is usually < half the target.
	out := make([]byte, 0, len(target)/2+16)

	litStart := 0
	emitLiteral := func(end int) {
		if end <= litStart {
			return
		}
		out = appendUvarint(out, insertToken(end-litStart))
		out = append(out, target[litStart:end]...)
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
				out = appendUvarint(out, copyToken(length))
				out = appendUvarint(out, uint64(bpos))
				i = tpos + length
				litStart = i
				continue
			}
		}
		i++
	}
	emitLiteral(len(target))

	return out
}

func Diff(base, target []byte) []byte {
	return NewIndex(base).Diff(target)
}

func Patch(base, delta []byte, resultSize int) ([]byte, error) {
	if resultSize < 0 {
		return nil, errors.New("delta: negative result size")
	}
	out := make([]byte, 0, resultSize)

	p := 0
	for p < len(delta) {
		tok, n := binary.Uvarint(delta[p:])
		if n <= 0 {
			return nil, errors.New("delta: malformed token header")
		}
		p += n

		length := tokenLen(tok)
		if length <= 0 {
			return nil, errors.New("delta: invalid token length")
		}
		if len(out)+length > resultSize {
			return nil, errors.New("delta: output exceeds result size")
		}

		switch tokenOp(tok) {
		case opInsert:
			if p+length > len(delta) {
				return nil, errors.New("delta: literal out of bounds")
			}
			out = append(out, delta[p:p+length]...)
			p += length

		case opCopy:
			off, n2 := binary.Uvarint(delta[p:])
			if n2 <= 0 {
				return nil, errors.New("delta: malformed copy offset")
			}
			p += n2
			o := int(off)
			if o > len(base) || o+length > len(base) {
				return nil, errors.New("delta: copy out of bounds")
			}
			out = append(out, base[o:o+length]...)
		}
	}

	if len(out) != resultSize {
		return nil, errors.New("delta: result size mismatch")
	}

	return out, nil
}

func appendUvarint(b []byte, v uint64) []byte {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	return append(b, tmp[:n]...)
}

func matchLen(a []byte, ai int, b []byte, bi int) int {
	n := 0
	for ai+n < len(a) && bi+n < len(b) && a[ai+n] == b[bi+n] {
		n++
	}
	return n
}
