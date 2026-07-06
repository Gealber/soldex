// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package dlmm

import (
	"math/big"

	"github.com/Gealber/soldex/math/common"
	dlmmmath "github.com/Gealber/soldex/math/dlmm"
)

type BinSwapQuote struct {
	AmountInWithFees     uint64
	AmountOut            uint64
	Fee                  uint64
	ProtocolFeeAfterHost uint64
	HostFee              uint64
	AmountIntoBin        uint64
	NewReserveX          uint64
	NewReserveY          uint64
}

func GetMaxAmountOut(reserveX uint64, reserveY uint64, swapForY bool) uint64 {
	if swapForY {
		return reserveY
	}
	return reserveX
}

func GetMaxAmountIn(maxAmountOut uint64, price *big.Int, swapForY bool) (uint64, error) {
	if swapForY {
		return dlmmmath.SafeShlDivCast[uint64](uint64ToBig(maxAmountOut), price, dlmmmath.ScaleOffset, dlmmmath.RoundingUp)
	}
	return dlmmmath.SafeMulShrCast[uint64](uint64ToBig(maxAmountOut), price, dlmmmath.ScaleOffset, dlmmmath.RoundingUp)
}

func GetAmountInFromAmountOut(amountOut uint64, price *big.Int, swapForY bool) (uint64, error) {
	return GetMaxAmountIn(amountOut, price, swapForY)
}

func GetAmountOutFromAmountIn(amountIn uint64, price *big.Int, swapForY bool) (uint64, error) {
	if swapForY {
		return dlmmmath.SafeMulShrCast[uint64](price, uint64ToBig(amountIn), dlmmmath.ScaleOffset, dlmmmath.RoundingDown)
	}
	return dlmmmath.SafeShlDivCast[uint64](uint64ToBig(amountIn), price, dlmmmath.ScaleOffset, dlmmmath.RoundingDown)
}

// QuoteSwapOnBin mirrors DLMM Bin::swap mechanics for a single bin.
func QuoteSwapOnBin(
	amountIn uint64,
	reserveX uint64,
	reserveY uint64,
	price *big.Int,
	swapForY bool,
	totalFeeRate uint64,
	protocolShareBps uint16,
	hostFeeBps *uint16,
) (*BinSwapQuote, error) {
	maxAmountOut := GetMaxAmountOut(reserveX, reserveY, swapForY)
	maxAmountInNoFee, err := GetMaxAmountIn(maxAmountOut, price, swapForY)
	if err != nil {
		return nil, err
	}
	maxFee, err := ComputeFee(maxAmountInNoFee, totalFeeRate)
	if err != nil {
		return nil, err
	}
	maxAmountInWithFee, err := addU64(maxAmountInNoFee, maxFee)
	if err != nil {
		return nil, err
	}

	amountInWithFees := amountIn
	amountOut := uint64(0)
	fee := uint64(0)
	protocolFee := uint64(0)

	if amountIn > maxAmountInWithFee {
		amountInWithFees = maxAmountInWithFee
		amountOut = maxAmountOut
		fee = maxFee
		protocolFee, err = ComputeProtocolFee(maxFee, protocolShareBps)
		if err != nil {
			return nil, err
		}
	} else {
		fee, err = ComputeFeeFromAmount(amountIn, totalFeeRate)
		if err != nil {
			return nil, err
		}
		amountInAfterFee, err := subU64(amountIn, fee)
		if err != nil {
			return nil, err
		}
		amountOut, err = GetAmountOutFromAmountIn(amountInAfterFee, price, swapForY)
		if err != nil {
			return nil, err
		}
		amountOut = minU64(amountOut, maxAmountOut)
		protocolFee, err = ComputeProtocolFee(fee, protocolShareBps)
		if err != nil {
			return nil, err
		}
	}

	hostFee := uint64(0)
	if hostFeeBps != nil {
		numerator := uint64ToBig(protocolFee)
		numerator.Mul(numerator, uint64ToBig(uint64(*hostFeeBps)))
		hf, err := common.BigToUint64Checked(new(big.Int).Quo(numerator, uint64ToBig(BasisPointMax)))
		if err != nil {
			return nil, err
		}
		hostFee = hf
	}
	protocolFeeAfterHost, err := subU64(protocolFee, hostFee)
	if err != nil {
		return nil, err
	}
	amountIntoBin, err := subU64(amountInWithFees, fee)
	if err != nil {
		return nil, err
	}

	newReserveX := reserveX
	newReserveY := reserveY
	if swapForY {
		newReserveX, err = addU64(reserveX, amountIntoBin)
		if err != nil {
			return nil, err
		}
		newReserveY, err = subU64(reserveY, amountOut)
		if err != nil {
			return nil, err
		}
	} else {
		newReserveY, err = addU64(reserveY, amountIntoBin)
		if err != nil {
			return nil, err
		}
		newReserveX, err = subU64(reserveX, amountOut)
		if err != nil {
			return nil, err
		}
	}

	return &BinSwapQuote{
		AmountInWithFees:     amountInWithFees,
		AmountOut:            amountOut,
		Fee:                  fee,
		ProtocolFeeAfterHost: protocolFeeAfterHost,
		HostFee:              hostFee,
		AmountIntoBin:        amountIntoBin,
		NewReserveX:          newReserveX,
		NewReserveY:          newReserveY,
	}, nil
}
