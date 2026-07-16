# soldex

Single source of truth for Solana DEX swap math in Go — fixed-point, exact-in
quote computation across the major concentrated-liquidity and constant-product
venues, decoded straight from on-chain account state.

`module github.com/Gealber/soldex` · Go 1.25 · solana-go v1.19

> **Note:** this code was mainly AI-generated.

## Supported venues

| Venue | Model | Quote |
|-------|-------|-------|
| **Meteora DLMM** | `models` (LbPair, BinArray, bitmap) | `quote/dlmm` — bin-crossing, base+variable fee |
| **Meteora DAMM v2** (cp-amm) | `models` (Pool) | `quote/damm` — concentrated + compounding, dynamic fee |
| **Orca Whirlpool** | `models` (Whirlpool, oracle, fixed & dynamic tick arrays) | `quote/orca` — adaptive-fee port |
| **Raydium CLMM** | `models` (PoolState, tick arrays) | `quote/raydium` |
| **Raydium CP-Swap** (CPMMoo8L) | `models` (PoolState, AmmConfig) | `quote/raycpmm` — constant product, fee-on-input |
| **Pump-AMM** (pAMMBay) | `models` (Pool, market-cap fee tiers) | `quote/pump` — constant product |
| **pump.fun bonding curve** (6EF8rrec) | `models` (BondingCurve) | `quote/pumpbc` — constant product on virtual reserves |

## Layout

```
models/         on-chain account decoders (discriminator-checked)
math/           fixed-point primitives — common, dlmm, damm, orca, raydium
quote/          exact-in swap math — dlmm, damm, orca, raydium, raycpmm, pump, pumpbc
soldex.go       unified Quoter over all venues
```

## Usage

Call a venue's quote package directly with its state:

```go
out, err := dlmm.QuoteExactIn(pool, swapForY, amountIn, ts, bins)
out, err := orca.QuoteExactIn(pool, aToB, amountIn, ticks)
out      := pump.SellExactIn(baseReserve, quoteReserve, amountIn, feeBps)
```

…or hold a heterogeneous set through the uniform `Quoter` (each adapter binds a
decoded pool plus its auxiliary state; `aToB` selects direction against the pool's
canonical token ordering):

```go
q := soldex.Orca(pool, ticks)                     // or DLMM / DAMMConcentrated / Raydium / Pump
out, err := q.QuoteExactIn(amountIn, aToB /*true = A→B*/)
```

The quote packages are pure and RPC-free — decode accounts with `models`, feed the
state in, get an exact-in output. Providing fresh bin/tick state is the caller's job.
