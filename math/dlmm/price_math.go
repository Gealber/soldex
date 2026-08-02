// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
)

const basisPointMax uint64 = 10_000

// GetPriceFromID returns the Q64.64 price of a bin. Memoised: the result depends only on the two
// arguments, and the swap loop asks for the same pairs over and over. See price_cache.go.
func GetPriceFromID(activeID int32, binStep uint16) (*big.Int, error) {
	return cachedPriceFromID(activeID, binStep)
}

// computePriceFromID is GetPriceFromID's arithmetic, unmemoised. Kept separate so the cache has
// something to call and so a test can compare the two.
func computePriceFromID(activeID int32, binStep uint16) (*big.Int, error) {
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
