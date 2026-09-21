package models

import (
	"math/big"
	"testing"
)

// Live rate-limiter pools sampled 2026-09-21.
var (
	// 8J2inRu54upzVo22KT2kK9iVYWawsjP62skuo46iDLa
	rlLive = DAMMBaseFee{
		Mode: DAMMBaseFeeModeRateLimiter, CliffFeeNumerator: 40_000_000,
		FeeIncrementBps: 2000, MaxLimiterDuration: 43200, MaxFeeBps: 9900,
		ReferenceAmount: 10_000,
	}
	// DNz8LHnket68Vn5d5hC5ZkaxG6eKoyY7Q1Bvah1oa1B — a gentler slope.
	rlGentle = DAMMBaseFee{
		Mode: DAMMBaseFeeModeRateLimiter, CliffFeeNumerator: 5_000_000,
		FeeIncrementBps: 50, MaxLimiterDuration: 43200, MaxFeeBps: 9900,
		ReferenceAmount: 100_000,
	}
	// The inert shape 1,689 of the 2,106 live pools carry.
	rlInert = DAMMBaseFee{
		Mode: DAMMBaseFeeModeRateLimiter, CliffFeeNumerator: 100_000,
		FeeIncrementBps: 1, MaxLimiterDuration: 1, MaxFeeBps: 1,
		ReferenceAmount: 1_000_000_000,
	}
)

// bruteForceRateLimiterFee is an INDEPENDENT reading of the same rule: walk the
// input in reference-amount chunks, charging chunk k at min(c + i*k, MAX_FEE).
// It shares no algebra with the closed form, so agreement between the two is
// real evidence rather than a restatement.
func bruteForceRateLimiterFee(f DAMMBaseFee, input uint64) uint64 {
	c := new(big.Int).SetUint64(f.CliffFeeNumerator)
	incr := new(big.Int).SetUint64(uint64(f.FeeIncrementBps) * bpsToFeeNumerator)
	maxFee := new(big.Int).SetUint64(uint64(f.MaxFeeBps) * bpsToFeeNumerator)
	x0 := f.ReferenceAmount

	total := new(big.Int)
	left := input
	for k := uint64(0); left > 0; k++ {
		chunk := x0
		if left < chunk {
			chunk = left
		}
		rate := new(big.Int).Add(c, new(big.Int).Mul(incr, new(big.Int).SetUint64(k)))
		if rate.Cmp(maxFee) > 0 {
			rate = maxFee
		}
		total.Add(total, new(big.Int).Mul(new(big.Int).SetUint64(chunk), rate))
		left -= chunk
	}
	den := big.NewInt(dammFeeDenominator)
	q, r := new(big.Int).QuoRem(total, den, new(big.Int))
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Uint64()
}

func TestRateLimiterMatchesChunkWalk(t *testing.T) {
	for name, f := range map[string]DAMMBaseFee{"live": rlLive, "gentle": rlGentle, "inert": rlInert} {
		// Sweep across the reference amount, over the cap, and well past it.
		for _, mult := range []uint64{0, 1, 2, 3, 7, 12, 50, 250} {
			for _, extra := range []uint64{0, 1, 7, 999} {
				in := f.ReferenceAmount*mult + extra
				if in == 0 {
					continue
				}
				got, err := f.RateLimiterFee(in)
				if err != nil {
					t.Fatalf("%s in=%d: %v", name, in, err)
				}
				if want := bruteForceRateLimiterFee(f, in); got != want {
					t.Fatalf("%s in=%d: closed form %d, chunk walk %d", name, in, got, want)
				}
			}
		}
	}
}

// TestRateLimiterExceedsTheCliff is the regression this change exists for: a
// large swap into a rate-limited pool pays MORE than the cliff fee. Reverting
// RateLimiterFee to the flat cliff fails here.
func TestRateLimiterExceedsTheCliff(t *testing.T) {
	// 40 reference amounts in. max_index = (990_000_000-40_000_000)/200_000_000 = 4,
	// so the cap binds long before this and most of the input pays 99%.
	const in = 10_000 * 40
	got, err := rlLive.RateLimiterFee(in)
	if err != nil {
		t.Fatalf("%v", err)
	}
	cliffOnly, err := (DAMMBaseFee{Mode: DAMMBaseFeeModeRateLimiter, CliffFeeNumerator: rlLive.CliffFeeNumerator}).RateLimiterFee(in)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got <= cliffOnly {
		t.Fatalf("rate limiter fee %d did not exceed the flat cliff fee %d", got, cliffOnly)
	}
	// Effective rate must sit between the cliff (4%) and the cap (99%).
	rate := float64(got) / float64(in) * 100
	if rate <= 4.0 || rate >= 99.0 {
		t.Fatalf("effective rate %.2f%% outside (4%%, 99%%)", rate)
	}
	t.Logf("in=%d fee=%d effective=%.2f%% vs cliff-only %d (%.2f%%)",
		in, got, rate, cliffOnly, float64(cliffOnly)/float64(in)*100)
}

func TestRateLimiterSmallSwapPaysCliff(t *testing.T) {
	// At or below the reference amount the limiter is not engaged at all.
	for _, in := range []uint64{1, 5_000, 10_000} {
		got, err := rlLive.RateLimiterFee(in)
		if err != nil {
			t.Fatalf("%v", err)
		}
		want := (in*rlLive.CliffFeeNumerator + dammFeeDenominator - 1) / dammFeeDenominator
		if got != want {
			t.Fatalf("in=%d: fee %d, want flat cliff %d", in, got, want)
		}
	}
}

func TestRateLimiterFeeIsMonotonic(t *testing.T) {
	prevRate := 0.0
	for mult := uint64(1); mult <= 60; mult++ {
		in := rlGentle.ReferenceAmount * mult
		got, err := rlGentle.RateLimiterFee(in)
		if err != nil {
			t.Fatalf("%v", err)
		}
		rate := float64(got) / float64(in)
		if rate < prevRate {
			t.Fatalf("mult=%d: effective rate fell %.6f -> %.6f", mult, prevRate, rate)
		}
		prevRate = rate
	}
	// And it never exceeds the configured cap.
	if prevRate > float64(rlGentle.MaxFeeBps)/10_000 {
		t.Fatalf("effective rate %.6f exceeded the cap", prevRate)
	}
}

func TestRateLimiterGating(t *testing.T) {
	const act = 1_000_000
	if !rlLive.RateLimiterApplies(act+100, act, true) {
		t.Fatal("should apply inside the window on a B->A swap")
	}
	if rlLive.RateLimiterApplies(act+100, act, false) {
		t.Fatal("must not apply on an A->B swap")
	}
	if rlLive.RateLimiterApplies(act+43201, act, true) {
		t.Fatal("must not apply past max_limiter_duration")
	}
	if rlLive.RateLimiterApplies(act-1, act, true) {
		t.Fatal("must not apply before activation")
	}
	// The inert shape has a one-second window.
	if rlInert.RateLimiterApplies(act+2, act, true) {
		t.Fatal("inert pool's window is 1s")
	}
}

func TestRateLimiterRejectsOtherModes(t *testing.T) {
	f := ParseDAMMBaseFee(blob(t, rawTimeLinear))
	if _, err := f.RateLimiterFee(1_000); err != ErrNotRateLimiter {
		t.Fatalf("err = %v, want ErrNotRateLimiter", err)
	}
	if _, err := (DAMMBaseFee{Mode: DAMMBaseFeeModeRateLimiter}).CurrentBaseFeeNumerator(1, 0); err != ErrRateLimiterNeedsAmount {
		t.Fatal("CurrentBaseFeeNumerator must refuse a rate limiter")
	}
}
