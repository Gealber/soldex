// NOTE: Ported from commons/src/quote.rs + extensions/lb_pair.rs. MM liquidity
// only (limit orders and token-2022 transfer fees are not modeled).
package dlmm

import (
	"math/big"

	dlmmmath "github.com/Gealber/soldex/math/dlmm"
	"github.com/Gealber/soldex/models"
)

const (
	minBinID int32 = -443636
	maxBinID int32 = 443636

	basisPointMaxU64 uint64 = 10_000

	collectFeeModeOnlyY uint8 = 1
)

// BinReserves is the swappable reserve of a single bin.
type BinReserves struct {
	AmountX uint64
	AmountY uint64
}

// BinProvider returns the reserves of the bin with the given id and whether that
// bin is available (its BinArray is cached). A missing bin stops the swap, i.e.
// it is treated as the edge of known liquidity.
type BinProvider func(binID int32) (BinReserves, bool)

// SwapPool holds the LbPair fields needed to quote a swap. The volatility fields
// are copied; QuoteExactIn mutates only its local copy.
type SwapPool struct {
	ActiveID int32
	BinStep  uint16

	BaseFactor               uint16
	BaseFeePowerFactor       uint8
	VariableFeeControl       uint32
	MaxVolatilityAccumulator uint32
	FilterPeriod             uint16
	DecayPeriod              uint16
	ReductionFactor          uint16
	CollectFeeMode           uint8

	VolatilityAccumulator uint32
	VolatilityReference   uint32
	IndexReference        int32
	LastUpdateTimestamp   int64
}

// QuoteResult is the output of QuoteExactInDetailed: the amount out, the input
// actually consumed, and the distinct bin-array indices the swap touched (in
// traversal order), which the executor passes as the on-chain swap's remaining
// accounts. AmountInConsumed is less than the requested amount when known
// liquidity runs out before the input is exhausted (the swap stops at a gap or
// the edge of the cached arrays); the executor uses it to skip opportunities the
// on-chain swap could not fully fill.
type QuoteResult struct {
	AmountOut        uint64
	AmountInConsumed uint64
	BinArrayIndices  []int64
}

// QuoteExactIn returns the output amount for swapping amountIn through the pool.
// swapForY: true swaps token X in for token Y out.
func QuoteExactIn(pool SwapPool, swapForY bool, amountIn uint64, currentTimestamp int64, bins BinProvider) (uint64, error) {
	res, err := QuoteExactInDetailed(pool, swapForY, amountIn, currentTimestamp, bins)
	if err != nil {
		return 0, err
	}
	return res.AmountOut, nil
}

// QuoteExactInDetailed crosses bins until the input is consumed or known
// liquidity runs out, reporting the amount out and the bin-array indices visited.
func QuoteExactInDetailed(pool SwapPool, swapForY bool, amountIn uint64, currentTimestamp int64, bins BinProvider) (QuoteResult, error) {
	pool.updateReferences(currentTimestamp)
	feeOnInput := pool.feeOnInput(swapForY)

	amountLeft := amountIn
	totalOut := uint64(0)
	indices := make([]int64, 0, 4)
	lastIndex := int64(0)
	haveIndex := false

	for amountLeft > 0 {
		reserves, ok := bins(pool.ActiveID)
		if !ok {
			break
		}

		index := models.BinIDToArrayIndex(pool.ActiveID)
		if !haveIndex || index != lastIndex {
			indices = append(indices, index)
			lastIndex = index
			haveIndex = true
		}

		maxOut := reserves.AmountX
		if swapForY {
			maxOut = reserves.AmountY
		}

		if maxOut > 0 {
			pool.updateVolatilityAccumulator()
			price, err := dlmmmath.GetPriceFromID(pool.ActiveID, pool.BinStep)
			if err != nil {
				return QuoteResult{}, err
			}

			consumed, out, err := pool.swapAtBin(amountLeft, maxOut, price, swapForY, feeOnInput)
			if err != nil {
				return QuoteResult{}, err
			}
			amountLeft -= consumed
			totalOut += out
		}

		if amountLeft > 0 {
			next := pool.ActiveID + 1
			if swapForY {
				next = pool.ActiveID - 1
			}
			if next < minBinID || next > maxBinID {
				break
			}
			pool.ActiveID = next
		}
	}

	return QuoteResult{AmountOut: totalOut, AmountInConsumed: amountIn - amountLeft, BinArrayIndices: indices}, nil
}

// swapAtBin fills one bin (MM liquidity only), returning the input consumed from
// amountLeft (fee-inclusive) and the fee-excluded output. Mirrors
// swap_exact_in_quote_at_bin.
func (pool *SwapPool) swapAtBin(amountIn, maxOut uint64, price *big.Int, swapForY, feeOnInput bool) (uint64, uint64, error) {
	totalFeeRate := pool.totalFeeRate()

	excludedIn := amountIn
	if feeOnInput {
		fee, err := ComputeFeeFromAmount(amountIn, totalFeeRate)
		if err != nil {
			return 0, 0, err
		}
		excludedIn = amountIn - fee
	}

	maxIn, err := GetAmountInFromAmountOut(maxOut, price, swapForY)
	if err != nil {
		return 0, 0, err
	}

	var out, leftover uint64
	if excludedIn >= maxIn {
		out = maxOut
		leftover = excludedIn - maxIn
	} else {
		out, err = GetAmountOutFromAmountIn(excludedIn, price, swapForY)
		if err != nil {
			return 0, 0, err
		}
	}

	includedFeeAmountIn := amountIn
	if leftover > 0 {
		consumedExcluded := excludedIn - leftover
		includedFeeAmountIn = consumedExcluded
		if feeOnInput {
			fee, err := ComputeFee(consumedExcluded, totalFeeRate)
			if err != nil {
				return 0, 0, err
			}
			includedFeeAmountIn = consumedExcluded + fee
		}
	}

	excludedOut := out
	if !feeOnInput {
		fee, err := ComputeFeeFromAmount(out, totalFeeRate)
		if err != nil {
			return 0, 0, err
		}
		excludedOut = out - fee
	}

	return includedFeeAmountIn, excludedOut, nil
}

// updateReferences decays the volatility reference based on elapsed time since
// the last swap. Mirrors LbPair::update_references.
func (pool *SwapPool) updateReferences(currentTimestamp int64) {
	elapsed := currentTimestamp - pool.LastUpdateTimestamp
	if elapsed < int64(pool.FilterPeriod) {
		return
	}

	pool.IndexReference = pool.ActiveID
	if elapsed < int64(pool.DecayPeriod) {
		pool.VolatilityReference = uint32(uint64(pool.VolatilityAccumulator) * uint64(pool.ReductionFactor) / basisPointMaxU64)
		return
	}
	pool.VolatilityReference = 0
}

// updateVolatilityAccumulator recomputes the accumulator for the current bin
// distance from the reference. Mirrors LbPair::update_volatility_accumulator.
func (pool *SwapPool) updateVolatilityAccumulator() {
	deltaID := int64(pool.IndexReference) - int64(pool.ActiveID)
	if deltaID < 0 {
		deltaID = -deltaID
	}

	va := uint64(pool.VolatilityReference) + uint64(deltaID)*basisPointMaxU64
	if max := uint64(pool.MaxVolatilityAccumulator); va > max {
		va = max
	}
	pool.VolatilityAccumulator = uint32(va)
}

// totalFeeRate returns base + variable fee rate, capped at MaxFeeRate.
func (pool *SwapPool) totalFeeRate() uint64 {
	// base = base_factor * bin_step * 10 * 10^base_fee_power_factor
	base := new(big.Int).SetUint64(uint64(pool.BaseFactor))
	base.Mul(base, new(big.Int).SetUint64(uint64(pool.BinStep)))
	base.Mul(base, big.NewInt(10))
	powTen := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(pool.BaseFeePowerFactor)), nil)
	base.Mul(base, powTen)

	total := new(big.Int).Add(base, pool.variableFeeRate())
	if maxRate := new(big.Int).SetUint64(MaxFeeRate); total.Cmp(maxRate) > 0 {
		return MaxFeeRate
	}
	return total.Uint64()
}

// variableFeeRate mirrors LbPair::compute_variable_fee for the current accumulator.
func (pool *SwapPool) variableFeeRate() *big.Int {
	if pool.VariableFeeControl == 0 {
		return big.NewInt(0)
	}

	// square_vfa_bin = (volatility_accumulator * bin_step)^2
	vfaBin := new(big.Int).Mul(
		new(big.Int).SetUint64(uint64(pool.VolatilityAccumulator)),
		new(big.Int).SetUint64(uint64(pool.BinStep)),
	)
	square := new(big.Int).Mul(vfaBin, vfaBin)
	// v_fee = variable_fee_control * square_vfa_bin
	vFee := new(big.Int).Mul(new(big.Int).SetUint64(uint64(pool.VariableFeeControl)), square)
	// scaled = (v_fee + 99_999_999_999) / 100_000_000_000
	vFee.Add(vFee, big.NewInt(99_999_999_999))
	return vFee.Quo(vFee, big.NewInt(100_000_000_000))
}

// feeOnInput mirrors LbPair::fee_on_input. InputOnly takes fees on input; OnlyY
// takes fees on input only when the input token is Y (swap_for_x).
func (pool *SwapPool) feeOnInput(swapForY bool) bool {
	if pool.CollectFeeMode == collectFeeModeOnlyY {
		return !swapForY
	}
	return true
}
