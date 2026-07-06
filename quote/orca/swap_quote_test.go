package orca

import (
	"math/big"
	"slices"
	"testing"

	orcamath "github.com/Gealber/soldex/math/orca"
)

// q64 shifts a small integer into Q64.64, matching the Rust tests' `n << 64`.
func q64(n int64) *big.Int {
	return new(big.Int).Lsh(big.NewInt(n), 64)
}

func mustBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad bigint: " + s)
	}
	return v
}

func sqrtAt(tick int32) *big.Int                { return orcamath.SqrtPriceFromTickIndex(tick) }
func tickFor(_ *testing.T, sqrt *big.Int) int32 { return orcamath.TickIndexFromSqrtPrice(sqrt) }
func minSqrtPrice() *big.Int                    { return orcamath.MinSqrtPrice }
func maxSqrtPrice() *big.Int                    { return orcamath.MaxSqrtPrice }

// TestComputeSwapStepAnchors locks computeSwapStep against the Orca program's own
// compute_swap unit tests (programs/whirlpool/src/math/swap_math.rs).
func TestComputeSwapStepAnchors(t *testing.T) {
	const twoPct = 20000 // 2% in hundredths of a basis point
	cases := []struct {
		name                     string
		amount                   uint64
		feeRate                  uint32
		liquidity                *big.Int
		current, target          *big.Int
		aToB                     bool
		wantIn, wantOut, wantFee uint64
		wantNext                 *big.Int
	}{
		{
			name: "a_to_b_input", amount: 100, feeRate: twoPct, liquidity: big.NewInt(1296),
			current: q64(9), target: q64(4), aToB: true,
			wantIn: 98, wantOut: 4723, wantFee: 2,
		},
		{
			name: "a_to_b_input_max", amount: 1000, feeRate: twoPct, liquidity: big.NewInt(1296),
			current: q64(9), target: q64(4), aToB: true,
			wantIn: 180, wantOut: 6480, wantFee: 4, wantNext: q64(4),
		},
		{
			name: "b_to_a_input", amount: 2000, feeRate: twoPct, liquidity: big.NewInt(1296),
			current: q64(9), target: q64(16), aToB: false,
			wantIn: 1960, wantOut: 20, wantFee: 40,
			wantNext: mustBig("193918550355107200012"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step, err := computeSwapStep(c.amount, c.feeRate, c.liquidity, c.current, c.target, c.aToB)
			if err != nil {
				t.Fatalf("computeSwapStep: %v", err)
			}
			if step.amountIn != c.wantIn || step.amountOut != c.wantOut || step.feeAmount != c.wantFee {
				t.Fatalf("got in=%d out=%d fee=%d, want in=%d out=%d fee=%d",
					step.amountIn, step.amountOut, step.feeAmount, c.wantIn, c.wantOut, c.wantFee)
			}
			if c.wantNext != nil && step.nextPrice.Cmp(c.wantNext) != 0 {
				t.Fatalf("next price = %s, want %s", step.nextPrice, c.wantNext)
			}
		})
	}
}

// staticTicks builds a TickProvider over a fixed set of initialized ticks plus a
// min/max edge so the loop terminates. It mirrors get_next_initialized_tick_index:
// a_to_b searches down (inclusive), b_to_a up (exclusive).
func staticTicks(nets map[int32]*big.Int) TickProvider {
	keys := make([]int32, 0, len(nets))
	for k := range nets {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	return func(fromTick int32, aToB bool) (TickBoundary, bool) {
		if aToB {
			for i := len(keys) - 1; i >= 0; i-- {
				if keys[i] <= fromTick {
					return TickBoundary{TickIndex: keys[i], LiquidityNet: nets[keys[i]], Initialized: true}, true
				}
			}
			return TickBoundary{TickIndex: -443636, Initialized: false}, true
		}
		for _, k := range keys {
			if k > fromTick {
				return TickBoundary{TickIndex: k, LiquidityNet: nets[k], Initialized: true}, true
			}
		}
		return TickBoundary{TickIndex: 443636, Initialized: false}, true
	}
}

// TestQuoteExactInSingleStep checks that a swap with no reachable initialized tick
// equals exactly one computeSwapStep to the price limit (constant liquidity).
func TestQuoteExactInSingleStep(t *testing.T) {
	pool := SwapPool{
		SqrtPrice: q64(9), Liquidity: big.NewInt(1_000_000),
		TickCurrentIndex: tickFor(t, q64(9)), TickSpacing: 64, FeeRate: 3000,
	}
	noTicks := func(int32, bool) (TickBoundary, bool) {
		return TickBoundary{TickIndex: -443636, Initialized: false}, true
	}

	got, err := QuoteExactIn(pool, true, 1000, noTicks)
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}

	target := minSqrtPrice()
	step, err := computeSwapStep(1000, uint32(pool.FeeRate), pool.Liquidity, pool.SqrtPrice, target, true)
	if err != nil {
		t.Fatalf("computeSwapStep: %v", err)
	}
	if got != step.amountOut {
		t.Fatalf("single-step quote = %d, want %d", got, step.amountOut)
	}
}

// TestQuoteExactInCrossesTick verifies the loop wiring (tick crossing +
// liquidity_net update) by reproducing the two steps by hand. A b_to_a swap walks
// up through one initialized tick that adds liquidity.
func TestQuoteExactInCrossesTick(t *testing.T) {
	const spacing uint16 = 64
	startTick := int32(0)
	crossTick := int32(128)
	net := big.NewInt(500_000)

	pool := SwapPool{
		SqrtPrice: sqrtAt(startTick), Liquidity: big.NewInt(2_000_000),
		TickCurrentIndex: startTick, TickSpacing: spacing, FeeRate: 3000,
	}
	ticks := staticTicks(map[int32]*big.Int{crossTick: net})

	got, err := QuoteExactIn(pool, false, 5_000_000, ticks)
	if err != nil {
		t.Fatalf("QuoteExactIn: %v", err)
	}

	// Manual replay: step 1 from start to the cross tick, then step 2 from the
	// cross tick to the max edge with liquidity increased by net.
	tickPrice := sqrtAt(crossTick)
	step1, err := computeSwapStep(5_000_000, uint32(pool.FeeRate), pool.Liquidity, pool.SqrtPrice, tickPrice, false)
	if err != nil {
		t.Fatalf("step1: %v", err)
	}
	if step1.nextPrice.Cmp(tickPrice) != 0 {
		t.Fatalf("step1 did not reach the cross tick (next=%s, tick=%s)", step1.nextPrice, tickPrice)
	}

	remaining := uint64(5_000_000) - step1.amountIn - step1.feeAmount
	liq2 := new(big.Int).Add(pool.Liquidity, net)
	step2, err := computeSwapStep(remaining, uint32(pool.FeeRate), liq2, tickPrice, maxSqrtPrice(), false)
	if err != nil {
		t.Fatalf("step2: %v", err)
	}

	want := step1.amountOut + step2.amountOut
	if got != want {
		t.Fatalf("crossing quote = %d, want %d (step1=%d step2=%d)", got, want, step1.amountOut, step2.amountOut)
	}
}
