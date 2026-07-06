// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

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

func SafeMulShrCast[T unsigned](x *big.Int, y *big.Int, offset uint8, rounding Rounding) (T, error) {
	v, err := MulShr(x, y, offset, rounding)
	if err != nil {
		var zero T
		return zero, err
	}
	return castToUnsigned[T](v)
}

func SafeShlDivCast[T unsigned](x *big.Int, y *big.Int, offset uint8, rounding Rounding) (T, error) {
	v, err := ShlDiv(x, y, offset, rounding)
	if err != nil {
		var zero T
		return zero, err
	}
	return castToUnsigned[T](v)
}

func SafeMulDivCast[T unsigned](x *big.Int, y *big.Int, denominator *big.Int, rounding Rounding) (T, error) {
	v, err := MulDiv(x, y, denominator, rounding)
	if err != nil {
		var zero T
		return zero, err
	}
	return castToUnsigned[T](v)
}
