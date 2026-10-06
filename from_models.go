package soldex

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/damm"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/orca"
	"github.com/Gealber/soldex/quote/raycpmm"
	soldexray "github.com/Gealber/soldex/quote/raydium"
)

// The From* constructors build a Quoter straight from a decoded model, so a
// caller never hand-maps a pool into a quote struct and leaves a fee at zero.
// They derive every fee, error rather than guess, and perform no I/O.

// ErrPoolNotQuotable is returned when a pool cannot be quoted at all, as opposed
// to quoting to zero.
var ErrPoolNotQuotable = errors.New("soldex: pool is not quotable")

// FromDAMMPool builds a Quoter for a Meteora DAMM v2 pool. CollectFeeMode 2 is
// routed to the compounding quote.
//
// currentPoint must be in the pool's OWN activation unit, slot or timestamp; the
// wrong unit silently resolves the fee schedule to the wrong period.
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

// DLMMSwapPool maps a decoded pool onto the quote package's input, refusing a
// pair that is not Enabled. FromDLMMPool is the usual entry point; this is for
// callers needing dlmm.QuoteExactInDetailed to detect a partial fill.
func DLMMSwapPool(pool *models.DLMMPool) (dlmm.SwapPool, error) {
	if pool == nil {
		return dlmm.SwapPool{}, fmt.Errorf("%w: nil DLMM pool", ErrPoolNotQuotable)
	}
	if pool.Status != dlmmPairStatusEnabled {
		return dlmm.SwapPool{}, fmt.Errorf("%w: DLMM pair status %d", ErrPoolNotQuotable, pool.Status)
	}
	sp := pool.Parameters
	vp := pool.VParameters
	return dlmm.SwapPool{
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
	}, nil
}

// FromDLMMPool builds a Quoter for a Meteora DLMM pool. A pair that is not
// Enabled is refused. A stale currentTimestamp over-states the variable fee, and
// a bin window too narrow silently truncates a large swap.
func FromDLMMPool(pool *models.DLMMPool, currentTimestamp int64, bins dlmm.BinProvider) (Quoter, error) {
	sp, err := DLMMSwapPool(pool)
	if err != nil {
		return nil, err
	}
	return DLMM(sp, currentTimestamp, bins), nil
}

// dlmmPairStatusEnabled is lb_clmm's PairStatus::Enabled.
const dlmmPairStatusEnabled uint8 = 0

// FromWhirlpool builds a Quoter for an Orca Whirlpool. A pool whose oracle gates
// trading until a future timestamp is refused. Pass nil for oracle only on a
// static-fee pool; nil on one that has an oracle drops the volatility surcharge.
func FromWhirlpool(
	pool *models.Whirlpool, oracle *models.WhirlpoolOracle,
	ticks orca.TickProvider, now uint64,
) (Quoter, error) {
	sp, err := WhirlpoolSwapPool(pool, oracle, now)
	if err != nil {
		return nil, err
	}
	return Orca(sp, ticks), nil
}

// FromVortex builds a Quoter for a Valiant Vortex pool, which swaps with the Whirlpool math. It refuses
// what that math was not verified on: a nonzero extension, and an adaptive fee tier.
func FromVortex(pool *models.Vortex, ticks orca.TickProvider, now uint64) (Quoter, error) {
	sp, err := VortexSwapPool(pool, now)
	if err != nil {
		return nil, err
	}
	return Orca(sp, ticks), nil
}

// VortexSwapPool is the orca.SwapPool FromVortex quotes, for callers that need
// orca.QuoteExactInDetailed. The same refusals apply.
func VortexSwapPool(pool *models.Vortex, now uint64) (orca.SwapPool, error) {
	if pool == nil {
		return orca.SwapPool{}, fmt.Errorf("%w: nil vortex", ErrPoolNotQuotable)
	}
	for _, b := range pool.Extension {
		if b != 0 {
			return orca.SwapPool{}, fmt.Errorf("%w: vortex extension is set", ErrPoolNotQuotable)
		}
	}
	// is_initialized_with_adaptive_fee_tier: such a pool charges through an oracle no Vortex pool has.
	if binary.LittleEndian.Uint16(pool.FeeTierIndexSeed[:]) != pool.TickSpacing {
		return orca.SwapPool{}, fmt.Errorf("%w: vortex adaptive fee tier", ErrPoolNotQuotable)
	}
	return WhirlpoolSwapPool(&pool.Whirlpool, nil, now)
}

// WhirlpoolSwapPool is the orca.SwapPool FromWhirlpool quotes, for callers that need
// orca.QuoteExactInDetailed. The same refusals and oracle rules apply.
func WhirlpoolSwapPool(pool *models.Whirlpool, oracle *models.WhirlpoolOracle, now uint64) (orca.SwapPool, error) {
	if pool == nil {
		return orca.SwapPool{}, fmt.Errorf("%w: nil whirlpool", ErrPoolNotQuotable)
	}
	if !oracle.TradableAt(now) {
		return orca.SwapPool{}, fmt.Errorf("%w: whirlpool opens for trading at %d", ErrPoolNotQuotable, oracle.TradeEnableTimestamp)
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
	return sp, nil
}

// FromRaydiumCLMM builds a Quoter for a Raydium CLMM pool. cfg is the linked
// AmmConfig, where the trade fee rate lives; the pool alone cannot be quoted.
// A hand-filled SwapPool missing FeeOn, Status or DynamicFee errors on nothing.
func FromRaydiumCLMM(
	pool *models.RaydiumCLMMPool, cfg *models.RaydiumAmmConfig,
	ticks soldexray.TickProvider, blockTime uint64,
) (Quoter, error) {
	sp, err := RaydiumCLMMSwapPool(pool, cfg, blockTime)
	if err != nil {
		return nil, err
	}
	return Raydium(sp, ticks), nil
}

// RaydiumCLMMSwapPool is the raydium.SwapPool FromRaydiumCLMM quotes, for callers that
// need raydium.QuoteExactInDetailed. The same refusals apply.
func RaydiumCLMMSwapPool(pool *models.RaydiumCLMMPool, cfg *models.RaydiumAmmConfig, blockTime uint64) (soldexray.SwapPool, error) {
	if pool == nil {
		return soldexray.SwapPool{}, fmt.Errorf("%w: nil Raydium CLMM pool", ErrPoolNotQuotable)
	}
	if cfg == nil {
		return soldexray.SwapPool{}, fmt.Errorf("%w: Raydium CLMM needs its AmmConfig for the trade fee rate", ErrPoolNotQuotable)
	}
	if pool.SwapDisabled() {
		return soldexray.SwapPool{}, fmt.Errorf("%w: Raydium CLMM pool has swaps disabled", ErrPoolNotQuotable)
	}
	return soldexray.SwapPool{
		SqrtPrice:      pool.SqrtPriceX64.BigInt(),
		Liquidity:      pool.Liquidity.BigInt(),
		TickCurrent:    pool.TickCurrent,
		TickSpacing:    pool.TickSpacing,
		FeeRate:        cfg.TradeFeeRate,
		FeeOn:          pool.FeeOn,
		Status:         pool.Status,
		DynamicFee:     pool.DynamicFee,
		BlockTimestamp: blockTime,
	}, nil
}

// FromRaydiumCPMM builds a Quoter for a Raydium CP-Swap pool. The vault balances
// are netted down by the fees the pool tracks, which sit in the vaults but are not swappable.
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
	if _, err := raycpmm.CreatorFeeOnInput(pool.CreatorFeeOn, true); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPoolNotQuotable, err)
	}
	reserve0, reserve1 := pool.NetReserves(vault0Balance, vault1Balance)
	if reserve0 == 0 || reserve1 == 0 {
		return nil, fmt.Errorf("%w: CP-Swap pool has an empty side", ErrPoolNotQuotable)
	}
	return RaydiumCPMM(reserve0, reserve1, cfg.TradeFeeRate, pool.EffectiveCreatorFeeRate(cfg), pool.CreatorFeeOn), nil
}

// FromRaydiumAMMV4 builds a Quoter for a Raydium AMM v4 pool on the no-orderbook
// path swap_base_in_v2 takes. now is the unix time a WaitingTrade pool opens against.
func FromRaydiumAMMV4(pool *models.RaydiumAMMV4Pool, coinVaultBalance, pcVaultBalance, now uint64) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil AMM v4 pool", ErrPoolNotQuotable)
	}
	if !pool.CanSwapNoOrderbook(now) {
		return nil, fmt.Errorf("%w: AMM v4 pool status %d does not swap without an orderbook at %d (opens %d)",
			ErrPoolNotQuotable, pool.Status, now, pool.PoolOpenTime)
	}
	if pool.SwapFeeDenominator == 0 || pool.SwapFeeNumerator >= pool.SwapFeeDenominator {
		return nil, fmt.Errorf("%w: AMM v4 swap fee %d/%d", ErrPoolNotQuotable, pool.SwapFeeNumerator, pool.SwapFeeDenominator)
	}
	coinReserve, pcReserve, err := pool.NetReserves(coinVaultBalance, pcVaultBalance)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPoolNotQuotable, err)
	}
	if coinReserve == 0 || pcReserve == 0 {
		return nil, fmt.Errorf("%w: AMM v4 pool has an empty side", ErrPoolNotQuotable)
	}
	return RaydiumAMMV4(coinReserve, pcReserve, pool.SwapFeeNumerator, pool.SwapFeeDenominator), nil
}

// FromPumpPool builds a Quoter for a Pump-AMM pool. The quote side is netted
// through EffectiveQuoteReserve, since newer pools hold reserve outside the
// vault. baseSupply feeds the market-cap fee tier and stays the caller's.
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

// FromBondingCurve builds a Quoter for a pump.fun PRE-graduation curve. feeBps
// stays the caller's, the schedule is not modelled here. A complete curve and a
// non-SOL-quoted one are both refused; the latter is an uncatchable unit error.
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

// FluxBeamSide is one side of a FluxBeam pool: its vault balance and the
// Token-2022 transfer fee its mint charges, nil for a mint that charges nothing.
type FluxBeamSide struct {
	Reserve     uint64
	TransferFee *models.TransferFeeConfig
}

// FromFluxBeamPool builds a Quoter for a FluxBeam pool. Nothing is netted out of
// the vault balances. epoch selects between a mint's staged fee settings; a curve
// this package does not model is refused at quote time, where the error names it.
func FromFluxBeamPool(pool *models.FluxBeamPool, a, b FluxBeamSide, epoch uint64) (Quoter, error) {
	if pool == nil {
		return nil, fmt.Errorf("%w: nil FluxBeam pool", ErrPoolNotQuotable)
	}
	if a.Reserve == 0 || b.Reserve == 0 {
		return nil, fmt.Errorf("%w: FluxBeam pool has an empty side", ErrPoolNotQuotable)
	}

	return FluxBeam(a, b, pool.CurveType, pool.Fees, epoch), nil
}
