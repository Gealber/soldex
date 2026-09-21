package dlmm

import (
	"math/big"
	"sync"
)

// The bin price is a pure function of (binStep, activeID), and GetPriceFromID
// costs 5,213 ns and 55 allocations. Cached it is 89 ns and 1, a 59x cut.
//
// It returns a CLONE: handing out the shared pointer would work today, but an
// exported function that lets a caller scale the price in place would corrupt
// every later quote, and that reads as a market move rather than a bug.

// priceCacheSize bounds the cache by construction: direct-mapped, overwrite on
// collision. A map keyed on on-chain pool state is a leak with an
// attacker-influenced key space. A collision just costs one recomputation.
const priceCacheBits = 12
const priceCacheSize = 1 << priceCacheBits

type priceEntry struct {
	key   uint64
	value *big.Int
}

var (
	priceCacheMu sync.RWMutex
	priceCache   [priceCacheSize]priceEntry
)

// priceKey packs the two inputs into one comparable word.
func priceKey(activeID int32, binStep uint16) uint64 {
	return uint64(uint32(activeID)) | uint64(binStep)<<32
}

// priceSlot mixes the key before indexing: Fibonacci hashing, so every input bit
// reaches the slot. Taking the key modulo the size instead mapped a whole pool to
// one slot, and only the benchmark caught it. A cache fails silently.
func priceSlot(key uint64) uint64 {
	const goldenRatio = 0x9E3779B97F4A7C15
	const shift = 64 - priceCacheBits
	return (key * goldenRatio) >> shift
}

// cachedPriceFromID returns a COPY of the cached price for these inputs, computing and storing it
// on a miss.
func cachedPriceFromID(activeID int32, binStep uint16) (*big.Int, error) {
	key := priceKey(activeID, binStep)
	slot := priceSlot(key)

	priceCacheMu.RLock()
	entry := priceCache[slot]
	priceCacheMu.RUnlock()
	if entry.value != nil && entry.key == key {
		return new(big.Int).Set(entry.value), nil
	}

	value, err := computePriceFromID(activeID, binStep)
	if err != nil {
		return nil, err
	}

	priceCacheMu.Lock()
	priceCache[slot] = priceEntry{key: key, value: value}
	priceCacheMu.Unlock()

	return new(big.Int).Set(value), nil
}
