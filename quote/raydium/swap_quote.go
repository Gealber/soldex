// Package raydium ports the Raydium CLMM exact-in swap quote (concentrated
// liquidity, Q64.64). It mirrors the on-chain swap_internal tick-walking loop;
// only the exact-in, fee-on-input path is implemented, and it walks a cached
// window of tick arrays (stopping at the edge of known liquidity) rather than
// the on-chain tickarray bitmap.
package raydium

import (
	"errors"
	"math"
	"math/big"

	raymath "github.com/Gealber/soldex/math/raydium"
)

var (
	ErrInvalidPool       = errors.New("raydium: invalid pool state")
	ErrAmountOverflow    = errors.New("raydium: amount exceeds u64")
	ErrNegativeLiquidity = errors.New("raydium: negative liquidity after tick cross")
)

// SwapPool holds the decoded Raydium CLMM fields needed to quote one exact-in
// swap. FeeRate is the linked AmmConfig trade_fee_rate (hundredths of a bip).
type SwapPool struct {
	SqrtPrice   *big.Int
	Liquidity   *big.Int
	TickCurrent int32
	TickSpacing uint16
	FeeRate     uint32
}

// TickBoundary is the next initialized tick (or tick-array edge) reachable in the
// swap direction; crossing an initialized tick changes liquidity by LiquidityNet.
type TickBoundary struct {
	TickIndex    int32
	LiquidityNet *big.Int
	Initialized  bool
}

// TickProvider returns the next boundary at or beyond fromTick in the swap
// direction (zeroForOne searches down, inclusive of fromTick; else up, exclusive).
// ok=false means no further tick array is cached, so the swap stops at the edge of
// known liquidity.
type TickProvider func(fromTick int32, zeroForOne bool) (TickBoundary, bool)

// QuoteExactIn swaps amountIn through the pool, walking initialized ticks until
// the input is consumed or known liquidity runs out. zeroForOne true sells
// token_0 for token_1 (price decreasing). Returns the net output amount.
func QuoteExactIn(pool SwapPool, zeroForOne bool, amountIn uint64, ticks TickProvider) (uint64, error) {
	if pool.SqrtPrice == nil || pool.Liquidity == nil {
		return 0, ErrInvalidPool
	}

	limit := raymath.MaxSqrtPrice
	if zeroForOne {
		limit = raymath.MinSqrtPrice
	}

	amountRemaining := amountIn
	amountOut := uint64(0)
	sqrtPrice := new(big.Int).Set(pool.SqrtPrice)
	liquidity := new(big.Int).Set(pool.Liquidity)
	currTick := pool.TickCurrent

	for amountRemaining > 0 && sqrtPrice.Cmp(limit) != 0 {
		boundary, ok := ticks(currTick, zeroForOne)
		if !ok {
			break
		}

		tickPrice := raymath.SqrtPriceFromTick(boundary.TickIndex)
		target := clampTarget(tickPrice, limit, zeroForOne)

		step := raymath.ComputeSwapStep(sqrtPrice, target, liquidity, amountRemaining, pool.FeeRate, zeroForOne)

		consumed, err := stepConsumed(step)
		if err != nil {
			return 0, err
		}
		if consumed > amountRemaining {
			return 0, ErrAmountOverflow
		}
		out, err := toU64(step.AmountOut)
		if err != nil {
			return 0, err
		}
		if amountOut > math.MaxUint64-out {
			return 0, ErrAmountOverflow
		}
		amountRemaining -= consumed
		amountOut += out

		currTick, err = advance(step, boundary, tickPrice, sqrtPrice, liquidity, zeroForOne)
		if err != nil {
			return 0, err
		}
		sqrtPrice = step.SqrtPriceNext
	}

	return amountOut, nil
}

// stepConsumed is the input drawn from the remaining amount: amount_in + fee.
func stepConsumed(step raymath.SwapStep) (uint64, error) {
	in, err := toU64(step.AmountIn)
	if err != nil {
		return 0, err
	}
	fee, err := toU64(step.FeeAmount)
	if err != nil {
		return 0, err
	}
	if in > math.MaxUint64-fee {
		return 0, ErrAmountOverflow
	}
	return in + fee, nil
}

// advance updates liquidity when an initialized tick is crossed and returns the
// next search tick. It mutates liquidity in place on a cross.
func advance(step raymath.SwapStep, boundary TickBoundary, tickPrice, sqrtPrice, liquidity *big.Int, zeroForOne bool) (int32, error) {
	if step.SqrtPriceNext.Cmp(tickPrice) == 0 {
		if boundary.Initialized {
			if err := crossLiquidity(liquidity, boundary.LiquidityNet, zeroForOne); err != nil {
				return 0, err
			}
		}
		// The zero_for_one search is inclusive of fromTick, so step left by one.
		if zeroForOne {
			return boundary.TickIndex - 1, nil
		}
		return boundary.TickIndex, nil
	}
	if step.SqrtPriceNext.Cmp(sqrtPrice) != 0 {
		return raymath.TickFromSqrtPrice(step.SqrtPriceNext), nil
	}
	return raymath.TickFromSqrtPrice(sqrtPrice), nil
}

// crossLiquidity applies liquidity += zeroForOne ? -net : +net.
func crossLiquidity(liquidity, net *big.Int, zeroForOne bool) error {
	if net == nil {
		return nil
	}
	if zeroForOne {
		liquidity.Sub(liquidity, net)
	} else {
		liquidity.Add(liquidity, net)
	}
	if liquidity.Sign() < 0 {
		return ErrNegativeLiquidity
	}
	return nil
}

// clampTarget bounds the next tick's sqrt price to the swap's price limit.
func clampTarget(tickPrice, limit *big.Int, zeroForOne bool) *big.Int {
	if zeroForOne && tickPrice.Cmp(limit) < 0 {
		return new(big.Int).Set(limit)
	}
	if !zeroForOne && tickPrice.Cmp(limit) > 0 {
		return new(big.Int).Set(limit)
	}
	return new(big.Int).Set(tickPrice)
}

var maxU64 = new(big.Int).SetUint64(math.MaxUint64)

func toU64(v *big.Int) (uint64, error) {
	if v.Sign() < 0 || v.Cmp(maxU64) > 0 {
		return 0, ErrAmountOverflow
	}
	return v.Uint64(), nil
}
