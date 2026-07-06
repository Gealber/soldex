package common

import (
	"math/big"
	"testing"
)

func TestEnsureUintWithin(t *testing.T) {
	if err := EnsureUintWithin(big.NewInt(0), MaxU128); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if err := EnsureUintWithin(big.NewInt(-1), MaxU128); err == nil {
		t.Fatalf("expected error for negative value")
	}

	over := new(big.Int).Add(MaxU128, big.NewInt(1))
	if err := EnsureUintWithin(over, MaxU128); err == nil {
		t.Fatalf("expected overflow error")
	}
}

func TestDivCeilAndDivFloor(t *testing.T) {
	num := big.NewInt(10)
	den := big.NewInt(3)

	floor, err := DivFloor(num, den)
	if err != nil {
		t.Fatalf("DivFloor error: %v", err)
	}
	if floor.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("expected floor=3, got %s", floor.String())
	}

	ceil, err := DivCeil(num, den)
	if err != nil {
		t.Fatalf("DivCeil error: %v", err)
	}
	if ceil.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("expected ceil=4, got %s", ceil.String())
	}
}

func TestMulCheckedOverflow(t *testing.T) {
	_, err := MulChecked(MaxU128, big.NewInt(2), MaxU128)
	if err == nil {
		t.Fatalf("expected overflow error")
	}
}

func TestBigToUint64Checked(t *testing.T) {
	v, err := BigToUint64Checked(new(big.Int).SetUint64(123))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 123 {
		t.Fatalf("expected 123, got %d", v)
	}

	over := new(big.Int).Lsh(big.NewInt(1), 65)
	if _, err := BigToUint64Checked(over); err == nil {
		t.Fatalf("expected cast error")
	}
}
