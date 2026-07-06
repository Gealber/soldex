package damm

import (
	"errors"
	"math/big"

	"github.com/Gealber/soldex/math/common"
	dammmath "github.com/Gealber/soldex/math/damm"
)

// CollectFeeMode values from cp-amm.
const (
	CollectFeeModeBothToken   uint8 = 0
	CollectFeeModeOnlyB       uint8 = 1
	CollectFeeModeCompounding uint8 = 2
)

// Max trading fee numerator per pool fee version (out of FeeDenominator).
const (
	maxFeeNumeratorV0 uint64 = 500_000_000 // 50%
	maxFeeNumeratorV1 uint64 = 990_000_000 // 99%
)

// variable fee scaling constants from cp-amm DynamicFeeStruct::get_variable_fee.
var (
	variableFeeRoundUp = big.NewInt(99_999_999_999)
	variableFeeScale   = big.NewInt(100_000_000_000)
)

// ErrPriceRangeViolation is returned when an exact-in swap would push the price
// past the pool's concentrated-liquidity range (mirrors PriceRangeViolation).
var ErrPriceRangeViolation = errors.New("price range violation")

// ConcentratedPool holds the decoded Pool fields needed to quote a
// concentrated-liquidity swap. All sqrt prices and liquidity are Q64.64 u128s.
type ConcentratedPool struct {
	SqrtPrice    *big.Int
	SqrtMinPrice *big.Int
	SqrtMaxPrice *big.Int
	Liquidity    *big.Int

	CollectFeeMode   uint8
	FeeVersion       uint8
	BaseFeeNumerator uint64

	// Dynamic (variable) fee state. Applied only when DynamicFeeInitialized.
	DynamicFeeInitialized bool
	VolatilityAccumulator *big.Int
	BinStep               uint16
	VariableFeeControl    uint32
}

// QuoteConcentratedExactIn returns the net output amount for an exact-in swap on
// a concentrated-liquidity DAMM pool, mirroring Pool::get_swap_result_from_exact_input.
func QuoteConcentratedExactIn(amountIn uint64, dir TradeDirection, pool ConcentratedPool) (uint64, error) {
	feeNumerator, err := totalTradingFeeNumerator(pool)
	if err != nil {
		return 0, err
	}
	feesOnInput := feeOnInput(pool.CollectFeeMode, dir)

	actualAmountIn := amountIn
	if feesOnInput {
		res, err := GetFeeOnAmount(amountIn, feeNumerator, 0, 0, 0, false)
		if err != nil {
			return 0, err
		}
		actualAmountIn = res.Amount
	}

	output, err := concentratedOutput(actualAmountIn, dir, pool)
	if err != nil {
		return 0, err
	}

	if !feesOnInput {
		res, err := GetFeeOnAmount(output, feeNumerator, 0, 0, 0, false)
		if err != nil {
			return 0, err
		}
		output = res.Amount
	}
	return output, nil
}

// concentratedOutput runs the curve for a single exact-in swap (no fees).
func concentratedOutput(amountIn uint64, dir TradeDirection, pool ConcentratedPool) (uint64, error) {
	if dir == TradeDirectionAtoB {
		next, err := dammmath.GetNextSqrtPriceFromInput(pool.SqrtPrice, pool.Liquidity, amountIn, true)
		if err != nil {
			return 0, err
		}
		if next.Cmp(pool.SqrtMinPrice) < 0 {
			return 0, ErrPriceRangeViolation
		}
		return dammmath.GetDeltaAmountB(next, pool.SqrtPrice, pool.Liquidity, false)
	}

	next, err := dammmath.GetNextSqrtPriceFromInput(pool.SqrtPrice, pool.Liquidity, amountIn, false)
	if err != nil {
		return 0, err
	}
	if next.Cmp(pool.SqrtMaxPrice) > 0 {
		return 0, ErrPriceRangeViolation
	}
	return dammmath.GetDeltaAmountA(pool.SqrtPrice, next, pool.Liquidity, false)
}

// feeOnInput mirrors FeeMode::get_fee_mode for the non-compounding modes: fees
// are taken on input only for OnlyB pools swapping B->A.
func feeOnInput(collectFeeMode uint8, dir TradeDirection) bool {
	return collectFeeMode == CollectFeeModeOnlyB && dir == TradeDirectionBtoA
}

// totalTradingFeeNumerator returns base + variable fee, capped to the per-version
// maximum. Uses the pool's stored dynamic-fee state (no pre-swap reference
// re-derivation), so it matches on-chain when the pool is not mid-decay.
func totalTradingFeeNumerator(pool ConcentratedPool) (uint64, error) {
	total := new(big.Int).SetUint64(pool.BaseFeeNumerator)
	if pool.DynamicFeeInitialized {
		variable, err := variableFee(pool)
		if err != nil {
			return 0, err
		}
		total.Add(total, variable)
	}

	maxNumerator := maxFeeNumeratorV1
	if pool.FeeVersion == 0 {
		maxNumerator = maxFeeNumeratorV0
	}
	if total.Cmp(new(big.Int).SetUint64(maxNumerator)) > 0 {
		return maxNumerator, nil
	}
	return total.Uint64(), nil
}

// variableFee mirrors DynamicFeeStruct::get_variable_fee using stored state.
func variableFee(pool ConcentratedPool) (*big.Int, error) {
	if pool.VolatilityAccumulator == nil {
		return nil, common.ErrMathOverflow
	}
	// square_vfa_bin = (volatility_accumulator * bin_step)^2
	vfaBin := new(big.Int).Mul(pool.VolatilityAccumulator, new(big.Int).SetUint64(uint64(pool.BinStep)))
	square := new(big.Int).Mul(vfaBin, vfaBin)
	// v_fee = square_vfa_bin * variable_fee_control
	vFee := new(big.Int).Mul(square, new(big.Int).SetUint64(uint64(pool.VariableFeeControl)))
	// scaled = (v_fee + 99_999_999_999) / 100_000_000_000
	vFee.Add(vFee, variableFeeRoundUp)
	return new(big.Int).Quo(vFee, variableFeeScale), nil
}
