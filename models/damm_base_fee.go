package models

import (
	"encoding/binary"
	"errors"
)

// DAMM v2 base fee modes. The mode byte at offset 8 of the 32-byte BaseFeeInfo
// blob selects which of three mutually exclusive layouts the rest carries.
const (
	// DAMMBaseFeeModeTimeLinear reduces the fee by a flat amount each period.
	DAMMBaseFeeModeTimeLinear uint8 = 0
	// DAMMBaseFeeModeTimeExponential reduces it by a constant factor each period.
	DAMMBaseFeeModeTimeExponential uint8 = 1
	// DAMMBaseFeeModeRateLimiter raises the fee ABOVE the cliff on large swaps.
	DAMMBaseFeeModeRateLimiter uint8 = 2
	// DAMMBaseFeeModeMcapLinear steps the fee down as the price rises.
	//
	// Its STEPPING FORMULA IS NOT ESTABLISHED: the layout is verified but how a
	// sqrt_price rise converts to a period is not, and the IDL does not document
	// it. Both a geometric and a linear reading were tested against real swaps and
	// both were falsified — one pool pays below its own schedule floor, which no
	// period model produces. Do not ship a formula until that is explained.
	DAMMBaseFeeModeMcapLinear uint8 = 3
	// DAMMBaseFeeModeMcapExponential does the same geometrically. Same caveat.
	DAMMBaseFeeModeMcapExponential uint8 = 4
)

// ErrUnsupportedBaseFeeMode is returned for a base fee mode whose schedule this
// package does not yet model. It is deliberately an error rather than a silent
// fallback to the cliff fee: the cliff is the STARTING fee, so returning it for a
// pool that has since scheduled down over-states the fee (and for a rate-limited
// pool under-states it). A caller that wants the old behaviour must ask for the
// cliff explicitly.
var ErrUnsupportedBaseFeeMode = errors.New("damm: unsupported base fee mode")

// DAMMBaseFee is the parsed 32-byte BaseFeeInfo blob. Only the fields belonging
// to Mode carry meaning; the rest are zero.
//
// Layouts (all exactly 32 bytes, cliff u64@0, mode u8@8, 5 bytes padding@9):
//
//	time scheduler  @14 number_of_period u16, @16 period_frequency u64,             @24 reduction_factor u64
//	rate limiter    @14 fee_increment_bps u16, @16 max_limiter_duration u32, @20 max_fee_bps u32, @24 reference_amount u64
//	mcap scheduler  @14 number_of_period u16, @16 sqrt_price_step_bps u32,   @20 scheduler_expiration_duration u32, @24 reduction_factor u64
type DAMMBaseFee struct {
	// CliffFeeNumerator is the fee at period 0, out of FEE_DENOMINATOR (1e9).
	CliffFeeNumerator uint64
	Mode              uint8

	// Scheduler fields (time and mcap modes).
	NumberOfPeriod  uint16
	ReductionFactor uint64

	// PeriodFrequency is the seconds/slots per period (time modes only).
	PeriodFrequency uint64

	// SqrtPriceStepBps and SchedulerExpirationDuration apply to mcap modes only.
	SqrtPriceStepBps            uint32
	SchedulerExpirationDuration uint32

	// Rate limiter fields.
	FeeIncrementBps    uint16
	MaxLimiterDuration uint32
	MaxFeeBps          uint32
	ReferenceAmount    uint64
}

// ParseDAMMBaseFee decodes the BaseFeeInfo blob according to its mode byte.
func ParseDAMMBaseFee(b [32]uint8) DAMMBaseFee {
	f := DAMMBaseFee{
		CliffFeeNumerator: binary.LittleEndian.Uint64(b[0:8]),
		Mode:              b[8],
	}
	switch f.Mode {
	case DAMMBaseFeeModeRateLimiter:
		f.FeeIncrementBps = binary.LittleEndian.Uint16(b[14:16])
		f.MaxLimiterDuration = binary.LittleEndian.Uint32(b[16:20])
		f.MaxFeeBps = binary.LittleEndian.Uint32(b[20:24])
		f.ReferenceAmount = binary.LittleEndian.Uint64(b[24:32])
	case DAMMBaseFeeModeMcapLinear, DAMMBaseFeeModeMcapExponential:
		f.NumberOfPeriod = binary.LittleEndian.Uint16(b[14:16])
		f.SqrtPriceStepBps = binary.LittleEndian.Uint32(b[16:20])
		f.SchedulerExpirationDuration = binary.LittleEndian.Uint32(b[20:24])
		f.ReductionFactor = binary.LittleEndian.Uint64(b[24:32])
	default: // time scheduler (linear or exponential)
		f.NumberOfPeriod = binary.LittleEndian.Uint16(b[14:16])
		f.PeriodFrequency = binary.LittleEndian.Uint64(b[16:24])
		f.ReductionFactor = binary.LittleEndian.Uint64(b[24:32])
	}
	return f
}

// IsStatic reports whether the schedule never moves the fee off the cliff. A
// time scheduler with no period frequency never advances, which is how a static
// pool is encoded — the large majority of them.
func (f DAMMBaseFee) IsStatic() bool {
	switch f.Mode {
	case DAMMBaseFeeModeTimeLinear, DAMMBaseFeeModeTimeExponential:
		return f.PeriodFrequency == 0 || f.NumberOfPeriod == 0 || f.ReductionFactor == 0
	default:
		return false
	}
}
