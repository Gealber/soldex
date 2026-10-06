// Package rayammv4 implements the Raydium AMM v4 constant-product swap quote over
// the pool's NET reserves (models.RaydiumAMMV4Pool.NetReserves).
package rayammv4

import "math/big"

// SwapBaseIn swaps amountIn into a pool holding the NET reserveIn / reserveOut,
// rounding the input fee up and the curve down as swap_base_in does.
func SwapBaseIn(reserveIn, reserveOut, amountIn, feeNumerator, feeDenominator uint64) uint64 {
	if feeDenominator == 0 {
		return 0
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(amountIn), new(big.Int).SetUint64(feeNumerator))
	fee.Add(fee, new(big.Int).SetUint64(feeDenominator-1))
	fee.Div(fee, new(big.Int).SetUint64(feeDenominator))
	netIn := new(big.Int).Sub(new(big.Int).SetUint64(amountIn), fee)
	if netIn.Sign() <= 0 {
		return 0
	}
	den := new(big.Int).Add(new(big.Int).SetUint64(reserveIn), netIn)
	out := new(big.Int).Mul(new(big.Int).SetUint64(reserveOut), netIn)
	return out.Div(out, den).Uint64()
}
