package damm

import (
	"math"
	"math/big"
	"testing"
)

func TestSafeMathU64(t *testing.T) {
	if _, err := SafeAddU64(math.MaxUint64, 1); err == nil {
		t.Fatalf("expected overflow on add")
	}
	if v, err := SafeAddU64(2, 3); err != nil || v != 5 {
		t.Fatalf("unexpected add result: %d err=%v", v, err)
	}

	if _, err := SafeSubU64(1, 2); err == nil {
		t.Fatalf("expected overflow on sub")
	}
	if _, err := SafeMulU64(math.MaxUint64, 2); err == nil {
		t.Fatalf("expected overflow on mul")
	}

	if _, err := SafeDivU64(1, 0); err == nil {
		t.Fatalf("expected divide by zero")
	}
	if _, err := SafeRemU64(1, 0); err == nil {
		t.Fatalf("expected divide by zero on rem")
	}

	if _, err := SafeShlU64(1, 64); err == nil {
		t.Fatalf("expected overflow on shl")
	}
	if v, err := SafeShrU64(8, 1); err != nil || v != 4 {
		t.Fatalf("unexpected shr result: %d err=%v", v, err)
	}
}

func TestSafeCastAndUtilsCasts(t *testing.T) {
	v, err := SafeCastU128ToU64(new(big.Int).SetUint64(42))
	if err != nil || v != 42 {
		t.Fatalf("unexpected cast result: %d err=%v", v, err)
	}

	if _, err := SafeMulDivCastU64[uint16](100, 3, 2, RoundingDown); err != nil {
		t.Fatalf("SafeMulDivCastU64 error: %v", err)
	}

	if _, err := SafeMulDivCastU64[uint8](200, 2, 1, RoundingDown); err == nil {
		t.Fatalf("expected cast overflow for uint8")
	}
}

func TestSqrtU256(t *testing.T) {
	root, err := SqrtU256(big.NewInt(16))
	if err != nil {
		t.Fatalf("SqrtU256 error: %v", err)
	}
	if root.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("expected 4, got %s", root.String())
	}

	root2, err := SqrtU256(big.NewInt(15))
	if err != nil {
		t.Fatalf("SqrtU256 error: %v", err)
	}
	if root2.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("expected floor sqrt=3, got %s", root2.String())
	}
}

func TestFeeMath(t *testing.T) {
	v, err := GetFeeInPeriod(1_000, 0, 100)
	if err != nil {
		t.Fatalf("GetFeeInPeriod error: %v", err)
	}
	if v != 1_000 {
		t.Fatalf("expected unchanged fee with zero reduction factor")
	}

	v2, err := GetFeeInPeriod(1_000, 100, 0)
	if err != nil {
		t.Fatalf("GetFeeInPeriod error: %v", err)
	}
	if v2 != 1_000 {
		t.Fatalf("expected unchanged fee at period 0")
	}

	p0, err := Pow(oneQ64, 0)
	if err != nil {
		t.Fatalf("Pow error: %v", err)
	}
	if p0.Cmp(oneQ64) != 0 {
		t.Fatalf("expected oneQ64 for exp=0")
	}

	p1, err := Pow(oneQ64, -42)
	if err != nil {
		t.Fatalf("Pow error: %v", err)
	}
	if p1.Sign() <= 0 {
		t.Fatalf("expected positive non-zero result for negative exponent")
	}
}
