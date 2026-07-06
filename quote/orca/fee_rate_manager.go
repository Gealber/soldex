package orca

import (
	"math/big"

	orcamath "github.com/Gealber/soldex/math/orca"
)

// feeRateManager mirrors Orca's FeeRateManager: it tracks the adaptive-fee state
// across a swap and yields the per-step total fee rate. A static-fee pool
// (adaptive==false) always returns the static rate and never bounds the target.
type feeRateManager struct {
	adaptive       bool
	aToB           bool
	tickGroupIndex int32
	staticFeeRate  uint16
	constants      AdaptiveFeeConstants
	variables      AdaptiveFeeVariables
	coreLower      *tickGroupBound
	coreUpper      *tickGroupBound
}

// tickGroupBound is one edge of the core tick-group range (the band where the
// volatility accumulator has not yet saturated to max). Outside it the rate is
// constant, so the swap step can skip straight to the edge.
type tickGroupBound struct {
	index     int32
	sqrtPrice *big.Int
}

// newFeeRateManager constructs the manager. info==nil yields a static manager;
// otherwise it primes the reference (decaying volatility for elapsed time) and
// precomputes the core tick-group range. ok=false on an out-of-order timestamp.
func newFeeRateManager(aToB bool, currentTickIndex int32, timestamp uint64, staticFeeRate uint16, info *AdaptiveFeeInfo) (feeRateManager, bool) {
	if info == nil {
		return feeRateManager{adaptive: false, staticFeeRate: staticFeeRate}, true
	}
	c := info.Constants
	v := info.Variables
	tickGroupIndex := floorDivision(currentTickIndex, int32(c.TickGroupSize))
	if !v.updateReference(tickGroupIndex, timestamp, &c) {
		return feeRateManager{}, false
	}

	// Beyond this many tick groups from the reference, the accumulator is pinned
	// at max_volatility_accumulator, so the rate is flat and steps can be skipped.
	maxDelta := ceilDivisionU32(c.MaxVolatilityAccumulator-v.VolatilityReference, volatilityAccumulatorScaleFactor)
	lowerIndex := v.TickGroupIndexReference - int32(maxDelta)
	upperIndex := v.TickGroupIndexReference + int32(maxDelta)
	lowerTick := lowerIndex * int32(c.TickGroupSize)
	upperTick := upperIndex*int32(c.TickGroupSize) + int32(c.TickGroupSize)

	mgr := feeRateManager{
		adaptive:       true,
		aToB:           aToB,
		tickGroupIndex: tickGroupIndex,
		staticFeeRate:  staticFeeRate,
		constants:      c,
		variables:      v,
	}
	if lowerTick > orcamath.MinTickIndex {
		mgr.coreLower = &tickGroupBound{lowerIndex, orcamath.SqrtPriceFromTickIndex(lowerTick)}
	}
	if upperTick < orcamath.MaxTickIndex {
		mgr.coreUpper = &tickGroupBound{upperIndex, orcamath.SqrtPriceFromTickIndex(upperTick)}
	}
	return mgr, true
}

// updateVolatilityAccumulator recomputes the accumulator for the current tick
// group (called at the start of every swap step).
func (m *feeRateManager) updateVolatilityAccumulator() {
	if !m.adaptive {
		return
	}
	m.variables.updateVolatilityAccumulator(m.tickGroupIndex, &m.constants)
}

// totalFeeRate is static + adaptive, capped at the hard limit (hundredths of bp).
func (m *feeRateManager) totalFeeRate() uint32 {
	if !m.adaptive {
		return uint32(m.staticFeeRate)
	}
	total := uint32(m.staticFeeRate) + computeAdaptiveFeeRate(&m.constants, &m.variables)
	if total > feeRateHardLimit {
		return feeRateHardLimit
	}
	return total
}

// advanceTickGroup shifts the tick group one step in the swap direction (no-skip).
func (m *feeRateManager) advanceTickGroup() {
	if !m.adaptive {
		return
	}
	if m.aToB {
		m.tickGroupIndex--
	} else {
		m.tickGroupIndex++
	}
}

// boundedSqrtPriceTarget bounds the next swap step to the current tick group's
// boundary so the fee rate is constant within the step. Returns (target, skip);
// skip==true means the adaptive rate is flat to the given target (control factor
// 0, zero liquidity, or outside the core range) and advanceTickGroupAfterSkip
// must be used instead of advanceTickGroup.
func (m *feeRateManager) boundedSqrtPriceTarget(sqrtPrice, currLiquidity *big.Int) (*big.Int, bool) {
	if !m.adaptive {
		return sqrtPrice, false
	}
	if m.constants.AdaptiveFeeControlFactor == 0 || currLiquidity.Sign() == 0 {
		return sqrtPrice, true
	}
	if m.coreLower != nil && m.tickGroupIndex < m.coreLower.index {
		if m.aToB {
			return sqrtPrice, true
		}
		return bigMin(sqrtPrice, m.coreLower.sqrtPrice), true
	}
	if m.coreUpper != nil && m.tickGroupIndex > m.coreUpper.index {
		if m.aToB {
			return bigMax(sqrtPrice, m.coreUpper.sqrtPrice), true
		}
		return sqrtPrice, true
	}

	ts := int32(m.constants.TickGroupSize)
	boundaryTick := m.tickGroupIndex * ts
	if !m.aToB {
		boundaryTick += ts
	}
	boundaryTick = clampTick(boundaryTick)
	boundarySqrtPrice := orcamath.SqrtPriceFromTickIndex(boundaryTick)
	if m.aToB {
		return bigMax(sqrtPrice, boundarySqrtPrice), false
	}
	return bigMin(sqrtPrice, boundarySqrtPrice), false
}

// advanceTickGroupAfterSkip realigns the tick group to the tick group actually
// reached after a skipped step, then shifts one step. Mirrors
// advance_tick_group_after_skip.
func (m *feeRateManager) advanceTickGroupAfterSkip(sqrtPrice, nextTickSqrtPrice *big.Int, nextTickIndex int32) {
	if !m.adaptive {
		return
	}
	ts := int32(m.constants.TickGroupSize)
	var tickIndex int32
	var onBoundary bool
	if sqrtPrice.Cmp(nextTickSqrtPrice) == 0 {
		tickIndex = nextTickIndex
		onBoundary = nextTickIndex%ts == 0
	} else {
		tickIndex = orcamath.TickIndexFromSqrtPrice(sqrtPrice)
		onBoundary = tickIndex%ts == 0 && sqrtPrice.Cmp(orcamath.SqrtPriceFromTickIndex(tickIndex)) == 0
	}

	var lastTraversed int32
	if onBoundary && !m.aToB {
		lastTraversed = tickIndex/ts - 1
	} else {
		lastTraversed = floorDivision(tickIndex, ts)
	}

	if (m.aToB && lastTraversed < m.tickGroupIndex) || (!m.aToB && lastTraversed > m.tickGroupIndex) {
		m.tickGroupIndex = lastTraversed
		m.variables.updateVolatilityAccumulator(m.tickGroupIndex, &m.constants)
	}
	if m.aToB {
		m.tickGroupIndex--
	} else {
		m.tickGroupIndex++
	}
}

// updateMajorSwapTimestamp records the swap time when the price moved at least
// major_swap_threshold_ticks, gating reference decay. Mirrors
// update_major_swap_timestamp (used only to keep the variables faithful when a
// quote is chained; harmless for a single quote).
func (m *feeRateManager) updateMajorSwapTimestamp(timestamp uint64, preSqrtPrice, postSqrtPrice *big.Int) {
	if !m.adaptive {
		return
	}
	if isMajorSwap(preSqrtPrice, postSqrtPrice, m.constants.MajorSwapThresholdTicks) {
		m.variables.LastMajorSwapTimestamp = timestamp
	}
}

// isMajorSwap reports whether the price moved at least the threshold ticks.
func isMajorSwap(preSqrtPrice, postSqrtPrice *big.Int, thresholdTicks uint16) bool {
	smaller, larger := preSqrtPrice, postSqrtPrice
	if smaller.Cmp(larger) > 0 {
		smaller, larger = larger, smaller
	}
	factor := orcamath.SqrtPriceFromTickIndex(int32(thresholdTicks))
	// target = smaller * factor >> 64
	target := new(big.Int).Mul(smaller, factor)
	target.Rsh(target, 64)
	return larger.Cmp(target) >= 0
}

func clampTick(t int32) int32 {
	if t < orcamath.MinTickIndex {
		return orcamath.MinTickIndex
	}
	if t > orcamath.MaxTickIndex {
		return orcamath.MaxTickIndex
	}
	return t
}

func bigMin(a, b *big.Int) *big.Int {
	if a.Cmp(b) <= 0 {
		return a
	}
	return b
}

func bigMax(a, b *big.Int) *big.Int {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}
