package orca

import (
	"errors"
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

// Q64Resolution is the fractional bit width of Orca's Q64.64 sqrt prices. It is
// HALF of cp-amm's 128-bit scale, so DAMM's curve math is NOT reusable here.
const Q64Resolution = 64

// q64Mask isolates the fractional part of a Q64.64 product (lowest 64 bits).
var q64Mask = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1))

var (
	ErrDivideByZero         = errors.New("orca: divide by zero")
	ErrSqrtPriceOutOfBounds = errors.New("orca: sqrt price out of bounds")
)

// order returns the two sqrt prices in increasing order without mutating them.
func order(a, b *big.Int) (*big.Int, *big.Int) {
	if a.Cmp(b) > 0 {
		return b, a
	}
	return a, b
}

// GetDeltaAmountA returns Δa = (L * (√upper-√lower) << 64) / (√upper*√lower),
// the token-A amount between two sqrt prices. Mirrors get_amount_delta_a. The
// result is exact and may exceed u64; the caller bounds it.
func GetDeltaAmountA(sqrtPrice0, sqrtPrice1, liquidity *big.Int, roundUp bool) *big.Int {
	lower, upper := order(sqrtPrice0, sqrtPrice1)
	denominator := new(big.Int).Mul(upper, lower)
	if denominator.Sign() == 0 {
		return big.NewInt(0)
	}

	diff := new(big.Int).Sub(upper, lower)
	numerator := new(big.Int).Lsh(new(big.Int).Mul(liquidity, diff), Q64Resolution)

	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	if roundUp && remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

// GetDeltaAmountB returns Δb = (L * (√upper-√lower)) >> 64, the token-B amount
// between two sqrt prices. Mirrors get_amount_delta_b.
func GetDeltaAmountB(sqrtPrice0, sqrtPrice1, liquidity *big.Int, roundUp bool) *big.Int {
	lower, upper := order(sqrtPrice0, sqrtPrice1)
	product := new(big.Int).Mul(liquidity, new(big.Int).Sub(upper, lower))

	result := new(big.Int).Rsh(product, Q64Resolution)
	if roundUp && new(big.Int).And(product, q64Mask).Sign() != 0 {
		result.Add(result, big.NewInt(1))
	}
	return result
}

// NextSqrtPrice returns the post-step sqrt price, dispatching on which token is
// fixed: token A when isInput == aToB, otherwise token B. Mirrors
// get_next_sqrt_price.
func NextSqrtPrice(sqrtPrice, liquidity, amount *big.Int, isInput, aToB bool) (*big.Int, error) {
	if isInput == aToB {
		return nextSqrtPriceFromA(sqrtPrice, liquidity, amount, isInput)
	}
	return nextSqrtPriceFromB(sqrtPrice, liquidity, amount, isInput)
}

// nextSqrtPriceFromA solves √P' = (L*√P << 64) / ((L << 64) ± √P*amount), rounded
// up. The denominator adds the product for input, subtracts for output. Mirrors
// get_next_sqrt_price_from_a_round_up.
func nextSqrtPriceFromA(sqrtPrice, liquidity, amount *big.Int, isInput bool) (*big.Int, error) {
	if amount.Sign() == 0 {
		return new(big.Int).Set(sqrtPrice), nil
	}

	product := new(big.Int).Mul(sqrtPrice, amount)
	liquidityShift := new(big.Int).Lsh(liquidity, Q64Resolution)

	var denominator *big.Int
	if isInput {
		denominator = new(big.Int).Add(liquidityShift, product)
	} else {
		if liquidityShift.Cmp(product) <= 0 {
			return nil, ErrDivideByZero
		}
		denominator = new(big.Int).Sub(liquidityShift, product)
	}

	numerator := new(big.Int).Lsh(new(big.Int).Mul(liquidity, sqrtPrice), Q64Resolution)
	return common.DivCeil(numerator, denominator)
}

// nextSqrtPriceFromB solves √P' = √P ± (amount << 64)/L: floor-then-add for
// input, ceil-then-subtract for output. Mirrors get_next_sqrt_price_from_b_round_down.
func nextSqrtPriceFromB(sqrtPrice, liquidity, amount *big.Int, isInput bool) (*big.Int, error) {
	amountX64 := new(big.Int).Lsh(amount, Q64Resolution)

	if isInput {
		delta, err := common.DivFloor(amountX64, liquidity)
		if err != nil {
			return nil, err
		}
		return new(big.Int).Add(sqrtPrice, delta), nil
	}

	delta, err := common.DivCeil(amountX64, liquidity)
	if err != nil {
		return nil, err
	}
	result := new(big.Int).Sub(sqrtPrice, delta)
	if result.Sign() < 0 {
		return nil, ErrSqrtPriceOutOfBounds
	}
	return result, nil
}
