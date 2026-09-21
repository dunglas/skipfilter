package skipfilter_test

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dunglas/skipfilter"
)

func TestMatchAnyConcurrentResults(t *testing.T) {
	keysets := [][]int{nil, {1}, {2}, {3}, {2, 2}, {2, 3, 5}, {5, 3, 2}, {2, 3, 2, 5, 3}}
	for _, limit := range []int{-1, 0, 1} {
		t.Run(fmt.Sprintf("cache size %d", limit), func(t *testing.T) {
			sf := skipfilter.New(func(value, filter int) bool { return value%filter == 0 }, limit)
			var active []int
			for phase := range 7 {
				for i := range 96 {
					value := phase*96 + i
					sf.Add(value)
					active = append(active, value)
				}

				// Warm the cache so removals leave bitmap entries for readers to prune.
				for _, keys := range keysets {
					want := matchingValues(active, keys)
					if got := sf.MatchAny(keys...); !slices.Equal(got, want) {
						t.Fatalf("phase %d: MatchAny(%v) = %v, want %v", phase, keys, got, want)
					}
				}

				var remaining []int
				for _, value := range active {
					if value%3 == phase%3 {
						sf.Remove(value)
					} else {
						remaining = append(remaining, value)
					}
				}
				active = remaining
				t.Run(fmt.Sprintf("phase %d", phase), func(t *testing.T) {
					checkConcurrentMatches(t, sf, active, keysets, 16)
				})
			}

			for _, value := range active {
				sf.Remove(value)
			}
			checkConcurrentMatches(t, sf, nil, keysets, 16)
		})
	}
}

func TestMatchAnyConcurrentPredicateCalls(t *testing.T) {
	const values = 256
	var calls [values * 2]atomic.Int64
	sf := skipfilter.New(func(value, filter int) bool {
		calls[value].Add(1)
		return value%filter == 0
	}, 0)

	var active []int
	for phase := range 2 {
		for value := phase * values; value < (phase+1)*values; value++ {
			sf.Add(value)
			active = append(active, value)
		}

		t.Run(fmt.Sprintf("phase %d", phase), func(t *testing.T) {
			checkConcurrentMatches(t, sf, active, [][]int{{2, 2, 2}}, 64)
			for _, value := range active {
				if got := calls[value].Load(); got != 1 {
					t.Errorf("value %d tested %d times, want once", value, got)
				}
			}
		})
	}
}

func matchingValues(values, keys []int) []int {
	var matches []int
	for _, value := range values {
		for _, key := range keys {
			if value%key == 0 {
				matches = append(matches, value)
				break
			}
		}
	}
	return matches
}

func checkConcurrentMatches(t *testing.T, sf *skipfilter.SkipFilter[int, int], values []int, keysets [][]int, readers int) {
	t.Helper()
	want := make([][]int, len(keysets))
	for i, keys := range keysets {
		want[i] = matchingValues(values, keys)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for iteration := range len(keysets) {
				i := (worker + iteration) % len(keysets)
				if got := sf.MatchAny(keysets[i]...); !slices.Equal(got, want[i]) {
					t.Errorf("MatchAny(%v) = %v, want %v", keysets[i], got, want[i])
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
