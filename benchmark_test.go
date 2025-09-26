package skipfilter_test

import (
	"testing"

	"github.com/kevburnsjr/skipfilter"
)

func BenchmarkSkipFilter_MatchAny_Read(b *testing.B) {
	sf := skipfilter.New(func(value, filter int) bool {
		return value%filter == 0
	}, 10)

	for i := 0; i < 1000; i++ {
		sf.Add(i)
	}

	for b.Loop() {
		for i := 1; i < 100; i++ {
			sf.MatchAny(i)
		}
	}
}

func BenchmarkSkipFilter_MatchAny_ReadWrite(b *testing.B) {
	sf := skipfilter.New(func(value, filter int) bool {
		return value%filter == 0
	}, 10)

	for i := 0; i < 1000; i++ {
		sf.Add(i)
	}

	for b.Loop() {
		for i := 1; i < 100; i++ {
			sf.MatchAny(i)
			sf.Add(i)
			sf.Remove(i - 1)
		}
	}
}
