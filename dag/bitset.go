package dag

import "math/bits"

// bitset - a small growable bit vector, used to record which of the current
// tips confirm a site. A site's coverage is sparse early and dense later, and
// there is one of these per tracked site, so the compactness matters more than
// the convenience of a map.
//
// Not safe for concurrent use; callers hold the tracker lock.
type bitset []uint64

const wordBits = 64

// set - record bit i, growing the set if needed. Reports whether the bit
// changed, so callers can keep a running population count without recounting.
func (b *bitset) set(i int) bool {
	if i < 0 {
		return false
	}
	w := i / wordBits
	for len(*b) <= w {
		*b = append(*b, 0)
	}
	mask := uint64(1) << uint(i%wordBits)
	if (*b)[w]&mask != 0 {
		return false
	}
	(*b)[w] |= mask
	return true
}

// clear - unset bit i. Reports whether the bit changed.
func (b *bitset) clear(i int) bool {
	if i < 0 {
		return false
	}
	w := i / wordBits
	if w >= len(*b) {
		return false
	}
	mask := uint64(1) << uint(i%wordBits)
	if (*b)[w]&mask == 0 {
		return false
	}
	(*b)[w] &^= mask
	return true
}

func (b bitset) test(i int) bool {
	if i < 0 {
		return false
	}
	w := i / wordBits
	if w >= len(b) {
		return false
	}
	return b[w]&(uint64(1)<<uint(i%wordBits)) != 0
}

func (b bitset) count() int {
	n := 0
	for _, w := range b {
		n += bits.OnesCount64(w)
	}
	return n
}
