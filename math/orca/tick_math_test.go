package orca

import (
	"math/big"
	"testing"
)

func TestSqrtPriceFromTickIndexAnchors(t *testing.T) {
	cases := []struct {
		tick int32
		want string
	}{
		{0, "18446744073709551616"},                     // 2^64 (price 1.0)
		{MinTickIndex, "4295048016"},                    // MIN_SQRT_PRICE
		{MaxTickIndex, "79226673515401279992447579055"}, // MAX_SQRT_PRICE
	}
	for _, c := range cases {
		got := SqrtPriceFromTickIndex(c.tick)
		if got.String() != c.want {
			t.Fatalf("SqrtPriceFromTickIndex(%d) = %s, want %s", c.tick, got, c.want)
		}
	}
}

func TestSqrtPriceMonotonicAroundZero(t *testing.T) {
	// price(t+1) must strictly exceed price(t); spot-check a window across zero.
	prev := SqrtPriceFromTickIndex(-5)
	for tick := int32(-4); tick <= 5; tick++ {
		cur := SqrtPriceFromTickIndex(tick)
		if cur.Cmp(prev) <= 0 {
			t.Fatalf("sqrt price not increasing at tick %d: %s <= %s", tick, cur, prev)
		}
		prev = cur
	}
}

func TestTickIndexFromSqrtPriceRoundTrip(t *testing.T) {
	// For any tick, deriving the sqrt-price then inverting must return the same
	// tick (the inverse floors into [tick, tick+1)).
	for _, tick := range []int32{-443636, -200000, -5632, -64, -1, 0, 1, 64, 5632, 200000, 443635} {
		sp := SqrtPriceFromTickIndex(tick)
		got := TickIndexFromSqrtPrice(sp)
		if got != tick {
			t.Fatalf("round trip tick %d -> %s -> %d", tick, sp, got)
		}
	}
}

func TestTickIndexFromSqrtPriceFloors(t *testing.T) {
	// A sqrt-price just above tick T's boundary still resolves to T.
	tick := int32(1234)
	sp := new(big.Int).Add(SqrtPriceFromTickIndex(tick), big.NewInt(10))
	if got := TickIndexFromSqrtPrice(sp); got != tick {
		t.Fatalf("floor: got %d, want %d", got, tick)
	}
}
