// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	"errors"

	dammmath "github.com/Gealber/soldex/math/damm"
)

func compoundingAtoBFromAmountIn(tokenA uint64, tokenB uint64, amountIn uint64) (uint64, error) {
	return dammmath.SafeMulDivCastU64[uint64](tokenB, amountIn, tokenA+amountIn, dammmath.RoundingDown)
}

func compoundingBtoAFromAmountIn(tokenA uint64, tokenB uint64, amountIn uint64) (uint64, error) {
	return dammmath.SafeMulDivCastU64[uint64](tokenA, amountIn, tokenB+amountIn, dammmath.RoundingDown)
}

func compoundingAtoBFromAmountOut(tokenA uint64, tokenB uint64, amountOut uint64) (uint64, error) {
	return dammmath.SafeMulDivCastU64[uint64](tokenA, amountOut, tokenB-amountOut, dammmath.RoundingUp)
}

func compoundingBtoAFromAmountOut(tokenA uint64, tokenB uint64, amountOut uint64) (uint64, error) {
	return dammmath.SafeMulDivCastU64[uint64](tokenB, amountOut, tokenA-amountOut, dammmath.RoundingUp)
}

// QuoteExactInCompounding calculates swap output for exact-in compounding AMM swap.
func QuoteExactInCompounding(
	amountIn uint64,
	tokenAReserve uint64,
	tokenBReserve uint64,
	tradeDirection TradeDirection,
	tradeFeeNumerator uint64,
	feeOnInput bool,
	hasReferral bool,
	protocolFeePercent uint8,
	compoundingFeeBps uint16,
	referralFeePercent uint8,
) (*SwapResult, error) {
	actualProtocolFee := uint64(0)
	actualClaimingFee := uint64(0)
	actualCompoundingFee := uint64(0)
	actualReferralFee := uint64(0)
	includedFeeInputAmount := amountIn
	excludedFeeInputAmount := amountIn

	actualAmountIn := amountIn
	if feeOnInput {
		feeRes, err := GetFeeOnAmount(amountIn, tradeFeeNumerator, protocolFeePercent, compoundingFeeBps, referralFeePercent, hasReferral)
		if err != nil {
			return nil, err
		}
		actualAmountIn = feeRes.Amount
		excludedFeeInputAmount = feeRes.Amount
		includedFeeInputAmount = amountIn
		actualProtocolFee = feeRes.ProtocolFee
		actualClaimingFee = feeRes.ClaimingFee
		actualCompoundingFee = feeRes.CompoundingFee
		actualReferralFee = feeRes.ReferralFee
	}

	outputAmount := uint64(0)
	var err error
	if tradeDirection == TradeDirectionAtoB {
		outputAmount, err = compoundingAtoBFromAmountIn(tokenAReserve, tokenBReserve, actualAmountIn)
	} else {
		outputAmount, err = compoundingBtoAFromAmountIn(tokenAReserve, tokenBReserve, actualAmountIn)
	}
	if err != nil {
		return nil, err
	}

	if !feeOnInput {
		feeRes, err := GetFeeOnAmount(outputAmount, tradeFeeNumerator, protocolFeePercent, compoundingFeeBps, referralFeePercent, hasReferral)
		if err != nil {
			return nil, err
		}
		outputAmount = feeRes.Amount
		excludedFeeInputAmount = amountIn
		includedFeeInputAmount = amountIn
		actualProtocolFee = feeRes.ProtocolFee
		actualClaimingFee = feeRes.ClaimingFee
		actualCompoundingFee = feeRes.CompoundingFee
		actualReferralFee = feeRes.ReferralFee
	}

	return &SwapResult{
		IncludedFeeInputAmount: includedFeeInputAmount,
		ExcludedFeeInputAmount: excludedFeeInputAmount,
		OutputAmount:           outputAmount,
		AmountLeft:             0,
		SplitFees: SplitFees{
			ClaimingFee:    actualClaimingFee,
			CompoundingFee: actualCompoundingFee,
			ProtocolFee:    actualProtocolFee,
			ReferralFee:    actualReferralFee,
		},
	}, nil
}

// ErrInsufficientLiquidity is returned when an exact-out quote asks for at least
// the whole reserve of the output token. The constant-product curve needs an
// unbounded input for that, and the subtraction would otherwise wrap.
var ErrInsufficientLiquidity = errors.New("damm: amount out is not less than the output reserve")

// compoundingInputForOutput is the curve input that yields exactly amountOut,
// before any fee. Rounds up, so the pool never comes out short.
func compoundingInputForOutput(tokenA, tokenB, amountOut uint64, dir TradeDirection) (uint64, error) {
	if dir == TradeDirectionAtoB {
		if amountOut >= tokenB {
			return 0, ErrInsufficientLiquidity
		}
		return compoundingAtoBFromAmountOut(tokenA, tokenB, amountOut)
	}
	if amountOut >= tokenA {
		return 0, ErrInsufficientLiquidity
	}
	return compoundingBtoAFromAmountOut(tokenA, tokenB, amountOut)
}

// QuoteExactOutCompounding calculates the input required for an exact-out swap on
// a compounding AMM pool.
//
// Both fee modes have to gross UP, because the caller is telling us what they want
// to RECEIVE and the fee is paid on top of that:
//
//   - fee on input: the curve needs `net`, so the caller must send
//     `net / (1 - fee)` for `net` to survive the fee and reach the curve.
//   - fee on output: the caller wants `amountOut` NET, so the curve must produce
//     `amountOut / (1 - fee)` gross, and the input follows from that gross figure.
//
// Reversing either direction — returning the net curve input, or sizing the input
// off the net output — understates what the swap actually costs by the fee.
func QuoteExactOutCompounding(
	amountOut uint64,
	tokenAReserve uint64,
	tokenBReserve uint64,
	tradeDirection TradeDirection,
	tradeFeeNumerator uint64,
	feeOnInput bool,
	hasReferral bool,
	protocolFeePercent uint8,
	compoundingFeeBps uint16,
	referralFeePercent uint8,
) (*SwapResult, error) {
	var (
		includedFeeInputAmount uint64
		excludedFeeInputAmount uint64
		tradingFee             uint64
	)

	if feeOnInput {
		// The curve consumes the fee-excluded amount; the caller pays it plus the fee.
		net, err := compoundingInputForOutput(tokenAReserve, tokenBReserve, amountOut, tradeDirection)
		if err != nil {
			return nil, err
		}
		included, fee, err := GetIncludedFeeAmount(tradeFeeNumerator, net)
		if err != nil {
			return nil, err
		}
		includedFeeInputAmount = included
		excludedFeeInputAmount = net
		tradingFee = fee
	} else {
		// The fee is taken out of the output, so the curve must produce more than
		// the caller asked to receive.
		grossOut, fee, err := GetIncludedFeeAmount(tradeFeeNumerator, amountOut)
		if err != nil {
			return nil, err
		}
		input, err := compoundingInputForOutput(tokenAReserve, tokenBReserve, grossOut, tradeDirection)
		if err != nil {
			return nil, err
		}
		includedFeeInputAmount = input
		excludedFeeInputAmount = input
		tradingFee = fee
	}

	split, err := SplitTradingFees(tradingFee, protocolFeePercent, compoundingFeeBps, referralFeePercent, hasReferral)
	if err != nil {
		return nil, err
	}

	return &SwapResult{
		IncludedFeeInputAmount: includedFeeInputAmount,
		ExcludedFeeInputAmount: excludedFeeInputAmount,
		OutputAmount:           amountOut,
		AmountLeft:             0,
		SplitFees:              split,
	}, nil
}
