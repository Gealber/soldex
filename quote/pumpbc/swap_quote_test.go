package pumpbc

import "testing"

// Expecteds pinned against the real v1 bonding-curve reserves
// (vSol=30590314250, vTok=1052293866163944), fee 100 bps.
func TestBuyExactIn(t *testing.T) {
	got := BuyExactIn(30590314250, 1052293866163944, 100_000_000, 100)
	if got != 3394910554245 {
		t.Errorf("got %d, want 3394910554245", got)
	}
}

func TestSellExactIn(t *testing.T) {
	got := SellExactIn(1052293866163944, 30590314250, 10_000_000_000, 100)
	if got != 287791 {
		t.Errorf("got %d, want 287791", got)
	}
}

// Buying then selling the same size back must not create SOL (round-trip <= in,
// minus fees) — a sanity invariant on the two directions.
func TestRoundTripLossy(t *testing.T) {
	const vSol, vTok = uint64(30590314250), uint64(1052293866163944)
	solIn := uint64(50_000_000)
	tok := BuyExactIn(vSol, vTok, solIn, 100)
	back := SellExactIn(vTok, vSol, tok, 100)
	if back >= solIn {
		t.Errorf("round-trip returned %d >= %d in (should lose to fees/curve)", back, solIn)
	}
}

func TestZeroReserves(t *testing.T) {
	if BuyExactIn(0, 0, 0, 100) != 0 {
		t.Error("buy zero-den should be 0")
	}
	if SellExactIn(0, 0, 0, 100) != 0 {
		t.Error("sell zero-den should be 0")
	}
}
