package rpcproviders

import (
	"iter"
)

// mapSeq performs a systematic transformation of each item in seq, producing
// a new sequence of the same length based on the results.
//
// (Hopefully there will be something like this in the Go standard library
// one day, at which point we should use it and remove this.)
func mapSeq[FromV, ToV any](seq iter.Seq[FromV], f func(v FromV) ToV) iter.Seq[ToV] {
	return func(yield func(ToV) bool) {
		for in := range seq {
			if !yield(f(in)) {
				return
			}
		}
	}
}

// mapSeq2 performs a systematic transformation of each item in seq, producing
// a new sequence of the same length based on the results.
//
// (Hopefully there will be something like this in the Go standard library
// one day, at which point we should use it and remove this.)
func mapSeq2[FromK, FromV, ToK, ToV any](seq iter.Seq2[FromK, FromV], f func(k FromK, v FromV) (ToK, ToV)) iter.Seq2[ToK, ToV] {
	return func(yield func(ToK, ToV) bool) {
		for k, v := range seq {
			if !yield(f(k, v)) {
				return
			}
		}
	}
}

// mapSeqToSeq2 is similar to [MapSeq] and [MapSeq2] but allows expanding
// from [iter.Seq] to [iter.Seq2] during transformation.
func mapSeqToSeq2[FromV, ToK, ToV any](seq iter.Seq[FromV], f func(v FromV) (ToK, ToV)) iter.Seq2[ToK, ToV] {
	return func(yield func(ToK, ToV) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

// mapSeq2ToSeq is similar to [MapSeq] and [MapSeq2] but allows reducing
// from [iter.Seq2] to [iter.Seq] during transformation.
func mapSeq2ToSeq[FromK, FromV, ToV any](seq iter.Seq2[FromK, FromV], f func(k FromK, v FromV) ToV) iter.Seq[ToV] {
	return func(yield func(ToV) bool) {
		for k, v := range seq {
			if !yield(f(k, v)) {
				return
			}
		}
	}
}
