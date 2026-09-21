package models

import (
	"errors"
	"math"
	"math/big"
)

const (
	// dammFeeDenominator is cp-amm's FEE_DENOMINATOR.
	dammFeeDenominator = 1_000_000_000
	// bpsToFeeNumerator converts basis points to fee-numerator units.
	bpsToFeeNumerator = dammFeeDenominator / 10_000
)

// ErrNotRateLimiter is returned when the rate-limiter fee is asked of a pool
// that does not run one.
var ErrNotRateLimiter = errors.New("damm: pool is not in rate limiter mode")

// ErrRateLimiterNeedsAmount is returned by CurrentBaseFeeNumerator for a
// rate-limited pool. A rate limiter has no single fee numerator: the rate rises
// with the size of the swap, so the fee can only be stated for a given input.
// Use RateLimiterFee instead.
var ErrRateLimiterNeedsAmount = errors.New("damm: rate limiter fee depends on the input amount, use RateLimiterFee")

// RateLimiterApplies reports whether the rate limiter is in force for this swap:
// inside the window opening at the pool's activation point, and only B->A (quote
// in, base out). Outside either, the pool charges its flat cliff fee.
//
// The IDL documents the fee curve but not this gating, so re-check it first if
// a quote ever disagrees with the chain.
func (f DAMMBaseFee) RateLimiterApplies(currentPoint, activationPoint uint64, isBtoA bool) bool {
	if f.Mode != DAMMBaseFeeModeRateLimiter || !isBtoA {
		return false
	}
	if f.ReferenceAmount == 0 || f.MaxLimiterDuration == 0 {
		return false
	}
	if currentPoint < activationPoint {
		return false
	}
	return currentPoint-activationPoint <= uint64(f.MaxLimiterDuration)
}

// RateLimiterFee returns the base trading fee IN TOKENS charged on inputAmount by
// a rate-limited pool. Callers should gate it on RateLimiterApplies; outside the
// window or direction the pool charges CliffFeeNumerator flat.
//
// The curve is BorshFeeRateLimiter's, from the cp-amm IDL. With x0 =
// reference_amount, c = cliff_fee_numerator and i = fee_increment, the input is
// charged in x0-sized chunks at a rate climbing by i per chunk, capped at MAX_FEE:
//
//	input <= x0                  fee = input * c
//	input  > x0, input = x0 + (a*x0 + b):
//	  a <  max_index             fee = x0*(c + c*a + i*a*(a+1)/2) + b*(c + i*(a+1))
//	  a >= max_index, a = max_index + d:
//	                             fee = x0*(c + c*max_index + i*max_index*(max_index+1)/2)
//	                                   + (d*x0 + b) * MAX_FEE
//
// max_index = (MAX_FEE - c) / i is the last chunk before the cap binds. The
// result is divided by FEE_DENOMINATOR, rounding UP to match
// GetExcludedFeeAmount. A degenerate config charges the flat cliff fee.
func (f DAMMBaseFee) RateLimiterFee(inputAmount uint64) (uint64, error) {
	if f.Mode != DAMMBaseFeeModeRateLimiter {
		return 0, ErrNotRateLimiter
	}

	c := new(big.Int).SetUint64(f.CliffFeeNumerator)
	x0 := new(big.Int).SetUint64(f.ReferenceAmount)
	incr := new(big.Int).SetUint64(uint64(f.FeeIncrementBps) * bpsToFeeNumerator)
	maxFee := new(big.Int).SetUint64(uint64(f.MaxFeeBps) * bpsToFeeNumerator)
	in := new(big.Int).SetUint64(inputAmount)

	flat := func() (uint64, error) { return ceilDivFeeDenominator(new(big.Int).Mul(in, c)) }

	// No reference amount, no increment, or a cap at or below the cliff all mean
	// the schedule can never raise the fee.
	if x0.Sign() == 0 || incr.Sign() == 0 || maxFee.Cmp(c) <= 0 {
		return flat()
	}
	if in.Cmp(x0) <= 0 {
		return flat()
	}

	delta := new(big.Int).Sub(in, x0)
	a, b := new(big.Int), new(big.Int)
	a.QuoRem(delta, x0, b)
	maxIndex := new(big.Int).Div(new(big.Int).Sub(maxFee, c), incr)

	// chunkSum(n) = c + c*n + i*n*(n+1)/2 — the per-x0 rate summed over chunks 0..n.
	chunkSum := func(n *big.Int) *big.Int {
		nPlus1 := new(big.Int).Add(n, big.NewInt(1))
		tri := new(big.Int).Mul(n, nPlus1)
		tri.Rsh(tri, 1) // n*(n+1) is always even
		tri.Mul(tri, incr)
		return new(big.Int).Add(new(big.Int).Add(c, new(big.Int).Mul(c, n)), tri)
	}

	total := new(big.Int)
	if a.Cmp(maxIndex) < 0 {
		total.Mul(x0, chunkSum(a))
		rate := new(big.Int).Add(c, new(big.Int).Mul(incr, new(big.Int).Add(a, big.NewInt(1))))
		total.Add(total, new(big.Int).Mul(b, rate))
	} else {
		total.Mul(x0, chunkSum(maxIndex))
		rest := new(big.Int).Mul(new(big.Int).Sub(a, maxIndex), x0)
		rest.Add(rest, b)
		total.Add(total, new(big.Int).Mul(rest, maxFee))
	}
	return ceilDivFeeDenominator(total)
}

// ceilDivFeeDenominator divides a fee-numerator product by FEE_DENOMINATOR,
// rounding up, and reports rather than truncates a result too large for a u64.
func ceilDivFeeDenominator(n *big.Int) (uint64, error) {
	den := big.NewInt(dammFeeDenominator)
	q, r := new(big.Int).QuoRem(n, den, new(big.Int))
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsUint64() || q.Uint64() > math.MaxUint64 {
		return 0, ErrValueOutOfRange
	}
	return q.Uint64(), nil
}
