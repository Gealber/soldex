package dlmm

import (
	"math/big"
	"sync"
)

// THE BIN PRICE IS A PURE FUNCTION OF TWO SMALL INTEGERS AND IT WAS RECOMPUTED EVERY TIME.
//
// GetPriceFromID(activeID, binStep) derives base = 1 + binStep/10000 in Q64.64 and raises it to
// activeID through 19 rounds of 128-bit multiply-and-shift with overflow checks. Benchmarked at
// 5,213 ns and 55 ALLOCATIONS per call.
//
// QuoteExactInDetailed calls it once per bin crossed, inside the swap loop. A caller sizing a trade
// probes the same pool dozens of times at different amounts, so the identical (binStep, activeID)
// pairs are recomputed hundreds of times for one decision. On the arbitrage bot's 2026-08-02
// profile that path was 2.98% of all CPU, and the allocation churn behind it does not show up in a
// CPU profile at all.
//
// Cached: 89 ns and 1 allocation, a 59x cut.
//
// IT RETURNS A CLONE, NOT THE CACHED POINTER. Every caller in this module treats the price as a
// read-only operand -- swapAtBin passes it to GetAmountInFromAmountOut / GetAmountOutFromAmountIn,
// which reach MulDiv, which does new(big.Int).Mul(x, y) and never assigns to x or y -- so handing
// out the shared value would work today. It would also be a trap: this is an exported function, a
// future caller that scales or rounds the price in place would silently corrupt every subsequent
// quote, and the corruption would look like a market move rather than a bug. 89 ns buys immunity
// from that.

// priceCacheSize bounds the cache by construction. DIRECT-MAPPED with overwrite on collision: no
// eviction policy, no growth, no bookkeeping. The alternative -- an unbounded map keyed on values
// derived from on-chain pool state -- is a memory leak with an attacker-influenced key space.
//
// 4096 entries covers what any real workload touches: binStep comes from a handful of values and a
// pool's swaps walk a narrow band of bin ids around the active one. A collision costs one
// recomputation, which is exactly what the uncached path did anyway.
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

// priceSlot mixes the key before taking the index, and the first version did NOT -- it packed
// activeID into the high 48 bits and used key % priceCacheSize. 1<<16 is a whole multiple of 4096,
// so the bin id vanished under the modulo and every id in a pool mapped to ONE slot. The cache
// missed on every call.
//
// Nothing caught it except the benchmark. The correctness tests passed, because a collision
// compares the full key and recomputes rather than returning the wrong price -- a broken cache and
// a working one are indistinguishable except by speed, which is the whole reason the benchmark
// exists. A cache is the one optimisation whose failure mode is silence.
//
// Fibonacci hashing: multiply by 2^64/phi and take the HIGH bits, which depend on every input bit.
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
