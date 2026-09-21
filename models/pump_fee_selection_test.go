package models

import (
	"math/big"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// liveFeeConfig mirrors the shape of the live fee_config read on 2026-09-21:
// flat 25/5/0, a SOL tier schedule starting at 420e9, a stable schedule starting
// at 59e9, and exotic flat 20/5/5.
func liveFeeConfig() *PumpFeeConfig {
	return &PumpFeeConfig{
		Flat: PumpFees{LpBps: 25, ProtocolBps: 5, CreatorBps: 0},
		Tiers: []PumpFeeTier{
			{MarketCapThreshold: big.NewInt(0), Fees: PumpFees{LpBps: 2, ProtocolBps: 93, CreatorBps: 30}},
			{MarketCapThreshold: big.NewInt(420_000_000_000), Fees: PumpFees{LpBps: 20, ProtocolBps: 5, CreatorBps: 95}},
		},
		StableTiers: []PumpFeeTier{
			{MarketCapThreshold: big.NewInt(0), Fees: PumpFees{LpBps: 2, ProtocolBps: 93, CreatorBps: 30}},
			{MarketCapThreshold: big.NewInt(59_000_000_000), Fees: PumpFees{LpBps: 20, ProtocolBps: 5, CreatorBps: 95}},
		},
		ExoticFlat: PumpFees{LpBps: 20, ProtocolBps: 5, CreatorBps: 5},
	}
}

func liveGlobal() *PumpGlobalConfig {
	return &PumpGlobalConfig{LpBps: 20, ProtocolBps: 5, CoinCreatorBps: 5,
		CreatorFeeConfigurable: true, MaxConfigurableCreatorFeeBps: 300}
}

// graduatePool builds a pool whose creator is the bonding-curve pool-authority
// PDA, which is what marks it a pump.fun graduate.
func graduatePool(quote solana.PublicKey, creatorFeeBps uint64) *PumpPool {
	base := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	pda, _, _ := solana.FindProgramAddress([][]byte{[]byte("pool-authority"), base.Bytes()}, pumpBondingProgram)
	return &PumpPool{
		BaseMint: base, QuoteMint: quote, Creator: pda,
		CoinCreator:   solana.MustPublicKeyFromBase58("11111111111111111111111111111112"),
		CreatorFeeBps: creatorFeeBps,
	}
}

// A stablecoin-quoted graduate must be tiered on the STABLE thresholds. At a
// market cap of 100e9 the two schedules disagree: stable is past its 59e9 tier,
// SOL is still below its 420e9 one.
func TestPumpFeeStableTiersUseTheirOwnThresholds(t *testing.T) {
	fc, g := liveFeeConfig(), liveGlobal()
	// quoteReserve*supply/baseReserve = 100e9
	const baseReserve, quoteReserve, supply = uint64(1), uint64(100_000_000_000), uint64(1)

	stable := PumpTotalFeeBps(g, fc, graduatePool(pumpUSDCMint, 0), baseReserve, quoteReserve, supply)
	sol := PumpTotalFeeBps(g, fc, graduatePool(pumpWSOLMint, 0), baseReserve, quoteReserve, supply)
	if stable == sol {
		t.Fatalf("stable and SOL schedules agreed at 100e9 (%d) — the stable tiers were not used", stable)
	}
	if want := uint64(20 + 5 + 95); stable != want {
		t.Fatalf("stable total = %d, want %d (tier 1)", stable, want)
	}
	if want := uint64(2 + 93 + 30); sol != want {
		t.Fatalf("SOL total = %d, want %d (tier 0)", sol, want)
	}
	// USDT classifies the same way as USDC.
	if got := PumpTotalFeeBps(g, fc, graduatePool(pumpUSDTMint, 0), baseReserve, quoteReserve, supply); got != stable {
		t.Fatalf("USDT total = %d, want %d", got, stable)
	}
}

// The regression that matters most: a per-pool creator fee REPLACES the
// schedule's creator component. 283 live pools carry one, 184 of them at 300 bps
// against a tier creator fee of 30 — ignoring it understates the fee by 270 bps.
func TestPumpFeeCreatorOverrideReplacesScheduleFee(t *testing.T) {
	fc, g := liveFeeConfig(), liveGlobal()
	const baseReserve, quoteReserve, supply = uint64(1), uint64(1_000_000), uint64(1)

	plain := PumpTotalFeeBps(g, fc, graduatePool(pumpWSOLMint, 0), baseReserve, quoteReserve, supply)
	over := PumpTotalFeeBps(g, fc, graduatePool(pumpWSOLMint, 300), baseReserve, quoteReserve, supply)

	if want := uint64(2 + 93 + 30); plain != want {
		t.Fatalf("no override: %d, want %d", plain, want)
	}
	// 2 + 93 + 300 — the override REPLACES the 30, it is not added to it.
	if want := uint64(2 + 93 + 300); over != want {
		t.Fatalf("override: %d, want %d (replace, not add)", over, want)
	}
	if over-plain != 270 {
		t.Fatalf("override moved the fee by %d bps, want 270", over-plain)
	}
}

// The override is capped by the global maximum and gated by the global flag.
func TestPumpFeeCreatorOverrideCapAndGate(t *testing.T) {
	fc, g := liveFeeConfig(), liveGlobal()
	const br, qr, sup = uint64(1), uint64(1_000_000), uint64(1)

	if got := PumpTotalFeeBps(g, fc, graduatePool(pumpWSOLMint, 5_000), br, qr, sup); got != 2+93+300 {
		t.Fatalf("uncapped override = %d, want the 300 bps cap applied", got)
	}
	off := liveGlobal()
	off.CreatorFeeConfigurable = false
	if got := PumpTotalFeeBps(off, fc, graduatePool(pumpWSOLMint, 300), br, qr, sup); got != 2+93+30 {
		t.Fatalf("override honoured while disabled globally: %d", got)
	}
}

// A holder reward REDIRECTS the creator fee; it must not add to the total.
func TestPumpFeeHolderRewardDoesNotAddFee(t *testing.T) {
	fc, g := liveFeeConfig(), liveGlobal()
	const br, qr, sup = uint64(1), uint64(1_000_000), uint64(1)
	p := graduatePool(pumpWSOLMint, 0)
	base := PumpTotalFeeBps(g, fc, p, br, qr, sup)
	p.IsHolderReward = true
	if got := PumpTotalFeeBps(g, fc, p, br, qr, sup); got != base {
		t.Fatalf("holder reward changed the fee %d -> %d", base, got)
	}
}

// An exotic quote mint and the plain flat schedule both total 30 bps, so this
// branch cannot move a quote — but it must still not fall through to the tiers.
func TestPumpFeeExoticQuoteIsFlat(t *testing.T) {
	fc, g := liveFeeConfig(), liveGlobal()
	exotic := solana.MustPublicKeyFromBase58("Hn6YPJUNh2f94hxumAYbRrMTSVL2D5Epj8AnuSU9QNNS")
	got := PumpTotalFeeBps(g, fc, graduatePool(exotic, 0), 1, 1_000_000_000_000, 1)
	if got != 20+5+5 {
		t.Fatalf("exotic total = %d, want 30", got)
	}
	if got != fc.Flat.LpBps+fc.Flat.ProtocolBps+fc.Flat.CreatorBps {
		t.Fatalf("exotic (%d) should still total the same 30 bps as flat_fees", got)
	}
}

// A non-graduate keeps the flat schedule regardless of quote mint.
func TestPumpFeeNonGraduateIsFlat(t *testing.T) {
	fc, g := liveFeeConfig(), liveGlobal()
	p := &PumpPool{
		BaseMint:  solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112"),
		QuoteMint: pumpWSOLMint,
		Creator:   solana.MustPublicKeyFromBase58("11111111111111111111111111111112"),
	}
	if got := PumpTotalFeeBps(g, fc, p, 1, 1_000_000_000_000, 1); got != 30 {
		t.Fatalf("non-graduate total = %d, want 30", got)
	}
}

// An unset quote mint must read as SOL, not exotic: the exotic schedule charges
// 30 bps against the tiers' 125, so defaulting the other way would under-charge
// the fee and over-state the output.
func TestPumpFeeUnsetQuoteMintReadsAsSOL(t *testing.T) {
	if got := ClassifyPumpQuoteMint(solana.PublicKey{}); got != PumpQuoteSOL {
		t.Fatalf("zero mint classified as %d, want PumpQuoteSOL", got)
	}
	fc, g := liveFeeConfig(), liveGlobal()
	p := graduatePool(pumpWSOLMint, 0)
	withSOL := PumpTotalFeeBps(g, fc, p, 1, 1_000_000, 1)
	p.QuoteMint = solana.PublicKey{}
	if got := PumpTotalFeeBps(g, fc, p, 1, 1_000_000, 1); got != withSOL {
		t.Fatalf("unset quote mint gave %d, want the SOL result %d", got, withSOL)
	}
}
