package soldex

import (
	"errors"
	"fmt"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
)

// The From* constructors build a Quoter straight from a decoded model, so a
// caller never hand-maps a pool into a per-venue quote struct. That mapping is
// where fees get silently left at zero: the quote structs accept a zero fee
// without complaint, and a zero fee over-states every output.
//
// Each one takes the model plus only what this package cannot derive — bin and
// tick providers, vault balances, the current time or point, linked config
// accounts — derives every fee itself, and returns an error rather than a guess
// when the pool cannot be quoted. None of them perform I/O; fetching stays the
// caller's job.

// ErrPoolNotQuotable is returned when a pool cannot be quoted at all, as opposed
// to quoting to zero.
var ErrPoolNotQuotable = errors.New("soldex: pool is not quotable")

// FromDAMMPool builds a Quoter for a Meteora DAMM v2 pool.
//
// currentPoint must be in the pool's own activation unit — a slot when
// ActivationType is 0, a unix timestamp when it is 1 — because it resolves the
// base fee schedule. Passing the wrong unit silently picks the wrong period, so
// this is the one argument worth double-checking.
//
// The fee is derived from the pool, not supplied: a scheduled pool charges
// neither its cliff nor zero, and both of those are easy to pass by accident.
// A CollectFeeMode 2 pool is routed to the compounding quote rather than
// refused, so callers get one door for all three fee modes.
func FromDAMMPool(pool *models.DAMMPool, currentPoint uint64) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil DAMM pool", ErrPoolNotQuotable)
	}
	feeNumerator, err := pool.CurrentBaseFeeNumerator(currentPoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPoolNotQuotable, err)
	}

	fees := pool.PoolFees
	if pool.CollectFeeMode == damm.CollectFeeModeCompounding {
		return DAMMCompounding(
			pool.TokenAAmount, pool.TokenBAmount, feeNumerator,
			false, false,
			fees.ProtocolFeePercent, fees.CompoundingFeeBps, fees.ReferralFeePercent,
		), nil
	}

	return DAMMConcentrated(damm.ConcentratedPool{
		SqrtPrice:    pool.SqrtPrice.BigInt(),
		SqrtMinPrice: pool.SqrtMinPrice.BigInt(),
		SqrtMaxPrice: pool.SqrtMaxPrice.BigInt(),
		Liquidity:    pool.Liquidity.BigInt(),

		CollectFeeMode:   pool.CollectFeeMode,
		FeeVersion:       pool.FeeVersion,
		BaseFeeNumerator: feeNumerator,

		DynamicFeeInitialized: fees.DynamicFee.Initialized != 0,
		VolatilityAccumulator: fees.DynamicFee.VolatilityAccumulator.BigInt(),
		BinStep:               fees.DynamicFee.BinStep,
		VariableFeeControl:    fees.DynamicFee.VariableFeeControl,
	}), nil
}

// FromDLMMPool builds a Quoter for a Meteora DLMM pool.
//
// currentTimestamp is the swap's block time; the variable fee decays against it,
// so a stale one over-states the fee. bins is the cached bin-array window the
// quote walks — it stops at the edge of what the provider knows, so a window too
// narrow silently truncates a large swap.
//
// A pair whose Status is not Enabled is refused: it cannot trade, and a number
// for a swap that would revert is worse than no number.
func FromDLMMPool(pool *models.DLMMPool, currentTimestamp int64, bins dlmm.BinProvider) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil DLMM pool", ErrPoolNotQuotable)
	}
	if pool.Status != dlmmPairStatusEnabled {
		return nil, fmt.Errorf("%w: DLMM pair status %d", ErrPoolNotQuotable, pool.Status)
	}
	sp := pool.Parameters
	vp := pool.VParameters
	return DLMM(dlmm.SwapPool{
		ActiveID: pool.ActiveID,
		BinStep:  pool.BinStep,

		BaseFactor:               sp.BaseFactor,
		BaseFeePowerFactor:       sp.BaseFeePowerFactor,
		VariableFeeControl:       sp.VariableFeeControl,
		MaxVolatilityAccumulator: sp.MaxVolatilityAccumulator,
		FilterPeriod:             sp.FilterPeriod,
		DecayPeriod:              sp.DecayPeriod,
		ReductionFactor:          sp.ReductionFactor,
		CollectFeeMode:           sp.CollectFeeMode,

		VolatilityAccumulator: vp.VolatilityAccumulator,
		VolatilityReference:   vp.VolatilityReference,
		IndexReference:        vp.IndexReference,
		LastUpdateTimestamp:   vp.LastUpdateTimestamp,
	}, currentTimestamp, bins), nil
}

// dlmmPairStatusEnabled is lb_clmm's PairStatus::Enabled.
const dlmmPairStatusEnabled uint8 = 0
