// Package soldex is Solana DEX swap math: account decoders (models/), fixed-point
// primitives (math/) and exact-in quotes (quote/), behind one Quoter.
//
// Start with FromAccount or a From* constructor. The adapters below bind an
// already-assembled quote struct and accept whatever fee you give them.
package soldex

import (
	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/fluxbeam"
	"github.com/Gealber/soldex/quote/orca"
	"github.com/Gealber/soldex/quote/pump"
	"github.com/Gealber/soldex/quote/pumpbc"
	"github.com/Gealber/soldex/quote/raycpmm"
	"github.com/Gealber/soldex/quote/raydium"
)

// Quoter is the uniform exact-in interface across every supported venue. aToB
// true means the pool's first token in (DLMM X, Orca token_a, Raydium token0,
// DAMM token_a, Pump base), false the reverse.
type Quoter interface {
	QuoteExactIn(amountIn uint64, aToB bool) (amountOut uint64, err error)
}

// quoterFunc adapts a plain closure to Quoter.
type quoterFunc func(amountIn uint64, aToB bool) (uint64, error)

func (f quoterFunc) QuoteExactIn(amountIn uint64, aToB bool) (uint64, error) {
	return f(amountIn, aToB)
}

// DLMM binds a Meteora DLMM pool with the swap-timestamp and a bin provider (the
// cached bin-array window the quote walks). aToB == swapForY.
func DLMM(pool dlmm.SwapPool, currentTimestamp int64, bins dlmm.BinProvider) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		return dlmm.QuoteExactIn(pool, aToB, amountIn, currentTimestamp, bins)
	})
}

// Orca binds a Whirlpool (including any adaptive-fee oracle carried on the pool)
// with its tick provider. aToB is Orca's native a-to-b flag.
func Orca(pool orca.SwapPool, ticks orca.TickProvider) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		return orca.QuoteExactIn(pool, aToB, amountIn, ticks)
	})
}

// Raydium binds a Raydium CLMM pool with its tick provider. aToB == zeroForOne.
// Populate FeeOn, Status, DynamicFee and BlockTimestamp too; zero quotes the
// pre-2026-07-31 program.
func Raydium(pool raydium.SwapPool, ticks raydium.TickProvider) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		return raydium.QuoteExactIn(pool, aToB, amountIn, ticks)
	})
}

// DAMMConcentrated binds a Meteora DAMM v2 concentrated pool (CollectFeeMode
// BothToken or OnlyB). CollectFeeMode 2 is refused with damm.ErrCompoundingPool.
func DAMMConcentrated(pool damm.ConcentratedPool) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		dir := damm.TradeDirectionBtoA
		if aToB {
			dir = damm.TradeDirectionAtoB
		}
		return damm.QuoteConcentratedExactIn(amountIn, dir, pool)
	})
}

// RaydiumCPMM binds a Raydium CP-Swap pool by its NET reserves (see
// models.RaydiumCPMMPool.NetReserves) and the total input-side fee rate out of
// 1e6, which is the AmmConfig rate PLUS EffectiveCreatorFeeRate.
func RaydiumCPMM(reserve0, reserve1, feeRate uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return raycpmm.SwapBaseInput(reserve0, reserve1, amountIn, feeRate), nil
		}
		return raycpmm.SwapBaseInput(reserve1, reserve0, amountIn, feeRate), nil
	})
}

// Pump binds a Pump-AMM pool. aToB == sell. feeBps comes from
// models.PumpTotalFeeBps.
//
// quoteReserve is NOT the vault balance: pass EffectiveQuoteReserve, or a buy
// over-predicts by up to 775% on a shallow pool.
func Pump(baseReserve, quoteReserve, feeBps uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return pump.SellExactIn(baseReserve, quoteReserve, amountIn, feeBps), nil
		}
		return pump.BuyExactIn(quoteReserve, baseReserve, amountIn, feeBps), nil
	})
}

// PumpBondingCurve binds a pump.fun PRE-graduation curve by its VIRTUAL reserves.
// aToB == sell. Check BondingCurve.IsSOLQuoted first: a non-zero QuoteMint prices
// in that mint, not lamports, and the reserves do not say which.
func PumpBondingCurve(virtualTokenReserves, virtualQuoteReserves, feeBps uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return pumpbc.SellExactIn(virtualTokenReserves, virtualQuoteReserves, amountIn, feeBps), nil
		}
		return pumpbc.BuyExactIn(virtualQuoteReserves, virtualTokenReserves, amountIn, feeBps), nil
	})
}

// DAMMCompounding binds a Meteora DAMM v2 CollectFeeMode 2 pool. feeNumerator is
// out of damm.FeeDenominator and must be the fee charged NOW, not the cliff:
// resolve it with models.DAMMPool.CurrentBaseFeeNumerator.
func DAMMCompounding(
	tokenAReserve, tokenBReserve, feeNumerator uint64,
	feeOnInput, hasReferral bool,
	protocolFeePercent uint8, compoundingFeeBps uint16, referralFeePercent uint8,
) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		dir := damm.TradeDirectionBtoA
		if aToB {
			dir = damm.TradeDirectionAtoB
		}
		res, err := damm.QuoteExactInCompounding(amountIn, tokenAReserve, tokenBReserve, dir,
			feeNumerator, feeOnInput, hasReferral, protocolFeePercent, compoundingFeeBps, referralFeePercent)
		if err != nil {
			return 0, err
		}
		return res.OutputAmount, nil
	})
}

// FluxBeam binds a FluxBeam constant-product pool by its two sides and its fees.
// Both pool fees come off the input; a Token-2022 transfer fee is charged around
// the curve, on the input and again on the output, as the program does it.
func FluxBeam(a, b FluxBeamSide, curveType uint8, fees models.FluxBeamFees, epoch uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		in, out := a, b
		if !aToB {
			in, out = b, a
		}

		if in.TransferFee != nil {
			amountIn = in.TransferFee.EpochFee(epoch).PostFeeAmount(amountIn)
		}

		amountOut, err := fluxbeam.QuoteExactIn(curveType, in.Reserve, out.Reserve, amountIn, fees)
		if err != nil {
			return 0, err
		}

		if out.TransferFee != nil {
			amountOut = out.TransferFee.EpochFee(epoch).PostFeeAmount(amountOut)
		}

		return amountOut, nil
	})
}
