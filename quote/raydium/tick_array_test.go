package raydium

import (
	"math/big"
	"slices"
	"sort"
	"testing"

	raymath "github.com/Gealber/soldex/math/raydium"
	"github.com/Gealber/soldex/models"
	bin "github.com/gagliardetto/binary"
)

const walkerSpacing = 10

// walkerSpan is one tick array at walkerSpacing.
const walkerSpan = walkerSpacing * models.RaydiumTicksPerArray

// walkerTick is one initialized tick of the fixture book.
type walkerTick struct {
	index  int32
	net    int64
	orders uint64
}

// walkerBook sits below tick 0 across three arrays with a gap at -1200; crossing all
// three drops the liquidity to zero.
var walkerBook = []walkerTick{
	{index: -300, net: 500_000_000_000},
	{index: -1500, net: 250_000_000_000},
	{index: -2700, net: 250_000_000_000},
}

func walkerPool() SwapPool {
	return SwapPool{
		SqrtPrice:   raymath.SqrtPriceFromTick(0),
		Liquidity:   big.NewInt(1_000_000_000_000),
		TickCurrent: 0,
		TickSpacing: walkerSpacing,
		FeeRate:     2500,
	}
}

func int128(value int64) bin.Int128 {
	hi := uint64(0)
	if value < 0 {
		hi = ^uint64(0)
	}
	return bin.Int128{Lo: uint64(value), Hi: hi}
}

func arrayStart(tick int32) int32 {
	return models.RaydiumTickArrayStartIndex(tick, walkerSpacing)
}

// walkerArrays lays the book out in tick arrays; only arrays holding a tick exist.
func walkerArrays(book []walkerTick) map[int32]TickArray {
	arrays := map[int32]TickArray{}
	for _, tick := range book {
		start := arrayStart(tick.index)
		tickArray := arrays[start]
		tickArray.StartTickIndex = start
		tickArray.Ticks[(tick.index-start)/walkerSpacing] = ArrayTick{
			HasLiquidity:       tick.net != 0,
			LiquidityNet:       int128(tick.net),
			LimitOrderUnfilled: tick.orders,
		}
		arrays[start] = tickArray
	}
	return arrays
}

// loaderOf returns the first stored array at or past start, as the pool bitmap would.
func loaderOf(arrays map[int32]TickArray) TickArrayLoader {
	starts := make([]int32, 0, len(arrays))
	for start := range arrays {
		starts = append(starts, start)
	}
	slices.Sort(starts)

	return func(start int32, zeroForOne bool) (TickArray, bool, error) {
		if zeroForOne {
			for i := len(starts) - 1; i >= 0; i-- {
				if starts[i] <= start {
					return arrays[starts[i]], true, nil
				}
			}
			return TickArray{}, false, nil
		}
		for _, candidate := range starts {
			if candidate >= start {
				return arrays[candidate], true, nil
			}
		}
		return TickArray{}, false, nil
	}
}

// flatProvider walks the whole book with no array limit, as the chain vector test does.
func flatProvider(book []walkerTick) TickProvider {
	bounds := make([]TickBoundary, 0, len(book))
	for _, tick := range book {
		bounds = append(bounds, TickBoundary{
			TickIndex:          tick.index,
			LiquidityNet:       big.NewInt(tick.net),
			Initialized:        tick.net != 0,
			LimitOrderUnfilled: tick.orders,
		})
	}
	sort.Slice(bounds, func(i, j int) bool { return bounds[i].TickIndex < bounds[j].TickIndex })

	return func(fromTick int32, zeroForOne bool) (TickBoundary, bool) {
		if zeroForOne {
			for i := len(bounds) - 1; i >= 0; i-- {
				if bounds[i].TickIndex <= fromTick {
					return bounds[i], true
				}
			}
			return TickBoundary{}, false
		}
		for i := range bounds {
			if bounds[i].TickIndex > fromTick {
				return bounds[i], true
			}
		}
		return TickBoundary{}, false
	}
}

// thirdArrayInput needs the third array (the -2700 tick) and stops short of emptying the book.
const thirdArrayInput = 55_000_000_000

func walkerQuote(t *testing.T, book []walkerTick, maxArrays int, amountIn uint64) QuoteResult {
	t.Helper()
	walker := NewTickArrayWalker(0, walkerSpacing, maxArrays, loaderOf(walkerArrays(book)))
	res, err := QuoteExactInDetailed(walkerPool(), true, amountIn, walker.Next)
	if err != nil {
		t.Fatalf("QuoteExactInDetailed: %v", err)
	}
	if walker.Err() != nil {
		t.Fatalf("walker: %v", walker.Err())
	}
	return res
}

func flatQuote(t *testing.T, book []walkerTick, amountIn uint64) QuoteResult {
	t.Helper()
	res, err := QuoteExactInDetailed(walkerPool(), true, amountIn, flatProvider(book))
	if err != nil {
		t.Fatalf("QuoteExactInDetailed: %v", err)
	}
	return res
}

// Across a missing array the walker must price exactly what an unlimited book does.
func TestTickArrayWalkerMatchesFlatBook(t *testing.T) {
	want := flatQuote(t, walkerBook, thirdArrayInput)
	if want.AmountInConsumed != thirdArrayInput {
		t.Fatalf("fixture must fill: consumed %d of %d", want.AmountInConsumed, uint64(thirdArrayInput))
	}

	got := walkerQuote(t, walkerBook, 3, thirdArrayInput)
	if got != want {
		t.Fatalf("walker %+v, flat book %+v", got, want)
	}
}

// Liquidity in an array past maxArrays is not sent to the swap, so it must not fill.
func TestTickArrayWalkerStopsAtMaxArrays(t *testing.T) {
	full := walkerQuote(t, walkerBook, 3, thirdArrayInput)
	if full.AmountInConsumed != thirdArrayInput {
		t.Fatalf("three arrays consumed %d of %d", full.AmountInConsumed, uint64(thirdArrayInput))
	}

	short := walkerQuote(t, walkerBook, 2, thirdArrayInput)
	if short.AmountInConsumed >= thirdArrayInput {
		t.Fatalf("two arrays consumed %d of %d, want a partial fill", short.AmountInConsumed, uint64(thirdArrayInput))
	}
}

// A tick holding only limit orders is a stop: walking past it quotes a fill the pool
// would not give.
func TestTickArrayWalkerStopsOnOrdersOnlyTick(t *testing.T) {
	book := append([]walkerTick{{index: -100, orders: 20_000_000_000}}, walkerBook...)

	want := flatQuote(t, book, thirdArrayInput)
	if without := flatQuote(t, walkerBook, thirdArrayInput); without == want {
		t.Fatalf("the orders do not move the quote: %+v", want)
	}

	got := walkerQuote(t, book, 3, thirdArrayInput)
	if got != want {
		t.Fatalf("walker %+v, flat book %+v", got, want)
	}
}

func TestSwapTickArrayStarts(t *testing.T) {
	book := append([]walkerTick{
		{index: 700, net: -100_000_000_000},
		{index: 2500, net: -100_000_000_000},
	}, walkerBook...)
	load := loaderOf(walkerArrays(book))

	tests := []struct {
		name        string
		tickCurrent int32
		zeroForOne  bool
		maxArrays   int
		want        []int32
	}{
		{"down, current array missing", 0, true, 3, []int32{-600, -1800, -3000}},
		{"down, capped", 0, true, 2, []int32{-600, -1800}},
		{"down, current array first", -1, true, 3, []int32{-600, -1800, -3000}},
		{"up, runs out", 0, false, 3, []int32{600, 2400}},
		{"up, current array first", 650, false, 3, []int32{600, 2400}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SwapTickArrayStarts(tt.tickCurrent, walkerSpacing, tt.zeroForOne, tt.maxArrays, load)
			if err != nil {
				t.Fatalf("SwapTickArrayStarts: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("starts %v, want %v", got, tt.want)
			}
		})
	}
}
