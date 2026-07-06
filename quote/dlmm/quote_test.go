// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math/big"
	"testing"

	dlmmmath "github.com/Gealber/soldex/math/dlmm"
)

func TestComputeFee(t *testing.T) {
	tests := []struct {
		amount     uint64
		feeRate    uint64
		expectErr  bool
		checkRange bool
	}{
		{1_000_000, 1_000_000, false, true},
		{100, 50_000_000, false, true},
		{u64(1), 1_000_000_000, true, false},
	}

	for _, tt := range tests {
		fee, err := ComputeFee(tt.amount, tt.feeRate)
		if (err != nil) != tt.expectErr {
			t.Fatalf("ComputeFee(%d, %d): expectErr=%v, got err=%v", tt.amount, tt.feeRate, tt.expectErr, err)
		}
		if !tt.expectErr && fee == 0 {
			t.Fatalf("ComputeFee(%d, %d): expected non-zero fee", tt.amount, tt.feeRate)
		}
	}
}

func TestComputeFeeFromAmount(t *testing.T) {
	tests := []struct {
		amountWithFees uint64
		feeRate        uint64
	}{
		{1_000_000, 1_000_000},
		{100, 50_000_000},
	}

	for _, tt := range tests {
		fee, err := ComputeFeeFromAmount(tt.amountWithFees, tt.feeRate)
		if err != nil {
			t.Fatalf("ComputeFeeFromAmount(%d, %d): %v", tt.amountWithFees, tt.feeRate, err)
		}
		if fee == 0 {
			t.Fatalf("ComputeFeeFromAmount(%d, %d): expected non-zero fee", tt.amountWithFees, tt.feeRate)
		}
	}
}

func TestQuoteSwapOnBin(t *testing.T) {
	price := new(big.Int).Lsh(big.NewInt(1), uint(dlmmmath.ScaleOffset))
	reserveX := uint64(1_000_000)
	reserveY := uint64(2_000_000)

	quote, err := QuoteSwapOnBin(
		100_000,
		reserveX,
		reserveY,
		price,
		true,
		1_000_000,
		2_500,
		nil,
	)
	if err != nil {
		t.Fatalf("QuoteSwapOnBin error: %v", err)
	}
	if quote == nil {
		t.Fatalf("expected non-nil quote")
	}
	if quote.AmountOut == 0 {
		t.Fatalf("expected non-zero AmountOut")
	}
}

func u64(v uint64) uint64 {
	return v
}
