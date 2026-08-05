package raydium

import (
	"math/big"

	raymath "github.com/Gealber/soldex/math/raydium"
)

// Denominators from the on-chain dynamic-fee model. The accumulator is scaled by
// 10_000 so repeated decay does not round it to zero.
const (
	volatilityAccumulatorScale   = 10_000
	reductionFactorDenominator   = 10_000
	dynamicFeeControlDenominator = 100_000
	// maxFeeRateNumerator caps the total fee at 10%.
	maxFeeRateNumerator = 100_000
)

// tickSpacingIndexFromTick maps a tick to its tick-spacing group, flooring toward
// negative infinity so groups are contiguous across zero.
func tickSpacingIndexFromTick(tick int32, tickSpacing uint16) int32 {
	spacing := int32(tickSpacing)
	if tick%spacing == 0 || tick >= 0 {
		return tick / spacing
	}
	return tick/spacing - 1
}

// updateReference decays the volatility reference by how long the pool has been
// quiet: untouched inside the filter period, decayed inside the decay period, and
// reset to zero past it. Runs once, before the first step.
func (s *swapState) updateReference(tickSpacingIndex int32, timestamp uint64) {
	if s.dyn == nil {
		return
	}
	var elapsed uint64
	if timestamp > s.dyn.LastUpdateTimestamp {
		elapsed = timestamp - s.dyn.LastUpdateTimestamp
	}
	if elapsed < uint64(s.dyn.FilterPeriod) {
		return
	}
	s.dyn.TickSpacingIndexReference = tickSpacingIndex
	s.dyn.LastUpdateTimestamp = timestamp
	if elapsed < uint64(s.dyn.DecayPeriod) {
		s.dyn.VolatilityReference = uint32(uint64(s.dyn.VolatilityAccumulator) *
			uint64(s.dyn.ReductionFactor) / reductionFactorDenominator)
		return
	}
	s.dyn.VolatilityReference = 0
}

// updateVolatilityAccumulator recomputes the accumulator from how far the price has
// travelled, in tick-spacing groups, since the reference — capped at the pool's
// configured maximum, which is what bounds the fee.
func (s *swapState) updateVolatilityAccumulator() {
	if s.dyn == nil {
		return
	}
	delta := s.dyn.TickSpacingIndexReference - s.tickSpacingIdx
	if delta < 0 {
		delta = -delta
	}
	accumulated := uint64(s.dyn.VolatilityReference) + uint64(delta)*volatilityAccumulatorScale
	if accumulated > uint64(s.dyn.MaxVolatilityAccumulator) {
		accumulated = uint64(s.dyn.MaxVolatilityAccumulator)
	}
	s.dyn.VolatilityAccumulator = uint32(accumulated)
}

// totalFeeRate is the rate this step is charged at: the AmmConfig base fee plus the
// dynamic component, capped at 10%.
func (s *swapState) totalFeeRate() uint32 {
	if s.dyn == nil {
		return s.baseFeeRate
	}
	total := uint64(s.baseFeeRate) + uint64(s.dynamicFeeRate())
	if total > maxFeeRateNumerator {
		return maxFeeRateNumerator
	}
	return uint32(total)
}

// dynamicFeeRate maps the squared volatility accumulator to a fee rate, so the fee
// ramps quadratically with how far the price has moved.
func (s *swapState) dynamicFeeRate() uint32 {
	crossed := new(big.Int).SetUint64(uint64(s.dyn.VolatilityAccumulator) * uint64(s.tickSpacing))
	squared := new(big.Int).Mul(crossed, crossed)

	denominator := big.NewInt(dynamicFeeControlDenominator)
	denominator.Mul(denominator, big.NewInt(volatilityAccumulatorScale))
	denominator.Mul(denominator, big.NewInt(volatilityAccumulatorScale))

	rate := new(big.Int).Mul(big.NewInt(int64(s.dyn.DynamicFeeControl)), squared)
	rate = ceilDivBig(rate, denominator)
	if rate.Cmp(big.NewInt(maxFeeRateNumerator)) > 0 {
		return maxFeeRateNumerator
	}
	return uint32(rate.Uint64())
}

// spacingBoundedPrice bounds a step to the current tick-spacing group so the
// accumulator can be advanced once per group. skipped reports that no bound applies
// — either the pool has no dynamic fee, or the fee is already capped, in which case
// the step runs all the way to target and the group index is recomputed afterwards.
func (s *swapState) spacingBoundedPrice(target *big.Int, zeroForOne bool) (skipped bool, price *big.Int, boundedTick *int32) {
	if s.dyn == nil || s.liquidity.Sign() == 0 ||
		s.dyn.VolatilityAccumulator == s.dyn.MaxVolatilityAccumulator {
		return true, target, nil
	}
	spacing := int32(s.tickSpacing)
	bounded := s.tickSpacingIdx * spacing
	if !zeroForOne {
		bounded = (s.tickSpacingIdx + 1) * spacing
	}
	bounded = min(max(bounded, raymath.MinTick), raymath.MaxTick)
	boundedPrice := raymath.SqrtPriceFromTick(bounded)

	if zeroForOne && target.Cmp(boundedPrice) > 0 {
		return false, target, nil
	}
	if !zeroForOne && target.Cmp(boundedPrice) < 0 {
		return false, target, nil
	}
	return false, boundedPrice, &bounded
}

// updateDynamicFeeIndex advances the tick-spacing group after a step. When the step
// was not bounded to a group it may have skipped several, so the group is recomputed
// from where the price actually landed before advancing.
func (s *swapState) updateDynamicFeeIndex(zeroForOne, skipped bool, tickPrice *big.Int, tickNext int32) {
	if s.dyn == nil {
		return
	}
	if skipped {
		landed := s.tick
		if s.sqrtPrice.Cmp(tickPrice) == 0 {
			landed = tickNext
		}
		index := tickSpacingIndexFromTick(landed, s.tickSpacing)
		if !zeroForOne && landed%int32(s.tickSpacing) == 0 {
			index--
		}
		s.tickSpacingIdx = index
		if s.dyn.VolatilityAccumulator != s.dyn.MaxVolatilityAccumulator {
			s.updateVolatilityAccumulator()
		}
	}
	if zeroForOne {
		s.tickSpacingIdx--
		return
	}
	s.tickSpacingIdx++
}

// ceilDivBig returns ceil(num/den) for non-negative inputs.
func ceilDivBig(num, den *big.Int) *big.Int {
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}
