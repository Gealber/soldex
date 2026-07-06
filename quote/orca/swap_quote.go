// Package orca ports the Orca Whirlpool exact-in swap quote (concentrated
// liquidity, Q64.64). It mirrors swap_manager::swap + math::compute_swap;
// only the exact-in path is implemented.
package orca

import (
	"errors"
	"math"
	"math/big"

	"github.com/Gealber/soldex/math/common"
	orcamath "github.com/Gealber/soldex/math/orca"
)

// feeRateMulValue is the fee-rate denominator: fee_rate is hundredths of a basis
// point, so the fee fraction is fee_rate / 1_000_000.
const feeRateMulValue int64 = 1_000_000

var maxU64 = new(big.Int).SetUint64(math.MaxUint64)

var (
	ErrInvalidPool       = errors.New("orca: invalid pool state")
	ErrAmountOverflow    = errors.New("orca: amount exceeds u64")
	ErrNegativeLiquidity = errors.New("orca: negative liquidity after tick cross")
)

// SwapPool holds the decoded Whirlpool fields needed to quote one exact-in swap.
type SwapPool struct {
	// Q64.64 current sqrt price.
	SqrtPrice *big.Int
	// Active liquidity in the current tick range.
	Liquidity        *big.Int
	TickCurrentIndex int32
	TickSpacing      uint16
	// fee_rate in hundredths of a basis point (1e-6).
	FeeRate uint16
	// AdaptiveFee is the pool's adaptive-fee state (oracle) when it is an
	// adaptive-fee pool; nil means a plain static-fee pool. When set, each swap
	// step charges static + a volatility surcharge that evolves across tick groups.
	AdaptiveFee *AdaptiveFeeInfo
	// Timestamp is the block time used to decay the adaptive-fee reference; the
	// caller passes the current unix time (the program uses the cluster clock).
	Timestamp uint64
}

// TickBoundary is the next price boundary reachable in the swap direction: either
// an initialized tick (crossing it changes liquidity by LiquidityNet) or an
// uninitialized tick-array edge (advance the search without changing liquidity).
type TickBoundary struct {
	TickIndex    int32
	LiquidityNet *big.Int
	Initialized  bool
}

// TickProvider returns the next boundary at or beyond fromTick in the swap
// direction (aToB searches down and is inclusive of fromTick; otherwise up and
// exclusive). ok=false means no further tick array is cached, so the swap stops
// at the edge of known liquidity (mirrors a missing DLMM bin array).
type TickProvider func(fromTick int32, aToB bool) (TickBoundary, bool)

// QuoteExactIn swaps amountIn through the pool, walking initialized ticks until
// the input is consumed or known liquidity runs out. aToB true sells token A for
// token B (price decreasing). Returns the net output amount.
func QuoteExactIn(pool SwapPool, aToB bool, amountIn uint64, ticks TickProvider) (uint64, error) {
	if pool.SqrtPrice == nil || pool.Liquidity == nil {
		return 0, ErrInvalidPool
	}

	limit := orcamath.MaxSqrtPrice
	if aToB {
		limit = orcamath.MinSqrtPrice
	}

	mgr, ok := newFeeRateManager(aToB, pool.TickCurrentIndex, pool.Timestamp, pool.FeeRate, pool.AdaptiveFee)
	if !ok {
		// Out-of-order timestamp vs the oracle reference; the on-chain swap would
		// revert, so refuse to quote rather than under-charge.
		return 0, ErrInvalidPool
	}

	amountRemaining := amountIn
	amountOut := uint64(0)
	sqrtPrice := new(big.Int).Set(pool.SqrtPrice)
	liquidity := new(big.Int).Set(pool.Liquidity)
	currTick := pool.TickCurrentIndex

	// Outer loop walks to the next initialized tick; inner loop runs swap steps
	// bounded to each tick group so the adaptive fee rate is constant per step.
	// Mirrors whirlpool manager::swap.
	for amountRemaining > 0 && sqrtPrice.Cmp(limit) != 0 {
		boundary, ok := ticks(currTick, aToB)
		if !ok {
			break
		}
		nextTickSqrtPrice := orcamath.SqrtPriceFromTickIndex(boundary.TickIndex)
		sqrtPriceTarget := clampTarget(nextTickSqrtPrice, limit, aToB)

		for {
			mgr.updateVolatilityAccumulator()
			totalFeeRate := mgr.totalFeeRate()
			boundedTarget, skip := mgr.boundedSqrtPriceTarget(sqrtPriceTarget, liquidity)

			step, err := computeSwapStep(amountRemaining, totalFeeRate, liquidity, sqrtPrice, boundedTarget, aToB)
			if err != nil {
				return 0, err
			}
			if step.amountIn > amountRemaining || step.feeAmount > amountRemaining-step.amountIn {
				return 0, ErrAmountOverflow
			}
			amountRemaining -= step.amountIn + step.feeAmount
			if amountOut > math.MaxUint64-step.amountOut {
				return 0, ErrAmountOverflow
			}
			amountOut += step.amountOut

			// Cross the initialized tick only when the step actually reached it;
			// a step that stopped at a tick-group boundary just moves the price.
			if step.nextPrice.Cmp(nextTickSqrtPrice) == 0 {
				if boundary.Initialized {
					if err := crossLiquidity(liquidity, boundary.LiquidityNet, aToB); err != nil {
						return 0, err
					}
				}
				// The a_to_b search is inclusive, so shift left by one to advance.
				if aToB {
					currTick = boundary.TickIndex - 1
				} else {
					currTick = boundary.TickIndex
				}
			} else if step.nextPrice.Cmp(sqrtPrice) != 0 {
				currTick = orcamath.TickIndexFromSqrtPrice(step.nextPrice)
			}

			sqrtPrice = step.nextPrice
			if skip {
				mgr.advanceTickGroupAfterSkip(sqrtPrice, nextTickSqrtPrice, boundary.TickIndex)
			} else {
				mgr.advanceTickGroup()
			}

			if amountRemaining == 0 || sqrtPrice.Cmp(sqrtPriceTarget) == 0 {
				break
			}
		}
	}

	return amountOut, nil
}

// crossLiquidity applies signed_liquidity_net = aToB ? -net : +net. Mirrors
// calculate_update.
func crossLiquidity(liquidity, net *big.Int, aToB bool) error {
	if net == nil {
		return nil
	}
	if aToB {
		liquidity.Sub(liquidity, net)
	} else {
		liquidity.Add(liquidity, net)
	}
	if liquidity.Sign() < 0 {
		return ErrNegativeLiquidity
	}
	return nil
}

// swapStep is one compute_swap result.
type swapStep struct {
	amountIn  uint64
	amountOut uint64
	feeAmount uint64
	nextPrice *big.Int
}

// computeSwapStep computes one exact-in step from current toward target. Fee is
// taken on input first (amount_calc), then the curve is run on the post-fee
// amount. Mirrors math::compute_swap with amount_specified_is_input = true.
func computeSwapStep(amountRemaining uint64, feeRate uint32, liquidity, current, target *big.Int, aToB bool) (swapStep, error) {
	amountCalc := new(big.Int).SetUint64(amountRemaining)
	amountCalc.Mul(amountCalc, big.NewInt(feeRateMulValue-int64(feeRate)))
	amountCalc.Quo(amountCalc, big.NewInt(feeRateMulValue))

	initialFixed := fixedDelta(current, target, liquidity, aToB)
	exceedsMax := initialFixed.Cmp(maxU64) > 0

	next := new(big.Int)
	if !exceedsMax && initialFixed.Cmp(amountCalc) <= 0 {
		next.Set(target)
	} else {
		n, err := orcamath.NextSqrtPrice(current, liquidity, amountCalc, true, aToB)
		if err != nil {
			return swapStep{}, err
		}
		next = n
	}

	isMaxSwap := next.Cmp(target) == 0

	fixed := initialFixed
	if !isMaxSwap || exceedsMax {
		fixed = fixedDelta(current, next, liquidity, aToB)
	}
	amountIn, err := toU64(fixed)
	if err != nil {
		return swapStep{}, err
	}
	amountOut, err := toU64(unfixedDelta(current, next, liquidity, aToB))
	if err != nil {
		return swapStep{}, err
	}

	feeAmount, err := stepFee(amountRemaining, amountIn, fixed, feeRate, isMaxSwap)
	if err != nil {
		return swapStep{}, err
	}
	return swapStep{amountIn: amountIn, amountOut: amountOut, feeAmount: feeAmount, nextPrice: next}, nil
}

// stepFee returns remaining-amount_in when the step did not reach the target
// (the user pays for all remaining input), else amount_in*fee/(1e6-fee) rounded
// up. Mirrors the fee_amount branch in compute_swap.
func stepFee(amountRemaining, amountIn uint64, fixed *big.Int, feeRate uint32, isMaxSwap bool) (uint64, error) {
	if !isMaxSwap {
		return amountRemaining - amountIn, nil
	}
	numerator := new(big.Int).Mul(fixed, big.NewInt(int64(feeRate)))
	denominator := big.NewInt(feeRateMulValue - int64(feeRate))
	fee, err := common.DivCeil(numerator, denominator)
	if err != nil {
		return 0, err
	}
	return toU64(fee)
}

// fixedDelta is the input-token delta to move from current to target: token A for
// aToB, token B otherwise, rounded up (input is fixed).
func fixedDelta(current, target, liquidity *big.Int, aToB bool) *big.Int {
	if aToB {
		return orcamath.GetDeltaAmountA(current, target, liquidity, true)
	}
	return orcamath.GetDeltaAmountB(current, target, liquidity, true)
}

// unfixedDelta is the output-token delta produced moving from current to next:
// token B for aToB, token A otherwise, rounded down.
func unfixedDelta(current, next, liquidity *big.Int, aToB bool) *big.Int {
	if aToB {
		return orcamath.GetDeltaAmountB(current, next, liquidity, false)
	}
	return orcamath.GetDeltaAmountA(current, next, liquidity, false)
}

// clampTarget bounds the next tick's sqrt price to the swap's price limit.
func clampTarget(tickPrice, limit *big.Int, aToB bool) *big.Int {
	if aToB && tickPrice.Cmp(limit) < 0 {
		return new(big.Int).Set(limit)
	}
	if !aToB && tickPrice.Cmp(limit) > 0 {
		return new(big.Int).Set(limit)
	}
	return new(big.Int).Set(tickPrice)
}

func toU64(v *big.Int) (uint64, error) {
	if v.Sign() < 0 || v.Cmp(maxU64) > 0 {
		return 0, ErrAmountOverflow
	}
	return v.Uint64(), nil
}
