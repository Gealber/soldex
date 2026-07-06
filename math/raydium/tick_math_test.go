package raydium

import (
	"math/big"
	"testing"
)

// Anchors taken from Raydium's own tick_math.rs tests plus the live mainnet pool
// 3ucNos4... (tick_current -26483 ⇒ sqrt_price 4907934225356241358).
func TestSqrtPriceFromTickAnchors(t *testing.T) {
	cases := []struct {
		tick int32
		want string
	}{
		{MinTick, "4295048016"},
		{MaxTick, "79226673521066979257578248091"},
		{0, "18446744073709551616"}, // 2^64
	}
	for _, c := range cases {
		got := SqrtPriceFromTick(c.tick)
		if got.String() != c.want {
			t.Fatalf("SqrtPriceFromTick(%d) = %s, want %s", c.tick, got, c.want)
		}
	}
}

// The on-chain sqrt_price must sit in [price(tick), price(tick+1)) for the pool's
// reported tick_current — a strong independent check of the table.
func TestSqrtPriceFromTickOnChainBracket(t *testing.T) {
	onchain := mustBig("4907934225356241358")
	lo := SqrtPriceFromTick(-26483)
	hi := SqrtPriceFromTick(-26482)
	if lo.Cmp(onchain) > 0 || onchain.Cmp(hi) >= 0 {
		t.Fatalf("on-chain sqrt_price %s not in [%s, %s) for tick -26483", onchain, lo, hi)
	}
}

func TestTickFromSqrtPriceOnChain(t *testing.T) {
	if got := TickFromSqrtPrice(mustBig("4907934225356241358")); got != -26483 {
		t.Fatalf("TickFromSqrtPrice(on-chain) = %d, want -26483", got)
	}
}

// Round-trip behaviour from Raydium's get_tick_at_sqrt_price test at tick -28861.
func TestTickFromSqrtPriceRoundTrip(t *testing.T) {
	sp := SqrtPriceFromTick(-28861)
	if got := TickFromSqrtPrice(sp); got != -28861 {
		t.Fatalf("TickFromSqrtPrice(price(-28861)) = %d, want -28861", got)
	}
	if got := TickFromSqrtPrice(new(big.Int).Add(sp, big.NewInt(1))); got != -28861 {
		t.Fatalf("TickFromSqrtPrice(price+1) = %d, want -28861", got)
	}
	if got := TickFromSqrtPrice(new(big.Int).Sub(sp, big.NewInt(1))); got != -28862 {
		t.Fatalf("TickFromSqrtPrice(price-1) = %d, want -28862", got)
	}
	belowNext := new(big.Int).Sub(SqrtPriceFromTick(-28860), big.NewInt(1))
	if got := TickFromSqrtPrice(belowNext); got != -28861 {
		t.Fatalf("TickFromSqrtPrice(price(-28860)-1) = %d, want -28861", got)
	}
}

// Full sweep round-trip: every tick's price must decode back to itself.
func TestSqrtPriceTickRoundTripSweep(t *testing.T) {
	for tick := int32(-443000); tick <= 443000; tick += 977 {
		sp := SqrtPriceFromTick(tick)
		if got := TickFromSqrtPrice(sp); got != tick {
			t.Fatalf("round trip tick %d -> price %s -> tick %d", tick, sp, got)
		}
	}
}
