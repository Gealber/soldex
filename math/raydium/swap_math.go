package raydium

import (
	"math"
	"math/big"
)

// FeeRateDenominator is the fee-rate denominator: trade_fee_rate is in hundredths
// of a basis point, so the fee fraction is trade_fee_rate / 1_000_000.
const FeeRateDenominator = 1_000_000

var maxU64Big = new(big.Int).SetUint64(math.MaxUint64)

// SwapStep is one compute_swap result. AmountIn is the pre-fee input consumed;
// the total input drawn from the remaining amount is AmountIn + FeeAmount.
type SwapStep struct {
	SqrtPriceNext *big.Int
	AmountIn      *big.Int
	AmountOut     *big.Int
	FeeAmount     *big.Int
}

// ComputeSwapStep mirrors swap_math::compute_swap for the exact-input,
// fee-on-input path (is_base_input = is_fee_on_input = true) — the standard
// Raydium CLMM swap (pool fee_on = FromInput). zeroForOne sells token_0 for
// token_1 (price decreasing). Pools configured with token-only fee collection
// (fee_on 1/2) are not modelled in this path.
func ComputeSwapStep(sqrtCurrent, sqrtTarget, liquidity *big.Int, amountRemaining uint64, feeRate uint32, zeroForOne bool) SwapStep {
	// Take the fee off the gross input before moving the price.
	remaining := new(big.Int).SetUint64(amountRemaining)
	amountForCalc := new(big.Int).Mul(remaining, big.NewInt(int64(FeeRateDenominator-feeRate)))
	amountForCalc.Quo(amountForCalc, big.NewInt(FeeRateDenominator))

	// Input needed to reach the target tick (round up); nil if it exceeds u64,
	// matching calculate_amount_in_range returning None on MaxTokenOverflow.
	amountToTarget := calcAmountInToTarget(sqrtCurrent, sqrtTarget, liquidity, zeroForOne)
	reachesTarget := amountToTarget != nil && amountForCalc.Cmp(amountToTarget) >= 0

	var next *big.Int
	if reachesTarget {
		next = new(big.Int).Set(sqrtTarget)
	} else {
		next = NextSqrtPriceFromInput(sqrtCurrent, liquidity, amountForCalc.Uint64(), zeroForOne)
	}

	max := sqrtTarget.Cmp(next) == 0

	var amountIn, amountOut *big.Int
	if zeroForOne {
		if max {
			amountIn = amountToTarget // exact input to target
		} else {
			amountIn = GetDeltaAmount0Unsigned(next, sqrtCurrent, liquidity, true)
		}
		amountOut = GetDeltaAmount1Unsigned(next, sqrtCurrent, liquidity, false)
	} else {
		if max {
			amountIn = amountToTarget
		} else {
			amountIn = GetDeltaAmount1Unsigned(sqrtCurrent, next, liquidity, true)
		}
		amountOut = GetDeltaAmount0Unsigned(sqrtCurrent, next, liquidity, false)
	}

	var fee *big.Int
	if !max {
		// Didn't reach the target: the user pays all the remaining input, the
		// part beyond amount_in is the fee.
		fee = new(big.Int).Sub(remaining, amountIn)
	} else {
		num := new(big.Int).Mul(amountIn, big.NewInt(int64(feeRate)))
		fee = ceilDiv(num, big.NewInt(int64(FeeRateDenominator-feeRate)))
	}

	return SwapStep{SqrtPriceNext: next, AmountIn: amountIn, AmountOut: amountOut, FeeAmount: fee}
}

// calcAmountInToTarget mirrors calculate_amount_in_range for is_base_input=true:
// the round-up input delta to move from current to target, or nil when it
// overflows u64 (the None case the caller treats as "target not reachable").
func calcAmountInToTarget(current, target, liquidity *big.Int, zeroForOne bool) *big.Int {
	var v *big.Int
	if zeroForOne {
		v = GetDeltaAmount0Unsigned(target, current, liquidity, true)
	} else {
		v = GetDeltaAmount1Unsigned(current, target, liquidity, true)
	}
	if v.Cmp(maxU64Big) > 0 {
		return nil
	}
	return v
}
