package orca

import "math/big"

// Adaptive-fee model, ported from Orca whirlpool program
// (state/oracle.rs + manager/fee_rate_manager.rs). Whirlpools created with an
// adaptive-fee tier charge a static base fee PLUS a volatility-driven surcharge
// that grows as the swap crosses tick groups and decays over time. The quote
// previously used only the static FeeRate, systematically over-quoting Orca
// legs (the surcharge peaks exactly during the volatility that creates a spread,
// which is when we fire), so detected round trips reverted on-chain.
const (
	// volatilityAccumulatorScaleFactor scales a 1-tick-group move to 10_000 so the
	// reduction factor doesn't decay a small accumulator straight to zero.
	volatilityAccumulatorScaleFactor uint32 = 10_000
	// reductionFactorDenominator: reduction_factor of 5_000 means 0.5.
	reductionFactorDenominator uint64 = 10_000
	// adaptiveFeeControlFactorDenominator: control_factor of 1_000 means 0.01.
	adaptiveFeeControlFactorDenominator uint64 = 100_000
	// maxReferenceAge forcibly resets a too-old reference (anti-DoS), seconds.
	maxReferenceAge uint64 = 3_600
	// feeRateHardLimit caps the total fee rate at 10% (hundredths of a bp).
	feeRateHardLimit uint32 = 100_000
)

// AdaptiveFeeConstants is the per-pool adaptive-fee configuration stored in the
// Oracle account (set once at tier init). Field order mirrors the on-chain
// AdaptiveFeeConstants for direct decode.
type AdaptiveFeeConstants struct {
	FilterPeriod             uint16
	DecayPeriod              uint16
	ReductionFactor          uint16
	AdaptiveFeeControlFactor uint32
	MaxVolatilityAccumulator uint32
	TickGroupSize            uint16
	MajorSwapThresholdTicks  uint16
}

// AdaptiveFeeVariables is the mutable adaptive-fee state in the Oracle account,
// updated on every swap. Field order mirrors the on-chain layout.
type AdaptiveFeeVariables struct {
	LastReferenceUpdateTimestamp uint64
	LastMajorSwapTimestamp       uint64
	VolatilityReference          uint32
	TickGroupIndexReference      int32
	VolatilityAccumulator        uint32
}

// AdaptiveFeeInfo bundles the constants and variables; nil means a static-fee
// pool (no adaptive surcharge).
type AdaptiveFeeInfo struct {
	Constants AdaptiveFeeConstants
	Variables AdaptiveFeeVariables
}

// floorDivision mirrors math::floor_division (truncates toward negative infinity
// for a positive divisor).
func floorDivision(dividend, divisor int32) int32 {
	if dividend%divisor == 0 || (dividend < 0) == (divisor < 0) {
		return dividend / divisor
	}
	return dividend/divisor - 1
}

// ceilDivisionU32 mirrors math::ceil_division_u32.
func ceilDivisionU32(dividend, divisor uint32) uint32 {
	q := dividend / divisor
	if q*divisor == dividend {
		return q
	}
	return q + 1
}

// updateReference decays the volatility reference based on time since the last
// reference/major-swap update. Mirrors AdaptiveFeeVariables::update_reference.
// Returns false on an out-of-order timestamp (the swap would revert on-chain).
func (v *AdaptiveFeeVariables) updateReference(tickGroupIndex int32, currentTimestamp uint64, c *AdaptiveFeeConstants) bool {
	maxTimestamp := v.LastReferenceUpdateTimestamp
	if v.LastMajorSwapTimestamp > maxTimestamp {
		maxTimestamp = v.LastMajorSwapTimestamp
	}
	if currentTimestamp < maxTimestamp {
		return false
	}

	if currentTimestamp-v.LastReferenceUpdateTimestamp > maxReferenceAge {
		v.TickGroupIndexReference = tickGroupIndex
		v.VolatilityReference = 0
		v.LastReferenceUpdateTimestamp = currentTimestamp
		return true
	}

	elapsed := currentTimestamp - maxTimestamp
	switch {
	case elapsed < uint64(c.FilterPeriod):
		// high-frequency trade: no change
	case elapsed < uint64(c.DecayPeriod):
		v.TickGroupIndexReference = tickGroupIndex
		v.VolatilityReference = uint32(uint64(v.VolatilityAccumulator) * uint64(c.ReductionFactor) / reductionFactorDenominator)
		v.LastReferenceUpdateTimestamp = currentTimestamp
	default:
		v.TickGroupIndexReference = tickGroupIndex
		v.VolatilityReference = 0
		v.LastReferenceUpdateTimestamp = currentTimestamp
	}
	return true
}

// updateVolatilityAccumulator recomputes the accumulator for the current tick
// group distance from the reference. Mirrors update_volatility_accumulator.
func (v *AdaptiveFeeVariables) updateVolatilityAccumulator(tickGroupIndex int32, c *AdaptiveFeeConstants) {
	delta := v.TickGroupIndexReference - tickGroupIndex
	if delta < 0 {
		delta = -delta
	}
	acc := uint64(v.VolatilityReference) + uint64(uint32(delta))*uint64(volatilityAccumulatorScaleFactor)
	if max := uint64(c.MaxVolatilityAccumulator); acc > max {
		acc = max
	}
	v.VolatilityAccumulator = uint32(acc)
}

// computeAdaptiveFeeRate maps the squared volatility accumulator to a fee rate
// (hundredths of a bp), capped at the hard limit. Mirrors compute_adaptive_fee_rate.
func computeAdaptiveFeeRate(c *AdaptiveFeeConstants, v *AdaptiveFeeVariables) uint32 {
	crossed := uint64(v.VolatilityAccumulator) * uint64(c.TickGroupSize)
	squared := new(big.Int).SetUint64(crossed)
	squared.Mul(squared, squared)

	num := new(big.Int).SetUint64(uint64(c.AdaptiveFeeControlFactor))
	num.Mul(num, squared)
	denom := new(big.Int).SetUint64(adaptiveFeeControlFactorDenominator)
	denom.Mul(denom, new(big.Int).SetUint64(uint64(volatilityAccumulatorScaleFactor)))
	denom.Mul(denom, new(big.Int).SetUint64(uint64(volatilityAccumulatorScaleFactor)))

	// ceil division
	q := new(big.Int).Quo(num, denom)
	if new(big.Int).Mul(q, denom).Cmp(num) != 0 {
		q.Add(q, big.NewInt(1))
	}
	if q.Cmp(new(big.Int).SetUint64(uint64(feeRateHardLimit))) > 0 {
		return feeRateHardLimit
	}
	return uint32(q.Uint64())
}
