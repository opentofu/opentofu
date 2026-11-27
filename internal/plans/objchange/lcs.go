// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"math"

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

// lcsMaxCost bounds how many steps the subsequence search may take before
// treating everything between the matched prefix and suffix as changed.
const lcsMaxCost = 1 << 27

// LongestCommonSubsequence finds a sequence of values that are common to both
// xs and ys, with the same relative ordering as in both collections. This result
// is useful as a first step towards computing a diff showing added/removed
// elements in a sequence.
//
// The search is Myers' O((N+M)·D) algorithm in its linear-space variant, where
// D is the number of elements that are not part of the subsequence. When the
// slices are so different that the search would be too expensive, only the
// matched prefix and suffix are returned and everything in between is treated
// as changed.
//
// A pair of lists may have multiple longest common subsequences. In that
// case, the one selected by this function is undefined.
func LongestCommonSubsequence[V any](xs, ys []V, equals func(x, y V) bool) []V {
	if len(xs) == 0 || len(ys) == 0 {
		return make([]V, 0)
	}

	// The common prefix and suffix always belong to the subsequence, so match
	// them up front. They are returned even when the search for the middle
	// runs out of budget.
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

	out = append(out, lcsMiddle(xs, ys, equals)...)
	return append(out, tail...)
}

// lcsMiddle returns a longest common subsequence of xs and ys, or nil when
// the search runs out of budget. The slices must not start or end with equal
// elements; the caller trims them first.
func lcsMiddle[V any](xs, ys []V, equals func(x, y V) bool) []V {
	s := &lcsSearcher[V]{equals: equals, budget: lcsMaxCost}
	// The empty slice must be non-nil: search returns nil only when the
	// budget runs out, and an empty result must not read as a failure.
	return s.search(xs, ys, []V{})
}

// lcsSearcher accumulates the subsequence while tracking the remaining budget.
type lcsSearcher[V any] struct {
	equals func(x, y V) bool
	budget int
}

// spend charges one search step to the budget and reports whether any remain.
func (s *lcsSearcher[V]) spend() bool {
	s.budget--
	return s.budget >= 0
}

// search appends a longest common subsequence of xs and ys to out and returns
// the longer slice, or nil if the search ran out of budget.
func (s *lcsSearcher[V]) search(xs, ys, out []V) []V {
	// The common prefix and suffix always belong to the subsequence, so match
	// them up front and only search through what remains in the middle.
	p := 0
	for p < len(xs) && p < len(ys) && s.equals(xs[p], ys[p]) {
		p++
	}
	out = append(out, xs[:p]...)
	xs, ys = xs[p:], ys[p:]
	if len(xs) == 0 || len(ys) == 0 {
		return out
	}

	suffix := 0
	for suffix < len(xs) && suffix < len(ys) && s.equals(xs[len(xs)-1-suffix], ys[len(ys)-1-suffix]) {
		suffix++
	}
	tail := xs[len(xs)-suffix:]
	xs, ys = xs[:len(xs)-suffix], ys[:len(ys)-suffix]
	if len(xs) == 0 || len(ys) == 0 {
		return append(out, tail...)
	}

	x, y, ok := s.middleSnake(xs, ys)
	if !ok {
		return nil
	}
	out = s.search(xs[:x], ys[:y], out)
	if out == nil {
		return nil
	}
	out = s.search(xs[x:], ys[y:], out)
	if out == nil {
		return nil
	}
	return append(out, tail...)
}

// middleSnake finds a point (x, y) that an optimal edit script between xs and
// ys passes through, as described in section 4b of Myers' paper "An O(ND)
// Difference Algorithm and Its Variations". The caller must trim equal
// prefixes and suffixes first, so the corner snakes are empty. It returns
// ok=false when the searches don't meet within the budget.
func (s *lcsSearcher[V]) middleSnake(xs, ys []V) (int, int, bool) {
	n, m := len(xs), len(ys)

	// vf and vb hold, per diagonal k = x - y, how far the forward search has
	// advanced from (0, 0) and how far the backward search has advanced from
	// (n, m) in the same coordinates. Diagonals not reached yet hold -1 in vf
	// and math.MaxInt in vb, so neither counts as a reach when the searches
	// take the best of their options.
	maxD := (n + m + 1) / 2
	offset := maxD + 1 + max(m-n, 0)
	vf := make([]int, 2*maxD+max(n-m, 0)+max(m-n, 0)+3)
	vb := make([]int, len(vf))
	for i := range vf {
		vf[i] = -1
		vb[i] = math.MaxInt
	}
	// The searches start at (0, 0) and (n, m) without having spent any edits.
	vf[offset] = 0
	vb[offset+n-m] = n

	// An optimal script has D ≡ n+m (mod 2) edits, so the searches can only
	// meet after a forward round when that count is odd, and after a backward
	// round when it is even.
	delta := n - m
	odd := delta%2 != 0

	for d := 1; d <= maxD; d++ {
		// Advance the forward search by one edit per round.
		for k := -d; k <= d; k += 2 {
			if !s.spend() {
				return 0, 0, false
			}
			off := offset + k
			// A path with d edits reaches diagonal k from diagonal k+1 by
			// moving down, or from diagonal k-1 by moving right. Keep the
			// stored reach when neither move fits in the grid.
			x := vf[off]
			if right := vf[off-1]; right >= 0 && right+1 > x {
				x = right + 1
			}
			if down := vf[off+1]; down >= 0 && down > x {
				x = down
			}
			if x < 0 || x > n || x-k > m {
				continue
			}
			y := x - k
			for x < n && y < m && s.equals(xs[x], ys[y]) {
				x++
				y++
				if !s.spend() {
					return 0, 0, false
				}
			}
			vf[off] = x
			if odd {
				if b := vb[off]; b <= x {
					return x, y, true
				}
			}
		}

		// Advance the backward search by one edit per round.
		for k := delta - d; k <= delta+d; k += 2 {
			if !s.spend() {
				return 0, 0, false
			}
			off := offset + k
			// A backward path with d edits reaches diagonal k from diagonal
			// k-1 by moving up, or from diagonal k+1 by moving left. Keep the
			// stored reach when neither move fits in the grid.
			x := vb[off]
			if up := vb[off-1]; up < x {
				x = up
			}
			if left := vb[off+1]; left-1 < x {
				x = left - 1
			}
			if x < 0 || x > n || x-k < 0 {
				continue
			}
			y := x - k
			for x > 0 && y > 0 && s.equals(xs[x-1], ys[y-1]) {
				x--
				y--
				if !s.spend() {
					return 0, 0, false
				}
			}
			vb[off] = x
			if !odd {
				if f := vf[off]; x <= f {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}
