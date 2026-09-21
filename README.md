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
| **FluxBeam** (FLUXubRm) | `models` (SwapV1, packed) | `quote/fluxbeam` — constant product, trade + owner fee, Token-2022 transfer fee |
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

These are not a rarity to be skipped, and they are spreading: `fee_on != 0` went from 794
pools to **9,706** (about 5% of them) in six weeks, and non-zero `dynamic_fee_info` from 547
to **1,208**. A fee-on-output pool is exactly the shape that reads as free arbitrage if you
ignore it.

The port is checked against the program itself, not just unit-tested:
`quote/raydium/clmm_chain_test.go` asserts the exact amount a real swap returned on a pool
running both a dynamic fee and a non-zero `fee_on`, with a companion test that fails if
either feature stops moving the number on that vector.

`QuoteExactIn` also refuses a pool whose `status` bit4 disables swaps, rather than returning
a tradable-looking number for a swap that cannot land.

## Layout

```
models/         on-chain account decoders (discriminator-checked)
math/           fixed-point primitives — common, dlmm, damm, orca, raydium
quote/          exact-in swap math — dlmm, damm, orca, raydium, raycpmm, fluxbeam, pump, pumpbc
soldex.go       unified Quoter, From* constructors and FromAccount dispatch
```

## Usage

### One door for every venue

If you hold accounts from several programs, dispatch on the owner an RPC already
gives you. Adding a venue costs you nothing — the case lives in soldex:

```go
q, err := soldex.FromAccount(acct.Owner, acct.Data, soldex.Aux{
    Now:           uint64(time.Now().Unix()),
    RaydiumTicks:  ticks,
    RaydiumConfig: ammConfig,
})
out, err := q.QuoteExactIn(amountIn, aToB /*true = A→B*/)
```

Dispatching on the **owner** is what makes this safe: Raydium CLMM and CP-Swap share
an account discriminator, so choosing a decoder by discriminator alone picks the
wrong one for whichever you check second.

### Per venue

When you already know the venue, build from the decoded model:

```go
q, err := soldex.FromDAMMPool(pool, currentPoint)
q, err := soldex.FromDLMMPool(pool, ts, bins)
q, err := soldex.FromWhirlpool(pool, oracle, ticks, now)
q, err := soldex.FromRaydiumCLMM(pool, cfg, ticks, blockTime)
q, err := soldex.FromRaydiumCPMM(pool, cfg, vault0, vault1)
q, err := soldex.FromFluxBeamPool(pool, sideA, sideB, epoch)
q, err := soldex.FromPumpPool(pool, global, feeCfg, baseVault, quoteVault, supply)
q, err := soldex.FromBondingCurve(curve, feeBps)
```

**Prefer these over filling a quote struct yourself.** They derive every fee from the
pool rather than accepting one, and they return an error instead of a misleading
number when a pool cannot be quoted — a schedule this package does not model, a
disabled pool, a missing linked config, an empty side. A hand-filled quote struct
takes a zero fee without complaint, and a zero fee over-states every output.

### Underneath

The quote packages stay pure and RPC-free — decode with `models`, feed state in, get
an exact-in output. Providing fresh bin/tick state is the caller's job:

```go
out, err := dlmm.QuoteExactIn(pool, swapForY, amountIn, ts, bins)
out, err := orca.QuoteExactIn(pool, aToB, amountIn, ticks)
out      := pump.SellExactIn(baseReserve, quoteReserve, amountIn, feeBps)
```

## Contributing

`CLAUDE.md` is the working agreement for changing this repo — how quote math is
verified against the chain, what a test has to prove, and how larger changes are
run in steps. Read it before a first change.
