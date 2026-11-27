// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"github.com/zclconf/go-cty/cty"
)

// ValueEqual provides an implementation of the equals function that can be
// passed into LongestCommonSubsequence when comparing cty.Value types.
func ValueEqual(x, y cty.Value) bool {
	unmarkedX, xMarks := x.UnmarkDeep()
	unmarkedY, yMarks := y.UnmarkDeep()
	eqV := unmarkedX.Equals(unmarkedY)
	if len(xMarks) != len(yMarks) {
		eqV = cty.False
	}
	if eqV.IsKnown() && eqV.True() {
		return true
	}
	return false
}

// Limits on how much work the subsequence search may do before treating
// everything between the matched prefix and suffix as changed. The trace of
// furthest reaches grows with the square of the round count, so the round
// limit keeps it under ~34 MB; the cost limit bounds the search time.
const (
	lcsMaxRounds = 2048
	lcsMaxCost   = 1 << 24
)

// LongestCommonSubsequence finds a sequence of values that are common to both
// xs and ys, with the same relative ordering as in both collections. This result
// is useful as a first step towards computing a diff showing added/removed
// elements in a sequence.
//
// The search is Myers' O((N+M)·D) algorithm, where D is the number of elements
// that are not part of the subsequence. When the slices are so different that
// the search would be too expensive, only the matched prefix and suffix are
// returned and everything in between is treated as changed.
//
// A pair of lists may have multiple longest common subsequences. In that
// case, the one selected by this function is undefined.
func LongestCommonSubsequence[V any](xs, ys []V, equals func(x, y V) bool) []V {
	if len(xs) == 0 || len(ys) == 0 {
		return make([]V, 0)
	}

	// The common prefix and suffix always belong to the subsequence, so match
	// them up front and only search through what remains in the middle.
	p := 0
	for p < len(xs) && p < len(ys) && equals(xs[p], ys[p]) {
		p++
	}
	out := make([]V, 0, min(len(xs), len(ys)))
	out = append(out, xs[:p]...)
	xs, ys = xs[p:], ys[p:]
	if len(xs) == 0 || len(ys) == 0 {
		return out
	}

	suffix := 0
	for suffix < len(xs) && suffix < len(ys) && equals(xs[len(xs)-1-suffix], ys[len(ys)-1-suffix]) {
		suffix++
	}
	tail := xs[len(xs)-suffix:]
	xs, ys = xs[:len(xs)-suffix], ys[:len(ys)-suffix]

	middle := lcsMiddle(xs, ys, equals)
	out = append(out, middle...)
	return append(out, tail...)
}

// lcsMiddle returns a longest common subsequence of xs and ys, or nil when
// the search exceeds the limits. The slices must not start or end with equal
// elements; the caller trims them first.
//
// The search walks a furthest-reaching path from (0, 0) one edit per round,
// as described in Myers' paper "An O(ND) Difference Algorithm and Its
// Variations". Each round's furthest reaches are recorded in a trace, which
// is walked back afterwards to collect the matches.
func lcsMiddle[V any](xs, ys []V, equals func(x, y V) bool) []V {
	n, m := len(xs), len(ys)

	// The end is reached after at most n+m edits.
	rounds := min(n+m, lcsMaxRounds)

	// v holds, per diagonal k = x - y, how far the search has advanced from
	// (0, 0); -1 means not reached yet.
	v := make([]int, 2*rounds+3)
	offset := rounds + 1
	for i := range v {
		v[i] = -1
	}
	v[offset+1] = 0

	// trace holds a snapshot of the reached diagonals after every round;
	// round D's snapshot starts at D*D and holds diagonals -D through D.
	var trace []int

	cost := lcsMaxCost
	d := 0
	for {
		if d > rounds {
			return nil
		}
		found := false
		for k := -d; k <= d; k += 2 {
			cost--
			if cost < 0 {
				return nil
			}
			off := offset + k
			// A path with d edits reaches diagonal k from diagonal k+1 by
			// moving down, or from diagonal k-1 by moving right. Keep the
			// stored reach when neither move fits in the grid.
			x := v[off]
			if right := v[off-1]; right >= 0 && right+1 > x {
				x = right + 1
			}
			if down := v[off+1]; down >= 0 && down > x {
				x = down
			}
			if x < 0 || x > n || x-k > m {
				continue
			}
			y := x - k
			for x < n && y < m && equals(xs[x], ys[y]) {
				x++
				y++
				cost--
				if cost < 0 {
					return nil
				}
			}
			v[off] = x
			if x == n && y == m {
				found = true
				break
			}
		}
		trace = append(trace, v[offset-d:offset+d+1]...)
		if found {
			break
		}
		d++
	}

	// Walk the trace back from (n, m), collecting the matched elements. The
	// matches come out in reverse order, so they are flipped at the end.
	seq := make([]V, 0, min(n, m))
	x, y := n, m
	for ; d > 0; d-- {
		r := trace[d*d : (d+1)*(d+1)] // round d's snapshot, indexed by k+d
		k := x - y
		prevK := k - 1
		if k == -d || (k != d && r[k-1+d] < r[k+1+d]) {
			prevK = k + 1
		}
		p := trace[(d-1)*(d-1) : d*d] // round d-1's snapshot, indexed by k+d-1
		px := p[prevK + d - 1]
		py := px - prevK

		// The edit lands at (px, py+1) when it moved down, or at (px+1, py)
		// when it moved right, and the matches run from there to (x, y). They
		// are collected back to front, like everything else on the way out.
		lx := px + 1
		if prevK == k+1 {
			lx = px
		}
		for ix := x - 1; ix >= lx; ix-- {
			seq = append(seq, xs[ix])
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		x--
		y--
		seq = append(seq, xs[x])
	}
	for i, j := 0, len(seq)-1; i < j; i, j = i+1, j-1 {
		seq[i], seq[j] = seq[j], seq[i]
	}
	return seq
}
