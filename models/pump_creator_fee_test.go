package models

import (
	"encoding/binary"
	"github.com/gagliardetto/solana-go"
	"math/big"
	"testing"
)

// buildPumpPool makes a Pool account of the given size with a valid
// discriminator, so the cohort-boundary behaviour can be exercised directly.
func buildPumpPool(size int, creatorFeeBps uint64, canEdit, holderReward bool) []byte {
	d := make([]byte, size)
	copy(d[0:8], PumpPoolDiscriminator[:])
	if size >= 269 {
		binary.LittleEndian.PutUint64(d[261:269], creatorFeeBps)
	}
	if size >= 270 && canEdit {
		d[269] = 1
	}
	if size >= 271 && holderReward {
		d[270] = 1
	}
	return d
}

// The live 301-byte cohort is the one that carries an override — 184 pools at
// 300 bps on 2026-09-21.
func TestDecodePumpPoolCreatorFeeFields(t *testing.T) {
	pool, err := DecodePumpPool(buildPumpPool(301, 300, false, true), solanaZero())
	if err != nil {
		t.Fatalf("301-byte pool: %v", err)
	}
	if pool.CreatorFeeBps != 300 {
		t.Fatalf("CreatorFeeBps = %d, want 300", pool.CreatorFeeBps)
	}
	if pool.CanEditCreatorFee {
		t.Fatal("CanEditCreatorFee should be false")
	}
	if !pool.IsHolderReward {
		t.Fatal("IsHolderReward should be true")
	}
}

// Every shorter cohort is still live and must decode, with the fields it cannot
// carry reading as zero rather than failing the account.
func TestDecodePumpPoolShortCohortsStillDecode(t *testing.T) {
	for _, size := range []int{211, 243, 244, 245, 261, 270, 271, 300, 301} {
		pool, err := DecodePumpPool(buildPumpPool(size, 300, true, true), solanaZero())
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		switch {
		case size < 269:
			if pool.CreatorFeeBps != 0 {
				t.Fatalf("size %d: CreatorFeeBps = %d, want 0", size, pool.CreatorFeeBps)
			}
		default:
			if pool.CreatorFeeBps != 300 {
				t.Fatalf("size %d: CreatorFeeBps = %d, want 300", size, pool.CreatorFeeBps)
			}
		}
		if size < 270 && pool.CanEditCreatorFee {
			t.Fatalf("size %d: CanEditCreatorFee set on an account too short to carry it", size)
		}
		if size < 271 && pool.IsHolderReward {
			t.Fatalf("size %d: IsHolderReward set on an account too short to carry it", size)
		}
	}
}

func TestDecodeBondingCurveCreatorFeeFields(t *testing.T) {
	d := make([]byte, 151)
	copy(d[0:8], BondingCurveDiscriminator[:])
	binary.LittleEndian.PutUint64(d[115:123], 300)
	d[124] = 1
	curve, err := DecodeBondingCurve(d, solanaZero())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if curve.CreatorFeeBps != 300 || curve.CanEditCreatorFee || !curve.IsHolderReward {
		t.Fatalf("got bps=%d canEdit=%v holder=%v, want 300/false/true",
			curve.CreatorFeeBps, curve.CanEditCreatorFee, curve.IsHolderReward)
	}
	// The 115-byte cohort predates all three and must still decode.
	short, err := DecodeBondingCurve(d[:115], solanaZero())
	if err != nil {
		t.Fatalf("115-byte curve: %v", err)
	}
	if short.CreatorFeeBps != 0 || short.IsHolderReward {
		t.Fatal("115-byte curve must not report creator fee fields")
	}
}

// Values here are the ones read off the live global_config on 2026-09-21.
func TestDecodePumpGlobalConfigCreatorFeeFlags(t *testing.T) {
	d := make([]byte, 949)
	binary.LittleEndian.PutUint64(d[40:], 20)
	binary.LittleEndian.PutUint64(d[48:], 5)
	binary.LittleEndian.PutUint64(d[313:], 5)
	d[940] = 1
	binary.LittleEndian.PutUint64(d[941:949], 300)

	g, err := DecodePumpGlobalConfig(d)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !g.CreatorFeeConfigurable {
		t.Fatal("CreatorFeeConfigurable should be true")
	}
	if g.MaxConfigurableCreatorFeeBps != 300 {
		t.Fatalf("MaxConfigurableCreatorFeeBps = %d, want 300", g.MaxConfigurableCreatorFeeBps)
	}
	if g.LpBps != 20 || g.ProtocolBps != 5 || g.CoinCreatorBps != 5 {
		t.Fatalf("base fields regressed: %+v", g)
	}

	// The pre-growth account must still decode, reporting "not configurable".
	old, err := DecodePumpGlobalConfig(d[:321])
	if err != nil {
		t.Fatalf("321-byte config: %v", err)
	}
	if old.CreatorFeeConfigurable || old.MaxConfigurableCreatorFeeBps != 0 {
		t.Fatal("a short global_config must not claim overrides are configurable")
	}
}

// The three schedules must come back as three DIFFERENT schedules — reading the
// stable tiers off the SOL offset is the mistake this guards.
func TestDecodePumpFeeConfigAllThreeSchedules(t *testing.T) {
	d := make([]byte, 4097)
	putFees := func(o int, lp, proto, creator uint64) {
		binary.LittleEndian.PutUint64(d[o:], lp)
		binary.LittleEndian.PutUint64(d[o+8:], proto)
		binary.LittleEndian.PutUint64(d[o+16:], creator)
	}
	putTier := func(base, i int, thr uint64, lp, proto, creator uint64) {
		o := base + 4 + i*40
		var buf [16]byte
		binary.LittleEndian.PutUint64(buf[:8], thr)
		copy(d[o:o+16], buf[:])
		putFees(o+16, lp, proto, creator)
	}
	putFees(41, 25, 5, 0) // flat_fees, live values
	binary.LittleEndian.PutUint32(d[65:], 2)
	putTier(65, 0, 0, 2, 93, 30)
	putTier(65, 1, 420_000_000_000, 20, 5, 95) // SOL schedule
	binary.LittleEndian.PutUint32(d[1069:], 2)
	putTier(1069, 0, 0, 2, 93, 30)
	putTier(1069, 1, 59_000_000_000, 20, 5, 95) // stable schedule
	putFees(2073, 20, 5, 5)                     // exotic

	cfg, err := DecodePumpFeeConfig(d)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if cfg.Flat != (PumpFees{LpBps: 25, ProtocolBps: 5, CreatorBps: 0}) {
		t.Fatalf("flat = %+v", cfg.Flat)
	}
	if len(cfg.Tiers) != 2 || len(cfg.StableTiers) != 2 {
		t.Fatalf("tiers=%d stable=%d, want 2 and 2", len(cfg.Tiers), len(cfg.StableTiers))
	}
	if cfg.Tiers[1].MarketCapThreshold.Cmp(big.NewInt(420_000_000_000)) != 0 {
		t.Fatalf("SOL tier 1 threshold = %s, want 420e9", cfg.Tiers[1].MarketCapThreshold)
	}
	if cfg.StableTiers[1].MarketCapThreshold.Cmp(big.NewInt(59_000_000_000)) != 0 {
		t.Fatalf("stable tier 1 threshold = %s, want 59e9", cfg.StableTiers[1].MarketCapThreshold)
	}
	if cfg.ExoticFlat != (PumpFees{LpBps: 20, ProtocolBps: 5, CreatorBps: 5}) {
		t.Fatalf("exotic = %+v, want 20/5/5", cfg.ExoticFlat)
	}
	// A pre-growth 69-byte account must still decode its SOL tiers only.
	small, err := DecodePumpFeeConfig(d[:69])
	if err != nil {
		t.Fatalf("69-byte config: %v", err)
	}
	if len(small.StableTiers) != 0 || small.ExoticFlat != (PumpFees{}) {
		t.Fatal("a short fee_config must not invent stable or exotic schedules")
	}
}

func solanaZero() solana.PublicKey { return solana.PublicKey{} }
