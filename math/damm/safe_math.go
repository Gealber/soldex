// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	"math"
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

func SafeAddU64(a uint64, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, common.ErrMathOverflow
	}
	return a + b, nil
}

func SafeSubU64(a uint64, b uint64) (uint64, error) {
	if a < b {
		return 0, common.ErrMathOverflow
	}
	return a - b, nil
}

func SafeMulU64(a uint64, b uint64) (uint64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxUint64/b {
		return 0, common.ErrMathOverflow
	}
	return a * b, nil
}

func SafeDivU64(a uint64, b uint64) (uint64, error) {
	if b == 0 {
		return 0, common.ErrDivideByZero
	}
	return a / b, nil
}

func SafeRemU64(a uint64, b uint64) (uint64, error) {
	if b == 0 {
		return 0, common.ErrDivideByZero
	}
	return a % b, nil
}

func SafeShlU64(a uint64, offset uint) (uint64, error) {
	if offset >= 64 {
		return 0, common.ErrMathOverflow
	}
	if a > (math.MaxUint64 >> offset) {
		return 0, common.ErrMathOverflow
	}
	return a << offset, nil
}

func SafeShrU64(a uint64, offset uint) (uint64, error) {
	if offset >= 64 {
		return 0, common.ErrMathOverflow
	}
	return a >> offset, nil
}

func SafeAddBig(a *big.Int, b *big.Int, max *big.Int) (*big.Int, error) {
	return common.AddChecked(a, b, max)
}

func SafeSubBig(a *big.Int, b *big.Int, max *big.Int) (*big.Int, error) {
	return common.SubChecked(a, b, max)
}

func SafeMulBig(a *big.Int, b *big.Int, max *big.Int) (*big.Int, error) {
	return common.MulChecked(a, b, max)
}

func SafeDivBig(a *big.Int, b *big.Int) (*big.Int, error) {
	return common.DivFloor(a, b)
}

func SafeShlBig(a *big.Int, offset uint, max *big.Int) (*big.Int, error) {
	if a == nil {
		return nil, common.ErrMathOverflow
	}
	if a.Sign() < 0 {
		return nil, common.ErrNegativeValue
	}
	out := new(big.Int).Lsh(a, offset)
	if out.Cmp(max) > 0 {
		return nil, common.ErrMathOverflow
	}
	return out, nil
}

func SafeShrBig(a *big.Int, offset uint) (*big.Int, error) {
	if a == nil {
		return nil, common.ErrMathOverflow
	}
	if a.Sign() < 0 {
		return nil, common.ErrNegativeValue
	}
	return new(big.Int).Rsh(a, offset), nil
}

func SafeCastU128ToU64(v *big.Int) (uint64, error) {
	if err := common.EnsureUintWithin(v, common.MaxU128); err != nil {
		return 0, err
	}
	return common.BigToUint64Checked(v)
}
