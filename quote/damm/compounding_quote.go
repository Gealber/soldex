// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
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

// QuoteExactOutCompounding calculates swap input for exact-out compounding AMM swap.
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
	actualProtocolFee := uint64(0)
	actualClaimingFee := uint64(0)
	actualCompoundingFee := uint64(0)
	actualReferralFee := uint64(0)

	var amountInBeforeFee uint64
	var err error

	if !feeOnInput {
		inputAmount := uint64(0)
		if tradeDirection == TradeDirectionAtoB {
			inputAmount, err = compoundingAtoBFromAmountOut(tokenAReserve, tokenBReserve, amountOut)
		} else {
			inputAmount, err = compoundingBtoAFromAmountOut(tokenAReserve, tokenBReserve, amountOut)
		}
		if err != nil {
			return nil, err
		}

		feeRes, err := GetFeeOnAmount(inputAmount, tradeFeeNumerator, protocolFeePercent, compoundingFeeBps, referralFeePercent, hasReferral)
		if err != nil {
			return nil, err
		}
		amountInBeforeFee = feeRes.Amount
		actualProtocolFee = feeRes.ProtocolFee
		actualClaimingFee = feeRes.ClaimingFee
		actualCompoundingFee = feeRes.CompoundingFee
		actualReferralFee = feeRes.ReferralFee
	} else {
		inputAmount := uint64(0)
		if tradeDirection == TradeDirectionAtoB {
			inputAmount, err = compoundingAtoBFromAmountOut(tokenAReserve, tokenBReserve, amountOut)
		} else {
			inputAmount, err = compoundingBtoAFromAmountOut(tokenAReserve, tokenBReserve, amountOut)
		}
		if err != nil {
			return nil, err
		}

		feeRes, err := GetFeeOnAmount(inputAmount, tradeFeeNumerator, protocolFeePercent, compoundingFeeBps, referralFeePercent, hasReferral)
		if err != nil {
			return nil, err
		}
		amountInBeforeFee = inputAmount
		actualProtocolFee = feeRes.ProtocolFee
		actualClaimingFee = feeRes.ClaimingFee
		actualCompoundingFee = feeRes.CompoundingFee
		actualReferralFee = feeRes.ReferralFee
	}

	return &SwapResult{
		IncludedFeeInputAmount: amountInBeforeFee,
		ExcludedFeeInputAmount: amountInBeforeFee,
		OutputAmount:           amountOut,
		AmountLeft:             0,
		SplitFees: SplitFees{
			ClaimingFee:    actualClaimingFee,
			CompoundingFee: actualCompoundingFee,
			ProtocolFee:    actualProtocolFee,
			ReferralFee:    actualReferralFee,
		},
	}, nil
}
