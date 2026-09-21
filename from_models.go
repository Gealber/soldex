package soldex

import (
	"errors"
	"fmt"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/orca"
	soldexray "github.com/Gealber/soldex/quote/raydium"
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

// FromWhirlpool builds a Quoter for an Orca Whirlpool.
//
// oracle is the pool's Oracle account, or nil for a plain static-fee pool — only
// adaptive-fee pools have one. Passing nil for a pool that HAS an oracle quotes
// it without the volatility surcharge, so fetch it before deciding.
//
// now is the current unix time, used both to decay the adaptive-fee reference
// and to check the pool has opened. A pool whose oracle gates trading until a
// future timestamp is refused.
func FromWhirlpool(
	pool *models.Whirlpool, oracle *models.WhirlpoolOracle,
	ticks orca.TickProvider, now uint64,
) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil whirlpool", ErrPoolNotQuotable)
	}
	if !oracle.TradableAt(now) {
		return nil, fmt.Errorf("%w: whirlpool opens for trading at %d", ErrPoolNotQuotable, oracle.TradeEnableTimestamp)
	}

	sp := orca.SwapPool{
		SqrtPrice:        pool.SqrtPrice.BigInt(),
		Liquidity:        pool.Liquidity.BigInt(),
		TickCurrentIndex: pool.TickCurrentIndex,
		TickSpacing:      pool.TickSpacing,
		FeeRate:          pool.FeeRate,
		Timestamp:        now,
	}
	if oracle != nil {
		sp.AdaptiveFee = &orca.AdaptiveFeeInfo{
			Constants: orca.AdaptiveFeeConstants{
				FilterPeriod:             oracle.FilterPeriod,
				DecayPeriod:              oracle.DecayPeriod,
				ReductionFactor:          oracle.ReductionFactor,
				AdaptiveFeeControlFactor: oracle.AdaptiveFeeControlFactor,
				MaxVolatilityAccumulator: oracle.MaxVolatilityAccumulator,
				TickGroupSize:            oracle.TickGroupSize,
				MajorSwapThresholdTicks:  oracle.MajorSwapThresholdTicks,
			},
			Variables: orca.AdaptiveFeeVariables{
				LastReferenceUpdateTimestamp: oracle.LastReferenceUpdateTimestamp,
				LastMajorSwapTimestamp:       oracle.LastMajorSwapTimestamp,
				VolatilityReference:          oracle.VolatilityReference,
				TickGroupIndexReference:      oracle.TickGroupIndexReference,
				VolatilityAccumulator:        oracle.VolatilityAccumulator,
			},
		}
	}
	return Orca(sp, ticks), nil
}

// FromRaydiumCLMM builds a Quoter for a Raydium CLMM pool.
//
// cfg is the linked AmmConfig, which is where the trade fee rate lives — the
// pool alone cannot be quoted. blockTime is the current unix time; the dynamic
// fee decays against it, so a stale one over-states the fee.
//
// This is the constructor that most needs to exist. The deployed program fills
// limit orders, adds a volatility surcharge on top of the config rate, and can
// take the fee out of the OUTPUT. A caller who fills SwapPool by hand and misses
// FeeOn, Status or DynamicFee gets no error at all — just a quote of the program
// as it behaved before those landed.
func FromRaydiumCLMM(
	pool *models.RaydiumCLMMPool, cfg *models.RaydiumAmmConfig,
	ticks soldexray.TickProvider, blockTime uint64,
) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil Raydium CLMM pool", ErrPoolNotQuotable)
	}
	if cfg == nil {
		return nil, fmt.Errorf("%w: Raydium CLMM needs its AmmConfig for the trade fee rate", ErrPoolNotQuotable)
	}
	if pool.SwapDisabled() {
		return nil, fmt.Errorf("%w: Raydium CLMM pool has swaps disabled", ErrPoolNotQuotable)
	}
	return Raydium(soldexray.SwapPool{
		SqrtPrice:      pool.SqrtPriceX64.BigInt(),
		Liquidity:      pool.Liquidity.BigInt(),
		TickCurrent:    pool.TickCurrent,
		TickSpacing:    pool.TickSpacing,
		FeeRate:        cfg.TradeFeeRate,
		FeeOn:          pool.FeeOn,
		Status:         pool.Status,
		DynamicFee:     pool.DynamicFee,
		BlockTimestamp: blockTime,
	}, ticks), nil
}

// FromRaydiumCPMM builds a Quoter for a Raydium CP-Swap constant-product pool.
//
// The two vault token-account balances are the caller's to fetch; soldex nets
// them down by the protocol, fund and creator fees the pool tracks, which sit in
// the vaults but are not swappable.
//
// cfg is the linked AmmConfig. The fee charged is its trade rate PLUS the pool's
// creator fee where the pool enables one, so quoting on the trade rate alone
// under-charges and over-states the output.
func FromRaydiumCPMM(
	pool *models.RaydiumCPMMPool, cfg *models.RaydiumCPMMConfig,
	vault0Balance, vault1Balance uint64,
) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil CP-Swap pool", ErrPoolNotQuotable)
	}
	if cfg == nil {
		return nil, fmt.Errorf("%w: CP-Swap needs its AmmConfig for the trade fee rate", ErrPoolNotQuotable)
	}
	if pool.SwapDisabled() {
		return nil, fmt.Errorf("%w: CP-Swap pool has swaps disabled", ErrPoolNotQuotable)
	}
	reserve0, reserve1 := pool.NetReserves(vault0Balance, vault1Balance)
	if reserve0 == 0 || reserve1 == 0 {
		return nil, fmt.Errorf("%w: CP-Swap pool has an empty side", ErrPoolNotQuotable)
	}
	return RaydiumCPMM(reserve0, reserve1, cfg.TradeFeeRate+pool.EffectiveCreatorFeeRate(cfg)), nil
}

// FromPumpPool builds a Quoter for a Pump-AMM pool.
//
// baseVaultBalance and quoteVaultBalance are the pool's two vault token-account
// balances. The quote side is netted through EffectiveQuoteReserve, because
// newer pools price against quote reserve held OUTSIDE the vault: quoting on the
// raw balance reads the pool as shallower than it is and over-predicts a buy.
//
// baseSupply is the base mint's supply, used for the market-cap fee tier. It is
// the caller's to supply — soldex will not assume a supply for a mayhem coin,
// since the fixed value the SDK implies has never been confirmed on chain.
//
// The fee is derived from the global config, the fee config and the pool
// together: the schedule depends on graduate status and quote mint, and a pool
// carrying its own creator fee replaces the schedule's creator component.
func FromPumpPool(
	pool *models.PumpPool, global *models.PumpGlobalConfig, feeCfg *models.PumpFeeConfig,
	baseVaultBalance, quoteVaultBalance, baseSupply uint64,
) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil Pump pool", ErrPoolNotQuotable)
	}
	if global == nil {
		return nil, fmt.Errorf("%w: Pump needs the global config for its fee rates", ErrPoolNotQuotable)
	}
	quoteReserve := pool.EffectiveQuoteReserve(quoteVaultBalance)
	if baseVaultBalance == 0 || quoteReserve == 0 {
		return nil, fmt.Errorf("%w: Pump pool has an empty side", ErrPoolNotQuotable)
	}
	feeBps := models.PumpTotalFeeBps(global, feeCfg, pool, baseVaultBalance, quoteReserve, baseSupply)
	return Pump(baseVaultBalance, quoteReserve, feeBps), nil
}

// FromBondingCurve builds a Quoter for a pump.fun bonding curve — the
// PRE-graduation curve, not the Pump-AMM pool a graduated token trades on.
//
// Unlike the pool constructors this one cannot derive the fee: the curve's fee
// schedule is not modelled here, so feeBps stays the caller's. What it does do
// is refuse the two curves that must not be quoted at all:
//
//   - a COMPLETE curve has migrated to the Pump-AMM and no longer trades here,
//     so its reserves describe a market that has moved on;
//   - a curve with a non-zero QuoteMint is NOT priced in lamports, and its
//     reserve fields are named for SOL, so quoting it as SOL is a unit error
//     that nothing else in the type system catches.
func FromBondingCurve(curve *models.BondingCurve, feeBps uint64) (Quoter, error) {
	if curve == nil {
		return nil, fmt.Errorf("%w: nil bonding curve", ErrPoolNotQuotable)
	}
	if curve.Complete {
		return nil, fmt.Errorf("%w: bonding curve has migrated to the Pump-AMM", ErrPoolNotQuotable)
	}
	if !curve.IsSOLQuoted() {
		return nil, fmt.Errorf("%w: bonding curve is quoted in %s, not SOL", ErrPoolNotQuotable, curve.QuoteMint)
	}
	if curve.VirtualTokenReserves == 0 || curve.VirtualSolReserves == 0 {
		return nil, fmt.Errorf("%w: bonding curve has an empty virtual reserve", ErrPoolNotQuotable)
	}
	return PumpBondingCurve(curve.VirtualTokenReserves, curve.VirtualSolReserves, feeBps), nil
}
