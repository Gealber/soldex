// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"github.com/Gealber/soldex/math/common"
)

const (
	FeePrecision  uint64 = 1_000_000_000
	MaxFeeRate    uint64 = 100_000_000
	BasisPointMax uint64 = 10_000
)

// ComputeFee mirrors DLMM LbPair::compute_fee for exact-in paths.
func ComputeFee(amount uint64, totalFeeRate uint64) (uint64, error) {
	if totalFeeRate >= FeePrecision {
		return 0, common.ErrMathOverflow
	}

	denominator := FeePrecision - totalFeeRate
	numerator := uint64ToBig(amount)
	numerator.Mul(numerator, uint64ToBig(totalFeeRate))
	numerator.Add(numerator, uint64ToBig(denominator-1))
	q, err := common.DivFloor(numerator, uint64ToBig(denominator))
	if err != nil {
		return 0, err
	}
	return common.BigToUint64Checked(q)
}

// ComputeFeeFromAmount mirrors DLMM LbPair::compute_fee_from_amount.
func ComputeFeeFromAmount(amountWithFees uint64, totalFeeRate uint64) (uint64, error) {
	numerator := uint64ToBig(amountWithFees)
	numerator.Mul(numerator, uint64ToBig(totalFeeRate))
	numerator.Add(numerator, uint64ToBig(FeePrecision-1))
	q, err := common.DivFloor(numerator, uint64ToBig(FeePrecision))
	if err != nil {
		return 0, err
	}
	return common.BigToUint64Checked(q)
}

// ComputeProtocolFee splits the total fee by protocol share basis points.
func ComputeProtocolFee(feeAmount uint64, protocolShareBps uint16) (uint64, error) {
	numerator := uint64ToBig(feeAmount)
	numerator.Mul(numerator, uint64ToBig(uint64(protocolShareBps)))
	q, err := common.DivFloor(numerator, uint64ToBig(BasisPointMax))
	if err != nil {
		return 0, err
	}
	return common.BigToUint64Checked(q)
}

// TotalFeeRate combines base and variable fee rates, capping at MaxFeeRate.
func TotalFeeRate(baseFeeRate uint64, variableFeeRate uint64) uint64 {
	total := baseFeeRate + variableFeeRate
	if total > MaxFeeRate {
		return MaxFeeRate
	}
	return total
}
