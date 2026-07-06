// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	"testing"
)

func TestFeeRoundtrip(t *testing.T) {
	included, fee, err := GetIncludedFeeAmount(10_000_000, 1000)
	if err != nil {
		t.Fatalf("GetIncludedFeeAmount error: %v", err)
	}
	if included <= 1000 || fee == 0 {
		t.Fatalf("expected included > excluded and non-zero fee")
	}

	excluded, fee2, err := GetExcludedFeeAmount(10_000_000, included)
	if err != nil {
		t.Fatalf("GetExcludedFeeAmount error: %v", err)
	}
	if excluded > included || fee2 == 0 {
		t.Fatalf("unexpected excluded/fee values")
	}
}

func TestQuoteExactInCompounding(t *testing.T) {
	res, err := QuoteExactInCompounding(
		1000,
		10_000_000,
		2_000_000,
		TradeDirectionAtoB,
		20,
		true,
		false,
		10,
		0,
		20,
	)
	if err != nil {
		t.Fatalf("QuoteExactInCompounding error: %v", err)
	}
	if res.OutputAmount == 0 {
		t.Fatalf("expected non-zero output")
	}
	if res.ExcludedFeeInputAmount >= 1000 {
		t.Fatalf("expected excluded input amount lower than included when feeOnInput")
	}
}

func TestQuoteExactOutCompounding(t *testing.T) {
	res, err := QuoteExactOutCompounding(
		1000,
		10_000_000,
		2_000_000,
		TradeDirectionAtoB,
		20,
		false,
		false,
		10,
		0,
		20,
	)
	if err != nil {
		t.Fatalf("QuoteExactOutCompounding error: %v", err)
	}
	if res.OutputAmount != 1000 {
		t.Fatalf("expected exact output amount")
	}
	if res.IncludedFeeInputAmount == 0 || res.ExcludedFeeInputAmount == 0 {
		t.Fatalf("expected non-zero input amounts")
	}
}
