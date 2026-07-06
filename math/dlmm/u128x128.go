// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

func MulDiv(x *big.Int, y *big.Int, denominator *big.Int, rounding Rounding) (*big.Int, error) {
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
	switch rounding {
	case RoundingUp:
		var err error
		out, err = common.DivCeil(prod, denominator)
		if err != nil {
			return nil, err
		}
	default:
		var err error
		out, err = common.DivFloor(prod, denominator)
		if err != nil {
			return nil, err
		}
	}

	if !common.IsUintWithin(out, common.MaxU128) {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}

func MulShr(x *big.Int, y *big.Int, offset uint8, rounding Rounding) (*big.Int, error) {
	if offset >= 128 {
		return nil, common.ErrMathOverflow
	}
	denominator := new(big.Int).Lsh(big.NewInt(1), uint(offset))
	return MulDiv(x, y, denominator, rounding)
}

func ShlDiv(x *big.Int, y *big.Int, offset uint8, rounding Rounding) (*big.Int, error) {
	if offset >= 128 {
		return nil, common.ErrMathOverflow
	}
	scale := new(big.Int).Lsh(big.NewInt(1), uint(offset))
	return MulDiv(x, scale, y, rounding)
}
