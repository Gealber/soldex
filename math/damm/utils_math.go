// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

type unsigned interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64
}

func castToUnsigned[T unsigned](v *big.Int) (T, error) {
	var zero T
	if v == nil || v.Sign() < 0 || v.BitLen() > 64 {
		return zero, common.ErrTypeCastFailed
	}
	u := v.Uint64()
	max := uint64(^T(0))
	if u > max {
		return zero, common.ErrTypeCastFailed
	}
	return T(u), nil
}

func SafeMulShrCast[T unsigned](x *big.Int, y *big.Int, offset uint8) (T, error) {
	v, err := MulShr(x, y, offset)
	if err != nil {
		var zero T
		return zero, err
	}
	return castToUnsigned[T](v)
}

func SafeMulShr256Cast[T unsigned](x *big.Int, y *big.Int, offset uint8) (T, error) {
	v, err := MulShr256(x, y, offset)
	if err != nil {
		var zero T
		return zero, err
	}
	return castToUnsigned[T](v)
}

func SafeMulDivCastU64[T unsigned](x uint64, y uint64, denominator uint64, rounding Rounding) (T, error) {
	var zero T
	if denominator == 0 {
		return zero, common.ErrDivideByZero
	}

	xBig := common.Uint64ToBig(x)
	yBig := common.Uint64ToBig(y)
	prod, err := common.MulChecked(xBig, yBig, common.MaxU128)
	if err != nil {
		return zero, err
	}

	den := common.Uint64ToBig(denominator)
	var out *big.Int
	if rounding == RoundingUp {
		out, err = common.DivCeil(prod, den)
	} else {
		out, err = common.DivFloor(prod, den)
	}
	if err != nil {
		return zero, err
	}

	return castToUnsigned[T](out)
}

func SafeMulDivCastU128(x *big.Int, y *big.Int, denominator *big.Int, rounding Rounding) (*big.Int, error) {
	if err := common.EnsureUintWithin(x, common.MaxU128); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(y, common.MaxU128); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(denominator, common.MaxU128); err != nil {
		return nil, err
	}
	if denominator.Sign() == 0 {
		return nil, common.ErrDivideByZero
	}

	prod := new(big.Int).Mul(x, y)
	var out *big.Int
	var err error
	if rounding == RoundingUp {
		out, err = common.DivCeil(prod, denominator)
	} else {
		out, err = common.DivFloor(prod, denominator)
	}
	if err != nil {
		return nil, err
	}
	if !common.IsUintWithin(out, common.MaxU128) {
		return nil, common.ErrTypeCastFailed
	}
	return out, nil
}

func SafeShlDivCast[T unsigned](x *big.Int, y *big.Int, offset uint8, rounding Rounding) (T, error) {
	v, err := ShlDiv(x, y, offset, rounding)
	if err != nil {
		var zero T
		return zero, err
	}
	return castToUnsigned[T](v)
}

func SqrtU256(radicand *big.Int) (*big.Int, error) {
	if err := common.EnsureUintWithin(radicand, common.MaxU256); err != nil {
		return nil, err
	}

	if radicand.Sign() == 0 {
		return big.NewInt(0), nil
	}

	maxShift := 255
	shift := (maxShift - (radicand.BitLen() - 1)) &^ 1
	bit := new(big.Int).Lsh(big.NewInt(1), uint(shift))

	n := common.Clone(radicand)
	result := big.NewInt(0)

	for bit.Sign() != 0 {
		resultWithBit := new(big.Int).Add(result, bit)
		if n.Cmp(resultWithBit) >= 0 {
			n.Sub(n, resultWithBit)
			result.Rsh(result, 1)
			result.Add(result, bit)
		} else {
			result.Rsh(result, 1)
		}
		bit.Rsh(bit, 2)
	}

	if !common.IsUintWithin(result, common.MaxU256) {
		return nil, common.ErrMathOverflow
	}
	return result, nil
}
