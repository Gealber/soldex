package soldex

import (
	"encoding/binary"
	"errors"
	"math/big"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/orca"
	soldexray "github.com/Gealber/soldex/quote/raydium"
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

// clmmModel mirrors the live vector frozen in quote/raydium: a pool that runs
// both a dynamic fee and a non-zero fee_on.
func clmmModel() *models.RaydiumCLMMPool {
	sqrtPrice, _ := new(big.Int).SetString("4434507698921048280", 10)
	return &models.RaydiumCLMMPool{
		SqrtPriceX64: u128From(sqrtPrice),
		Liquidity:    bin.Uint128{Lo: 5_885_836},
		TickCurrent:  -28_511,
		TickSpacing:  120,
		FeeOn:        1,
		Status:       0,
		DynamicFee: models.RaydiumDynamicFee{
			FilterPeriod: 180, DecayPeriod: 3600, ReductionFactor: 7000,
			DynamicFeeControl: 25_000, MaxVolatilityAccumulator: 80_000,
			TickSpacingIndexReference: -236, VolatilityAccumulator: 20_000,
			LastUpdateTimestamp: 1_789_971_237,
		},
	}
}

func u128From(v *big.Int) bin.Uint128 {
	mask := new(big.Int).SetUint64(^uint64(0))
	lo := new(big.Int).And(v, mask).Uint64()
	hi := new(big.Int).Rsh(v, 64).Uint64()
	return bin.Uint128{Lo: lo, Hi: hi}
}

func clmmTicks() soldexray.TickProvider {
	bounds := []soldexray.TickBoundary{
		{TickIndex: -16_320, LiquidityNet: big.NewInt(740_027_133), Initialized: true},
		{TickIndex: -14_760, LiquidityNet: big.NewInt(1_942_587_532), Initialized: true},
		{TickIndex: -10_800, LiquidityNet: big.NewInt(-1_942_587_532), Initialized: true},
	}
	return func(fromTick int32, zeroForOne bool) (soldexray.TickBoundary, bool) {
		if zeroForOne {
			for i := len(bounds) - 1; i >= 0; i-- {
				if bounds[i].TickIndex <= fromTick {
					return bounds[i], true
				}
			}
			return soldexray.TickBoundary{}, false
		}
		for i := range bounds {
			if bounds[i].TickIndex > fromTick {
				return bounds[i], true
			}
		}
		return soldexray.TickBoundary{}, false
	}
}

// The constructor must reproduce the amount the PROGRAM returned for this swap.
// A caller who fills SwapPool by hand and misses FeeOn or DynamicFee gets no
// error, just the pre-2026-07-31 program's answer — so this pins it.
func TestFromRaydiumCLMMMatchesTheChainVector(t *testing.T) {
	cfg := &models.RaydiumAmmConfig{TradeFeeRate: 20_000}
	q, err := FromRaydiumCLMM(clmmModel(), cfg, clmmTicks(), 1_789_982_160)
	if err != nil {
		t.Fatalf("%v", err)
	}
	got, err := q.QuoteExactIn(5_000_000, false)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if want := uint64(27_438_436); got != want {
		t.Fatalf("quote = %d, the program returned %d", got, want)
	}
}

func TestFromRaydiumCLMMRefusals(t *testing.T) {
	cfg := &models.RaydiumAmmConfig{TradeFeeRate: 20_000}
	if _, err := FromRaydiumCLMM(clmmModel(), nil, clmmTicks(), 0); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a missing AmmConfig must be refused, not quoted at a zero fee rate")
	}
	disabled := clmmModel()
	disabled.Status = 1 << 4
	if _, err := FromRaydiumCLMM(disabled, cfg, clmmTicks(), 0); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a swap-disabled pool must be refused")
	}
	if _, err := FromRaydiumCLMM(nil, cfg, clmmTicks(), 0); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("nil pool must be refused")
	}
}

func cpmmModel(enableCreatorFee bool, creatorFees0, creatorFees1 uint64) *models.RaydiumCPMMPool {
	return &models.RaydiumCPMMPool{
		ProtocolFeesToken0: 1_000,
		ProtocolFeesToken1: 2_000,
		FundFeesToken0:     3_000,
		FundFeesToken1:     4_000,
		EnableCreatorFee:   enableCreatorFee,
		CreatorFeesToken0:  creatorFees0,
		CreatorFeesToken1:  creatorFees1,
	}
}

// Creator fees sit in the vault but are not swappable, and the creator rate is
// charged on top of the trade rate. Ignoring either over-states the output.
func TestFromRaydiumCPMMChargesTheCreatorFee(t *testing.T) {
	cfg := &models.RaydiumCPMMConfig{TradeFeeRate: 2_500, CreatorFeeRate: 7_500}
	const v0, v1, in = uint64(1_000_000_000), uint64(1_000_000_000), uint64(10_000_000)

	// Identical accruals either side: only the enable flag differs, so the rate is
	// the one variable. Varying the accruals too shifts the reserve ratio and can
	// swamp the fee entirely.
	with, err := FromRaydiumCPMM(cpmmModel(true, 0, 0), cfg, v0, v1)
	if err != nil {
		t.Fatalf("%v", err)
	}
	without, err := FromRaydiumCPMM(cpmmModel(false, 0, 0), cfg, v0, v1)
	if err != nil {
		t.Fatalf("%v", err)
	}
	a, err := with.QuoteExactIn(in, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	b, err := without.QuoteExactIn(in, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if a >= b {
		t.Fatalf("creator-fee pool quoted %d, plain pool %d — the creator fee was not applied", a, b)
	}
}

// The accrued creator fees must come out of the reserves even when the rate is
// what changes the quote, so this isolates the reserve half.
func TestFromRaydiumCPMMNetsOutAccruedFees(t *testing.T) {
	cfg := &models.RaydiumCPMMConfig{TradeFeeRate: 2_500}
	const v0, v1, in = uint64(1_000_000_000), uint64(1_000_000_000), uint64(10_000_000)

	// Same rates either way (creator fee disabled), only the accruals differ.
	lean, err := FromRaydiumCPMM(cpmmModel(false, 400_000_000, 0), cfg, v0, v1)
	if err != nil {
		t.Fatalf("%v", err)
	}
	full, err := FromRaydiumCPMM(cpmmModel(false, 0, 0), cfg, v0, v1)
	if err != nil {
		t.Fatalf("%v", err)
	}
	a, _ := lean.QuoteExactIn(in, true)
	b, _ := full.QuoteExactIn(in, true)
	if a == b {
		t.Fatalf("accrued creator fees did not change the reserves (%d both)", a)
	}
}

func TestFromRaydiumCPMMRefusals(t *testing.T) {
	cfg := &models.RaydiumCPMMConfig{TradeFeeRate: 2_500}
	if _, err := FromRaydiumCPMM(cpmmModel(false, 0, 0), nil, 1_000, 1_000); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a missing AmmConfig must be refused")
	}
	disabled := cpmmModel(false, 0, 0)
	disabled.Status = 1 << 2
	if _, err := FromRaydiumCPMM(disabled, cfg, 1_000_000, 1_000_000); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a swap-disabled pool must be refused")
	}
	// Vaults holding only accrued fees leave nothing swappable.
	if _, err := FromRaydiumCPMM(cpmmModel(false, 0, 0), cfg, 4_000, 1_000_000); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("an empty side must be refused, not quoted to zero")
	}
	if _, err := FromRaydiumCPMM(nil, cfg, 1, 1); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("nil pool must be refused")
	}
}

func pumpPoolModel(virtualQuote int64, creatorFeeBps uint64) *models.PumpPool {
	base := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	pda, _, _ := solana.FindProgramAddress([][]byte{[]byte("pool-authority"), base.Bytes()},
		solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"))
	return &models.PumpPool{
		BaseMint:             base,
		QuoteMint:            solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112"),
		Creator:              pda,
		CoinCreator:          solana.MustPublicKeyFromBase58("11111111111111111111111111111112"),
		VirtualQuoteReserves: virtualQuote,
		CreatorFeeBps:        creatorFeeBps,
	}
}

func pumpConfigs() (*models.PumpGlobalConfig, *models.PumpFeeConfig) {
	return &models.PumpGlobalConfig{
			LpBps: 20, ProtocolBps: 5, CoinCreatorBps: 5,
			CreatorFeeConfigurable: true, MaxConfigurableCreatorFeeBps: 300,
		}, &models.PumpFeeConfig{
			Flat: models.PumpFees{LpBps: 25, ProtocolBps: 5},
			Tiers: []models.PumpFeeTier{
				{MarketCapThreshold: big.NewInt(0),
					Fees: models.PumpFees{LpBps: 2, ProtocolBps: 93, CreatorBps: 30}},
			},
		}
}

// The virtual quote reserve is held outside the vault; pricing on the raw vault
// balance reads the pool as shallower than it is and over-predicts a buy.
func TestFromPumpPoolUsesTheEffectiveQuoteReserve(t *testing.T) {
	g, fc := pumpConfigs()
	const baseVault, quoteVault = uint64(1_000_000_000_000), uint64(300_000_000_000)

	withVirtual, err := FromPumpPool(pumpPoolModel(17_585_000_000, 0), g, fc, baseVault, quoteVault, 1_000_000_000_000)
	if err != nil {
		t.Fatalf("%v", err)
	}
	none, err := FromPumpPool(pumpPoolModel(0, 0), g, fc, baseVault, quoteVault, 1_000_000_000_000)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Buying base with quote: deeper quote reserve returns LESS base per unit in.
	a, err := withVirtual.QuoteExactIn(1_000_000_000, false)
	if err != nil {
		t.Fatalf("%v", err)
	}
	b, err := none.QuoteExactIn(1_000_000_000, false)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if a >= b {
		t.Fatalf("virtual reserve ignored: %d vs %d", a, b)
	}
}

// A per-pool creator fee replaces the schedule's creator component, so it must
// reach the quote.
func TestFromPumpPoolAppliesTheCreatorOverride(t *testing.T) {
	g, fc := pumpConfigs()
	const baseVault, quoteVault, supply = uint64(1_000_000_000_000), uint64(300_000_000_000), uint64(1_000_000_000_000)

	plain, err := FromPumpPool(pumpPoolModel(0, 0), g, fc, baseVault, quoteVault, supply)
	if err != nil {
		t.Fatalf("%v", err)
	}
	override, err := FromPumpPool(pumpPoolModel(0, 300), g, fc, baseVault, quoteVault, supply)
	if err != nil {
		t.Fatalf("%v", err)
	}
	a, _ := plain.QuoteExactIn(1_000_000_000, false)
	b, _ := override.QuoteExactIn(1_000_000_000, false)
	if b >= a {
		t.Fatalf("a 300 bps creator override should return less: %d vs %d", b, a)
	}
}

func TestFromPumpPoolRefusals(t *testing.T) {
	g, fc := pumpConfigs()
	if _, err := FromPumpPool(pumpPoolModel(0, 0), nil, fc, 1, 1, 1); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a missing global config must be refused")
	}
	if _, err := FromPumpPool(pumpPoolModel(0, 0), g, fc, 0, 1_000, 1); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("an empty base side must be refused")
	}
	// A negative virtual reserve can net the quote side to nothing.
	if _, err := FromPumpPool(pumpPoolModel(-1_000, 0), g, fc, 1_000, 500, 1); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a quote side netted to zero must be refused")
	}
	if _, err := FromPumpPool(nil, g, fc, 1, 1, 1); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("nil pool must be refused")
	}
}

func bondingCurve() *models.BondingCurve {
	return &models.BondingCurve{
		VirtualTokenReserves: 1_000_000_000_000_000,
		VirtualSolReserves:   30_000_000_000,
		RealTokenReserves:    800_000_000_000_000,
		TokenTotalSupply:     1_000_000_000_000_000,
	}
}

func TestFromBondingCurveQuotesBothDirections(t *testing.T) {
	q, err := FromBondingCurve(bondingCurve(), 100)
	if err != nil {
		t.Fatalf("%v", err)
	}
	buy, err := q.QuoteExactIn(1_000_000_000, false)
	if err != nil || buy == 0 {
		t.Fatalf("buy = %d, err = %v", buy, err)
	}
	sell, err := q.QuoteExactIn(1_000_000_000, true)
	if err != nil || sell == 0 {
		t.Fatalf("sell = %d, err = %v", sell, err)
	}
	// The fee must reach the quote.
	free, err := FromBondingCurve(bondingCurve(), 0)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if b, _ := free.QuoteExactIn(1_000_000_000, false); b <= buy {
		t.Fatalf("fee-free buy %d should beat fee'd %d", b, buy)
	}
}

// The two curves that must never be quoted: one that has migrated, and one
// whose reserves are not lamports at all.
func TestFromBondingCurveRefusals(t *testing.T) {
	done := bondingCurve()
	done.Complete = true
	if _, err := FromBondingCurve(done, 100); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a migrated curve no longer trades here and must be refused")
	}

	foreign := bondingCurve()
	foreign.QuoteMint = solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	if _, err := FromBondingCurve(foreign, 100); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("a non-SOL-quoted curve must be refused: its reserves are not lamports")
	}

	empty := bondingCurve()
	empty.VirtualSolReserves = 0
	if _, err := FromBondingCurve(empty, 100); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("an empty virtual reserve must be refused")
	}
	if _, err := FromBondingCurve(nil, 100); !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatal("nil curve must be refused")
	}
}
