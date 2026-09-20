package skipfilter_test

import (
	"sync"
	"testing"

	"github.com/dunglas/skipfilter"
)

// MatchAny both reads a filter's bitmap and, when it finds ids of removed
// values, writes to it. Concurrent callers therefore read one filter while
// another prunes it, so every access to the bitmap has to be guarded.
func TestMatchAnyConcurrent(_ *testing.T) {
	sf := skipfilter.New(func(value, filter int) bool {
		return value%filter == 0
	}, 100)

	for i := range 2000 {
		sf.Add(i)
	}

	const readers = 8

	var wg sync.WaitGroup

	// Removals leave ids in the filters, which is what makes MatchAny prune
	// them on the next call.
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := range 2000 {
			sf.Remove(i)
			sf.Add(i + 2000)
		}
	}()

	for range readers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 200 {
				// Several filters per call: they share the pruning loop.
				sf.MatchAny(2, 3, 5, 7)
			}
		}()
	}

	wg.Wait()
}
