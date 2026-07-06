package orca

import (
	"math/big"
	"testing"
)

// Vectors copied verbatim from the Orca whirlpool program tests
// (manager/fee_rate_manager.rs). They are the program's own pre-computed
// expected fee rates, so matching them proves the port is bit-faithful.

// Mirrors test_get_total_fee_rate: static 1% + adaptive, walking the tick group
// outward one step per iteration from the reference.
func TestAdaptiveTotalFeeRateMatchesOrcaVector(t *testing.T) {
	info := &AdaptiveFeeInfo{
		Constants: AdaptiveFeeConstants{
			FilterPeriod:             30,
			DecayPeriod:              600,
			ReductionFactor:          5000,
			AdaptiveFeeControlFactor: 1500,
			MaxVolatilityAccumulator: 450_000,
			TickGroupSize:            64,
			MajorSwapThresholdTicks:  64,
		},
	}
	const staticFeeRate uint16 = 10_000 // 1%
	const timestamp uint64 = 1738863309

	mgr, ok := newFeeRateManager(true, 1024, timestamp, staticFeeRate, info)
	if !ok {
		t.Fatal("newFeeRateManager returned not-ok")
	}

	want := []uint32{
		10000, 10062, 10246, 10553, 10984, 11536, 12212, 13011, 13933, 14977, 16144, 17435,
		18848, 20384, 22043, 23824, 25729, 27757, 29907, 32180, 34576, 37096, 39737, 42502,
		45390, 48400, 51534, 54790, 58169, 61672, 65296, 69044, 72915, 76909, 81025, 85264,
		89627, 94112, 98720, 100000, 100000, 100000, 100000, 100000, 100000, 100000, 100000,
		100000, 100000, 100000,
	}
	for i, exp := range want {
		mgr.updateVolatilityAccumulator()
		if got := mgr.totalFeeRate(); got != exp {
			t.Fatalf("step %d: total fee rate = %d, want %d", i, got, exp)
		}
		mgr.advanceTickGroup()
	}
}

// Mirrors test_max_volatility_accumulator_should_bound_fee_rate: adaptive rate
// alone (static 0), capped by max_volatility_accumulator.
func TestComputeAdaptiveFeeRateMatchesOrcaVector(t *testing.T) {
	c := AdaptiveFeeConstants{
		FilterPeriod:             30,
		DecayPeriod:              600,
		ReductionFactor:          5000,
		AdaptiveFeeControlFactor: 1500,
		MaxVolatilityAccumulator: 350_000,
		TickGroupSize:            64,
		MajorSwapThresholdTicks:  64,
	}
	want := []uint32{
		0, 62, 246, 553, 984, 1536, 2212, 3011, 3933, 4977, 6144, 7435, 8848, 10384,
		12043, 13824, 15729, 17757, 19907, 22180, 24576, 27096, 29737, 32502, 35390,
		38400, 41534, 44790, 48169, 51672, 55296, 59044, 62915, 66909, 71025, 75264,
		75264, 75264, 75264, 75264, 75264, 75264, 75264, 75264, 75264, 75264, 75264,
		75264, 75264, 75264,
	}
	v := AdaptiveFeeVariables{}
	const base int32 = 16
	if !v.updateReference(base, 1738863309, &c) {
		t.Fatal("updateReference not-ok")
	}
	for delta, exp := range want {
		v.updateVolatilityAccumulator(base+int32(delta), &c)
		if got := computeAdaptiveFeeRate(&c, &v); got != exp {
			t.Fatalf("delta %d: adaptive fee = %d, want %d", delta, got, exp)
		}
	}
}

func TestStaticFeeRateManagerReturnsStatic(t *testing.T) {
	mgr, ok := newFeeRateManager(false, 0, 0, 3000, nil)
	if !ok {
		t.Fatal("not-ok")
	}
	if got := mgr.totalFeeRate(); got != 3000 {
		t.Fatalf("static fee = %d, want 3000", got)
	}
	tgt, skip := mgr.boundedSqrtPriceTarget(big.NewInt(123), big.NewInt(1_000_000))
	if skip || tgt.Cmp(big.NewInt(123)) != 0 {
		t.Fatalf("static manager must not bound target: tgt=%v skip=%v", tgt, skip)
	}
}
