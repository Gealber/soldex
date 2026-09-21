// Package soldex is a single source of truth for Solana DEX swap math: on-chain
// account decoders (models/), fixed-point primitives (math/), and exact-in swap
// quotes (quote/) for Orca Whirlpool, Meteora DLMM, Meteora DAMM v2 (cp-amm),
// Raydium CLMM, Raydium CP-Swap (constant-product AMM), FluxBeam, Pump-AMM and
// the pump.fun bonding curve.
//
// Each venue's quote lives in its own quote/<dex> package with the exact state it
// needs (bin arrays, tick arrays, oracles, fee configs). This top-level package
// adds a uniform Quoter over them so a caller can hold a heterogeneous set of pools
// and quote them through one call site.
//
// Start with FromAccount, which decodes an account and dispatches on its owning
// program, or the per-venue From* constructors when the venue is already known.
// Those derive every fee from the pool and refuse a pool that cannot be quoted.
//
// The adapters below (DLMM, Orca, Raydium, ...) are the layer underneath: they
// bind an already-assembled quote struct. They accept whatever fee you give them,
// including none, so prefer a From* constructor unless you are deliberately
// overriding something.
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

// Quoter is the uniform exact-in interface across every supported venue.
//
// aToB fixes the swap direction against the pool's canonical token ordering: when
// true the input is the pool's first token (DLMM X, Orca token_a, Raydium token0,
// DAMM token_a, Pump base) and the output is the second; when false the reverse.
// It maps to each venue's native flag (swapForY / aToB / zeroForOne / TradeDirection
// / sell-vs-buy) inside the adapter.
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
//
// Populate SwapPool.FeeOn, Status, DynamicFee and BlockTimestamp from the decoded pool,
// not just the price and liquidity — the deployed program fills limit orders, charges a
// volatility-driven fee on top of the AmmConfig rate, and can take that fee out of the
// OUTPUT. Leaving them zero quotes the pre-2026-07-31 program.
func Raydium(pool raydium.SwapPool, ticks raydium.TickProvider) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		return raydium.QuoteExactIn(pool, aToB, amountIn, ticks)
	})
}

// DAMMConcentrated binds a Meteora DAMM v2 concentrated-liquidity pool
// (CollectFeeMode BothToken or OnlyB). aToB maps to TradeDirectionAtoB.
//
// A CollectFeeMode 2 (Compounding) pool is refused with damm.ErrCompoundingPool
// rather than quoted on the wrong curve.
func DAMMConcentrated(pool damm.ConcentratedPool) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		dir := damm.TradeDirectionBtoA
		if aToB {
			dir = damm.TradeDirectionAtoB
		}
		return damm.QuoteConcentratedExactIn(amountIn, dir, pool)
	})
}

// RaydiumCPMM binds a Raydium CP-Swap (CPMMoo8L…) constant-product pool by its two
// net vault reserves — the raw vault balances minus the protocol, fund AND creator
// fees the pool tracks; use models.RaydiumCPMMPool.NetReserves to compute them —
// and the total input-side fee rate (out of 1e6). aToB swaps token_0 in for
// token_1 out; !aToB reverses.
//
// feeRate must be the AmmConfig trade fee rate PLUS the pool's effective creator
// fee rate (models.RaydiumCPMMPool.EffectiveCreatorFeeRate).
func RaydiumCPMM(reserve0, reserve1, feeRate uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return raycpmm.SwapBaseInput(reserve0, reserve1, amountIn, feeRate), nil
		}
		return raycpmm.SwapBaseInput(reserve1, reserve0, amountIn, feeRate), nil
	})
}

// Pump binds a Pump-AMM constant-product pool by its base vault reserve, its EFFECTIVE
// quote reserve and the total fee (basis points; compute via models.PumpTotalFeeBps).
// aToB == sell (base in, quote out); !aToB == buy (quote in, base out).
//
// quoteReserve is NOT the quote vault balance: newer pools price with additional
// quote-side reserve held outside the vault, so pass
// models.PumpPool.EffectiveQuoteReserve(vaultQuoteBalance). Passing the raw vault
// balance over-predicts a buy by the offset's share of the pool — 5.5% on a 317 SOL
// pool, 775% on a 2.2 SOL one — which reads as free arbitrage that is not there.
func Pump(baseReserve, quoteReserve, feeBps uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return pump.SellExactIn(baseReserve, quoteReserve, amountIn, feeBps), nil
		}
		return pump.BuyExactIn(quoteReserve, baseReserve, amountIn, feeBps), nil
	})
}

// PumpBondingCurve binds a pump.fun bonding curve (the PRE-graduation curve, not
// the Pump-AMM pool) by its VIRTUAL reserves and total fee in basis points.
// aToB == sell (token in, quote out); !aToB == buy (quote in, token out).
//
// The curve prices on its virtual reserves, not the real ones — pass
// BondingCurve.VirtualTokenReserves and VirtualSolReserves. Check
// BondingCurve.IsSOLQuoted first: a curve carrying a non-zero QuoteMint prices in
// that mint, not lamports, and the reserve fields say nothing about which.
func PumpBondingCurve(virtualTokenReserves, virtualQuoteReserves, feeBps uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return pumpbc.SellExactIn(virtualTokenReserves, virtualQuoteReserves, amountIn, feeBps), nil
		}
		return pumpbc.BuyExactIn(virtualQuoteReserves, virtualTokenReserves, amountIn, feeBps), nil
	})
}

// DAMMCompounding binds a Meteora DAMM v2 pool that collects fees by COMPOUNDING
// them back into liquidity (CollectFeeMode 2) — the mode DAMMConcentrated refuses.
// aToB maps to TradeDirectionAtoB.
//
// feeNumerator is out of damm.FeeDenominator (1e9) and must be the fee the pool
// charges NOW, which for a scheduled pool is not its cliff: resolve it with
// models.DAMMPool.CurrentBaseFeeNumerator. feeOnInput mirrors the pool's fee mode.
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

// FluxBeam binds a FluxBeam constant-product pool by its two vault token-account
// balances and its fees. aToB swaps token A in for token B out.
//
// The fee is the pool's own: FluxBeam pools carry arbitrary creator-set fees, and
// BOTH the trade and owner trade fee come off the input.
func FluxBeam(reserveA, reserveB uint64, curveType uint8, fees models.FluxBeamFees) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return fluxbeam.QuoteExactIn(curveType, reserveA, reserveB, amountIn, fees)
		}

		return fluxbeam.QuoteExactIn(curveType, reserveB, reserveA, amountIn, fees)
	})
}
