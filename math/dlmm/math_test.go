package dlmm

import (
	"math"
	"math/big"
	"testing"
)

func bi(v uint64) *big.Int {
	return new(big.Int).SetUint64(v)
}

func TestMulDivRounding(t *testing.T) {
	down, err := MulDiv(bi(5), bi(5), bi(3), RoundingDown)
	if err != nil {
		t.Fatalf("MulDiv down error: %v", err)
	}
	if down.Cmp(bi(8)) != 0 {
		t.Fatalf("expected 8, got %s", down.String())
	}

	up, err := MulDiv(bi(5), bi(5), bi(3), RoundingUp)
	if err != nil {
		t.Fatalf("MulDiv up error: %v", err)
	}
	if up.Cmp(bi(9)) != 0 {
		t.Fatalf("expected 9, got %s", up.String())
	}
}

func TestMulDivDivideByZero(t *testing.T) {
	if _, err := MulDiv(bi(1), bi(2), bi(0), RoundingDown); err == nil {
		t.Fatalf("expected divide by zero error")
	}
}

func TestMulShrAndShlDiv(t *testing.T) {
	v, err := MulShr(bi(10), bi(3), 1, RoundingDown)
	if err != nil {
		t.Fatalf("MulShr error: %v", err)
	}
	if v.Cmp(bi(15)) != 0 {
		t.Fatalf("expected 15, got %s", v.String())
	}

	v2, err := ShlDiv(bi(3), bi(2), 2, RoundingDown)
	if err != nil {
		t.Fatalf("ShlDiv error: %v", err)
	}
	if v2.Cmp(bi(6)) != 0 {
		t.Fatalf("expected 6, got %s", v2.String())
	}

	if _, err := MulShr(bi(1), bi(1), 128, RoundingDown); err == nil {
		t.Fatalf("expected overflow error for offset")
	}
}

func TestPowBoundaries(t *testing.T) {
	v, err := Pow(One, 0)
	if err != nil {
		t.Fatalf("Pow error: %v", err)
	}
	if v.Cmp(One) != 0 {
		t.Fatalf("expected One for exp=0")
	}

	v2, err := Pow(One, -123)
	if err != nil {
		t.Fatalf("Pow negative error: %v", err)
	}
	if v2.Sign() <= 0 {
		t.Fatalf("expected positive non-zero result for negative exponent")
	}

	if _, err := Pow(One, int32(maxExponential)); err == nil {
		t.Fatalf("expected overflow for exponent >= maxExponential")
	}

	if _, err := Pow(One, math.MinInt32); err == nil {
		t.Fatalf("expected overflow for MinInt32")
	}
}

func TestGetPriceFromID(t *testing.T) {
	p0, err := GetPriceFromID(0, 25)
	if err != nil {
		t.Fatalf("GetPriceFromID error: %v", err)
	}
	if p0.Cmp(One) != 0 {
		t.Fatalf("expected One for activeID=0")
	}

	p1, err := GetPriceFromID(1, 100)
	if err != nil {
		t.Fatalf("GetPriceFromID error: %v", err)
	}
	bps := new(big.Int).Div(new(big.Int).Lsh(big.NewInt(100), uint(ScaleOffset)), big.NewInt(10_000))
	expected := new(big.Int).Add(new(big.Int).Set(One), bps)
	if p1.Cmp(expected) != 0 {
		t.Fatalf("unexpected price for activeID=1: got %s expected %s", p1.String(), expected.String())
	}
}

func TestSafeCastHelpers(t *testing.T) {
	v, err := SafeMulDivCast[uint16](bi(100), bi(3), bi(2), RoundingDown)
	if err != nil {
		t.Fatalf("SafeMulDivCast error: %v", err)
	}
	if v != 150 {
		t.Fatalf("expected 150, got %d", v)
	}

	if _, err := SafeMulDivCast[uint8](bi(200), bi(2), bi(1), RoundingDown); err == nil {
		t.Fatalf("expected cast failure for uint8")
	}
}
