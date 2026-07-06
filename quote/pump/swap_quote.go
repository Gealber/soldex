// Package pump implements the Pump-AMM (pAMMBay…) constant-product swap quote —
// where pump.fun tokens trade after they graduate off the bonding curve. The pool
// is plain x*y=k over its two vault balances; the only subtlety is the fee, which
// is charged on the OUTPUT for a sell (base→quote) and on the INPUT for a buy
// (quote→base). Compute the total fee (lp+protocol+creator, market-cap tier for
// graduates) with models.PumpTotalFeeBps and pass it here.
package pump

import "math/big"

const feeDenominator = 10_000

// SellExactIn returns the quote-token output for selling amountIn of the base
// token into a Pump-AMM pool: constant product runs fee-free, then feeBps is taken
// off the output — out = quoteReserve*amountIn/(baseReserve+amountIn) *
// (10000-feeBps)/10000.
func SellExactIn(baseReserve, quoteReserve, amountIn, feeBps uint64) uint64 {
	num := new(big.Int).Mul(new(big.Int).SetUint64(quoteReserve), new(big.Int).SetUint64(amountIn))
	den := new(big.Int).Add(new(big.Int).SetUint64(baseReserve), new(big.Int).SetUint64(amountIn))
	if den.Sign() == 0 {
		return 0
	}
	gross := num.Div(num, den)
	gross.Mul(gross, new(big.Int).SetUint64(feeDenominator-feeBps))
	gross.Div(gross, big.NewInt(feeDenominator))
	return gross.Uint64()
}

// BuyExactIn returns the base-token output for spending amountIn of the quote token
// (exact quote in): feeBps is added on the input — effectiveQuote =
// amountIn*10000/(10000+feeBps) — then constant product gives
// out = baseReserve*eff/(quoteReserve+eff).
func BuyExactIn(quoteReserve, baseReserve, amountIn, feeBps uint64) uint64 {
	eff := new(big.Int).Mul(new(big.Int).SetUint64(amountIn), big.NewInt(feeDenominator))
	eff.Div(eff, new(big.Int).SetUint64(feeDenominator+feeBps))
	num := new(big.Int).Mul(new(big.Int).SetUint64(baseReserve), eff)
	den := new(big.Int).Add(new(big.Int).SetUint64(quoteReserve), eff)
	if den.Sign() == 0 {
		return 0
	}
	return num.Div(num, den).Uint64()
}
