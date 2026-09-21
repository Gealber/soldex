package soldex

import (
	"encoding/binary"
	"errors"
	"math/big"
	"testing"

	bin "github.com/gagliardetto/binary"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/orca"
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

// dlmmBins is a bin window with liquidity on the Y side of the active bin.
func dlmmBins() dlmm.BinProvider {
	// Deep enough that a sub-basis-point variable fee is still worth whole units;
	// on a tiny bin it rounds to zero and stops discriminating.
	bins := map[int32]dlmm.BinReserves{
		0: {AmountY: 50_000_000}, -1: {AmountY: 50_000_000}, -2: {AmountY: 50_000_000},
	}
	return func(id int32) (dlmm.BinReserves, bool) {
		r, ok := bins[id]
		return r, ok
	}
}

func dlmmModel(status uint8) *models.DLMMPool {
	p := &models.DLMMPool{ActiveID: 0, BinStep: 10, Status: status}
	p.Parameters.BaseFactor = 5_000
	p.Parameters.VariableFeeControl = 40_000
	p.Parameters.MaxVolatilityAccumulator = 350_000
	p.Parameters.FilterPeriod = 30
	p.Parameters.DecayPeriod = 600
	p.Parameters.ReductionFactor = 5_000
	p.VParameters.VolatilityAccumulator = 100_000
	p.VParameters.IndexReference = 0
	p.VParameters.LastUpdateTimestamp = 1_700_000_000
	return p
}

// The constructor must carry EVERY fee parameter across. Dropping any one of
// them silently changes the fee, so this compares it against the same pool
// mapped by hand.
func TestFromDLMMPoolMapsEveryFeeParameter(t *testing.T) {
	pool := dlmmModel(0)
	const ts = int64(1_700_000_300)

	q, err := FromDLMMPool(pool, ts, dlmmBins())
	if err != nil {
		t.Fatalf("%v", err)
	}
	got, err := q.QuoteExactIn(20_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}

	sp, vp := pool.Parameters, pool.VParameters
	want, err := DLMM(dlmm.SwapPool{
		ActiveID: pool.ActiveID, BinStep: pool.BinStep,
		BaseFactor: sp.BaseFactor, BaseFeePowerFactor: sp.BaseFeePowerFactor,
		VariableFeeControl:       sp.VariableFeeControl,
		MaxVolatilityAccumulator: sp.MaxVolatilityAccumulator,
		FilterPeriod:             sp.FilterPeriod, DecayPeriod: sp.DecayPeriod,
		ReductionFactor: sp.ReductionFactor, CollectFeeMode: sp.CollectFeeMode,
		VolatilityAccumulator: vp.VolatilityAccumulator,
		VolatilityReference:   vp.VolatilityReference,
		IndexReference:        vp.IndexReference,
		LastUpdateTimestamp:   vp.LastUpdateTimestamp,
	}, ts, dlmmBins()).QuoteExactIn(20_000_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got != want {
		t.Fatalf("constructor quoted %d, hand-mapped pool quoted %d — a parameter was dropped", got, want)
	}
	if got == 0 {
		t.Fatal("fixture quoted zero, so it cannot detect a dropped parameter")
	}
}

// A fee parameter that is actually load-bearing: with no variable fee the quote
// must differ, which is what makes the mapping test above meaningful.
func TestFromDLMMPoolFeeParametersAreLoadBearing(t *testing.T) {
	const ts = int64(1_700_000_300)
	withVar, err := FromDLMMPool(dlmmModel(0), ts, dlmmBins())
	if err != nil {
		t.Fatalf("%v", err)
	}
	flat := dlmmModel(0)
	flat.Parameters.VariableFeeControl = 0
	flat.VParameters.VolatilityAccumulator = 0
	noVar, err := FromDLMMPool(flat, ts, dlmmBins())
	if err != nil {
		t.Fatalf("%v", err)
	}
	a, _ := withVar.QuoteExactIn(20_000_000, true)
	b, _ := noVar.QuoteExactIn(20_000_000, true)
	if a == b {
		t.Fatalf("variable fee is inert on this fixture (%d both), so the mapping test proves little", a)
	}
}

// A disabled pair cannot trade, so quoting it returns a number for a swap that
// would revert.
func TestFromDLMMPoolRefusesDisabledPair(t *testing.T) {
	if _, err := FromDLMMPool(dlmmModel(1), 0, dlmmBins()); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a non-enabled pair must be refused")
	}
	if _, err := FromDLMMPool(nil, 0, dlmmBins()); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("nil pool must be refused")
	}
}

func whirlpoolModel() *models.Whirlpool {
	return &models.Whirlpool{
		SqrtPrice:        bin.Uint128{Hi: 1}, // 2^64, price 1
		Liquidity:        bin.Uint128{Lo: 2_000_000},
		TickCurrentIndex: 0,
		TickSpacing:      64,
		FeeRate:          3_000, // 0.3%
	}
}

// orcaTicks reports the far edge of the tick range with no liquidity change, so
// the swap runs on the active range alone.
func orcaTicks() orca.TickProvider {
	return func(fromTick int32, aToB bool) (orca.TickBoundary, bool) {
		if aToB {
			return orca.TickBoundary{TickIndex: -443_636, LiquidityNet: big.NewInt(0)}, true
		}
		return orca.TickBoundary{TickIndex: 443_636, LiquidityNet: big.NewInt(0)}, true
	}
}

func adaptiveOracle(enableAt uint64) *models.WhirlpoolOracle {
	return &models.WhirlpoolOracle{
		TradeEnableTimestamp:         enableAt,
		FilterPeriod:                 30,
		DecayPeriod:                  600,
		ReductionFactor:              500,
		AdaptiveFeeControlFactor:     4_000,
		MaxVolatilityAccumulator:     350_000,
		TickGroupSize:                64,
		MajorSwapThresholdTicks:      64,
		VolatilityAccumulator:        200_000,
		TickGroupIndexReference:      0,
		LastReferenceUpdateTimestamp: 1_700_000_000,
	}
}

// An adaptive-fee pool quoted without its oracle loses the volatility surcharge
// entirely, so the constructor must carry it.
func TestFromWhirlpoolAppliesTheOracle(t *testing.T) {
	const now = uint64(1_700_000_100)
	pool := whirlpoolModel()

	withOracle, err := FromWhirlpool(pool, adaptiveOracle(0), orcaTicks(), now)
	if err != nil {
		t.Fatalf("%v", err)
	}
	staticOnly, err := FromWhirlpool(pool, nil, orcaTicks(), now)
	if err != nil {
		t.Fatalf("%v", err)
	}

	a, err := withOracle.QuoteExactIn(100_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	b, err := staticOnly.QuoteExactIn(100_000, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if a == 0 || b == 0 {
		t.Fatalf("fixture quoted zero (%d, %d)", a, b)
	}
	if a >= b {
		t.Fatalf("adaptive fee quoted %d, static-only %d — the surcharge was not applied", a, b)
	}
}

// A pool that has not opened yet must be refused, not quoted.
func TestFromWhirlpoolRefusesUntradablePool(t *testing.T) {
	const now = uint64(1_700_000_100)
	if _, err := FromWhirlpool(whirlpoolModel(), adaptiveOracle(now+1), orcaTicks(), now); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a pool gated until a future timestamp must be refused")
	}
	// At the enable time it becomes quotable.
	if _, err := FromWhirlpool(whirlpoolModel(), adaptiveOracle(now), orcaTicks(), now); err != nil {
		t.Fatalf("pool should be quotable at its enable time: %v", err)
	}
	if _, err := FromWhirlpool(nil, nil, orcaTicks(), now); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("nil pool must be refused")
	}
}
