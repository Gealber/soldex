// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math"
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

func uint64ToBig(v uint64) *big.Int {
	return new(big.Int).SetUint64(v)
}

func minU64(a uint64, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func addU64(a uint64, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, common.ErrMathOverflow
	}
	return a + b, nil
}

func subU64(a uint64, b uint64) (uint64, error) {
	if a < b {
		return 0, common.ErrMathOverflow
	}
	return a - b, nil
}
