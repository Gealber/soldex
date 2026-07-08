// Package pumpbc implements the pump.fun BONDING-CURVE swap quote (program
// 6EF8rrec…) — the pre-graduation curve, distinct from the Pump-AMM (quote/pump).
// It is constant product x*y=k over the curve's VIRTUAL reserves (from
// models.BondingCurve), with the fee taken on the SOL (quote) side both ways:
// added to the input on a buy, subtracted from the output on a sell. Pass the total
// fee bps (protocol + creator + any config tier); the caller computes it.
package pumpbc

import "math/big"

const feeDenominator = 10_000

// BuyExactIn returns the base-token amount out for spending solIn lamports
// (fee-inclusive) on the curve: the curve sees solIn minus the fee, then constant
// product — eff = solIn*10000/(10000+feeBps);
// out = vTokenReserves*eff/(vSolReserves+eff).
func BuyExactIn(vSolReserves, vTokenReserves, solIn, feeBps uint64) uint64 {
	eff := new(big.Int).Mul(new(big.Int).SetUint64(solIn), big.NewInt(feeDenominator))
	eff.Div(eff, new(big.Int).SetUint64(feeDenominator+feeBps))
	num := new(big.Int).Mul(new(big.Int).SetUint64(vTokenReserves), eff)
	den := new(big.Int).Add(new(big.Int).SetUint64(vSolReserves), eff)
	if den.Sign() == 0 {
		return 0
	}
	return num.Div(num, den).Uint64()
}

// SellExactIn returns the SOL (lamports) out for selling tokenIn base tokens into
// the curve: constant product runs fee-free, then feeBps is taken off the SOL out —
// gross = vSolReserves*tokenIn/(vTokenReserves+tokenIn);
// out = gross*(10000-feeBps)/10000.
func SellExactIn(vTokenReserves, vSolReserves, tokenIn, feeBps uint64) uint64 {
	num := new(big.Int).Mul(new(big.Int).SetUint64(vSolReserves), new(big.Int).SetUint64(tokenIn))
	den := new(big.Int).Add(new(big.Int).SetUint64(vTokenReserves), new(big.Int).SetUint64(tokenIn))
	if den.Sign() == 0 {
		return 0
	}
	gross := num.Div(num, den)
	gross.Mul(gross, new(big.Int).SetUint64(feeDenominator-feeBps))
	gross.Div(gross, big.NewInt(feeDenominator))
	return gross.Uint64()
}
