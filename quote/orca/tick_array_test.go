package orca

import (
	"errors"
	"math/big"
	"slices"
	"testing"

	orcamath "github.com/Gealber/soldex/math/orca"
	bin "github.com/gagliardetto/binary"
)

const walkerSpacing = 64

// walkerSpan is one tick array at walkerSpacing.
const walkerSpan = walkerSpacing * 88

// walkerNets are the initialized ticks: one each side of tick 0, each dropping a tenth of
// the liquidity when crossed away from the price.
var walkerNets = map[int32]int64{-640: 100_000_000_000, 640: -100_000_000_000}

func walkerPool() SwapPool {
	return SwapPool{
		SqrtPrice:        orcamath.SqrtPriceFromTickIndex(0),
		Liquidity:        big.NewInt(1_000_000_000_000),
		TickCurrentIndex: 0,
		TickSpacing:      walkerSpacing,
		FeeRate:          3000,
	}
}

func int128(value int64) bin.Int128 {
	hi := uint64(0)
	if value < 0 {
		hi = ^uint64(0)
	}
	return bin.Int128{Lo: uint64(value), Hi: hi}
}

// walkerArrays lays walkerNets out in tick arrays, plus the empty ones listed.
func walkerArrays(emptyStarts ...int32) map[int32]TickArray {
	arrays := map[int32]TickArray{}
	for tick, net := range walkerNets {
		start := tick - ((tick%walkerSpan)+walkerSpan)%walkerSpan
		ticks := arrays[start]
		ticks[(tick-start)/walkerSpacing] = ArrayTick{Initialized: true, LiquidityNet: int128(net)}
		arrays[start] = ticks
	}
	for _, start := range emptyStarts {
		arrays[start] = TickArray{}
	}
	return arrays
}

func loaderOf(arrays map[int32]TickArray) TickArrayLoader {
	return func(start int32) (TickArray, bool, error) {
		ticks, ok := arrays[start]
		return ticks, ok, nil
	}
}

func referenceTicks() TickProvider {
	nets := map[int32]*big.Int{}
	for tick, net := range walkerNets {
		nets[tick] = big.NewInt(net)
	}
	return staticTicks(nets)
}

// The start indexes follow get_start_tick_indexes, including the b-to-a shift and the
// arrays dropped at the edges of the tick range.
func TestSwapTickArrayStartsMirrorsTheProgram(t *testing.T) {
	tests := []struct {
		name string
		tick int32
		aToB bool
		want []int32
	}{
		{"a to b at 0", 0, true, []int32{0, -walkerSpan, -2 * walkerSpan}},
		{"b to a at 0", 0, false, []int32{0, walkerSpan, 2 * walkerSpan}},
		{"b to a one below the shift", walkerSpan - walkerSpacing - 1, false, []int32{0, walkerSpan, 2 * walkerSpan}},
		{"b to a on the last tick shifts", walkerSpan - walkerSpacing, false, []int32{walkerSpan, 2 * walkerSpan, 3 * walkerSpan}},
		{"a to b below zero", -1, true, []int32{-walkerSpan, -2 * walkerSpan, -3 * walkerSpan}},
		{"b to a near the max tick", 440_000, false, []int32{439_296}},
		{"a to b at the min tick keeps the left-edge array", minTickIndex, true, []int32{-444_928}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SwapTickArrayStarts(tt.tick, walkerSpacing, tt.aToB); !slices.Equal(got, tt.want) {
				t.Fatalf("starts = %v, want %v", got, tt.want)
			}
		})
	}
}

// Inside the swap's arrays the walker prices like a plain walk over the same ticks,
// crossing an initialized tick on the way.
func TestTickArrayWalkerMatchesAPlainWalk(t *testing.T) {
	const amountIn = 100_000_000_000
	pool := walkerPool()

	for _, aToB := range []bool{true, false} {
		walker := NewTickArrayWalker(pool.TickCurrentIndex, pool.TickSpacing, aToB, loaderOf(walkerArrays()))
		got, err := QuoteExactInDetailed(pool, aToB, amountIn, walker.Next)
		if err != nil {
			t.Fatalf("aToB=%v: walker quote: %v", aToB, err)
		}
		want, err := QuoteExactIn(pool, aToB, amountIn, referenceTicks())
		if err != nil {
			t.Fatalf("aToB=%v: reference quote: %v", aToB, err)
		}
		if got.AmountOut != want || got.AmountInConsumed != amountIn {
			t.Fatalf("aToB=%v: out %d consumed %d, want out %d consumed %d", aToB, got.AmountOut, got.AmountInConsumed, want, uint64(amountIn))
		}

		uncrossed, err := QuoteExactIn(pool, aToB, amountIn, staticTicks(map[int32]*big.Int{}))
		if err != nil {
			t.Fatalf("aToB=%v: uncrossed quote: %v", aToB, err)
		}
		if uncrossed == got.AmountOut {
			t.Fatalf("aToB=%v: the quote never crossed an initialized tick", aToB)
		}
	}
}

// Past the swap's three arrays the walker stops, so the quote consumes less than the input.
func TestTickArrayWalkerStopsAtTheSwapArrays(t *testing.T) {
	const amountIn = 1_000_000_000_000_000
	pool := walkerPool()

	for _, aToB := range []bool{true, false} {
		walker := NewTickArrayWalker(pool.TickCurrentIndex, pool.TickSpacing, aToB, loaderOf(walkerArrays()))
		res, err := QuoteExactInDetailed(pool, aToB, amountIn, walker.Next)
		if err != nil {
			t.Fatalf("aToB=%v: %v", aToB, err)
		}
		if res.AmountInConsumed == 0 || res.AmountInConsumed >= amountIn {
			t.Fatalf("aToB=%v: consumed %d of %d, want a partial fill", aToB, res.AmountInConsumed, uint64(amountIn))
		}
	}
}

// A missing array prices the same as an empty one, as swap_v2's zeroed proxy does.
func TestTickArrayWalkerReadsAMissingArrayAsEmpty(t *testing.T) {
	// Enough b-to-a input to cross 640 and run into the second array.
	const amountIn = 400_000_000_000
	pool := walkerPool()

	missing := NewTickArrayWalker(pool.TickCurrentIndex, pool.TickSpacing, false, loaderOf(walkerArrays()))
	gotMissing, err := QuoteExactInDetailed(pool, false, amountIn, missing.Next)
	if err != nil {
		t.Fatalf("missing: %v", err)
	}
	empty := NewTickArrayWalker(pool.TickCurrentIndex, pool.TickSpacing, false, loaderOf(walkerArrays(walkerSpan)))
	gotEmpty, err := QuoteExactInDetailed(pool, false, amountIn, empty.Next)
	if err != nil {
		t.Fatalf("empty: %v", err)
	}

	if gotMissing != gotEmpty || gotMissing.AmountInConsumed != amountIn {
		t.Fatalf("missing array %+v, empty array %+v, want equal and fully consumed", gotMissing, gotEmpty)
	}
}

func TestTickArrayWalkerReportsTheLoaderError(t *testing.T) {
	loadErr := errors.New("store unavailable")
	pool := walkerPool()
	walker := NewTickArrayWalker(pool.TickCurrentIndex, pool.TickSpacing, true, func(int32) (TickArray, bool, error) {
		return TickArray{}, false, loadErr
	})

	if _, err := QuoteExactInDetailed(pool, true, 1_000_000, walker.Next); err != nil {
		t.Fatalf("quote: %v", err)
	}
	if !errors.Is(walker.Err(), loadErr) {
		t.Fatalf("walker err = %v, want %v", walker.Err(), loadErr)
	}
}

// farArrays leaves the first two swap arrays empty and initializes one tick in the third,
// on the a-to-b side (negative) or the b-to-a side (positive).
func farArrays(aToB bool) (map[int32]TickArray, int32, *big.Int) {
	sign := int32(1)
	if aToB {
		sign = -1
	}
	far := sign*2*walkerSpan + 640
	thirdStart := sign * 2 * walkerSpan
	arrays := map[int32]TickArray{0: {}, sign * walkerSpan: {}}
	net := -int64(sign) * 100_000_000_000
	ticks := TickArray{}
	ticks[(far-thirdStart)/walkerSpacing] = ArrayTick{Initialized: true, LiquidityNet: int128(net)}
	arrays[thirdStart] = ticks
	return arrays, far, big.NewInt(net)
}

// An empty array is searched through, as get_next_initialized_tick_index does; only the last
// swap array yields its edge (a-to-b its start, b-to-a start + span - 1).
func TestTickArrayWalkerSearchesPastEmptyArrays(t *testing.T) {
	for _, aToB := range []bool{true, false} {
		arrays, far, _ := farArrays(aToB)
		walker := NewTickArrayWalker(0, walkerSpacing, aToB, loaderOf(arrays))
		got, ok := walker.Next(0, aToB)
		if !ok || got.TickIndex != far || !got.Initialized {
			t.Fatalf("aToB=%v: next = %+v ok=%v, want initialized tick %d", aToB, got, ok, far)
		}

		empty := NewTickArrayWalker(0, walkerSpacing, aToB, loaderOf(map[int32]TickArray{}))
		edge, ok := empty.Next(0, aToB)
		wantEdge := int32(-2 * walkerSpan)
		if !aToB {
			wantEdge = 3*walkerSpan - 1
		}
		if !ok || edge.TickIndex != wantEdge || edge.Initialized {
			t.Fatalf("aToB=%v: edge = %+v ok=%v, want uninitialized %d", aToB, edge, ok, wantEdge)
		}
	}
}

// Across empty arrays the walker prices exactly like a plain walk over the same ticks: a stop
// at each empty array's edge is an extra floor-rounded step the program does not take.
func TestTickArrayWalkerPricesEmptyArraysLikeAPlainWalk(t *testing.T) {
	pool := walkerPool()
	for _, aToB := range []bool{true, false} {
		arrays, far, net := farArrays(aToB)
		nets := map[int32]*big.Int{far: net}
		mismatched := 0
		for amountIn := uint64(1_000_000_000_000); amountIn < 3_000_000_000_000; amountIn += 7_919_000_000 {
			walker := NewTickArrayWalker(0, walkerSpacing, aToB, loaderOf(arrays))
			got, err := QuoteExactInDetailed(pool, aToB, amountIn, walker.Next)
			if err != nil {
				t.Fatalf("aToB=%v: walker quote: %v", aToB, err)
			}
			want, err := QuoteExactInDetailed(pool, aToB, amountIn, staticTicks(nets))
			if err != nil {
				t.Fatalf("aToB=%v: reference quote: %v", aToB, err)
			}
			if got.AmountInConsumed == amountIn && got != want {
				mismatched++
			}
		}
		if mismatched > 0 {
			t.Fatalf("aToB=%v: %d amounts priced differently from a plain walk", aToB, mismatched)
		}
	}
}

// Fogo testnet Vortex pool ALnwuEBBf8F3f3h5MYBXepNRif7gy7yJAknhH7gDXXGu (an Orca fork), slot 1191295540:
// the program paid 21,986,259 for 40,000,000 a-to-b, crossing tick -832 then two empty arrays.
func TestTickArrayWalkerChainVector(t *testing.T) {
	sqrtPrice, _ := new(big.Int).SetString("18143792258912789137", 10)
	pool := SwapPool{SqrtPrice: sqrtPrice, Liquidity: big.NewInt(70_200_000), TickCurrentIndex: -332, TickSpacing: walkerSpacing, FeeRate: 3000}
	ticks := TickArray{}
	ticks[(-832+walkerSpan)/walkerSpacing] = ArrayTick{Initialized: true, LiquidityNet: int128(20_000_000)}

	walker := NewTickArrayWalker(pool.TickCurrentIndex, pool.TickSpacing, true, loaderOf(map[int32]TickArray{-walkerSpan: ticks}))
	got, err := QuoteExactInDetailed(pool, true, 40_000_000, walker.Next)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if got.AmountOut != 21_986_259 || got.AmountInConsumed != 40_000_000 {
		t.Fatalf("out %d consumed %d, want 21986259 and 40000000", got.AmountOut, got.AmountInConsumed)
	}
}

// The min and max tick arrays end the search at the tick bounds, not at the array edge.
func TestTickArrayWalkerEndsAtTheTickBounds(t *testing.T) {
	aToB := NewTickArrayWalker(minTickIndex, walkerSpacing, true, loaderOf(map[int32]TickArray{}))
	if got, ok := aToB.Next(minTickIndex, true); !ok || got.TickIndex != minTickIndex {
		t.Fatalf("a-to-b next = %+v ok=%v, want %d", got, ok, minTickIndex)
	}
	bToA := NewTickArrayWalker(440_000, walkerSpacing, false, loaderOf(map[int32]TickArray{}))
	if got, ok := bToA.Next(440_000, false); !ok || got.TickIndex != maxTickIndex {
		t.Fatalf("b-to-a next = %+v ok=%v, want %d", got, ok, maxTickIndex)
	}
}
