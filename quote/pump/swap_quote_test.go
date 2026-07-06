package pump

import "testing"

func TestSellExactIn(t *testing.T) {
	// Equal reserves, 100 bps fee. Gross = 1e9*1e6/(1e9+1e6) ≈ 999000, then
	// ×(9900/10000) ≈ 989010.
	out := SellExactIn(1_000_000_000, 1_000_000_000, 1_000_000, 100)
	if out == 0 || out >= 1_000_000 {
		t.Fatalf("sell out %d out of expected range", out)
	}
	// Fee-free must exceed a 100bps-fee quote at the same size.
	if SellExactIn(1_000_000_000, 1_000_000_000, 1_000_000, 0) <= out {
		t.Fatal("zero-fee sell should beat fee'd sell")
	}
	// Zero reserves => no output, no panic.
	if SellExactIn(0, 0, 1_000_000, 100) != 0 {
		t.Fatal("empty pool should quote 0")
	}
}

func TestBuyExactIn(t *testing.T) {
	out := BuyExactIn(1_000_000_000, 1_000_000_000, 1_000_000, 100)
	if out == 0 || out >= 1_000_000 {
		t.Fatalf("buy out %d out of expected range", out)
	}
	if BuyExactIn(1_000_000_000, 1_000_000_000, 1_000_000, 0) <= out {
		t.Fatal("zero-fee buy should beat fee'd buy")
	}
	if BuyExactIn(0, 0, 1_000_000, 100) != 0 {
		t.Fatal("empty pool should quote 0")
	}
}
