// Package soldex is a single source of truth for Solana DEX swap math: on-chain
// account decoders (models/), fixed-point primitives (math/), and exact-in swap
// quotes (quote/) for Orca Whirlpool, Meteora DLMM, Meteora DAMM v2 (cp-amm),
// Raydium CLMM, Raydium CP-Swap (constant-product AMM), and Pump-AMM.
//
// Each venue's quote lives in its own quote/<dex> package with the exact state it
// needs (bin arrays, tick arrays, oracles, fee configs). This top-level package
// adds a uniform Quoter over them so a caller can hold a heterogeneous set of pools
// and quote them through one call site — the adapter constructors bind a decoded
// pool plus its auxiliary state and expose the common signature.
package soldex

import (
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/orca"
	"github.com/Gealber/soldex/quote/pump"
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
func Raydium(pool raydium.SwapPool, ticks raydium.TickProvider) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		return raydium.QuoteExactIn(pool, aToB, amountIn, ticks)
	})
}

// DAMMConcentrated binds a Meteora DAMM v2 concentrated-liquidity pool
// (CollectFeeMode BothToken or OnlyB). aToB maps to TradeDirectionAtoB.
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
// net vault reserves — the raw vault balances minus the protocol+fund fees the pool
// tracks; use models.RaydiumCPMMPool.NetReserves to compute them — and the trade fee
// rate (out of 1e6, from the linked AmmConfig). aToB swaps token_0 in for token_1
// out; !aToB reverses.
func RaydiumCPMM(reserve0, reserve1, tradeFeeRate uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return raycpmm.SwapBaseInput(reserve0, reserve1, amountIn, tradeFeeRate), nil
		}
		return raycpmm.SwapBaseInput(reserve1, reserve0, amountIn, tradeFeeRate), nil
	})
}

// Pump binds a Pump-AMM constant-product pool by its two vault reserves and the
// total fee (basis points; compute via models.PumpTotalFeeBps). aToB == sell
// (base in, quote out); !aToB == buy (quote in, base out).
func Pump(baseReserve, quoteReserve, feeBps uint64) Quoter {
	return quoterFunc(func(amountIn uint64, aToB bool) (uint64, error) {
		if aToB {
			return pump.SellExactIn(baseReserve, quoteReserve, amountIn, feeBps), nil
		}
		return pump.BuyExactIn(quoteReserve, baseReserve, amountIn, feeBps), nil
	})
}
