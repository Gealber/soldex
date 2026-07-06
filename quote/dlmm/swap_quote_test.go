package dlmm

import "testing"

// providerFromMap builds a BinProvider over a fixed set of bins.
func providerFromMap(bins map[int32]BinReserves) BinProvider {
	return func(binID int32) (BinReserves, bool) {
		r, ok := bins[binID]
		return r, ok
	}
}

// zeroFeePool centers liquidity at bin id 0 (price 1.0) with no fees.
func zeroFeePool() SwapPool {
	return SwapPool{
		ActiveID:       0,
		BinStep:        10,
		BaseFactor:     0, // no base fee
		CollectFeeMode: 0,
	}
}

func TestQuoteExactInStaysInActiveBin(t *testing.T) {
	pool := zeroFeePool()
	bins := providerFromMap(map[int32]BinReserves{0: {AmountY: 1000}})

	// Price 1.0, no fee: 100 X in -> 100 Y out, no bin crossing.
	out, err := QuoteExactIn(pool, true, 100, 0, bins)
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}
	if out != 100 {
		t.Fatalf("out = %d, want 100", out)
	}
}

func TestQuoteExactInCrossesBins(t *testing.T) {
	pool := zeroFeePool()
	bins := providerFromMap(map[int32]BinReserves{
		0:  {AmountY: 1000},
		-1: {AmountY: 1000},
		-2: {AmountY: 1000},
	})

	out, err := QuoteExactIn(pool, true, 2500, 0, bins)
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}
	// Must consume more than one bin's worth of output (price ~1 across bins).
	if out <= 1000 {
		t.Fatalf("out = %d, want > 1000 (bin crossing)", out)
	}
	if out > 2500 {
		t.Fatalf("out = %d, want <= input 2500", out)
	}
}

func TestQuoteExactInStopsAtMissingBin(t *testing.T) {
	pool := zeroFeePool()
	bins := providerFromMap(map[int32]BinReserves{0: {AmountY: 1000}})

	// Input exceeds the only known bin; next bin is missing -> stop.
	out, err := QuoteExactIn(pool, true, 5000, 0, bins)
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}
	if out != 1000 {
		t.Fatalf("out = %d, want 1000 (bounded by known liquidity)", out)
	}
}

func TestQuoteExactInFeeReducesOutput(t *testing.T) {
	bins := providerFromMap(map[int32]BinReserves{0: {AmountY: 1_000_000}})

	noFee := zeroFeePool()
	outNoFee, err := QuoteExactIn(noFee, true, 100_000, 0, bins)
	if err != nil {
		t.Fatalf("no-fee quote: %v", err)
	}

	withFee := zeroFeePool()
	withFee.BaseFactor = 10_000 // base fee = 10000*10*10 = 1e6 (0.1%)
	outWithFee, err := QuoteExactIn(withFee, true, 100_000, 0, bins)
	if err != nil {
		t.Fatalf("with-fee quote: %v", err)
	}

	if outWithFee >= outNoFee {
		t.Fatalf("fee did not reduce output: withFee=%d noFee=%d", outWithFee, outNoFee)
	}
}
