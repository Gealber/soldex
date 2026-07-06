// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

import (
	dammmath "github.com/Gealber/soldex/math/damm"
)

const (
	FeeDenominator uint64 = 1_000_000_000
	MaxBasisPoint  uint64 = 10_000
)

// GetExcludedFeeAmount extracts the amount and fee from an amount that includes fees.
func GetExcludedFeeAmount(tradeFeeNumerator uint64, includedFeeAmount uint64) (uint64, uint64, error) {
	tradingFee, err := dammmath.SafeMulDivCastU64[uint64](includedFeeAmount, tradeFeeNumerator, FeeDenominator, dammmath.RoundingUp)
	if err != nil {
		return 0, 0, err
	}
	amount := includedFeeAmount - tradingFee
	return amount, tradingFee, nil
}

// GetIncludedFeeAmount calculates the total amount (including fees) from an excluded amount.
func GetIncludedFeeAmount(tradeFeeNumerator uint64, excludedFeeAmount uint64) (uint64, uint64, error) {
	den := FeeDenominator - tradeFeeNumerator
	included, err := dammmath.SafeMulDivCastU64[uint64](excludedFeeAmount, FeeDenominator, den, dammmath.RoundingUp)
	if err != nil {
		return 0, 0, err
	}
	fee := included - excludedFeeAmount
	return included, fee, nil
}

// SplitTradingFees splits trading fees into protocol, compounding, claiming, and optional referral portions.
func SplitTradingFees(
	feeAmount uint64,
	protocolFeePercent uint8,
	compoundingFeeBps uint16,
	referralFeePercent uint8,
	hasReferral bool,
) (SplitFees, error) {
	protocolFee, err := dammmath.SafeMulDivCastU64[uint64](feeAmount, uint64(protocolFeePercent), 100, dammmath.RoundingDown)
	if err != nil {
		return SplitFees{}, err
	}

	tradingFee := feeAmount - protocolFee
	compoundingFee := uint64(0)
	claimingFee := tradingFee

	if compoundingFeeBps > 0 {
		compoundingFee, err = dammmath.SafeMulDivCastU64[uint64](tradingFee, uint64(compoundingFeeBps), MaxBasisPoint, dammmath.RoundingDown)
		if err != nil {
			return SplitFees{}, err
		}
		claimingFee = tradingFee - compoundingFee
	}

	referralFee := uint64(0)
	if hasReferral {
		referralFee, err = dammmath.SafeMulDivCastU64[uint64](protocolFee, uint64(referralFeePercent), 100, dammmath.RoundingDown)
		if err != nil {
			return SplitFees{}, err
		}
		protocolFee -= referralFee
	}

	return SplitFees{
		ClaimingFee:    claimingFee,
		CompoundingFee: compoundingFee,
		ProtocolFee:    protocolFee,
		ReferralFee:    referralFee,
	}, nil
}

// GetFeeOnAmount computes all fee components from a total amount.
func GetFeeOnAmount(
	amount uint64,
	tradeFeeNumerator uint64,
	protocolFeePercent uint8,
	compoundingFeeBps uint16,
	referralFeePercent uint8,
	hasReferral bool,
) (FeeOnAmountResult, error) {
	excluded, tradingFee, err := GetExcludedFeeAmount(tradeFeeNumerator, amount)
	if err != nil {
		return FeeOnAmountResult{}, err
	}

	split, err := SplitTradingFees(tradingFee, protocolFeePercent, compoundingFeeBps, referralFeePercent, hasReferral)
	if err != nil {
		return FeeOnAmountResult{}, err
	}

	return FeeOnAmountResult{
		Amount:         excluded,
		ClaimingFee:    split.ClaimingFee,
		CompoundingFee: split.CompoundingFee,
		ProtocolFee:    split.ProtocolFee,
		ReferralFee:    split.ReferralFee,
	}, nil
}
