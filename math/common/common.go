// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package common

import (
	"errors"
	"math/big"
)

var (
	ErrMathOverflow   = errors.New("math overflow")
	ErrDivideByZero   = errors.New("divide by zero")
	ErrTypeCastFailed = errors.New("type cast failed")
	ErrNegativeValue  = errors.New("negative value")
)

var (
	Zero = big.NewInt(0)
	One  = big.NewInt(1)

	MaxU128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	MaxU256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	MaxU512 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 512), big.NewInt(1))
)

func Clone(x *big.Int) *big.Int {
	if x == nil {
		return nil
	}
	return new(big.Int).Set(x)
}

func IsUintWithin(x *big.Int, max *big.Int) bool {
	if x == nil {
		return false
	}
	if x.Sign() < 0 {
		return false
	}
	return x.Cmp(max) <= 0
}

func EnsureUintWithin(x *big.Int, max *big.Int) error {
	if x == nil {
		return ErrMathOverflow
	}
	if x.Sign() < 0 {
		return ErrNegativeValue
	}
	if x.Cmp(max) > 0 {
		return ErrMathOverflow
	}
	return nil
}

func MulChecked(a *big.Int, b *big.Int, max *big.Int) (*big.Int, error) {
	if err := EnsureUintWithin(a, max); err != nil {
		return nil, err
	}
	if err := EnsureUintWithin(b, max); err != nil {
		return nil, err
	}
	res := new(big.Int).Mul(a, b)
	if res.Cmp(max) > 0 {
		return nil, ErrMathOverflow
	}
	return res, nil
}

func AddChecked(a *big.Int, b *big.Int, max *big.Int) (*big.Int, error) {
	if err := EnsureUintWithin(a, max); err != nil {
		return nil, err
	}
	if err := EnsureUintWithin(b, max); err != nil {
		return nil, err
	}
	res := new(big.Int).Add(a, b)
	if res.Cmp(max) > 0 {
		return nil, ErrMathOverflow
	}
	return res, nil
}

func SubChecked(a *big.Int, b *big.Int, max *big.Int) (*big.Int, error) {
	if err := EnsureUintWithin(a, max); err != nil {
		return nil, err
	}
	if err := EnsureUintWithin(b, max); err != nil {
		return nil, err
	}
	if a.Cmp(b) < 0 {
		return nil, ErrMathOverflow
	}
	return new(big.Int).Sub(a, b), nil
}

func DivFloor(num *big.Int, den *big.Int) (*big.Int, error) {
	if den == nil || den.Sign() == 0 {
		return nil, ErrDivideByZero
	}
	if num == nil {
		return nil, ErrMathOverflow
	}
	if num.Sign() < 0 || den.Sign() < 0 {
		return nil, ErrNegativeValue
	}
	return new(big.Int).Quo(num, den), nil
}

func DivCeil(num *big.Int, den *big.Int) (*big.Int, error) {
	if den == nil || den.Sign() == 0 {
		return nil, ErrDivideByZero
	}
	if num == nil {
		return nil, ErrMathOverflow
	}
	if num.Sign() < 0 || den.Sign() < 0 {
		return nil, ErrNegativeValue
	}
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() != 0 {
		q.Add(q, One)
	}
	return q, nil
}

func Uint64ToBig(v uint64) *big.Int {
	return new(big.Int).SetUint64(v)
}

func BigToUint64Checked(v *big.Int) (uint64, error) {
	if v == nil {
		return 0, ErrTypeCastFailed
	}
	if v.Sign() < 0 || v.BitLen() > 64 {
		return 0, ErrTypeCastFailed
	}
	return v.Uint64(), nil
}
