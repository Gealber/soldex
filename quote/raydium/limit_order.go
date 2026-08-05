package raydium

import (
	"math"
	"math/big"

	raymath "github.com/Gealber/soldex/math/raydium"
)

// matchLimitOrders fills resting limit orders at an initialized tick before the swap
// crosses it. Orders execute at the TICK price rather than along the curve, so this
// is a fixed-rate conversion, not a swap step — a quote that walks liquidity alone
// misses the fill entirely.
//
// Returns the order size still unfilled after the match.
func (s *swapState) matchLimitOrders(unfilled uint64, tickSqrtPrice *big.Int, feeRate uint32, zeroForOne, feeOnInput bool) (uint64, error) {
	if s.amountRemaining == 0 || unfilled == 0 {
		return unfilled, nil
	}
	// The order is the other side of the swap, so its price rounds against us.
	price := raymath.PriceFromSqrtPrice(tickSqrtPrice, !zeroForOne)
	remaining := new(big.Int).SetUint64(s.amountRemaining)
	resting := new(big.Int).SetUint64(unfilled)

	amountIn := remaining
	fee := big.NewInt(0)
	if feeOnInput {
		fee = ceilDivBig(new(big.Int).Mul(remaining, big.NewInt(int64(feeRate))),
			big.NewInt(raymath.FeeRateDenominator))
		amountIn = new(big.Int).Sub(remaining, fee)
	}

	amountOut := raymath.LimitOrderOutput(amountIn, price, zeroForOne)
	if amountOut.Cmp(resting) > 0 {
		// The book runs out before the input does: price the fill off the book size.
		amountOut = resting
		amountIn = raymath.LimitOrderInput(resting, price, !zeroForOne)
		if feeOnInput {
			fee = ceilDivBig(new(big.Int).Mul(amountIn, big.NewInt(int64(feeRate))),
				big.NewInt(raymath.FeeRateDenominator-int64(feeRate)))
		}
	}

	// Orders are consumed at the GROSS output, before any fee-on-output deduction.
	filled, err := toU64(amountOut)
	if err != nil {
		return 0, err
	}
	if filled > unfilled {
		return 0, ErrAmountOverflow
	}
	unfilled -= filled

	if !feeOnInput {
		fee = ceilDivBig(new(big.Int).Mul(amountOut, big.NewInt(int64(feeRate))),
			big.NewInt(raymath.FeeRateDenominator))
		amountOut = new(big.Int).Sub(amountOut, fee)
	}
	if err := s.apply(amountIn, amountOut, fee, feeOnInput); err != nil {
		return 0, err
	}
	return unfilled, nil
}

// tickAfterCross is the tick the pool sits on once a boundary tick is consumed. A
// tick that still holds unfilled orders is not fully crossed, which flips the side
// the pool lands on.
func tickAfterCross(tickIndex int32, hasLimitOrders, zeroForOne bool) int32 {
	if zeroForOne != hasLimitOrders {
		return tickIndex - 1
	}
	return tickIndex
}

// apply books one fill against the swap state. With fee-on-input the trader pays
// amountIn + fee; with fee-on-output the fee has already been taken out of
// amountOut, so charging it again would double it.
func (s *swapState) apply(amountIn, amountOut, fee *big.Int, feeOnInput bool) error {
	in, err := toU64(amountIn)
	if err != nil {
		return err
	}
	consumed := in
	if feeOnInput {
		feeAmount, err := toU64(fee)
		if err != nil {
			return err
		}
		if in > math.MaxUint64-feeAmount {
			return ErrAmountOverflow
		}
		consumed = in + feeAmount
	}
	if consumed > s.amountRemaining {
		return ErrAmountOverflow
	}
	out, err := toU64(amountOut)
	if err != nil {
		return err
	}
	if s.amountOut > math.MaxUint64-out {
		return ErrAmountOverflow
	}
	s.amountRemaining -= consumed
	s.amountOut += out
	return nil
}
