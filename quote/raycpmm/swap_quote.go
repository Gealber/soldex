// Package raycpmm implements the Raydium CP-Swap (CPMMoo8L…) constant-product
// swap quote — the standard AMM (no orderbook) many tokens trade on. The pool is
// plain x*y=k over its two vault balances, net of the protocol and fund fees the
// pool tracks (see models.RaydiumCPMMPool.NetReserves). The trade fee is charged
// on the INPUT and is denominated in hundredths of a bip (out of 1e6); read it
// from the linked AmmConfig (models.RaydiumCPMMConfig.TradeFeeRate).
package raycpmm

import "math/big"

// FeeRateDenominator is the CP-Swap fee denominator (FEE_RATE_DENOMINATOR_VALUE):
// trade_fee_rate is expressed in hundredths of a basis point (10^-6).
const FeeRateDenominator = 1_000_000

// SwapBaseInput returns the output-token amount for swapping amountIn of the input
// token into a CP-Swap pool holding reserveIn / reserveOut (net reserves). The
// trade fee is taken off the input first — tradeFee = ceil(amountIn*feeRate/1e6) —
// then constant product gives out = reserveOut*netIn/(reserveIn+netIn), floored.
// Protocol and fund fees are carved out of the trade fee, so they do not reduce
// the output further and are not needed here.
func SwapBaseInput(reserveIn, reserveOut, amountIn, feeRate uint64) uint64 {
	// tradeFee = ceil(amountIn * feeRate / FeeRateDenominator).
	fee := new(big.Int).Mul(new(big.Int).SetUint64(amountIn), new(big.Int).SetUint64(feeRate))
	fee.Add(fee, big.NewInt(FeeRateDenominator-1))
	fee.Div(fee, big.NewInt(FeeRateDenominator))

	netIn := new(big.Int).Sub(new(big.Int).SetUint64(amountIn), fee)
	if netIn.Sign() <= 0 {
		return 0
	}

	num := new(big.Int).Mul(new(big.Int).SetUint64(reserveOut), netIn)
	den := new(big.Int).Add(new(big.Int).SetUint64(reserveIn), netIn)
	if den.Sign() == 0 {
		return 0
	}
	return num.Div(num, den).Uint64()
}
