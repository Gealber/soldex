// Package raycpmm implements the Raydium CP-Swap constant-product swap quote over
// the pool's NET reserves (models.RaydiumCPMMPool.NetReserves), rates out of 1e6.
package raycpmm

import (
	"errors"
	"fmt"
	"math/big"
)

// FeeRateDenominator is the CP-Swap fee denominator (FEE_RATE_DENOMINATOR_VALUE):
// trade_fee_rate is expressed in hundredths of a basis point (10^-6).
const FeeRateDenominator = 1_000_000

// creator_fee_on values, as CreatorFeeOn in the program's states/pool.rs.
const (
	CreatorFeeOnBothToken  uint8 = 0
	CreatorFeeOnOnlyToken0 uint8 = 1
	CreatorFeeOnOnlyToken1 uint8 = 2
)

// ErrUnknownCreatorFeeOn is returned for a creator_fee_on the program does not define.
var ErrUnknownCreatorFeeOn = errors.New("raycpmm: unknown creator_fee_on")

// CreatorFeeOnInput reports whether a swap pays the creator fee from its input.
// When false it comes off the output, which is the case for one direction of modes 1 and 2.
func CreatorFeeOnInput(creatorFeeOn uint8, zeroForOne bool) (bool, error) {
	switch creatorFeeOn {
	case CreatorFeeOnBothToken:
		return true, nil
	case CreatorFeeOnOnlyToken0:
		return zeroForOne, nil
	case CreatorFeeOnOnlyToken1:
		return !zeroForOne, nil
	}
	return false, fmt.Errorf("%w: %d", ErrUnknownCreatorFeeOn, creatorFeeOn)
}

// SwapBaseInput swaps amountIn into a pool holding the NET reserveIn / reserveOut,
// rounding fees up and the curve down as the program does.
func SwapBaseInput(reserveIn, reserveOut, amountIn, tradeFeeRate, creatorFeeRate uint64, creatorFeeOnInput bool) uint64 {
	inputFeeRate := tradeFeeRate
	if creatorFeeOnInput {
		inputFeeRate += creatorFeeRate
	}
	netIn := new(big.Int).Sub(new(big.Int).SetUint64(amountIn), ceilFee(new(big.Int).SetUint64(amountIn), inputFeeRate))
	if netIn.Sign() <= 0 {
		return 0
	}
	den := new(big.Int).Add(new(big.Int).SetUint64(reserveIn), netIn)
	if den.Sign() == 0 {
		return 0
	}
	swapped := new(big.Int).Mul(new(big.Int).SetUint64(reserveOut), netIn)
	swapped.Div(swapped, den)
	if creatorFeeOnInput {
		return swapped.Uint64()
	}
	out := swapped.Sub(swapped, ceilFee(new(big.Int).Set(swapped), creatorFeeRate))
	if out.Sign() <= 0 {
		return 0
	}
	return out.Uint64()
}

func ceilFee(amount *big.Int, rate uint64) *big.Int {
	fee := amount.Mul(amount, new(big.Int).SetUint64(rate))
	fee.Add(fee, big.NewInt(FeeRateDenominator-1))
	return fee.Div(fee, big.NewInt(FeeRateDenominator))
}
