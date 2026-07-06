package damm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

// twoPow128 is the Q64.64 scale used by cp-amm sqrt-price math.
var twoPow128 = new(big.Int).Lsh(big.NewInt(1), 128)

// GetNextSqrtPriceFromInput returns the post-swap sqrt price after adding
// amountIn of token a (aForB) or token b into a concentrated-liquidity range.
// Mirrors cp-amm get_next_sqrt_price_from_input.
func GetNextSqrtPriceFromInput(sqrtPrice, liquidity *big.Int, amountIn uint64, aForB bool) (*big.Int, error) {
	if sqrtPrice.Sign() <= 0 || liquidity.Sign() <= 0 {
		return nil, common.ErrDivideByZero
	}
	if amountIn == 0 {
		return common.Clone(sqrtPrice), nil
	}

	amount := new(big.Int).SetUint64(amountIn)
	if aForB {
		// √P' = L * √P / (L + amount * √P), rounded up.
		product := new(big.Int).Mul(amount, sqrtPrice)
		denominator := new(big.Int).Add(liquidity, product)
		numerator := new(big.Int).Mul(liquidity, sqrtPrice)
		return common.DivCeil(numerator, denominator)
	}

	// √P' = √P + (amount << 128) / L, rounded down.
	quotient, err := common.DivFloor(new(big.Int).Lsh(amount, 128), liquidity)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Add(sqrtPrice, quotient), nil
}

// GetDeltaAmountA returns Δa = L * (upper-lower) / (lower*upper), capped to u64.
// Mirrors cp-amm get_delta_amount_a_unsigned. Requires lower <= upper.
func GetDeltaAmountA(lower, upper, liquidity *big.Int, roundUp bool) (uint64, error) {
	if lower.Sign() <= 0 || upper.Sign() <= 0 {
		return 0, common.ErrDivideByZero
	}

	numerator := new(big.Int).Mul(liquidity, new(big.Int).Sub(upper, lower))
	denominator := new(big.Int).Mul(lower, upper)

	var result *big.Int
	var err error
	if roundUp {
		result, err = common.DivCeil(numerator, denominator)
	} else {
		result, err = common.DivFloor(numerator, denominator)
	}
	if err != nil {
		return 0, err
	}
	return common.BigToUint64Checked(result)
}

// GetDeltaAmountB returns Δb = L * (upper-lower) >> 128, capped to u64.
// Mirrors cp-amm get_delta_amount_b_unsigned. Requires lower <= upper.
func GetDeltaAmountB(lower, upper, liquidity *big.Int, roundUp bool) (uint64, error) {
	delta := new(big.Int).Sub(upper, lower)
	if delta.Sign() < 0 {
		return 0, common.ErrMathOverflow
	}
	prod := new(big.Int).Mul(liquidity, delta)

	var result *big.Int
	if roundUp {
		var err error
		result, err = common.DivCeil(prod, twoPow128)
		if err != nil {
			return 0, err
		}
	} else {
		result = new(big.Int).Rsh(prod, 128)
	}
	return common.BigToUint64Checked(result)
}
