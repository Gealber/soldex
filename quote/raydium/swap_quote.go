// Package raydium ports the Raydium CLMM exact-in swap quote (concentrated
// liquidity, Q64.64). It mirrors the on-chain swap_internal tick-walking loop and
// walks a cached window of tick arrays (stopping at the edge of known liquidity)
// rather than the on-chain tickarray bitmap.
//
// Beyond plain tick crossing it models the three things the 2026-07-31 program
// upgrade added, because each one changes the amount out:
//
//   - LIMIT ORDERS resting at an initialized tick, filled at the tick price before
//     the swap crosses it (see limit_order.go),
//   - a DYNAMIC FEE that adds to the AmmConfig fee as the price travels, stepping
//     the swap one tick-spacing group at a time (see dynamic_fee.go),
//   - fee_on, which for one direction takes the fee out of the OUTPUT instead of
//     the input.
//
// Only the exact-in direction is implemented.
package raydium

import (
	"errors"
	"math/big"

	raymath "github.com/Gealber/soldex/math/raydium"
	"github.com/Gealber/soldex/models"
)

var (
	ErrInvalidPool       = errors.New("raydium: invalid pool state")
	ErrAmountOverflow    = errors.New("raydium: amount exceeds u64")
	ErrNegativeLiquidity = errors.New("raydium: negative liquidity after tick cross")
	ErrSwapDisabled      = errors.New("raydium: pool has swap disabled")
)

// SwapPool holds the decoded Raydium CLMM fields needed to quote one exact-in swap.
type SwapPool struct {
	SqrtPrice   *big.Int
	Liquidity   *big.Int
	TickCurrent int32
	TickSpacing uint16
	// FeeRate is the linked AmmConfig trade_fee_rate (hundredths of a bip). When
	// DynamicFee is set this is only the BASE — the charged rate is higher.
	FeeRate uint32
	// FeeOn mirrors PoolState.fee_on: 0 FromInput, 1 Token0Only, 2 Token1Only.
	FeeOn uint8
	// Status mirrors PoolState.status; bit4 set means swaps are disabled.
	Status uint8
	// DynamicFee is the pool's DynamicFeeInfo. The zero value means no dynamic fee,
	// which is the common case.
	DynamicFee models.RaydiumDynamicFee
	// BlockTimestamp is the swap's block time (unix seconds). Only read when
	// DynamicFee is set, to decay the volatility reference — pass the CURRENT time,
	// since a stale one under-decays and over-quotes the fee.
	BlockTimestamp uint64
}

// IsFeeOnInput reports whether this direction pays the fee out of the input token.
func (p SwapPool) IsFeeOnInput(zeroForOne bool) bool {
	switch p.FeeOn {
	case 1:
		return zeroForOne
	case 2:
		return !zeroForOne
	default:
		return true
	}
}

// TickBoundary is the next initialized tick reachable in the swap direction.
type TickBoundary struct {
	TickIndex int32
	// LiquidityNet is applied when the tick is crossed.
	LiquidityNet *big.Int
	// Initialized reports gross liquidity at the tick — whether crossing it changes
	// the pool's active liquidity.
	Initialized bool
	// LimitOrderUnfilled is orders_amount + part_filled_orders_remaining resting
	// here. A tick can carry orders with NO liquidity, so the provider must treat a
	// non-zero value as a stop in its own right (models.RaydiumTick.Initialized does).
	LimitOrderUnfilled uint64
}

// TickProvider returns the next boundary at or beyond fromTick in the swap
// direction (zeroForOne searches down, inclusive of fromTick; else up, exclusive).
// ok=false means no further tick array is cached, so the swap stops at the edge of
// known liquidity.
type TickProvider func(fromTick int32, zeroForOne bool) (TickBoundary, bool)

// swapState is the running position of one quote through the book.
type swapState struct {
	amountRemaining uint64
	amountOut       uint64
	sqrtPrice       *big.Int
	liquidity       *big.Int
	tick            int32
	tickSpacing     uint16
	baseFeeRate     uint32
	// tickSpacingIdx is the current tick-spacing group; only meaningful with a
	// dynamic fee.
	tickSpacingIdx int32
	// dyn is nil when the pool charges no dynamic fee. It is a COPY of the pool's
	// state: the quote advances it exactly as the swap would, without touching the
	// caller's decoded pool.
	dyn *models.RaydiumDynamicFee
}

// QuoteExactIn swaps amountIn through the pool, walking initialized ticks until the
// input is consumed or known liquidity runs out. zeroForOne true sells token_0 for
// token_1 (price decreasing). Returns the net output amount.
func QuoteExactIn(pool SwapPool, zeroForOne bool, amountIn uint64, ticks TickProvider) (uint64, error) {
	if pool.SqrtPrice == nil || pool.Liquidity == nil {
		return 0, ErrInvalidPool
	}
	if pool.Status&(1<<4) != 0 {
		return 0, ErrSwapDisabled
	}
	limit := raymath.MaxSqrtPrice
	if zeroForOne {
		limit = raymath.MinSqrtPrice
	}
	state := newSwapState(pool, amountIn)
	feeOnInput := pool.IsFeeOnInput(zeroForOne)

	for state.amountRemaining > 0 && state.sqrtPrice.Cmp(limit) != 0 {
		boundary, ok := ticks(state.tick, zeroForOne)
		if !ok {
			break
		}
		if err := state.stepToBoundary(boundary, limit, zeroForOne, feeOnInput); err != nil {
			return 0, err
		}
	}
	return state.amountOut, nil
}

// newSwapState seeds the walk from the pool, decaying the volatility reference once
// up front exactly as SwapState::new does.
func newSwapState(pool SwapPool, amountIn uint64) *swapState {
	state := &swapState{
		amountRemaining: amountIn,
		sqrtPrice:       new(big.Int).Set(pool.SqrtPrice),
		liquidity:       new(big.Int).Set(pool.Liquidity),
		tick:            pool.TickCurrent,
		tickSpacing:     pool.TickSpacing,
		baseFeeRate:     pool.FeeRate,
	}
	if pool.DynamicFee.Enabled() {
		dyn := pool.DynamicFee
		state.dyn = &dyn
		state.tickSpacingIdx = tickSpacingIndexFromTick(pool.TickCurrent, pool.TickSpacing)
		state.updateReference(state.tickSpacingIdx, pool.BlockTimestamp)
	}
	return state
}

// stepToBoundary swaps toward one initialized tick. With a dynamic fee the move is
// broken into tick-spacing groups so the fee can rise as the price travels, so this
// is a loop rather than a single step; without one it runs at most twice.
func (s *swapState) stepToBoundary(boundary TickBoundary, limit *big.Int, zeroForOne, feeOnInput bool) error {
	tickPrice := raymath.SqrtPriceFromTick(boundary.TickIndex)
	target := clampTarget(tickPrice, limit, zeroForOne)
	liquidityNext := new(big.Int).Set(s.liquidity)
	unfilled := boundary.LimitOrderUnfilled

	for {
		s.updateVolatilityAccumulator()
		feeRate := s.totalFeeRate()
		skipped, bounded, boundedTick := s.spacingBoundedPrice(target, zeroForOne)

		step := raymath.SwapStep{SqrtPriceNext: bounded}
		if s.sqrtPrice.Cmp(bounded) != 0 {
			step = raymath.ComputeSwapStep(
				s.sqrtPrice, bounded, s.liquidity, s.amountRemaining, feeRate, zeroForOne, feeOnInput)
			if err := s.apply(step.AmountIn, step.AmountOut, step.FeeAmount, feeOnInput); err != nil {
				return err
			}
		}

		reached := tickPrice.Cmp(step.SqrtPriceNext) == 0
		if reached {
			var err error
			if unfilled, err = s.matchLimitOrders(unfilled, tickPrice, feeRate, zeroForOne, feeOnInput); err != nil {
				return err
			}
			// A tick still holding orders is not fully crossed, so liquidity stays.
			if boundary.Initialized && unfilled == 0 {
				if err := crossLiquidity(liquidityNext, boundary.LiquidityNet, zeroForOne); err != nil {
					return err
				}
			}
			s.tick = tickAfterCross(boundary.TickIndex, unfilled > 0, zeroForOne)
		} else if s.sqrtPrice.Cmp(step.SqrtPriceNext) != 0 {
			s.tick = landedTick(step.SqrtPriceNext, bounded, boundedTick)
		}

		s.sqrtPrice = step.SqrtPriceNext
		s.updateDynamicFeeIndex(zeroForOne, skipped, tickPrice, boundary.TickIndex)
		if s.amountRemaining == 0 || s.sqrtPrice.Cmp(target) == 0 {
			break
		}
	}
	s.liquidity = liquidityNext
	return nil
}

// landedTick is the tick for a price that stopped short of the boundary, reusing the
// spacing bound when the step ended exactly on it.
func landedTick(sqrtPrice, bounded *big.Int, boundedTick *int32) int32 {
	if boundedTick != nil && sqrtPrice.Cmp(bounded) == 0 {
		return *boundedTick
	}
	return raymath.TickFromSqrtPrice(sqrtPrice)
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

// clampTarget bounds the next tick's price by the swap's price limit.
func clampTarget(tickPrice, limit *big.Int, zeroForOne bool) *big.Int {
	if zeroForOne {
		if tickPrice.Cmp(limit) < 0 {
			return limit
		}
		return tickPrice
	}
	if tickPrice.Cmp(limit) > 0 {
		return limit
	}
	return tickPrice
}

// toU64 narrows a non-negative big.Int, rejecting anything the program could not
// have produced.
func toU64(v *big.Int) (uint64, error) {
	if v == nil {
		return 0, nil
	}
	if v.Sign() < 0 || !v.IsUint64() {
		return 0, ErrAmountOverflow
	}
	return v.Uint64(), nil
}
