package damm

import (
	"math/big"
	"testing"
)

// pool centered at price 1 (sqrtPrice = 2^64) with a wide [0.25, 4] range.
func testConcentratedPool(liquidity *big.Int, baseFeeNumerator uint64) ConcentratedPool {
	one := new(big.Int).Lsh(big.NewInt(1), 64)
	return ConcentratedPool{
		SqrtPrice:        one,
		SqrtMinPrice:     new(big.Int).Lsh(big.NewInt(1), 63), // price 0.25
		SqrtMaxPrice:     new(big.Int).Lsh(big.NewInt(1), 65), // price 4
		Liquidity:        liquidity,
		CollectFeeMode:   CollectFeeModeBothToken,
		FeeVersion:       1,
		BaseFeeNumerator: baseFeeNumerator,
	}
}

func TestConcentratedQuotePositiveBothDirections(t *testing.T) {
	pool := testConcentratedPool(new(big.Int).Lsh(big.NewInt(1), 96), 0)

	outAB, err := QuoteConcentratedExactIn(1_000_000, TradeDirectionAtoB, pool)
	if err != nil || outAB == 0 {
		t.Fatalf("A->B quote = %d, err = %v; want positive", outAB, err)
	}
	outBA, err := QuoteConcentratedExactIn(1_000_000, TradeDirectionBtoA, pool)
	if err != nil || outBA == 0 {
		t.Fatalf("B->A quote = %d, err = %v; want positive", outBA, err)
	}
}

func TestConcentratedQuotePriceImpact(t *testing.T) {
	pool := testConcentratedPool(new(big.Int).Lsh(big.NewInt(1), 96), 0)

	const small = uint64(200_000_000)
	const large = uint64(3_500_000_000)

	outSmall, err := QuoteConcentratedExactIn(small, TradeDirectionBtoA, pool)
	if err != nil {
		t.Fatalf("small quote err: %v", err)
	}
	outLarge, err := QuoteConcentratedExactIn(large, TradeDirectionBtoA, pool)
	if err != nil {
		t.Fatalf("large quote err: %v", err)
	}

	// Effective rate (out/in) must degrade with size: outSmall/small > outLarge/large
	// <=> outSmall*large > outLarge*small.
	lhs := new(big.Int).Mul(new(big.Int).SetUint64(outSmall), new(big.Int).SetUint64(large))
	rhs := new(big.Int).Mul(new(big.Int).SetUint64(outLarge), new(big.Int).SetUint64(small))
	if lhs.Cmp(rhs) <= 0 {
		t.Fatalf("expected price impact: small eff rate %d/%d should exceed large %d/%d",
			outSmall, small, outLarge, large)
	}
}

func TestConcentratedQuoteFeeReducesOutput(t *testing.T) {
	noFee := testConcentratedPool(new(big.Int).Lsh(big.NewInt(1), 96), 0)
	withFee := testConcentratedPool(new(big.Int).Lsh(big.NewInt(1), 96), 3_000_000) // 0.3%

	const amount = uint64(1_000_000_000)
	outNoFee, err := QuoteConcentratedExactIn(amount, TradeDirectionAtoB, noFee)
	if err != nil {
		t.Fatalf("no-fee quote err: %v", err)
	}
	outWithFee, err := QuoteConcentratedExactIn(amount, TradeDirectionAtoB, withFee)
	if err != nil {
		t.Fatalf("with-fee quote err: %v", err)
	}
	if outWithFee >= outNoFee {
		t.Fatalf("fee did not reduce output: withFee=%d, noFee=%d", outWithFee, outNoFee)
	}
}

func TestConcentratedQuoteRangeViolation(t *testing.T) {
	// Tiny liquidity so a modest input blows past the price range.
	pool := testConcentratedPool(big.NewInt(1_000), 0)
	if _, err := QuoteConcentratedExactIn(1_000_000_000, TradeDirectionBtoA, pool); err != ErrPriceRangeViolation {
		t.Fatalf("expected ErrPriceRangeViolation, got %v", err)
	}
}
