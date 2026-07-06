package raydium

import (
	"math/big"
	"testing"
)

func mustBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad big int " + s)
	}
	return v
}

// farBoundary returns a single uninitialized boundary well past the price in the
// swap direction, so a modest swap consumes its input without crossing a tick.
func farBoundary(zeroForOne bool) TickProvider {
	return func(fromTick int32, _ bool) (TickBoundary, bool) {
		if zeroForOne {
			return TickBoundary{TickIndex: fromTick - 5000, Initialized: false}, true
		}
		return TickBoundary{TickIndex: fromTick + 5000, Initialized: false}, true
	}
}

// Price sanity: the live pool's sqrt_price implies ~0.0707876 USDC-base per
// lamport. A small SOL->USDC (zeroForOne) swap on a deep synthetic book must land
// within a hair of amount_in_after_fee * price — an anchor independent of the
// swap-math internals (it only uses the price the pool reports), so it catches
// any scaling / Q-resolution / decimals error.
func TestQuoteExactInPriceAnchor(t *testing.T) {
	sqrt := mustBig("4907934225356241358") // live pool 3ucNos4...
	pool := SwapPool{
		SqrtPrice:   sqrt,
		Liquidity:   mustBig("1000000000000000000"), // deep, so price impact ~0
		TickCurrent: -26483,
		TickSpacing: 1,
		FeeRate:     400, // 0.04%
	}
	const amountIn = uint64(1_000_000) // 0.001 SOL

	out, err := QuoteExactIn(pool, true, amountIn, farBoundary(true))
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}

	// expected ≈ floor(amountIn * (1 - fee)) * (sqrt/2^64)^2
	afterFee := new(big.Float).SetInt64(int64(amountIn) * (1_000_000 - 400) / 1_000_000)
	twoPow64 := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 64))
	ratio := new(big.Float).Quo(new(big.Float).SetInt(sqrt), twoPow64)
	price := new(big.Float).Mul(ratio, ratio)
	expectedF := new(big.Float).Mul(afterFee, price)
	expected, _ := expectedF.Float64()

	if expected < 1 {
		t.Fatalf("bad expected %f", expected)
	}
	diff := float64(out) - expected
	if diff < 0 {
		diff = -diff
	}
	if diff/expected > 0.0005 { // within 0.05% of the price-implied output
		t.Fatalf("out %d vs price-implied %.1f (%.4f%% off)", out, expected, 100*diff/expected)
	}
}

// More input must yield more output (monotonicity of the exact-in curve).
func TestQuoteExactInMonotonic(t *testing.T) {
	pool := SwapPool{
		SqrtPrice:   mustBig("4907934225356241358"),
		Liquidity:   mustBig("98132489249010"),
		TickCurrent: -26483,
		TickSpacing: 1,
		FeeRate:     400,
	}
	var prev uint64
	for _, amt := range []uint64{1_000, 10_000, 100_000, 1_000_000} {
		out, err := QuoteExactIn(pool, true, amt, farBoundary(true))
		if err != nil {
			t.Fatalf("QuoteExactIn(%d): %v", amt, err)
		}
		if out <= prev {
			t.Fatalf("non-monotonic: in %d -> out %d, previous out %d", amt, out, prev)
		}
		prev = out
	}
}

// Crossing an initialized tick applies its liquidity_net; the swap then stops at
// the edge of known liquidity when the provider runs out (ok=false).
func TestQuoteExactInCrossesTickThenStops(t *testing.T) {
	pool := SwapPool{
		SqrtPrice:   mustBig("4907934225356241358"),
		Liquidity:   mustBig("98132489249010"),
		TickCurrent: -26483,
		TickSpacing: 1,
		FeeRate:     400,
	}
	// One initialized tick just below, then no more arrays cached.
	crossed := false
	provider := func(fromTick int32, zeroForOne bool) (TickBoundary, bool) {
		if fromTick == -26483 {
			return TickBoundary{TickIndex: -26490, LiquidityNet: big.NewInt(1_000_000), Initialized: true}, true
		}
		crossed = true
		return TickBoundary{}, false // edge of known liquidity
	}

	out, err := QuoteExactIn(pool, true, 1_000_000_000_000, provider) // large input forces the cross
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}
	if !crossed {
		t.Fatal("expected the swap to cross the first tick and then ask for the next array")
	}
	if out == 0 {
		t.Fatal("expected non-zero output up to the liquidity edge")
	}
}
