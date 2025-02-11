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

func Collect[V any](seq iter.Seq[V]) []V {
	var result []V
	for v := range seq {
		result = append(result, v)
	}
	return result
}
