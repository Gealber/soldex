// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math"
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

const (
	Precision      uint64 = 1_000_000_000_000
	ScaleOffset    uint8  = 64
	maxExponential uint32 = 0x80000
)

var One = new(big.Int).Lsh(big.NewInt(1), uint(ScaleOffset))

func Pow(base *big.Int, exp int32) (*big.Int, error) {
	if err := common.EnsureUintWithin(base, common.MaxU128); err != nil {
		return nil, err
	}

	invert := exp < 0
	if exp == 0 {
		return common.Clone(One), nil
	}

	absExp := uint32(exp)
	if invert {
		if exp == math.MinInt32 {
			return nil, common.ErrMathOverflow
		}
		absExp = uint32(-exp)
	}

	if absExp >= maxExponential {
		return nil, common.ErrMathOverflow
	}

	squaredBase := common.Clone(base)
	result := common.Clone(One)

	if squaredBase.Cmp(result) >= 0 {
		squaredBase = new(big.Int).Quo(common.MaxU128, squaredBase)
		invert = !invert
	}

	for i := uint(0); i < 19; i++ {
		bit := uint32(1) << i
		if absExp&bit > 0 {
			mul, err := common.MulChecked(result, squaredBase, common.MaxU128)
			if err != nil {
				return nil, err
			}
			result.Rsh(mul, uint(ScaleOffset))
		}

		if i < 18 {
			mul, err := common.MulChecked(squaredBase, squaredBase, common.MaxU128)
			if err != nil {
				return nil, err
			}
			squaredBase.Rsh(mul, uint(ScaleOffset))
		}
	}

	if result.Sign() == 0 {
		return nil, common.ErrMathOverflow
	}

	if invert {
		result = new(big.Int).Quo(common.MaxU128, result)
	}

	if !common.IsUintWithin(result, common.MaxU128) {
		return nil, common.ErrMathOverflow
	}

	return result, nil
}
