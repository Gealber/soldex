package orca

import (
	"github.com/Gealber/soldex/models"
	bin "github.com/gagliardetto/binary"
)

// SwapTickArrays is how many tick arrays swap_v2 takes; a quote walking further prices
// liquidity the swap cannot reach.
const SwapTickArrays = 3

// Whirlpool tick bounds (tick.rs MIN_TICK_INDEX / MAX_TICK_INDEX).
const (
	minTickIndex = -443636
	maxTickIndex = 443636
)

// ArrayTick is the part of a tick a quote reads.
type ArrayTick struct {
	Initialized  bool
	LiquidityNet bin.Int128
}

type TickArray [models.TicksPerArray]ArrayTick

// TickArrayFromModel keeps each tick's initialized flag and liquidity net.
func TickArrayFromModel(tickArray *models.TickArray) TickArray {
	var ticks TickArray
	for i, tick := range tickArray.Ticks {
		ticks[i] = ArrayTick{Initialized: tick.Initialized, LiquidityNet: tick.LiquidityNet}
	}
	return ticks
}

// TickArrayLoader returns the tick array starting at startTickIndex, and false when the
// account does not exist. swap_v2 reads a missing array as empty (sparse_swap.rs).
type TickArrayLoader func(startTickIndex int32) (TickArray, bool, error)

// SwapTickArrayStarts mirrors get_start_tick_indexes in sparse_swap.rs: the start indexes
// swap_v2 walks, in order. b-to-a starts one array up when the next tick is past this one.
func SwapTickArrayStarts(tickCurrentIndex int32, tickSpacing uint16, aToB bool) []int32 {
	span := int32(tickSpacing) * models.TicksPerArray
	base := models.TickArrayStartIndex(tickCurrentIndex, tickSpacing)

	offsets := [SwapTickArrays]int32{0, -1, -2}
	if !aToB {
		offsets = [SwapTickArrays]int32{0, 1, 2}
		if tickCurrentIndex+int32(tickSpacing) >= base+span {
			offsets = [SwapTickArrays]int32{1, 2, 3}
		}
	}

	starts := make([]int32, 0, SwapTickArrays)
	for _, offset := range offsets {
		start := base + offset*span
		if validStartTick(start, tickSpacing) {
			starts = append(starts, start)
		}
	}
	return starts
}

// validStartTick mirrors Tick::check_is_valid_start_tick: the left-edge array may start
// below the min tick, no other array may leave the tick range.
func validStartTick(start int32, tickSpacing uint16) bool {
	span := int32(tickSpacing) * models.TicksPerArray
	if start < minTickIndex || start > maxTickIndex {
		if start > minTickIndex {
			return false
		}
		return start == minTickIndex-(minTickIndex%span+span)
	}
	return start%span == 0
}

// TickArrayWalker is a TickProvider over the tick arrays one swap_v2 walks, built per quote
// and never shared. Past those arrays it reports no boundary, so QuoteExactInDetailed
// consumes less than the input instead of pricing liquidity the swap cannot reach.
type TickArrayWalker struct {
	tickSpacing int32
	starts      []int32
	load        TickArrayLoader

	start  int32
	loaded bool
	ticks  TickArray
	err    error
}

// NewTickArrayWalker walks the arrays SwapTickArrayStarts gives for the pool and direction.
func NewTickArrayWalker(tickCurrentIndex int32, tickSpacing uint16, aToB bool, load TickArrayLoader) *TickArrayWalker {
	return &TickArrayWalker{
		tickSpacing: int32(tickSpacing),
		starts:      SwapTickArrayStarts(tickCurrentIndex, tickSpacing, aToB),
		load:        load,
	}
}

// Err is the first loader error; it stops the walk.
func (w *TickArrayWalker) Err() error {
	return w.err
}

// Next mirrors the whirlpool tick search: a-to-b searches down from fromTick inclusive,
// b-to-a up from the next tick, and an array with no initialized tick yields its edge.
func (w *TickArrayWalker) Next(fromTick int32, aToB bool) (TickBoundary, bool) {
	if w.err != nil {
		return TickBoundary{}, false
	}

	tick := fromTick
	if !aToB {
		tick = w.floorToSpacing(fromTick) + w.tickSpacing
	}

	start := models.TickArrayStartIndex(tick, uint16(w.tickSpacing))
	if !w.reachable(start) || !w.loadArray(start) {
		return TickBoundary{}, false
	}

	offset := int((tick - start) / w.tickSpacing)
	if aToB {
		for i := offset; i >= 0; i-- {
			if w.ticks[i].Initialized {
				return w.boundary(start, i), true
			}
		}
		return TickBoundary{TickIndex: start}, true
	}

	for i := offset; i < models.TicksPerArray; i++ {
		if w.ticks[i].Initialized {
			return w.boundary(start, i), true
		}
	}
	return TickBoundary{TickIndex: start + (models.TicksPerArray-1)*w.tickSpacing}, true
}

func (w *TickArrayWalker) boundary(start int32, offset int) TickBoundary {
	return TickBoundary{
		TickIndex:    start + int32(offset)*w.tickSpacing,
		LiquidityNet: w.ticks[offset].LiquidityNet.BigInt(),
		Initialized:  true,
	}
}

func (w *TickArrayWalker) reachable(start int32) bool {
	for _, reachable := range w.starts {
		if start == reachable {
			return true
		}
	}
	return false
}

// loadArray reads the array starting at start unless it is the one already held.
func (w *TickArrayWalker) loadArray(start int32) bool {
	if w.loaded && w.start == start {
		return true
	}

	ticks, found, err := w.load(start)
	if err != nil {
		w.err = err
		return false
	}
	if !found {
		ticks = TickArray{}
	}

	w.ticks = ticks
	w.start = start
	w.loaded = true
	return true
}

// floorToSpacing rounds tick down to a multiple of the tick spacing, negatives included.
func (w *TickArrayWalker) floorToSpacing(tick int32) int32 {
	floored := tick / w.tickSpacing * w.tickSpacing
	if tick < 0 && tick%w.tickSpacing != 0 {
		floored -= w.tickSpacing
	}
	return floored
}
