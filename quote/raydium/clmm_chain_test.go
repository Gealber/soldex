package raydium

import (
	"math/big"
	"sort"
	"testing"

	"github.com/Gealber/soldex/models"
)

// This is the check the v0.2.0 Raydium CLMM port never had: a swap quoted here
// against the amount the PROGRAM ITSELF returned for the same swap on the same
// state.
//
// Captured 2026-09-21 from pool 8BpQLg98HAyGRp4bvP7RUESdQhEvSH9xSPLTJ2pmozGL by
// opt/clmmsim, which builds a real swap, simulates it, and decodes the program's
// own SwapEvent. The pool was chosen because it runs BOTH features the port added
// and neither is inert on it — see TestCLMMChainVectorFeaturesAreLive, which
// fails if a future change makes either stop mattering, since a match would then
// be silent about the very code this vector exists to cover.
//
// The dynamic fee decays against BlockTimestamp, so that is frozen here too; a
// wall clock would make this test drift.
const (
	chainVectorAmountIn = uint64(5_000_000)
	chainVectorExpected = uint64(27_438_436) // the program's SwapEvent amount
	chainVectorBlockTs  = uint64(1_789_982_160)
)

func chainVectorPool() SwapPool {
	sqrtPrice, _ := new(big.Int).SetString("4434507698921048280", 10)
	return SwapPool{
		SqrtPrice:   sqrtPrice,
		Liquidity:   big.NewInt(5_885_836),
		TickCurrent: -28_511,
		TickSpacing: 120,
		FeeRate:     20_000,
		FeeOn:       1,
		Status:      0,
		DynamicFee: models.RaydiumDynamicFee{
			FilterPeriod:              180,
			DecayPeriod:               3600,
			ReductionFactor:           7000,
			DynamicFeeControl:         25_000,
			MaxVolatilityAccumulator:  80_000,
			TickSpacingIndexReference: -236,
			VolatilityReference:       0,
			VolatilityAccumulator:     20_000,
			LastUpdateTimestamp:       1_789_971_237,
		},
		BlockTimestamp: chainVectorBlockTs,
	}
}

// The three initialized ticks the swap walked.
func chainVectorTicks() TickProvider {
	type entry struct {
		index int32
		net   string
	}
	raw := []entry{
		{-16_320, "740027133"},
		{-14_760, "1942587532"},
		{-10_800, "-1942587532"},
	}
	bounds := make([]TickBoundary, 0, len(raw))
	for _, e := range raw {
		net, _ := new(big.Int).SetString(e.net, 10)
		bounds = append(bounds, TickBoundary{TickIndex: e.index, LiquidityNet: net, Initialized: true})
	}
	sort.Slice(bounds, func(i, j int) bool { return bounds[i].TickIndex < bounds[j].TickIndex })

	return func(fromTick int32, zeroForOne bool) (TickBoundary, bool) {
		if zeroForOne {
			for i := len(bounds) - 1; i >= 0; i-- {
				if bounds[i].TickIndex <= fromTick {
					return bounds[i], true
				}
			}
			return TickBoundary{}, false
		}
		for i := range bounds {
			if bounds[i].TickIndex > fromTick {
				return bounds[i], true
			}
		}
		return TickBoundary{}, false
	}
}

func TestCLMMQuoteMatchesChain(t *testing.T) {
	got, err := QuoteExactIn(chainVectorPool(), false, chainVectorAmountIn, chainVectorTicks())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got != chainVectorExpected {
		t.Fatalf("quote = %d, the program returned %d (off by %+d)",
			got, chainVectorExpected, int64(got)-int64(chainVectorExpected))
	}
}

// A match only means something if the features under test actually move the
// number. Both were measured live against this vector: neutering the dynamic fee
// shifts it ~874 bps and neutering fee_on ~188 bps. If either goes inert, the
// vector above has stopped covering it and must be recaptured.
func TestCLMMChainVectorFeaturesAreLive(t *testing.T) {
	ticks := chainVectorTicks()
	full, err := QuoteExactIn(chainVectorPool(), false, chainVectorAmountIn, ticks)
	if err != nil {
		t.Fatalf("%v", err)
	}

	noDyn := chainVectorPool()
	noDyn.DynamicFee = models.RaydiumDynamicFee{}
	gotNoDyn, err := QuoteExactIn(noDyn, false, chainVectorAmountIn, ticks)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if gotNoDyn == full {
		t.Fatal("dynamic fee is inert on this vector — a chain match proves nothing about it")
	}

	noFeeOn := chainVectorPool()
	noFeeOn.FeeOn = 0
	gotNoFeeOn, err := QuoteExactIn(noFeeOn, false, chainVectorAmountIn, ticks)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if gotNoFeeOn == full {
		t.Fatal("fee_on is inert on this vector — a chain match proves nothing about it")
	}

	t.Logf("dynamic fee moves the quote by %+d, fee_on by %+d",
		int64(full)-int64(gotNoDyn), int64(full)-int64(gotNoFeeOn))
}
