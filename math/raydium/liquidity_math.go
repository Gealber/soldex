package raydium

import "math/big"

// q64 is 2^64, the Q64.64 unit.
var q64 = new(big.Int).Lsh(big.NewInt(1), 64)

// ceilDiv returns ceil(num/den) for non-negative inputs.
func ceilDiv(num, den *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}

// GetDeltaAmount0Unsigned mirrors liquidity_math::get_delta_amount_0_unsigned:
// the token_0 amount between two sqrt prices for the given liquidity. The roundUp
// path uses Raydium's exact two-step ceil — ceil(ceil(L<<64 * (b-a) / b) / a) —
// which is NOT the same rounding as Orca's single ceil and must be preserved so
// on-chain amounts match. Returns the raw value; the caller bounds it to u64.
func GetDeltaAmount0Unsigned(sqrtA, sqrtB, liquidity *big.Int, roundUp bool) *big.Int {
	a, b := sqrtA, sqrtB
	if a.Cmp(b) > 0 {
		a, b = b, a
	}
	numerator1 := new(big.Int).Lsh(liquidity, 64)
	numerator2 := new(big.Int).Sub(b, a)
	num := new(big.Int).Mul(numerator1, numerator2)
	if roundUp {
		return ceilDiv(ceilDiv(num, b), a)
	}
	q := new(big.Int).Quo(num, b)
	return q.Quo(q, a)
}

// GetDeltaAmount1Unsigned mirrors liquidity_math::get_delta_amount_1_unsigned:
// the token_1 amount between two sqrt prices, = L*(b-a)/2^64, rounded up or down.
func GetDeltaAmount1Unsigned(sqrtA, sqrtB, liquidity *big.Int, roundUp bool) *big.Int {
	a, b := sqrtA, sqrtB
	if a.Cmp(b) > 0 {
		a, b = b, a
	}
	product := new(big.Int).Mul(liquidity, new(big.Int).Sub(b, a))
	if roundUp {
		return ceilDiv(product, q64)
	}
	return new(big.Int).Quo(product, q64)
}
