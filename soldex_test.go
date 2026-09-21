package soldex

import "testing"

// Exercises the uniform Quoter through the self-contained Pump adapter (the other
// venues need bin/tick provider state; their math is covered in quote/<dex>).
func TestPumpQuoterDirections(t *testing.T) {
	var q Quoter = Pump(1_000_000_000, 1_000_000_000, 100)

	sell, err := q.QuoteExactIn(1_000_000, true) // base in, quote out
	if err != nil || sell == 0 {
		t.Fatalf("sell: out=%d err=%v", sell, err)
	}
	buy, err := q.QuoteExactIn(1_000_000, false) // quote in, base out
	if err != nil || buy == 0 {
		t.Fatalf("buy: out=%d err=%v", buy, err)
	}
	// A larger fee must reduce the output for the same size/direction.
	hi := Pump(1_000_000_000, 1_000_000_000, 500)
	loFee, _ := q.QuoteExactIn(1_000_000, true)
	hiFee, _ := hi.QuoteExactIn(1_000_000, true)
	if hiFee >= loFee {
		t.Fatalf("higher fee should lower output: 100bps=%d 500bps=%d", loFee, hiFee)
	}
}

// The bonding-curve and compounding adapters exist so every venue in the library
// is reachable through Quoter; both were long-standing gaps.
func TestPumpBondingCurveAdapter(t *testing.T) {
	const vTok, vQuote, feeBps = uint64(1_000_000_000_000), uint64(30_000_000_000), uint64(100)

	q := PumpBondingCurve(vTok, vQuote, feeBps)
	buy, err := q.QuoteExactIn(1_000_000_000, false)
	if err != nil || buy == 0 {
		t.Fatalf("buy = %d, err = %v", buy, err)
	}
	sell, err := q.QuoteExactIn(1_000_000_000, true)
	if err != nil || sell == 0 {
		t.Fatalf("sell = %d, err = %v", sell, err)
	}
	// A fee'd curve must return less than a fee-free one in both directions.
	free := PumpBondingCurve(vTok, vQuote, 0)
	if b, _ := free.QuoteExactIn(1_000_000_000, false); b <= buy {
		t.Fatalf("fee-free buy %d should beat fee'd %d", b, buy)
	}
	if s, _ := free.QuoteExactIn(1_000_000_000, true); s <= sell {
		t.Fatalf("fee-free sell %d should beat fee'd %d", s, sell)
	}
}

func TestDAMMCompoundingAdapter(t *testing.T) {
	const rA, rB = uint64(10_000_000_000), uint64(4_000_000_000)

	q := DAMMCompounding(rA, rB, 2_500_000, true, false, 10, 0, 20)
	out, err := q.QuoteExactIn(1_000_000, true)
	if err != nil || out == 0 {
		t.Fatalf("A->B out = %d, err = %v", out, err)
	}
	// The opposite direction runs the other side of the curve.
	rev, err := q.QuoteExactIn(1_000_000, false)
	if err != nil || rev == 0 {
		t.Fatalf("B->A out = %d, err = %v", rev, err)
	}
	if out == rev {
		t.Fatal("both directions returned the same amount on an asymmetric pool")
	}
	// A higher fee must return less.
	dearer := DAMMCompounding(rA, rB, 50_000_000, true, false, 10, 0, 20)
	if hi, _ := dearer.QuoteExactIn(1_000_000, true); hi >= out {
		t.Fatalf("higher fee returned %d, not less than %d", hi, out)
	}
}
