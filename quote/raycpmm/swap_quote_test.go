package raycpmm

import "testing"

func TestSwapBaseInput(t *testing.T) {
	// Equal reserves, 2500/1e6 = 0.25% fee. netIn = 1e6 - ceil(1e6*2500/1e6) =
	// 1e6 - 2500 = 997500; out = 1e9*997500/(1e9+997500) ≈ 996504.
	out := SwapBaseInput(1_000_000_000, 1_000_000_000, 1_000_000, 2500)
	if out == 0 || out >= 1_000_000 {
		t.Fatalf("out %d out of expected range", out)
	}
	// Fee-free must beat a fee'd quote at the same size.
	if SwapBaseInput(1_000_000_000, 1_000_000_000, 1_000_000, 0) <= out {
		t.Fatal("zero-fee swap should beat fee'd swap")
	}
	// Direction asymmetry: a deeper output reserve yields more out.
	deep := SwapBaseInput(1_000_000_000, 2_000_000_000, 1_000_000, 2500)
	if deep <= out {
		t.Fatal("larger output reserve should yield more out")
	}
	// Zero reserves => no output, no panic.
	if SwapBaseInput(0, 0, 1_000_000, 2500) != 0 {
		t.Fatal("empty pool should quote 0")
	}
	// Fee that consumes the whole input yields nothing.
	if SwapBaseInput(1_000_000_000, 1_000_000_000, 10, FeeRateDenominator) != 0 {
		t.Fatal("100% fee should quote 0")
	}
}

func TestSwapBaseInputFeeIsCeil(t *testing.T) {
	// amountIn=1, feeRate=1 => ceil(1*1/1e6)=1 => netIn=0 => out=0.
	if got := SwapBaseInput(1_000_000, 1_000_000, 1, 1); got != 0 {
		t.Fatalf("ceil fee should zero out a 1-unit input, got %d", got)
	}
}
