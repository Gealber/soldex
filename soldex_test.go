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
