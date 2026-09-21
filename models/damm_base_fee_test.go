package models

import (
	"encoding/hex"
	"testing"
)

// Raw BaseFeeInfo blobs sliced out of live cp-amm Pool accounts on 2026-09-21.
const (
	// 11p89pdmnmtEvudYGS72QbvRgFQH6fB5BydfXET6iXa — time linear, 50% -> 6.00%.
	rawTimeLinear = "0065cd1d0000000000000000000090005802000000000000c49f2e0000000000"
	// 15Y6uBZRxkNxXdfMnKAksfwnWEzzntRSuqEtBkqWhNr — time exponential, 50% -> 6.01%.
	rawTimeExponential = "0065cd1d0000000001000000000078003c00000000000000af00000000000000"
	// 1c9ND2Hxbu3oo1budR83M9jQfLhfohLxJwwT62YhRN6 — rate limiter.
	rawRateLimiter = "a0860100000000000200000000000100010000000100000000ca9a3b00000000"
	// 1dcV5GJh6vspqvgjW4ujikKtipDsA6EK1okoqEcRnHM — mcap linear, 6% -> 0.3%.
	rawMcapLinear = "0087930300000000030000000000b400a5060000004eed00fad4040000000000"
)

func blob(t *testing.T, s string) [32]uint8 {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("bad fixture %q: %v", s, err)
	}
	var out [32]uint8
	copy(out[:], b)
	return out
}

func TestParseDAMMBaseFeeLayouts(t *testing.T) {
	lin := ParseDAMMBaseFee(blob(t, rawTimeLinear))
	if lin.Mode != DAMMBaseFeeModeTimeLinear || lin.CliffFeeNumerator != 500_000_000 ||
		lin.NumberOfPeriod != 144 || lin.PeriodFrequency != 600 || lin.ReductionFactor != 3_055_556 {
		t.Fatalf("time linear parsed wrong: %+v", lin)
	}

	exp := ParseDAMMBaseFee(blob(t, rawTimeExponential))
	if exp.Mode != DAMMBaseFeeModeTimeExponential || exp.CliffFeeNumerator != 500_000_000 ||
		exp.NumberOfPeriod != 120 || exp.PeriodFrequency != 60 || exp.ReductionFactor != 175 {
		t.Fatalf("time exponential parsed wrong: %+v", exp)
	}

	// The rate limiter reuses @16/@20 as two u32s, so reading it with the time
	// layout would give period_frequency = 4294967297 instead of 1 and 1.
	rl := ParseDAMMBaseFee(blob(t, rawRateLimiter))
	if rl.Mode != DAMMBaseFeeModeRateLimiter || rl.CliffFeeNumerator != 100_000 ||
		rl.FeeIncrementBps != 1 || rl.MaxLimiterDuration != 1 || rl.MaxFeeBps != 1 ||
		rl.ReferenceAmount != 1_000_000_000 {
		t.Fatalf("rate limiter parsed wrong: %+v", rl)
	}
	if rl.PeriodFrequency != 0 {
		t.Fatalf("rate limiter must not populate PeriodFrequency, got %d", rl.PeriodFrequency)
	}

	mc := ParseDAMMBaseFee(blob(t, rawMcapLinear))
	if mc.Mode != DAMMBaseFeeModeMcapLinear || mc.CliffFeeNumerator != 60_000_000 ||
		mc.NumberOfPeriod != 180 || mc.SqrtPriceStepBps != 1701 ||
		mc.SchedulerExpirationDuration != 15_552_000 || mc.ReductionFactor != 316_666 {
		t.Fatalf("mcap linear parsed wrong: %+v", mc)
	}
}

// TestDAMMBaseFeeIsNotTheCliff is the regression this whole change exists for:
// a scheduled pool that has run to the end of its schedule charges its FLOOR
// fee, not its cliff. Reverting CurrentBaseFeeNumerator to return the cliff
// fails this test by a factor of 8.3.
func TestDAMMBaseFeeIsNotTheCliff(t *testing.T) {
	const activation = 1_000_000

	lin := ParseDAMMBaseFee(blob(t, rawTimeLinear))
	// 144 periods * 600s, plus slack so the schedule is fully elapsed.
	got, err := lin.CurrentBaseFeeNumerator(activation+144*600+1, activation)
	if err != nil {
		t.Fatalf("linear: %v", err)
	}
	// 500_000_000 - 144*3_055_556 = 59_999_936 (6.00% of 1e9).
	if want := uint64(59_999_936); got != want {
		t.Fatalf("linear floor = %d, want %d", got, want)
	}
	if got == lin.CliffFeeNumerator {
		t.Fatal("linear floor equals the cliff — the scheduler did not run")
	}

	exp := ParseDAMMBaseFee(blob(t, rawTimeExponential))
	got, err = exp.CurrentBaseFeeNumerator(activation+120*60+1, activation)
	if err != nil {
		t.Fatalf("exponential: %v", err)
	}
	// 5e8 * (1 - 175/10000)^120 ~= 6.01e7. Independently: ln(0.9825)*120 = -2.1186,
	// e^-2.1186 = 0.12021, * 5e8 = 60_104_xxx.
	if got < 59_900_000 || got > 60_300_000 {
		t.Fatalf("exponential floor = %d, want ~6.01%% of 1e9", got)
	}
	if got == exp.CliffFeeNumerator {
		t.Fatal("exponential floor equals the cliff — the scheduler did not run")
	}
}

func TestDAMMBaseFeeScheduleProgression(t *testing.T) {
	const activation = 1_000_000
	for _, raw := range []string{rawTimeLinear, rawTimeExponential} {
		f := ParseDAMMBaseFee(blob(t, raw))

		// Before and at activation the pool sits on its cliff.
		for _, point := range []uint64{0, activation - 1, activation} {
			got, err := f.CurrentBaseFeeNumerator(point, activation)
			if err != nil {
				t.Fatalf("mode %d at %d: %v", f.Mode, point, err)
			}
			if got != f.CliffFeeNumerator {
				t.Fatalf("mode %d at point %d = %d, want cliff %d", f.Mode, point, got, f.CliffFeeNumerator)
			}
		}

		// The fee decreases monotonically and never rises again past the end.
		prev := f.CliffFeeNumerator
		for p := uint64(1); p <= uint64(f.NumberOfPeriod)+5; p++ {
			got, err := f.CurrentBaseFeeNumerator(activation+p*f.PeriodFrequency, activation)
			if err != nil {
				t.Fatalf("mode %d period %d: %v", f.Mode, p, err)
			}
			if got > prev {
				t.Fatalf("mode %d period %d: fee rose %d -> %d", f.Mode, p, prev, got)
			}
			prev = got
		}
	}
}

// A static pool is encoded as a time scheduler whose factors are zero. It must
// keep returning the cliff — 1,314,392 of 1,520,729 live pools are this shape.
func TestDAMMBaseFeeStaticPoolKeepsCliff(t *testing.T) {
	var b [32]uint8
	b[0] = 0xA0
	b[1] = 0x86
	b[2] = 0x01 // cliff = 100_000
	f := ParseDAMMBaseFee(b)
	if !f.IsStatic() {
		t.Fatal("all-zero factors must read as static")
	}
	got, err := f.CurrentBaseFeeNumerator(9_999_999, 0)
	if err != nil {
		t.Fatalf("static: %v", err)
	}
	if got != 100_000 {
		t.Fatalf("static fee = %d, want the cliff 100000", got)
	}
}

// The modes we have NOT verified must refuse rather than quietly hand back the
// cliff, which is the bug this change is fixing. The market-cap schedulers are
// still unmodelled; the rate limiter is modelled but has no single numerator, so
// it refuses with its own error pointing at RateLimiterFee.
func TestDAMMBaseFeeUnsupportedModesError(t *testing.T) {
	f := ParseDAMMBaseFee(blob(t, rawMcapLinear))
	if _, err := f.CurrentBaseFeeNumerator(9_999_999, 0); err != ErrUnsupportedBaseFeeMode {
		t.Fatalf("mode %d: err = %v, want ErrUnsupportedBaseFeeMode", f.Mode, err)
	}
	rl := ParseDAMMBaseFee(blob(t, rawRateLimiter))
	if _, err := rl.CurrentBaseFeeNumerator(9_999_999, 0); err != ErrRateLimiterNeedsAmount {
		t.Fatalf("mode %d: err = %v, want ErrRateLimiterNeedsAmount", rl.Mode, err)
	}
}
