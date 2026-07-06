package orca

import (
	"math/big"
	"testing"
)

// TestQuoteAdaptiveFeeReducesOutput proves the adaptive surcharge flows through
// QuoteExactIn: a swap that crosses several tick groups on an adaptive-fee pool
// must yield strictly less output than the same pool quoted with the static fee
// only, because the volatility surcharge grows as groups are crossed. This is
// exactly the over-quote the static-only path produced.
func TestQuoteAdaptiveFeeReducesOutput(t *testing.T) {
	const spacing uint16 = 64
	startTick := int32(0)
	crossTick := int32(64 * 40) // far enough to cross many tick groups

	base := SwapPool{
		SqrtPrice:        sqrtAt(startTick),
		Liquidity:        big.NewInt(2_000_000_000),
		TickCurrentIndex: startTick,
		TickSpacing:      spacing,
		FeeRate:          3000, // 0.3% static
	}
	ticks := staticTicks(map[int32]*big.Int{crossTick: big.NewInt(0)})

	const amountIn = uint64(50_000_000)
	staticOut, err := QuoteExactIn(base, false, amountIn, ticks)
	if err != nil {
		t.Fatalf("static quote: %v", err)
	}

	adaptive := base
	adaptive.Timestamp = 10_000_000 // far past any reference -> reference resets to current group
	adaptive.AdaptiveFee = &AdaptiveFeeInfo{
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
	adaptiveOut, err := QuoteExactIn(adaptive, false, amountIn, ticks)
	if err != nil {
		t.Fatalf("adaptive quote: %v", err)
	}

	if adaptiveOut >= staticOut {
		t.Fatalf("adaptive output %d must be < static output %d (surcharge not applied)", adaptiveOut, staticOut)
	}
}
