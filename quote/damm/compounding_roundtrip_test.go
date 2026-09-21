package damm

import "testing"

// A compounding exact-out quote must ask for the amount an exact-in quote would
// have needed to produce that same output. Round-tripping the two is an
// independent check on the fee direction: it shares no algebra with either
// implementation, it just requires them to agree.
//
// The bug this guards was that exact-out grossed the fee the WRONG WAY — it
// returned the net curve input (fee-on-input) and sized the input off the net
// output (fee-on-output), so it understated the cost of the swap by the fee.
func TestCompoundingExactOutRoundTripsExactIn(t *testing.T) {
	const (
		reserveA = uint64(10_000_000_000)
		reserveB = uint64(4_000_000_000)
		// 0.25%, a realistic DAMM base fee.
		feeNumerator = uint64(2_500_000)
	)

	for _, feeOnInput := range []bool{true, false} {
		for _, dir := range []TradeDirection{TradeDirectionAtoB, TradeDirectionBtoA} {
			for _, amountIn := range []uint64{1_000, 1_000_000, 50_000_000, 250_000_000} {
				in, err := QuoteExactInCompounding(amountIn, reserveA, reserveB, dir,
					feeNumerator, feeOnInput, false, 10, 0, 20)
				if err != nil {
					t.Fatalf("exact-in feeOnInput=%v dir=%d amt=%d: %v", feeOnInput, dir, amountIn, err)
				}
				if in.OutputAmount == 0 {
					continue
				}

				out, err := QuoteExactOutCompounding(in.OutputAmount, reserveA, reserveB, dir,
					feeNumerator, feeOnInput, false, 10, 0, 20)
				if err != nil {
					t.Fatalf("exact-out feeOnInput=%v dir=%d amt=%d: %v", feeOnInput, dir, amountIn, err)
				}

				got := out.IncludedFeeInputAmount
				// Rounding is up on both legs, so the round trip may ask for a hair
				// more than the original input, never meaningfully less.
				tol := amountIn/100_000 + 4
				if got+tol < amountIn || got > amountIn+tol {
					t.Fatalf("feeOnInput=%v dir=%d: exact-in %d -> out %d -> exact-out %d (off by %+d, tol %d)",
						feeOnInput, dir, amountIn, in.OutputAmount, got, int64(got)-int64(amountIn), tol)
				}
			}
		}
	}
}

// Exact-out must cost MORE input than the fee-free curve alone, in both fee
// modes. Understating the gross-up shows up here as an input at or below the
// raw curve figure.
func TestCompoundingExactOutChargesTheFee(t *testing.T) {
	const (
		reserveA  = uint64(10_000_000_000)
		reserveB  = uint64(4_000_000_000)
		amountOut = uint64(100_000_000)
		feeNum    = uint64(2_500_000)
	)
	raw, err := compoundingInputForOutput(reserveA, reserveB, amountOut, TradeDirectionAtoB)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, feeOnInput := range []bool{true, false} {
		res, err := QuoteExactOutCompounding(amountOut, reserveA, reserveB, TradeDirectionAtoB,
			feeNum, feeOnInput, false, 10, 0, 20)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if res.IncludedFeeInputAmount <= raw {
			t.Fatalf("feeOnInput=%v: input %d does not exceed the fee-free curve input %d",
				feeOnInput, res.IncludedFeeInputAmount, raw)
		}
		total := res.ClaimingFee + res.CompoundingFee + res.ProtocolFee + res.ReferralFee
		if total == 0 {
			t.Fatalf("feeOnInput=%v: no fee reported", feeOnInput)
		}
	}
}

// Asking for the whole output reserve has no finite answer on a constant product
// curve; it must error rather than wrap the subtraction.
func TestCompoundingExactOutRejectsDrainingThePool(t *testing.T) {
	for _, dir := range []TradeDirection{TradeDirectionAtoB, TradeDirectionBtoA} {
		reserveOut := uint64(4_000_000_000)
		if dir == TradeDirectionBtoA {
			reserveOut = 10_000_000_000
		}
		for _, amountOut := range []uint64{reserveOut, reserveOut + 1} {
			if _, err := QuoteExactOutCompounding(amountOut, 10_000_000_000, 4_000_000_000, dir,
				2_500_000, false, false, 10, 0, 20); err != ErrInsufficientLiquidity {
				t.Fatalf("dir=%d out=%d: err = %v, want ErrInsufficientLiquidity", dir, amountOut, err)
			}
		}
	}
}
