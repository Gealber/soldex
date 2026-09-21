package models

import (
	dammmath "github.com/Gealber/soldex/math/damm"
)

// CurrentBaseFeeNumerator returns the base fee the pool charges AT currentPoint,
// out of FEE_DENOMINATOR (1e9).
//
// currentPoint must be expressed in the pool's own activation unit: a slot when
// ActivationType is 0 and a unix timestamp when it is 1. Passing the wrong unit
// silently yields a wildly wrong period, so callers should read it off the same
// clock the pool was activated against.
//
// This is what the swap actually charges. The cliff alone (what
// DAMMPool.TradingFeeNumerator carries) is only right for a static pool; a
// scheduled pool has since moved off it. Roughly one live pool in seven is
// scheduled.
func (f DAMMBaseFee) CurrentBaseFeeNumerator(currentPoint, activationPoint uint64) (uint64, error) {
	switch f.Mode {
	case DAMMBaseFeeModeTimeLinear, DAMMBaseFeeModeTimeExponential:
		if f.IsStatic() {
			return f.CliffFeeNumerator, nil
		}
		period := timeSchedulerPeriod(f, currentPoint, activationPoint)
		if f.Mode == DAMMBaseFeeModeTimeLinear {
			return dammmath.GetFeeInPeriodLinear(f.CliffFeeNumerator, f.ReductionFactor, period), nil
		}
		return dammmath.GetFeeInPeriod(f.CliffFeeNumerator, f.ReductionFactor, period)
	case DAMMBaseFeeModeRateLimiter:
		// A rate limiter has no single numerator — the rate depends on the size of
		// the swap. RateLimiterFee answers for a given input.
		return 0, ErrRateLimiterNeedsAmount
	default:
		// The two market-cap schedulers are not modelled yet. See
		// ErrUnsupportedBaseFeeMode for why this is not a cliff fallback.
		return 0, ErrUnsupportedBaseFeeMode
	}
}

// timeSchedulerPeriod is how many whole periods have elapsed since activation,
// capped at the schedule's length. A pool that has not activated yet sits at
// period 0 (the cliff).
func timeSchedulerPeriod(f DAMMBaseFee, currentPoint, activationPoint uint64) uint16 {
	if currentPoint <= activationPoint || f.PeriodFrequency == 0 {
		return 0
	}
	elapsed := (currentPoint - activationPoint) / f.PeriodFrequency
	if elapsed >= uint64(f.NumberOfPeriod) {
		return f.NumberOfPeriod
	}
	return uint16(elapsed)
}

// BaseFee parses the pool's base fee schedule out of its raw BaseFeeInfo blob.
func (p *DAMMPool) BaseFee() DAMMBaseFee {
	return ParseDAMMBaseFee(p.PoolFees.BaseFee.BaseFeeInfo)
}

// CurrentBaseFeeNumerator is a convenience wrapper that resolves the pool's fee
// at currentPoint against its own activation point.
func (p *DAMMPool) CurrentBaseFeeNumerator(currentPoint uint64) (uint64, error) {
	return p.BaseFee().CurrentBaseFeeNumerator(currentPoint, p.ActivationPoint)
}
