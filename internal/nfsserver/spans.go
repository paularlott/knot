package nfsserver

import (
	"math"
	"sort"
)

// span is the bytes [off, end) of a file.
type span struct{ off, end int64 }

// spans is a set of byte ranges, in order, none overlapping or touching.
type spans []span

// add puts [off, end) in the set, joining what it overlaps or touches.
func (s *spans) add(off, end int64) {
	if end <= off {
		return
	}
	ss := *s
	// The first span that ends at or after off could join it.
	i := sort.Search(len(ss), func(i int) bool { return ss[i].end >= off })
	j := i
	for j < len(ss) && ss[j].off <= end {
		off, end = min(off, ss[j].off), max(end, ss[j].end)
		j++
	}
	out := append(ss[:i:i], span{off, end})
	*s = append(out, ss[j:]...)
}

// trim removes everything at or past n.
func (s *spans) trim(n int64) {
	ss := *s
	i := sort.Search(len(ss), func(i int) bool { return ss[i].end > n })
	if i < len(ss) && ss[i].off < n {
		ss[i].end = n
		i++
	}
	*s = ss[:i]
}

// at says whether off is in the set, and where that stops being so: the end
// of its span, or the start of the next.
func (s spans) at(off int64) (inside bool, until int64) {
	i := sort.Search(len(s), func(i int) bool { return s[i].end > off })
	if i == len(s) {
		return false, math.MaxInt64
	}
	if s[i].off <= off {
		return true, s[i].end
	}
	return false, s[i].off
}

// lastEnd is where the last span ends, 0 for none.
func lastEnd(s spans) int64 {
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1].end
}

// size is how many bytes the set holds.
func (s spans) size() int64 {
	var n int64
	for _, sp := range s {
		n += sp.end - sp.off
	}
	return n
}
