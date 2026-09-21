// Package fluxbeam implements the FluxBeam (FLUXubRm…) swap quote. FluxBeam is
// an SPL token-swap fork, so a pool is plain x*y=k over its two vault balances
// with the fees taken off the INPUT before the curve runs.
package fluxbeam

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/Gealber/soldex/models"
)

// ErrZeroOutput is returned when the swap would move no destination token. The
// program treats that as a failure rather than a zero-value swap.
var ErrZeroOutput = errors.New("fluxbeam: swap yields no output")

// ErrUnsupportedCurve is returned for a curve this package does not model.
//
// Only constant product is modelled, which covers all but a few dozen live
// pools. The constant-price and offset curves price differently enough that
// running them through the constant-product formula would not be an
// approximation, it would be a different number.
var ErrUnsupportedCurve = errors.New("fluxbeam: unsupported curve type")

// QuoteExactIn is the curve-aware entry point: it refuses a pool whose curve
// this package does not model, then prices it.
//
// Prefer this over SwapExactIn unless you have already established the pool is
// constant product — SwapExactIn assumes it and cannot tell.
func QuoteExactIn(curveType uint8, reserveIn, reserveOut, amountIn uint64, fees models.FluxBeamFees) (uint64, error) {
	if curveType != models.FluxBeamCurveConstantProduct {
		return 0, fmt.Errorf("%w: %d", ErrUnsupportedCurve, curveType)
	}

	return SwapExactIn(reserveIn, reserveOut, amountIn, fees)
}

// SwapExactIn returns the destination amount for swapping amountIn into a
// constant-product pool holding reserveIn / reserveOut, which are the two vault
// token-account balances.
//
// BOTH the trade fee and the owner trade fee come off the input before the
// curve — the program adds them together and subtracts the total. The withdraw
// fee applies to withdrawals and the host fee is carved out of the owner fee for
// a referrer, so neither changes what a swap returns.
//
// Ignoring the owner fee is not a rounding matter here: pool creators set their
// own fees and plenty of live pools carry an owner trade fee of 90%.
func SwapExactIn(reserveIn, reserveOut, amountIn uint64, fees models.FluxBeamFees) (uint64, error) {
	tradeFee := calculateFee(amountIn, fees.TradeFeeNumerator, fees.TradeFeeDenominator)
	ownerFee := calculateFee(amountIn, fees.OwnerTradeFeeNumerator, fees.OwnerTradeFeeDenominator)

	total := new(big.Int).Add(tradeFee, ownerFee)
	net := new(big.Int).Sub(new(big.Int).SetUint64(amountIn), total)
	if net.Sign() <= 0 {
		return 0, ErrZeroOutput
	}

	src := new(big.Int).SetUint64(reserveIn)
	dst := new(big.Int).SetUint64(reserveOut)

	// out = dst - ceil(src*dst / (src+net)), which is floor(dst*net / (src+net)).
	den := new(big.Int).Add(src, net)
	if den.Sign() == 0 {
		return 0, ErrZeroOutput
	}
	out := new(big.Int).Mul(dst, net)
	out.Div(out, den)

	if out.Sign() <= 0 {
		return 0, ErrZeroOutput
	}
	if !out.IsUint64() {
		return 0, ErrZeroOutput
	}

	return out.Uint64(), nil
}

// calculateFee mirrors the program's calculate_fee: a zero numerator means no
// fee, and a fee that rounds down to zero is charged as one token.
func calculateFee(amount, numerator, denominator uint64) *big.Int {
	if numerator == 0 || amount == 0 || denominator == 0 {
		return big.NewInt(0)
	}

	fee := new(big.Int).Mul(new(big.Int).SetUint64(amount), new(big.Int).SetUint64(numerator))
	fee.Div(fee, new(big.Int).SetUint64(denominator))
	if fee.Sign() == 0 {
		return big.NewInt(1)
	}

	return fee
}
