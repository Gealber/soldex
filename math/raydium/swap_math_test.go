package raydium

import (
	"math/big"
	"testing"
)

// When the price target is not reached, the whole remaining input is consumed
// (amount_in + fee == amount_remaining) and the price moves down for zeroForOne.
// Mirrors Raydium's own swap_math invariant.
func TestComputeSwapStepConservationWhenTargetNotReached(t *testing.T) {
	current := mustBig("4907934225356241358")
	target := mustBig("4000000000000000000") // far below: unreachable with a small input
	liquidity := big.NewInt(1_000_000_000_000)

	step := ComputeSwapStep(current, target, liquidity, 1_000_000, 2500, true)

	consumed := new(big.Int).Add(step.AmountIn, step.FeeAmount)
	if consumed.Uint64() != 1_000_000 {
		t.Fatalf("consumed = %s, want 1000000 (all remaining when target not reached)", consumed)
	}
	if step.SqrtPriceNext.Cmp(current) >= 0 {
		t.Fatalf("zeroForOne price should decrease: next %s >= current %s", step.SqrtPriceNext, current)
	}
	if step.SqrtPriceNext.Cmp(target) == 0 {
		t.Fatal("should not have reached the far target")
	}
}

// With a tiny price gap and a large input, the step reaches the target exactly
// and consumes no more than the remaining input.
func TestComputeSwapStepReachesTarget(t *testing.T) {
	current := mustBig("4907934225356241358")
	target := new(big.Int).Sub(current, big.NewInt(100_000))
	liquidity := big.NewInt(1_000_000_000_000)

	step := ComputeSwapStep(current, target, liquidity, 1_000_000_000, 2500, true)

	if step.SqrtPriceNext.Cmp(target) != 0 {
		t.Fatalf("next %s should equal target %s", step.SqrtPriceNext, target)
	}
	consumed := new(big.Int).Add(step.AmountIn, step.FeeAmount)
	if consumed.Uint64() > 1_000_000_000 {
		t.Fatalf("consumed %s exceeds remaining", consumed)
	}
}

// one_for_zero (token_1 -> token_0) must move the price up.
func TestComputeSwapStepOneForZeroPriceUp(t *testing.T) {
	current := mustBig("4907934225356241358")
	target := new(big.Int).Add(current, mustBig("900000000000000000"))
	liquidity := big.NewInt(1_000_000_000_000)

	step := ComputeSwapStep(current, target, liquidity, 1_000_000, 2500, false)

	if step.SqrtPriceNext.Cmp(current) <= 0 {
		t.Fatalf("oneForZero price should increase: next %s <= current %s", step.SqrtPriceNext, current)
	}
}
