package damm

import (
	"math/big"
	"testing"

	"github.com/Gealber/soldex/math/common"
)

func bi(v uint64) *big.Int {
	return new(big.Int).SetUint64(v)
}

func TestMulShrAndMulShr256(t *testing.T) {
	v, err := MulShr(bi(10), bi(3), 1)
	if err != nil {
		t.Fatalf("MulShr error: %v", err)
	}
	if v.Cmp(bi(15)) != 0 {
		t.Fatalf("expected 15, got %s", v.String())
	}

	v2, err := MulShr256(bi(10), bi(3), 1)
	if err != nil {
		t.Fatalf("MulShr256 error: %v", err)
	}
	if v2.Cmp(bi(15)) != 0 {
		t.Fatalf("expected 15, got %s", v2.String())
	}
}

func TestShlDivRoundingAndOverflow(t *testing.T) {
	down, err := ShlDiv(bi(5), bi(3), 1, RoundingDown)
	if err != nil {
		t.Fatalf("ShlDiv down error: %v", err)
	}
	if down.Cmp(bi(3)) != 0 {
		t.Fatalf("expected 3, got %s", down.String())
	}

	up, err := ShlDiv(bi(5), bi(3), 1, RoundingUp)
	if err != nil {
		t.Fatalf("ShlDiv up error: %v", err)
	}
	if up.Cmp(bi(4)) != 0 {
		t.Fatalf("expected 4, got %s", up.String())
	}

	if _, err := ShlDiv(bi(1), bi(0), 1, RoundingDown); err == nil {
		t.Fatalf("expected divide by zero error")
	}

	x := new(big.Int).Lsh(big.NewInt(1), 255)
	if _, err := ShlDiv(x, bi(2), 2, RoundingDown); err == nil {
		t.Fatalf("expected overflow for 256-bit left shift")
	}
}

func TestShlDiv256AndMulDivU256(t *testing.T) {
	v, err := ShlDiv256(bi(5), bi(3), 1)
	if err != nil {
		t.Fatalf("ShlDiv256 error: %v", err)
	}
	if v.Cmp(bi(3)) != 0 {
		t.Fatalf("expected 3, got %s", v.String())
	}

	down, err := MulDivU256(bi(5), bi(5), bi(3), RoundingDown)
	if err != nil {
		t.Fatalf("MulDivU256 down error: %v", err)
	}
	if down.Cmp(bi(8)) != 0 {
		t.Fatalf("expected 8, got %s", down.String())
	}

	up, err := MulDivU256(bi(5), bi(5), bi(3), RoundingUp)
	if err != nil {
		t.Fatalf("MulDivU256 up error: %v", err)
	}
	if up.Cmp(bi(9)) != 0 {
		t.Fatalf("expected 9, got %s", up.String())
	}

	if _, err := MulDivU256(common.MaxU256, common.MaxU256, bi(1), RoundingDown); err == nil {
		t.Fatalf("expected overflow for huge multiplication")
	}
}
