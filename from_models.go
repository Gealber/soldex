package soldex

import (
	"errors"
	"fmt"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
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
