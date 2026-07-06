// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	"math"
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

const (
	maxBasisPoint  uint64 = 10_000
	scaleOffsetU32 uint   = 64
	maxExponential uint32 = 0x80000
)

var oneQ64 = new(big.Int).Lsh(big.NewInt(1), 64)

func GetFeeInPeriod(cliffFeeNumerator uint64, reductionFactor uint64, passedPeriod uint16) (uint64, error) {
	if reductionFactor == 0 {
		return cliffFeeNumerator, nil
	}

	bpsNum := new(big.Int).Lsh(common.Uint64ToBig(reductionFactor), 64)
	bps, err := common.DivFloor(bpsNum, common.Uint64ToBig(maxBasisPoint))
	if err != nil {
		return 0, err
	}

	base, err := common.SubChecked(oneQ64, bps, common.MaxU128)
	if err != nil {
		return 0, err
	}

	result, err := Pow(base, int32(passedPeriod))
	if err != nil {
		return 0, err
	}

	mul, err := common.MulChecked(result, common.Uint64ToBig(cliffFeeNumerator), common.MaxU128)
	if err != nil {
		return 0, err
	}
	fee := new(big.Int).Rsh(mul, 64)
	return common.BigToUint64Checked(fee)
}

func Pow(base *big.Int, exp int32) (*big.Int, error) {
	if exp == math.MinInt32 {
		return nil, common.ErrMathOverflow
	}
	if err := common.EnsureUintWithin(base, common.MaxU128); err != nil {
		return nil, err
	}

	invert := exp < 0
	if exp == 0 {
		return common.Clone(oneQ64), nil
	}

	absExp := uint32(exp)
	if invert {
		absExp = uint32(-exp)
	}

	if absExp >= maxExponential {
		return nil, common.ErrMathOverflow
	}

	squaredBase := common.Clone(base)
	result := common.Clone(oneQ64)

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
			result.Rsh(mul, scaleOffsetU32)
		}

		if i < 18 {
			mul, err := common.MulChecked(squaredBase, squaredBase, common.MaxU128)
			if err != nil {
				return nil, err
			}
			squaredBase.Rsh(mul, scaleOffsetU32)
		}
	}

	if result.Sign() == 0 {
		return nil, common.ErrMathOverflow
	}

	if invert {
		result = new(big.Int).Quo(common.MaxU128, result)
	}

	return result, nil
}
