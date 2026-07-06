// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

const basisPointMax uint64 = 10_000

func GetPriceFromID(activeID int32, binStep uint16) (*big.Int, error) {
	bpsNum := new(big.Int).Lsh(new(big.Int).SetUint64(uint64(binStep)), uint(ScaleOffset))
	bps, err := common.DivFloor(bpsNum, new(big.Int).SetUint64(basisPointMax))
	if err != nil {
		return nil, err
	}

	base, err := common.AddChecked(One, bps, common.MaxU128)
	if err != nil {
		return nil, err
	}

	return Pow(base, activeID)
}
