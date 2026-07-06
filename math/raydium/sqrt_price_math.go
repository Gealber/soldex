package raydium

import "math/big"

// NextSqrtPriceFromInput mirrors sqrt_price_math::get_next_sqrt_price_from_input:
// the sqrt price after adding amountIn of the input token, rounded so the swap
// never overshoots the target. zeroForOne adds token_0 (price down), else token_1
// (price up). liquidity must be > 0.
func NextSqrtPriceFromInput(sqrtPrice, liquidity *big.Int, amountIn uint64, zeroForOne bool) *big.Int {
	if zeroForOne {
		return nextSqrtPriceFromAmount0RoundingUp(sqrtPrice, liquidity, amountIn, true)
	}
	return nextSqrtPriceFromAmount1RoundingDown(sqrtPrice, liquidity, amountIn, true)
}

// nextSqrtPriceFromAmount0RoundingUp mirrors the rust function of the same name.
// big.Int has no 256-bit overflow, so the denominator >= numerator_1 check the
// rust uses to detect U256 wraparound is always satisfied here (product >= 0),
// which is the same value the non-overflow branch computes — faithful because
// the inputs (L<2^128, amount<2^64, sqrt<2^128) can never reach 2^256.
func nextSqrtPriceFromAmount0RoundingUp(sqrtPrice, liquidity *big.Int, amount uint64, add bool) *big.Int {
	if amount == 0 {
		return new(big.Int).Set(sqrtPrice)
	}
	amt := new(big.Int).SetUint64(amount)
	numerator1 := new(big.Int).Lsh(liquidity, 64)
	product := new(big.Int).Mul(amt, sqrtPrice)

	var denominator *big.Int
	if add {
		denominator = new(big.Int).Add(numerator1, product)
	} else {
		denominator = new(big.Int).Sub(numerator1, product)
	}
	return ceilDiv(new(big.Int).Mul(numerator1, sqrtPrice), denominator)
}

// nextSqrtPriceFromAmount1RoundingDown mirrors the rust function of the same name.
func nextSqrtPriceFromAmount1RoundingDown(sqrtPrice, liquidity *big.Int, amount uint64, add bool) *big.Int {
	if amount == 0 {
		return new(big.Int).Set(sqrtPrice)
	}
	amtShifted := new(big.Int).Lsh(new(big.Int).SetUint64(amount), 64)
	if add {
		quotient := new(big.Int).Quo(amtShifted, liquidity)
		return new(big.Int).Add(sqrtPrice, quotient)
	}
	quotient := ceilDiv(amtShifted, liquidity)
	return new(big.Int).Sub(sqrtPrice, quotient)
}
