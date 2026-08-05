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
| **Raydium CLMM** | `models` (PoolState, tick arrays, dynamic-fee info) | `quote/raydium` — limit orders, dynamic fee, fee_on |
| **Raydium CP-Swap** (CPMMoo8L) | `models` (PoolState, AmmConfig) | `quote/raycpmm` — constant product, fee-on-input |
| **Pump-AMM** (pAMMBay) | `models` (Pool, market-cap fee tiers) | `quote/pump` — constant product |
| **pump.fun bonding curve** (6EF8rrec) | `models` (BondingCurve) | `quote/pumpbc` — constant product on virtual reserves |

## Raydium CLMM: limit orders, dynamic fee, fee_on

The CLMM program was upgraded on 2026-07-31 with three things that change the amount out,
all of which `quote/raydium` now models:

- **Limit orders** resting at an initialized tick, filled at the tick price before the
  swap crosses it. A tick can be initialized on orders *alone*, so `RaydiumTick.Initialized`
  reports liquidity **or** orders — checking gross liquidity walks straight past such a tick.
- **Dynamic fee** — a volatility accumulator added to the AmmConfig fee
  (`total = base_fee_rate + dynamic_fee_rate`, capped at 10%). The swap steps one
  tick-spacing group at a time so the fee can rise as the price travels, which is why
  `SwapPool.BlockTimestamp` matters: a stale timestamp under-decays the reference and
  over-quotes the fee.
- **`fee_on`** (0 FromInput, 1 Token0Only, 2 Token1Only) — for one direction the fee comes
  out of the **output**. On a curve with real price impact that is strictly worse for the
  trader than the same rate on the input.

Measured on chain 2026-08-05: of 178,353 pools, **547** carry a non-zero `dynamic_fee_info`,
**794** have `fee_on != 0`, and **354** `LimitOrderState` accounts exist. Rare — but a
fee-on-output pool is exactly the shape that reads as free arbitrage if you ignore it.

`QuoteExactIn` also refuses a pool whose `status` bit4 disables swaps, rather than returning
a tradable-looking number for a swap that cannot land.

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
