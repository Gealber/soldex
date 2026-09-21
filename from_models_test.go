package soldex

import (
	"encoding/binary"
	"errors"
	"testing"

	bin "github.com/gagliardetto/binary"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
)

// dammPool builds a decoded pool with a concentrated range wide enough to quote
// and a base fee blob in the given mode.
func dammPool(collectFeeMode uint8, baseFee [32]uint8, activation uint64) *models.DAMMPool {
	// Centred at price 1 (sqrtPrice 2^64) with a wide [0.25, 4] range, matching
	// the fixture quote/damm's own tests use.
	p := &models.DAMMPool{
		SqrtPrice:       bin.Uint128{Hi: 1},       // 2^64
		SqrtMinPrice:    bin.Uint128{Lo: 1 << 63}, // 2^63
		SqrtMaxPrice:    bin.Uint128{Hi: 2},       // 2^65
		Liquidity:       bin.Uint128{Hi: 1 << 32}, // 2^96
		CollectFeeMode:  collectFeeMode,
		ActivationPoint: activation,
		TokenAAmount:    10_000_000_000,
		TokenBAmount:    4_000_000_000,
	}
	p.PoolFees.BaseFee.BaseFeeInfo = baseFee
	return p
}

// staticBaseFee is a time-scheduler blob whose factors are zero, i.e. a pool that
// never moves off its cliff.
func staticBaseFee(cliff uint64) [32]uint8 {
	var b [32]uint8
	binary.LittleEndian.PutUint64(b[0:8], cliff)
	return b
}

// scheduledBaseFee decays linearly from cliff by reduction per period.
func scheduledBaseFee(cliff, reduction uint64, periods uint16, freq uint64) [32]uint8 {
	var b [32]uint8
	binary.LittleEndian.PutUint64(b[0:8], cliff)
	b[8] = models.DAMMBaseFeeModeTimeLinear
	binary.LittleEndian.PutUint16(b[14:16], periods)
	binary.LittleEndian.PutUint64(b[16:24], freq)
	binary.LittleEndian.PutUint64(b[24:32], reduction)
	return b
}

// The bug this constructor exists to make unrepresentable: a caller filling
// damm.ConcentratedPool by hand leaves BaseFeeNumerator at zero, and a zero fee
// silently over-states every output.
func TestFromDAMMPoolChargesAFee(t *testing.T) {
	const activation = 1_000_000
	pool := dammPool(0, staticBaseFee(2_500_000), activation)

	q, err := FromDAMMPool(pool, activation)
	if err != nil {
		t.Fatalf("%v", err)
	}
	withFee, err := q.QuoteExactIn(1_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// What the hand-mapped path produced: same pool, fee left at zero.
	noFee, err := DAMMConcentrated(damm.ConcentratedPool{
		SqrtPrice:    pool.SqrtPrice.BigInt(),
		SqrtMinPrice: pool.SqrtMinPrice.BigInt(),
		SqrtMaxPrice: pool.SqrtMaxPrice.BigInt(),
		Liquidity:    pool.Liquidity.BigInt(),
		FeeVersion:   pool.FeeVersion,
	}).QuoteExactIn(1_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if withFee >= noFee {
		t.Fatalf("constructor quoted %d, a zero-fee pool quotes %d — the fee was not applied", withFee, noFee)
	}
}

// A scheduled pool must be quoted on the fee it charges NOW, not its cliff.
func TestFromDAMMPoolResolvesTheSchedule(t *testing.T) {
	const activation, freq = uint64(1_000_000), uint64(600)
	// 50% cliff decaying to 6% over 144 periods.
	pool := dammPool(0, scheduledBaseFee(500_000_000, 3_055_556, 144, freq), activation)

	atCliff, err := FromDAMMPool(pool, activation)
	if err != nil {
		t.Fatalf("%v", err)
	}
	decayed, err := FromDAMMPool(pool, activation+144*freq+1)
	if err != nil {
		t.Fatalf("%v", err)
	}

	cliffOut, err := atCliff.QuoteExactIn(1_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	decayedOut, err := decayed.QuoteExactIn(1_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if decayedOut <= cliffOut {
		t.Fatalf("a decayed schedule (%d) should return MORE than the cliff (%d)", decayedOut, cliffOut)
	}
}

// CollectFeeMode 2 must be routed to the compounding quote, not refused: the
// point of the constructor is one door for every fee mode.
func TestFromDAMMPoolRoutesCompounding(t *testing.T) {
	const activation = 1_000_000
	pool := dammPool(damm.CollectFeeModeCompounding, staticBaseFee(2_500_000), activation)

	q, err := FromDAMMPool(pool, activation)
	if err != nil {
		t.Fatalf("compounding pool should be quotable: %v", err)
	}
	out, err := q.QuoteExactIn(1_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if out == 0 {
		t.Fatal("compounding quote returned zero")
	}
	// The concentrated path refuses it, which is what makes the routing matter.
	if _, err := DAMMConcentrated(damm.ConcentratedPool{
		SqrtPrice:      pool.SqrtPrice.BigInt(),
		SqrtMinPrice:   pool.SqrtMinPrice.BigInt(),
		SqrtMaxPrice:   pool.SqrtMaxPrice.BigInt(),
		Liquidity:      pool.Liquidity.BigInt(),
		CollectFeeMode: damm.CollectFeeModeCompounding,
	}).QuoteExactIn(1_000_000, true); !errors.Is(err, damm.ErrCompoundingPool) {
		t.Fatalf("concentrated path should refuse compounding, got %v", err)
	}
}

// A mode whose schedule is not modelled must refuse rather than quote on a
// fallback fee.
func TestFromDAMMPoolRefusesUnmodelledSchedule(t *testing.T) {
	var mcap [32]uint8
	binary.LittleEndian.PutUint64(mcap[0:8], 60_000_000)
	mcap[8] = models.DAMMBaseFeeModeMcapLinear

	if _, err := FromDAMMPool(dammPool(0, mcap, 1_000_000), 2_000_000); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatalf("err = %v, want ErrPoolNotQuotable", err)
	}
	if _, err := FromDAMMPool(nil, 0); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatalf("nil pool: err = %v, want ErrPoolNotQuotable", err)
	}
}
