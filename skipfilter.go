package skipfilter

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/MauriceGit/skiplist"
	"github.com/RoaringBitmap/roaring/v2/roaring64"
	"github.com/maypok86/otter/v2"
)

// SkipFilter combines a skip list with a cache of roaring bitmaps
type SkipFilter[V any, F comparable] struct {
	i     uint64
	idx   map[interface{}]uint64
	list  skiplist.SkipList
	cache *otter.Cache[F, *filter]
	test  func(V, F) bool
	mutex sync.RWMutex
}

// New creates a new SkipFilter.
//
// The test function should return true if the value passes the provided filter.
// The maximumSize parameter controls the maximum size of the cache: 0 keeps every filter bitmap,
// a negative value disables the cache. When positive, it should be tuned to match or exceed the
// expected filter cardinality.
func New[V any, F comparable](test func(value V, filter F) bool, maximumSize int) *SkipFilter[V, F] {
	sf := &SkipFilter[V, F]{
		idx:  make(map[interface{}]uint64),
		list: skiplist.New(),
		test: test,
	}

	if maximumSize >= 0 {
		sf.cache = otter.Must(&otter.Options[F, *filter]{MaximumSize: maximumSize})
	}

	return sf
}

// Add adds a value to the set
func (sf *SkipFilter[V, F]) Add(value V) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	el := &entry[V]{sf.i, value}
	sf.list.Insert(el)
	sf.idx[value] = sf.i
	sf.i++
}

// Remove removes a value from the set
func (sf *SkipFilter[V, F]) Remove(value V) {
	sf.mutex.Lock()
	defer sf.mutex.Unlock()
	if id, ok := sf.idx[value]; ok {
		sf.list.Delete(&entry[V]{id: id})
		delete(sf.idx, value)
	}
}

// Len returns the number of values in the set
func (sf *SkipFilter[V, F]) Len() int {
	sf.mutex.RLock()
	defer sf.mutex.RUnlock()
	return sf.list.GetNodeCount()
}

// MatchAny returns a slice of values in the set matching any of the provided filters
func (sf *SkipFilter[V, F]) MatchAny(filterKeys ...F) []V {
	sf.mutex.RLock()
	defer sf.mutex.RUnlock()
	var filters = make([]*filter, len(filterKeys))
	// The union is built into a bitmap of our own, each filter read under its
	// own lock: sf.mutex is shared, so a concurrent MatchAny may be pruning
	// these very bitmaps below.
	var set = roaring64.New()
	for i, k := range filterKeys {
		f := sf.getFilter(k)
		filters[i] = f
		f.mutex.RLock()
		set.Or(f.set)
		f.mutex.RUnlock()
	}
	values, notfound := sf.getValues(set)
	if len(notfound) > 0 {
		// Clean up references to removed values
		for _, f := range filters {
			f.mutex.Lock()
			for _, id := range notfound {
				f.set.Remove(id)
			}
			f.mutex.Unlock()
		}
	}
	return values
}

// Walk executes callback for each value in the set beginning at `start` index.
// Return true in callback to continue iterating, false to stop.
// Returned uint64 is index of `next` element (send as `start` to continue iterating)
func (sf *SkipFilter[V, F]) Walk(start uint64, callback func(val V) bool) uint64 {
	sf.mutex.RLock()
	defer sf.mutex.RUnlock()
	var i uint64
	var id = start
	var prev uint64
	var first = true
	el, ok := sf.list.FindGreaterOrEqual(&entry[V]{id: start})
	for ok && el != nil {
		if id = el.GetValue().(*entry[V]).id; !first && id <= prev {
			// skiplist loops back to first element so we have to detect loop and break manually
			id = prev + 1
			break
		}
		i++
		if !callback(el.GetValue().(*entry[V]).val) {
			id++
			break
		}
		prev = id
		el = sf.list.Next(el)
		first = false
	}
	return id
}

func (sf *SkipFilter[V, F]) getFilter(k F) *filter {
	var f *filter
	if sf.cache != nil {
		// Computed under the cache's bucket lock so concurrent callers share one
		// filter instead of each scanning the whole list into a copy of their own.
		f, _ = sf.cache.ComputeIfAbsent(k, func() (*filter, bool) {
			return &filter{i: 0, set: roaring64.New()}, false
		})
	} else {
		f = &filter{i: 0, set: roaring64.New()}
	}

	var id uint64
	var prev uint64
	var first = true
	if atomic.LoadUint64(&f.i) < sf.i {
		f.mutex.Lock()
		defer f.mutex.Unlock()
		// Re-read now that the write lock is held: another caller may have
		// brought the filter up to date between the check and the lock, and
		// re-scanning from a stale f.i would repeat its work.
		if atomic.LoadUint64(&f.i) >= sf.i {
			return f
		}
		for el, ok := sf.list.FindGreaterOrEqual(&entry[V]{id: f.i}); ok && el != nil; el = sf.list.Next(el) {
			if id = el.GetValue().(*entry[V]).id; !first && id <= prev {
				// skiplist loops back to first element so we have to detect loop and break manually
				break
			}
			if sf.test(el.GetValue().(*entry[V]).val, k) {
				f.set.Add(id)
			}
			prev = id
			first = false
		}
		// Paired with the atomic.LoadUint64 above, which runs without f.mutex.
		atomic.StoreUint64(&f.i, sf.i)
	}
	return f
}

func (sf *SkipFilter[V, F]) getValues(set *roaring64.Bitmap) ([]V, []uint64) {
	idBuf := make([]uint64, 512)
	iter := set.ManyIterator()
	values := []V{}
	notfound := []uint64{}
	e := &entry[V]{}
	for n := iter.NextMany(idBuf); n > 0; n = iter.NextMany(idBuf) {
		for i := 0; i < n; i++ {
			e.id = idBuf[i]
			el, ok := sf.list.Find(e)
			if ok {
				values = append(values, el.GetValue().(*entry[V]).val)
			} else {
				notfound = append(notfound, idBuf[i])
			}
		}
	}
	return values, notfound
}

type entry[V any] struct {
	id  uint64
	val V
}

func (e *entry[V]) ExtractKey() float64 {
	return float64(e.id)
}

func (e *entry[V]) String() string {
	return fmt.Sprintf("%16x", e.id)
}

type filter struct {
	i     uint64
	mutex sync.RWMutex
	set   *roaring64.Bitmap
}
