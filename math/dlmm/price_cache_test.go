package dlmm

import (
	"math/big"
	"sync"
	"testing"
)

// resetPriceCache empties the cache so a test starts from a known miss.
func resetPriceCache() {
	priceCacheMu.Lock()
	priceCache = [priceCacheSize]priceEntry{}
	priceCacheMu.Unlock()
}

// THE COMMON PATH FIRST: the memoised function must agree with the arithmetic it replaced, for
// every input a swap actually walks, on both a cold cache and a warm one.
//
// Driven across a band of bin ids and the bin steps Meteora ships, twice, so the second pass is
// served from the cache and compared against the same source of truth as the first.
func TestCachedPriceMatchesTheArithmetic(t *testing.T) {
	resetPriceCache()

	steps := []uint16{1, 2, 5, 10, 15, 20, 25, 50, 80, 100, 200, 400}
	for pass := 0; pass < 2; pass++ {
		for _, step := range steps {
			for id := int32(-600); id <= 600; id += 7 {
				want, wantErr := computePriceFromID(id, step)
				got, gotErr := GetPriceFromID(id, step)
				if (wantErr == nil) != (gotErr == nil) {
					t.Fatalf("pass %d id=%d step=%d: err mismatch %v vs %v", pass, id, step, gotErr, wantErr)
				}
				if wantErr != nil {
					continue
				}
				if got.Cmp(want) != 0 {
					t.Fatalf("pass %d id=%d step=%d: cached %s, computed %s",
						pass, id, step, got, want)
				}
			}
		}
	}
}

// A CACHED VALUE HANDED OUT BY POINTER WOULD BE CORRUPTED BY ITS OWN CALLER.
//
// Every caller in this module treats the price as a read-only operand today, so returning the
// stored pointer would work and would be faster. It would also mean the first caller to scale or
// round the price in place silently poisons every later quote for that bin -- and the damage would
// read as a market move, not as a bug. This pins the copy.
func TestCachedPriceIsNotAliased(t *testing.T) {
	resetPriceCache()

	const id, step = 42, 10
	first, err := GetPriceFromID(id, step)
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Set(first)

	// A caller mutating what it was given.
	first.Mul(first, big.NewInt(999))

	second, err := GetPriceFromID(id, step)
	if err != nil {
		t.Fatal(err)
	}
	if second.Cmp(want) != 0 {
		t.Fatalf("the cache handed out its own pointer: after a caller mutated the result, "+
			"the next call returned %s instead of %s", second, want)
	}
}

// A COLLISION MUST MISS, NOT LIE. The cache is direct-mapped and overwrites on collision, so two
// keys landing in one slot must each still return their own price -- the slot check is on the FULL
// key, not on the slot index.
func TestCachedPriceCollisionReturnsTheRightValue(t *testing.T) {
	resetPriceCache()

	// Find two bin ids that genuinely share a slot under the current hash, rather than assuming a
	// pair: the first version of this test hardcoded two ids that collided only because the hash
	// was broken and mapped EVERYTHING to one slot.
	const step = 10
	a := int32(1)
	var b int32
	for id := int32(2); id < 200_000; id++ {
		if priceSlot(priceKey(id, step)) == priceSlot(priceKey(a, step)) {
			b = id
			break
		}
	}
	if b == 0 {
		t.Skip("no colliding pair found in the scanned range")
	}

	for i := 0; i < 3; i++ {
		for _, id := range []int32{a, b} {
			want, err := computePriceFromID(id, step)
			if err != nil {
				t.Fatal(err)
			}
			got, err := GetPriceFromID(id, step)
			if err != nil {
				t.Fatal(err)
			}
			if got.Cmp(want) != 0 {
				t.Fatalf("round %d id=%d: got %s, want %s -- a collision returned the other "+
					"key's price", i, id, got, want)
			}
		}
	}
}

// Quotes run from several goroutines at once, so the cache must be safe under -race.
func TestCachedPriceIsConcurrencySafe(t *testing.T) {
	resetPriceCache()

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(seed int32) {
			defer wg.Done()
			for i := int32(0); i < 500; i++ {
				id := (seed*97 + i) % 300
				got, err := GetPriceFromID(id, 10)
				if err != nil {
					t.Error(err)
					return
				}
				want, err := computePriceFromID(id, 10)
				if err != nil {
					t.Error(err)
					return
				}
				if got.Cmp(want) != 0 {
					t.Errorf("id=%d: got %s, want %s", id, got, want)
					return
				}
			}
		}(int32(w))
	}
	wg.Wait()
}

func BenchmarkGetPriceFromID(b *testing.B) {
	b.Run("uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := computePriceFromID(int32(i%64), 10); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cached", func(b *testing.B) {
		resetPriceCache()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := GetPriceFromID(int32(i%64), 10); err != nil {
				b.Fatal(err)
			}
		}
	})
}
