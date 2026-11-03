package av1decode

import (
	"runtime"
	"sync"
	"testing"
)

func resetPoolForTest() {
	av1Pool.mu.Lock()
	for av1Pool.lru.Len() > 0 {
		av1Pool.evictLocked(av1Pool.lru.Back())
	}
	av1Pool.mu.Unlock()
}

// TestDecodeSampleCachedConcurrentMixedSizes hammers one configuration with
// temporal units of very different sizes so goroutines keep replacing
// undersized decoders while others decode. Run with -race.
func TestDecodeSampleCachedConcurrentMixedSizes(t *testing.T) {
	small := readAV1(t, "tu_small.obu")
	large := readAV1(t, "tu_large.obu")
	resetPoolForTest()
	defer resetPoolForTest()

	workers := 4 * runtime.GOMAXPROCS(0)
	iters := 12
	if testing.Short() {
		iters = 4
	}
	var wg sync.WaitGroup
	errc := make(chan error, workers*iters)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				s := small
				if (w+i)%2 == 1 {
					s = large
				}
				th, err := DecodeSampleCached(nil, s)
				if err != nil {
					errc <- err
					return
				}
				if b := th.YCbCr.Bounds(); b.Dx() != 128 || b.Dy() != 72 {
					t.Errorf("bounds %v", b)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatal(err)
	}
	av1Pool.mu.Lock()
	defer av1Pool.mu.Unlock()
	el := av1Pool.keys[configCacheKey(nil)]
	if el == nil {
		t.Fatal("no pool entry")
	}
	e := el.Value.(*poolEntry)
	if e.inUse != 0 || len(e.idle) > perKeyLimit() {
		t.Fatalf("inUse=%d idle=%d limit=%d", e.inUse, len(e.idle), perKeyLimit())
	}
}

// TestPoolLRUEvictsOnlyIdle checks that evicting a key while one of its
// decoders is checked out leaves that decoder usable and closes it on return.
func TestPoolLRUEvictsOnlyIdle(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	resetPoolForTest()
	defer resetPoolForTest()

	e, dec, maxUnit := av1Pool.checkout("held")
	if _, err := decodeHeld(&dec, &maxUnit, nil, sample); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxPoolKeys; i++ {
		ce, cd, cm := av1Pool.checkout(string(rune('a' + i)))
		av1Pool.checkin(ce, cd, cm)
	}
	av1Pool.mu.Lock()
	_, still := av1Pool.keys["held"]
	n := av1Pool.lru.Len()
	av1Pool.mu.Unlock()
	if still || n != maxPoolKeys {
		t.Fatalf("held key still pooled=%v keys=%d", still, n)
	}
	if _, err := dec.DecodeSample(nil, sample); err != nil {
		t.Fatalf("checked-out decoder closed by eviction: %v", err)
	}
	av1Pool.checkin(e, dec, maxUnit)
	if dec.kd != nil {
		t.Fatal("decoder of evicted key not closed on checkin")
	}
}
