package raydium

import (
	"errors"
	"math/big"
	"testing"

	"github.com/Gealber/soldex/models"
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

// deepPool is the price-anchor pool: deep enough that price impact is negligible,
// so a fee change shows up as a clean proportional change in output.
func deepPool() SwapPool {
	return SwapPool{
		SqrtPrice:   mustBig("4907934225356241358"),
		Liquidity:   mustBig("1000000000000000000"),
		TickCurrent: -26483,
		TickSpacing: 1,
		FeeRate:     400, // 0.04%
	}
}

// fee_on = Token0Only takes the fee out of token0. Buying token0 that is the OUTPUT
// side, so the fee comes off what the trader receives rather than what they pay.
//
// The pool is shallow enough for real price impact, which is what separates the two:
// on a near-linear book taking the fee before or after the swap gives the same number,
// and the test would pass without the fee_on path existing at all.
func TestQuoteExactInFeeOnOutput(t *testing.T) {
	const amountIn = uint64(5_000_000_000)
	const feeRate = 400

	shallow := deepPool()
	shallow.Liquidity = mustBig("100000000000")

	free := shallow
	free.FeeRate = 0
	gross, err := QuoteExactIn(free, false, amountIn, farBoundary(false))
	if err != nil {
		t.Fatal(err)
	}

	onToken0 := shallow
	onToken0.FeeOn = 1
	if onToken0.IsFeeOnInput(false) {
		t.Fatal("Token0Only buying token0 must be fee-on-output")
	}
	got, err := QuoteExactIn(onToken0, false, amountIn, farBoundary(false))
	if err != nil {
		t.Fatal(err)
	}
	fee := (gross*feeRate + 999_999) / 1_000_000 // ceil, as the program rounds
	if want := gross - fee; got != want {
		t.Fatalf("fee-on-output out = %d, want gross %d less fee %d = %d", got, gross, fee, want)
	}

	// Charging the same rate on the input instead moves the price less, so a concave
	// curve pays out strictly more. Equality here means fee_on was ignored.
	onInput, err := QuoteExactIn(shallow, false, amountIn, farBoundary(false))
	if err != nil {
		t.Fatal(err)
	}
	if onInput <= got {
		t.Fatalf("fee-on-input %d must exceed fee-on-output %d", onInput, got)
	}

	// The same pool selling token0 keeps the fee on the input, so nothing changes.
	if !onToken0.IsFeeOnInput(true) {
		t.Fatal("Token0Only selling token0 must be fee-on-input")
	}
	sellBase, err := QuoteExactIn(shallow, true, amountIn, farBoundary(true))
	if err != nil {
		t.Fatal(err)
	}
	sellFeeOn, err := QuoteExactIn(onToken0, true, amountIn, farBoundary(true))
	if err != nil {
		t.Fatal(err)
	}
	if sellBase != sellFeeOn {
		t.Fatalf("fee-on-input direction changed: %d vs %d", sellBase, sellFeeOn)
	}
}

// A tick carrying resting limit orders fills the swap at the TICK price. The book
// here ends at that tick, so the swap arrives with input to spare and consumes the
// orders whole — which must add EXACTLY the resting size to the output, since a
// fully-consumed order pays out its own size and the fee is on the input.
func TestQuoteExactInFillsLimitOrders(t *testing.T) {
	pool := deepPool()
	pool.Liquidity = mustBig("100000000000")
	const amountIn = uint64(5_000_000_000)

	// One initialized tick that removes all liquidity, and nothing cached past it.
	lastTick := func(unfilled uint64) TickProvider {
		spent := false
		return func(fromTick int32, _ bool) (TickBoundary, bool) {
			if spent {
				return TickBoundary{}, false
			}
			spent = true
			return TickBoundary{
				TickIndex:          fromTick - 10,
				Initialized:        true,
				LiquidityNet:       mustBig("100000000000"),
				LimitOrderUnfilled: unfilled,
			}, true
		}
	}

	liquidityOnly, err := QuoteExactIn(pool, true, amountIn, lastTick(0))
	if err != nil {
		t.Fatal(err)
	}
	for _, resting := range []uint64{1_000_000, 100_000_000} {
		out, err := QuoteExactIn(pool, true, amountIn, lastTick(resting))
		if err != nil {
			t.Fatal(err)
		}
		if want := liquidityOnly + resting; out != want {
			t.Fatalf("resting %d: out = %d, want %d (liquidity-only %d plus the whole book)",
				resting, out, want, liquidityOnly)
		}
	}
}

// A dynamic fee adds to the AmmConfig fee, so an otherwise identical pool with one
// configured must return strictly less.
func TestQuoteExactInDynamicFeeReducesOutput(t *testing.T) {
	const amountIn = uint64(100_000_000)

	base := deepPool()
	dynamic := deepPool()
	dynamic.DynamicFee = models.RaydiumDynamicFee{
		FilterPeriod:              10,
		DecayPeriod:               120,
		ReductionFactor:           5_000,
		DynamicFeeControl:         50_000,
		MaxVolatilityAccumulator:  350_000,
		TickSpacingIndexReference: -26483,
		VolatilityReference:       200_000,
		VolatilityAccumulator:     200_000,
		LastUpdateTimestamp:       1_000,
	}
	dynamic.BlockTimestamp = 1_005 // inside the filter period: no decay

	if !dynamic.DynamicFee.Enabled() {
		t.Fatal("a configured dynamic fee must read as enabled")
	}
	baseOut, err := QuoteExactIn(base, true, amountIn, farBoundary(true))
	if err != nil {
		t.Fatal(err)
	}
	dynamicOut, err := QuoteExactIn(dynamic, true, amountIn, farBoundary(true))
	if err != nil {
		t.Fatal(err)
	}
	if dynamicOut >= baseOut {
		t.Fatalf("dynamic fee must reduce output: %d vs base %d", dynamicOut, baseOut)
	}
}

// Quoting a pool the program would reject is worse than quoting nothing: it feeds a
// tradable-looking number for a swap that cannot land.
func TestQuoteExactInSwapDisabled(t *testing.T) {
	pool := deepPool()
	pool.Status = 1 << 4
	if _, err := QuoteExactIn(pool, true, 1_000_000, farBoundary(true)); !errors.Is(err, ErrSwapDisabled) {
		t.Fatalf("err = %v, want ErrSwapDisabled", err)
	}
}
