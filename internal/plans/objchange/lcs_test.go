// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestLongestCommonSubsequence(t *testing.T) {
	tests := []struct {
		xs   []cty.Value
		ys   []cty.Value
		want []cty.Value
	}{
		{
			[]cty.Value{},
			[]cty.Value{},
			[]cty.Value{},
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(3), cty.NumberIntVal(4)},
			[]cty.Value{},
		},
		{
			[]cty.Value{cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(2)},
		},
		{
			[]cty.Value{cty.NumberIntVal(1)},
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(1)},
		},
		{
			[]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(1)},
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)},
			[]cty.Value{cty.NumberIntVal(1)}, // arbitrarily selected 1; 2 would also be valid
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3), cty.NumberIntVal(4)},
			[]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(4), cty.NumberIntVal(5)},
			[]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(4)},
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3), cty.NumberIntVal(4)},
			[]cty.Value{cty.NumberIntVal(4), cty.NumberIntVal(2), cty.NumberIntVal(5)},
			[]cty.Value{cty.NumberIntVal(4)}, // 2 would also be valid
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3), cty.NumberIntVal(5)},
			[]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(4), cty.NumberIntVal(5)},
			[]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(5)},
		},

		// unknowns never compare as equal
		{
			[]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number), cty.NumberIntVal(3)},
			[]cty.Value{cty.NumberIntVal(1), cty.UnknownVal(cty.Number), cty.NumberIntVal(3)},
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(3)},
		},
		{
			[]cty.Value{cty.UnknownVal(cty.Number)},
			[]cty.Value{cty.UnknownVal(cty.Number)},
			[]cty.Value{},
		},

		// marked values
		{
			[]cty.Value{cty.NumberIntVal(1).Mark("foo"), cty.NumberIntVal(2).Mark("foo"), cty.NumberIntVal(3)},
			[]cty.Value{cty.NumberIntVal(1).Mark("foo"), cty.NumberIntVal(2).Mark("foo")},
			[]cty.Value{cty.NumberIntVal(1).Mark("foo"), cty.NumberIntVal(2).Mark("foo")},
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2).Mark("foo"), cty.NumberIntVal(3)},
			[]cty.Value{cty.NumberIntVal(2), cty.NumberIntVal(3)},
			[]cty.Value{cty.NumberIntVal(3)},
		},
		{
			[]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2).Mark("foo")},
			[]cty.Value{cty.NumberIntVal(2)},
			[]cty.Value{},
		},
		{
			[]cty.Value{
				cty.MapVal(map[string]cty.Value{"a": cty.StringVal("x").Mark(marks.Sensitive)}),
				cty.MapVal(map[string]cty.Value{"b": cty.StringVal("y")}),
			},
			[]cty.Value{
				cty.MapVal(map[string]cty.Value{"a": cty.StringVal("x").Mark(marks.Sensitive)}),
				cty.MapVal(map[string]cty.Value{"b": cty.StringVal("y")}),
				cty.MapVal(map[string]cty.Value{"c": cty.StringVal("z")}),
			},
			[]cty.Value{
				cty.MapVal(map[string]cty.Value{"a": cty.StringVal("x").Mark(marks.Sensitive)}),
				cty.MapVal(map[string]cty.Value{"b": cty.StringVal("y")}),
			},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v,%#v", test.xs, test.ys), func(t *testing.T) {
			got := LongestCommonSubsequence(test.xs, test.ys, ValueEqual)

			wrong := func() {
				t.Fatalf(
					"wrong result\nX:    %#v\nY:    %#v\ngot:  %#v\nwant: %#v",
					test.xs, test.ys, got, test.want,
				)
			}

			if len(got) != len(test.want) {
				wrong()
			}

			for i := range got {
				if got[i] == cty.NilVal {
					wrong()
				}
				if !got[i].RawEquals(test.want[i]) {
					wrong()
				}
			}
		})
	}
}

// lcsLengthReference returns the length of the longest common subsequence of
// xs and ys, using the classic quadratic dynamic programming recurrence.
func lcsLengthReference(xs, ys []string) int {
	table := make([][]int, len(xs)+1)
	for i := range table {
		table[i] = make([]int, len(ys)+1)
	}
	for x := 1; x <= len(xs); x++ {
		for y := 1; y <= len(ys); y++ {
			if xs[x-1] == ys[y-1] {
				table[x][y] = table[x-1][y-1] + 1
			} else if table[x-1][y] >= table[x][y-1] {
				table[x][y] = table[x-1][y]
			} else {
				table[x][y] = table[x][y-1]
			}
		}
	}
	return table[len(xs)][len(ys)]
}

// isSubsequence reports whether seq appears in xs in the same order.
func isSubsequence(seq, xs []string) bool {
	i := 0
	for _, x := range xs {
		if i < len(seq) && seq[i] == x {
			i++
		}
	}
	return i == len(seq)
}

func TestLongestCommonSubsequenceMatchesReference(t *testing.T) {
	equals := func(x, y string) bool { return x == y }

	// Enumerate every string up to the given length over the given alphabet
	// and compare against the reference for every pair.
	allWords := func(alphabet []string, maxLen int) [][]string {
		words := [][]string{{}}
		current := [][]string{{}}
		for length := 0; length < maxLen; length++ {
			var next [][]string
			for _, word := range current {
				for _, letter := range alphabet {
					next = append(next, append(append([]string{}, word...), letter))
				}
			}
			words = append(words, next...)
			current = next
		}
		return words
	}

	for _, alphabet := range [][]string{{"a", "b"}, {"a", "b", "c"}} {
		maxLen := 7
		if len(alphabet) == 3 {
			maxLen = 5
		}
		words := allWords(alphabet, maxLen)
		for _, xs := range words {
			for _, ys := range words {
				got := LongestCommonSubsequence(xs, ys, equals)
				if want := lcsLengthReference(xs, ys); len(got) != want {
					t.Fatalf("wrong subsequence length\nX:    %q\nY:    %q\ngot:  %q (length %d)\nwant length %d",
						xs, ys, got, len(got), want)
				}
				if !isSubsequence(got, xs) || !isSubsequence(got, ys) {
					t.Fatalf("result is not a common subsequence\nX:    %q\nY:    %q\ngot:  %q",
						xs, ys, got)
				}
			}
		}
	}

	// Randomized longer cases, biased towards repeat-heavy inputs and small
	// edits between the two sides.
	rnd := rand.New(rand.NewSource(4883))
	randomWord := func(length, alphabetSize int) []string {
		word := make([]string, length)
		for i := range word {
			word[i] = fmt.Sprintf("e%d", rnd.Intn(alphabetSize))
		}
		return word
	}
	for i := 0; i < 2000; i++ {
		xs := randomWord(rnd.Intn(150), 1+rnd.Intn(26))
		var ys []string
		switch rnd.Intn(3) {
		case 0: // completely unrelated words
			ys = randomWord(rnd.Intn(150), 1+rnd.Intn(26))
		case 1: // ys is xs with a few scattered edits
			ys = append([]string{}, xs...)
			for edits := rnd.Intn(10); edits > 0; edits-- {
				pos := rnd.Intn(len(ys) + 1)
				switch rnd.Intn(3) {
				case 0: // replace
					if pos < len(ys) {
						ys[pos] = randomWord(1, 1+rnd.Intn(26))[0]
					}
				case 1: // insert
					ys = append(ys[:pos], append(randomWord(1, 1+rnd.Intn(26)), ys[pos:]...)...)
				default: // delete
					if pos < len(ys) {
						ys = append(ys[:pos], ys[pos+1:]...)
					}
				}
			}
		default: // ys is a shuffled copy of xs
			ys = append([]string{}, xs...)
			rnd.Shuffle(len(ys), func(i, j int) {
				ys[i], ys[j] = ys[j], ys[i]
			})
		}

		got := LongestCommonSubsequence(xs, ys, equals)
		if want := lcsLengthReference(xs, ys); len(got) != want {
			t.Fatalf("wrong subsequence length\nX:    %q\nY:    %q\ngot:  %q (length %d)\nwant length %d",
				xs, ys, got, len(got), want)
		}
		if !isSubsequence(got, xs) || !isSubsequence(got, ys) {
			t.Fatalf("result is not a common subsequence\nX:    %q\nY:    %q\ngot:  %q",
				xs, ys, got)
		}
	}
}

func TestLongestCommonSubsequenceCostCap(t *testing.T) {
	equals := func(x, y string) bool { return x == y }

	// Two slices with nothing in common exceed the limits, and the result is
	// just the matched prefix and suffix, which is nothing at all.
	xs := make([]string, 3000)
	ys := make([]string, 3000)
	for i := range xs {
		xs[i] = fmt.Sprintf("before-%04d", i)
		ys[i] = fmt.Sprintf("after-%04d", i)
	}
	if got := LongestCommonSubsequence(xs, ys, equals); len(got) != 0 {
		t.Fatalf("expected no matches for slices with nothing in common, got %d elements", len(got))
	}

	// A common prefix and suffix survive even when the middle is too
	// expensive to search.
	for i := range xs {
		xs[i] = fmt.Sprintf("line-%05d", i)
		ys[i] = fmt.Sprintf("changed-%04d", i)
	}
	copy(ys[:10], xs[:10])
	copy(ys[len(ys)-10:], xs[len(xs)-10:])
	got := LongestCommonSubsequence(xs, ys, equals)
	want := append(append([]string{}, xs[:10]...), xs[len(xs)-10:]...)
	if len(got) != len(want) {
		t.Fatalf("wrong subsequence length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("element %d: got %q, want %q", i, got[i], want[i])
		}
	}

	// Diffs that the previous implementation could compute cheaply must not
	// be given up on.
	for _, test := range []struct {
		lines    int
		everyNth int // 0 means all but the first and last lines change
		want     int
	}{
		{2000, 3, 1334},
		{1500, 2, 750},
		{600, 0, 2},
	} {
		xs = make([]string, test.lines)
		ys = make([]string, test.lines)
		for i := range xs {
			xs[i] = fmt.Sprintf("line-%05d", i)
			ys[i] = xs[i]
			switch {
			case test.everyNth > 0 && i%test.everyNth == test.everyNth-1:
				ys[i] += " changed"
			case test.everyNth == 0 && i != 0 && i != test.lines-1:
				ys[i] = fmt.Sprintf("changed-%04d", i)
			}
		}
		got = LongestCommonSubsequence(xs, ys, equals)
		if len(got) != test.want {
			t.Fatalf("%d lines, every %dth changed: got %d common elements, want %d",
				test.lines, test.everyNth, len(got), test.want)
		}
	}

	// A few scattered changes inside two long slices are cheap enough.
	for i := range xs {
		xs[i] = fmt.Sprintf("line-%05d", i)
		ys[i] = xs[i]
	}
	changes := 0
	for i := 0; i < len(ys); i += 1500 {
		ys[i] += " changed"
		changes++
	}
	got = LongestCommonSubsequence(xs, ys, equals)
	if want := len(xs) - changes; len(got) != want {
		t.Fatalf("wrong subsequence length: got %d, want %d", len(got), want)
	}
}

func BenchmarkLongestCommonSubsequence(b *testing.B) {
	// The reproduction shape from GitHub issue #4883.
	const lines = 13797
	xs := make([]string, lines)
	ys := make([]string, lines)
	for i := range xs {
		xs[i] = fmt.Sprintf("  line_%05d: some multi-line config text", i+1)
		ys[i] = xs[i]
	}
	for i := 900; i < lines; i += 900 {
		ys[i] += " # changed"
	}

	equals := func(x, y string) bool { return x == y }
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got := LongestCommonSubsequence(xs, ys, equals); len(got) != lines-15 {
			b.Fatalf("wrong subsequence length: %d", len(got))
		}
	}
}
