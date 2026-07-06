// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

type Rounding uint8

const (
	RoundingUp Rounding = iota
	RoundingDown
)

func MulShr(x *big.Int, y *big.Int, offset uint8) (*big.Int, error) {
	if err := common.EnsureUintWithin(x, common.MaxU128); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(y, common.MaxU128); err != nil {
		return nil, err
	}
	prod := new(big.Int).Mul(x, y)
	out := new(big.Int).Rsh(prod, uint(offset))
	if !common.IsUintWithin(out, common.MaxU128) {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}

func MulShr256(x *big.Int, y *big.Int, offset uint8) (*big.Int, error) {
	if err := common.EnsureUintWithin(x, common.MaxU256); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(y, common.MaxU256); err != nil {
		return nil, err
	}
	prod, err := common.MulChecked(x, y, common.MaxU512)
	if err != nil {
		return nil, err
	}
	out := new(big.Int).Rsh(prod, uint(offset))
	if !common.IsUintWithin(out, common.MaxU128) {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}

func ShlDiv(x *big.Int, y *big.Int, offset uint8, rounding Rounding) (*big.Int, error) {
	if err := common.EnsureUintWithin(x, common.MaxU128); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(y, common.MaxU128); err != nil {
		return nil, err
	}
	if y.Sign() == 0 {
		return nil, common.ErrDivideByZero
	}
	if x.Sign() > 0 && x.BitLen()+int(offset) > 256 {
		return nil, common.ErrMathOverflow
	}

	prod := new(big.Int).Lsh(x, uint(offset))
	var out *big.Int
	var err error
	if rounding == RoundingUp {
		out, err = common.DivCeil(prod, y)
	} else {
		out, err = common.DivFloor(prod, y)
	}
	if err != nil {
		return nil, err
	}
	if !common.IsUintWithin(out, common.MaxU128) {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}

func ShlDiv256(x *big.Int, y *big.Int, offset uint8) (*big.Int, error) {
	if err := common.EnsureUintWithin(x, common.MaxU128); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(y, common.MaxU128); err != nil {
		return nil, err
	}
	if y.Sign() == 0 {
		return nil, common.ErrDivideByZero
	}
	if x.Sign() > 0 && x.BitLen()+int(offset) > 256 {
		return nil, common.ErrMathOverflow
	}

	prod := new(big.Int).Lsh(x, uint(offset))
	out, err := common.DivFloor(prod, y)
	if err != nil {
		return nil, err
	}
	if !common.IsUintWithin(out, common.MaxU256) {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}

func MulDivU256(x *big.Int, y *big.Int, denominator *big.Int, rounding Rounding) (*big.Int, error) {
	if err := common.EnsureUintWithin(x, common.MaxU256); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(y, common.MaxU256); err != nil {
		return nil, err
	}
	if err := common.EnsureUintWithin(denominator, common.MaxU256); err != nil {
		return nil, err
	}
	if denominator.Sign() == 0 {
		return nil, common.ErrDivideByZero
	}

	prod, err := common.MulChecked(x, y, common.MaxU512)
	if err != nil {
		return nil, err
	}

	var out *big.Int
	if rounding == RoundingUp {
		out, err = common.DivCeil(prod, denominator)
	} else {
		out, err = common.DivFloor(prod, denominator)
	}
	if err != nil {
		return nil, err
	}
	if !common.IsUintWithin(out, common.MaxU256) {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}
