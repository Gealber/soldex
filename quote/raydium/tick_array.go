package raydium

import (
	"github.com/Gealber/soldex/models"
	bin "github.com/gagliardetto/binary"
)

// ArrayTick is the part of a tick a quote reads.
type ArrayTick struct {
	HasLiquidity       bool
	LiquidityNet       bin.Int128
	LimitOrderUnfilled uint64
}

// TickArray is one TickArrayState reduced to what a quote reads.
type TickArray struct {
	StartTickIndex int32
	Ticks          [models.RaydiumTicksPerArray]ArrayTick
}

// TickArrayFromModel keeps each tick's liquidity and resting limit orders.
func TickArrayFromModel(tickArray *models.RaydiumTickArray) TickArray {
	ticks := TickArray{StartTickIndex: tickArray.StartTickIndex}
	for i, tick := range tickArray.Ticks {
		ticks.Ticks[i] = ArrayTick{
			HasLiquidity:       tick.HasLiquidity(),
			LiquidityNet:       tick.LiquidityNet,
			LimitOrderUnfilled: tick.LimitOrderUnfilled(),
		}
	}
	return ticks
}

// TickArrayLoader returns the first initialized tick array starting at or past
// startTickIndex in the swap direction, and false when there is none. Initialized is
// the pool bitmap's bit, set while the array's initialized_tick_count is non-zero.
type TickArrayLoader func(startTickIndex int32, zeroForOne bool) (TickArray, bool, error)

// SwapTickArrayStarts returns the start indexes of the first maxArrays initialized tick
// arrays swap_v2 walks, in order, beginning at the array holding tickCurrent.
func SwapTickArrayStarts(tickCurrent int32, tickSpacing uint16, zeroForOne bool, maxArrays int, load TickArrayLoader) ([]int32, error) {
	walker := NewTickArrayWalker(tickCurrent, tickSpacing, maxArrays, load)
	starts := make([]int32, 0, maxArrays)
	for i := 0; i < maxArrays; i++ {
		tickArray, ok := walker.array(i, zeroForOne)
		if !ok {
			break
		}
		starts = append(starts, tickArray.StartTickIndex)
	}
	return starts, walker.Err()
}

// TickArrayWalker is a TickProvider over the initialized tick arrays one swap_v2 is
// given, built per quote and never shared. Past maxArrays it reports no boundary, so
// QuoteExactInDetailed consumes less than the input instead of pricing liquidity the
// swap cannot reach.
type TickArrayWalker struct {
	tickCurrent int32
	tickSpacing int32
	maxArrays   int
	load        TickArrayLoader

	arrays  []TickArray
	current int
	done    bool
	err     error
}

// NewTickArrayWalker walks up to maxArrays initialized arrays from the one holding tickCurrent.
func NewTickArrayWalker(tickCurrent int32, tickSpacing uint16, maxArrays int, load TickArrayLoader) *TickArrayWalker {
	return &TickArrayWalker{
		tickCurrent: tickCurrent,
		tickSpacing: int32(tickSpacing),
		maxArrays:   maxArrays,
		load:        load,
	}
}

// Err is the first loader error; it stops the walk.
func (w *TickArrayWalker) Err() error {
	return w.err
}

// Next mirrors the swap_internal tick search: zeroForOne searches down from fromTick
// inclusive, else up exclusive, moving to the next initialized array when one runs out.
func (w *TickArrayWalker) Next(fromTick int32, zeroForOne bool) (TickBoundary, bool) {
	for {
		tickArray, ok := w.array(w.current, zeroForOne)
		if !ok {
			return TickBoundary{}, false
		}
		if boundary, found := w.search(tickArray, fromTick, zeroForOne); found {
			return boundary, true
		}
		w.current++
	}
}

func (w *TickArrayWalker) search(tickArray TickArray, fromTick int32, zeroForOne bool) (TickBoundary, bool) {
	if zeroForOne {
		for i := models.RaydiumTicksPerArray - 1; i >= 0; i-- {
			tickIndex := tickArray.StartTickIndex + int32(i)*w.tickSpacing
			if tickIndex <= fromTick && w.stops(tickArray.Ticks[i]) {
				return w.boundary(tickArray.Ticks[i], tickIndex), true
			}
		}
		return TickBoundary{}, false
	}

	for i := 0; i < models.RaydiumTicksPerArray; i++ {
		tickIndex := tickArray.StartTickIndex + int32(i)*w.tickSpacing
		if tickIndex > fromTick && w.stops(tickArray.Ticks[i]) {
			return w.boundary(tickArray.Ticks[i], tickIndex), true
		}
	}
	return TickBoundary{}, false
}

// stops mirrors TickState::is_initialized: liquidity or resting limit orders.
func (w *TickArrayWalker) stops(tick ArrayTick) bool {
	return tick.HasLiquidity || tick.LimitOrderUnfilled > 0
}

func (w *TickArrayWalker) boundary(tick ArrayTick, tickIndex int32) TickBoundary {
	return TickBoundary{
		TickIndex:          tickIndex,
		LiquidityNet:       tick.LiquidityNet.BigInt(),
		Initialized:        tick.HasLiquidity,
		LimitOrderUnfilled: tick.LimitOrderUnfilled,
	}
}

// array returns the i-th initialized array in the swap direction, loading it on first use.
func (w *TickArrayWalker) array(i int, zeroForOne bool) (TickArray, bool) {
	for len(w.arrays) <= i {
		if w.done || w.err != nil || len(w.arrays) == w.maxArrays {
			return TickArray{}, false
		}

		start := models.RaydiumTickArrayStartIndex(w.tickCurrent, uint16(w.tickSpacing))
		if len(w.arrays) > 0 {
			start = w.nextStart(w.arrays[len(w.arrays)-1].StartTickIndex, zeroForOne)
		}
		tickArray, found, err := w.load(start, zeroForOne)
		if err != nil {
			w.err = err
			return TickArray{}, false
		}
		if !found {
			w.done = true
			return TickArray{}, false
		}
		w.arrays = append(w.arrays, tickArray)
	}
	return w.arrays[i], true
}

// nextStart is the array start one span past start in the swap direction.
func (w *TickArrayWalker) nextStart(start int32, zeroForOne bool) int32 {
	span := w.tickSpacing * models.RaydiumTicksPerArray
	if zeroForOne {
		return start - span
	}
	return start + span
}
