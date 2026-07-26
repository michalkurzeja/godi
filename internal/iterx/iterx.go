package iterx

import (
	"iter"
)

func Values[K, V any](seq iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}
}

// Flatten yields everything f produces for each element of seq, in order. It is
// what a lookup over a chain of scopes is: the same question asked of each,
// with the answers run together and nothing collected on the way.
func Flatten[A, B any](seq iter.Seq[A], f func(A) iter.Seq[B]) iter.Seq[B] {
	return func(yield func(B) bool) {
		for a := range seq {
			for b := range f(a) {
				if !yield(b) {
					return
				}
			}
		}
	}
}

// First is the first value a sequence yields, and whether it yielded one.
func First[V any](seq iter.Seq[V]) (V, bool) {
	for v := range seq {
		return v, true
	}

	var zero V
	return zero, false
}
