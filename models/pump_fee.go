package models

import (
	"encoding/binary"
	"math/big"

	"github.com/gagliardetto/solana-go"
)

// pumpBondingProgram is the pump.fun bonding-curve program. A pool whose creator
// is PDA["pool-authority", base_mint] under it is a graduated pump pool, which
// uses the market-cap fee TIERS (not the flat fees non-pump pools get).
var pumpBondingProgram = solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")

// PumpFees is one fee schedule (basis points).
type PumpFees struct {
	LpBps       uint64
	ProtocolBps uint64
	CreatorBps  uint64
}

// PumpGlobalConfig holds the Pump-AMM global_config fields the fee math needs.
// Fixed-layout account; offsets are post-8-byte-discriminator.
type PumpGlobalConfig struct {
	LpBps          uint64
	ProtocolBps    uint64
	CoinCreatorBps uint64

	// CreatorFeeConfigurable (offset 940) gates whether a pool's own
	// CreatorFeeBps override is honoured at all.
	CreatorFeeConfigurable bool
	// MaxConfigurableCreatorFeeBps (u64 at offset 941) caps that override.
	MaxConfigurableCreatorFeeBps uint64
}

// DecodePumpGlobalConfig reads global_config (ADyA8hde…). Layout: disc(8),
// admin(32)@8, lp_bps@40, protocol_bps@48, disable_flags(1)@56,
// protocol_fee_recipients(8*32)@57, coin_creator_bps@313, and — on the grown
// 949-byte account — creator_fee_configurable@940 and
// max_configurable_creator_fee_bps@941. The trailing pair decodes as zero on an
// account too short to carry it, which reads as "overrides not configurable".
func DecodePumpGlobalConfig(data []byte) (*PumpGlobalConfig, error) {
	if len(data) < 321 {
		return nil, ErrInsufficientData
	}
	cfg := &PumpGlobalConfig{
		LpBps:          binary.LittleEndian.Uint64(data[40:]),
		ProtocolBps:    binary.LittleEndian.Uint64(data[48:]),
		CoinCreatorBps: binary.LittleEndian.Uint64(data[313:]),
	}
	if len(data) >= pumpGlobalConfigCreatorFeeEnd {
		cfg.CreatorFeeConfigurable = data[940] != 0
		cfg.MaxConfigurableCreatorFeeBps = binary.LittleEndian.Uint64(data[941:949])
	}
	return cfg, nil
}

// pumpGlobalConfigCreatorFeeEnd is the offset past max_configurable_creator_fee_bps.
const pumpGlobalConfigCreatorFeeEnd = 949

// PumpFeeTier maps a market-cap threshold (lamports) to a fee schedule.
type PumpFeeTier struct {
	MarketCapThreshold *big.Int
	Fees               PumpFees
}

// PumpFeeConfig holds the fee_config account: flat fees (used by non-pump pools)
// and the market-cap tiers (used by pump graduates).
type PumpFeeConfig struct {
	Flat  PumpFees
	Tiers []PumpFeeTier

	// StableTiers is the market-cap schedule for a pool quoted in a STABLECOIN
	// rather than SOL. It is a different schedule, not a rescaling — its
	// thresholds are roughly an order of magnitude lower.
	StableTiers []PumpFeeTier
	// ExoticFlat is the flat schedule for a pool quoted in anything that is
	// neither SOL nor a stablecoin.
	ExoticFlat PumpFees
}

// DecodePumpFeeConfig reads fee_config (under pfeeUxB…). Layout: disc(8), bump(1)@8,
// admin(32)@9, flat_fees(3*u64=24)@41, fee_tiers Vec: len u32@65, then entries of
// {market_cap_threshold u128(16), fees 3*u64(24)} = 40 bytes each at 69+i*40.
//
// The account has since grown to 4097 bytes and carries two more schedules:
// stable_fee_tiers as a second Vec at 1069 (same entry layout) and
// exotic_flat_fees at 2073. Which of the three a swap uses is decided by the
// pool's QUOTE MINT — see PumpTotalFeeBps.
func DecodePumpFeeConfig(data []byte) (*PumpFeeConfig, error) {
	if len(data) < 69 {
		return nil, ErrInsufficientData
	}
	cfg := &PumpFeeConfig{Flat: decodePumpFees(data, 41)}
	cfg.Tiers = decodePumpTierVec(data, 65)
	cfg.StableTiers = decodePumpTierVec(data, pumpStableTiersOffset)
	if len(data) >= pumpExoticFlatOffset+24 {
		cfg.ExoticFlat = decodePumpFees(data, pumpExoticFlatOffset)
	}
	return cfg, nil
}

const (
	// pumpStableTiersOffset is the stable_fee_tiers Vec length prefix.
	pumpStableTiersOffset = 1069
	// pumpExoticFlatOffset is the exotic_flat_fees triple.
	pumpExoticFlatOffset = 2073
	// pumpTierEntryLen is {market_cap_threshold u128, fees 3*u64}.
	pumpTierEntryLen = 40
	// pumpMaxTiers bounds a length prefix read out of an account that is shorter
	// or differently shaped than expected, so a bad read cannot allocate wildly.
	pumpMaxTiers = 64
)

// decodePumpTierVec reads a Vec<FeeTier> whose u32 length prefix sits at off.
func decodePumpTierVec(data []byte, off int) []PumpFeeTier {
	if len(data) < off+4 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(data[off : off+4]))
	if n > pumpMaxTiers {
		n = pumpMaxTiers
	}
	var tiers []PumpFeeTier
	for i := range n {
		o := off + 4 + i*pumpTierEntryLen
		if o+pumpTierEntryLen > len(data) {
			break
		}
		thr := new(big.Int).SetBytes(reverse(data[o : o+16])) // u128 little-endian
		tiers = append(tiers, PumpFeeTier{MarketCapThreshold: thr, Fees: decodePumpFees(data, o+16)})
	}
	return tiers
}

// decodePumpFees reads a PumpFees (lp/protocol/creator bps, 3×u64) at offset o.
func decodePumpFees(data []byte, o int) PumpFees {
	return PumpFees{
		LpBps:       binary.LittleEndian.Uint64(data[o : o+8]),
		ProtocolBps: binary.LittleEndian.Uint64(data[o+8 : o+16]),
		CreatorBps:  binary.LittleEndian.Uint64(data[o+16 : o+24]),
	}
}

// PumpMarketCap = quote_reserve * base_mint_supply / base_reserve (lamports).
//
// baseSupply is the caller's to supply, deliberately: the pump SDK implies a
// mayhem-mode coin is valued against a FIXED 1e15 supply rather than its mint
// supply, but that has never been confirmed against the program, so this package
// will not bake the constant in. A caller that needs mayhem pricing should verify
// it on chain first — see BondingCurve.IsMayhemMode and PumpPool flags.
func PumpMarketCap(quoteReserve, baseReserve, baseSupply uint64) *big.Int {
	if baseReserve == 0 {
		return big.NewInt(0)
	}
	out := new(big.Int).SetUint64(quoteReserve)
	out.Mul(out, new(big.Int).SetUint64(baseSupply))
	out.Div(out, new(big.Int).SetUint64(baseReserve))
	return out
}

// IsPumpBondingPool reports whether creator == PDA["pool-authority", base_mint]
// under the bonding-curve program — i.e. the pool graduated from pump.fun and so
// uses the market-cap fee tiers rather than the flat fees.
func IsPumpBondingPool(baseMint, creator solana.PublicKey) bool {
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("pool-authority"), baseMint.Bytes()}, pumpBondingProgram)
	return err == nil && pda.Equals(creator)
}

// Quote mints that decide which fee schedule a Pump-AMM pool uses.
var (
	pumpWSOLMint = solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	pumpUSDCMint = solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	pumpUSDTMint = solana.MustPublicKeyFromBase58("Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB")
)

// PumpQuoteClass is the quote-mint class that selects a fee schedule.
type PumpQuoteClass uint8

const (
	// PumpQuoteSOL is a WSOL-quoted pool: the standard market-cap tiers.
	PumpQuoteSOL PumpQuoteClass = iota
	// PumpQuoteStable is a stablecoin-quoted pool: the stable market-cap tiers,
	// whose thresholds are denominated in the stablecoin, not in lamports.
	PumpQuoteStable
	// PumpQuoteExotic is anything else.
	PumpQuoteExotic
)

// ClassifyPumpQuoteMint maps a pool's quote mint to its fee schedule class.
//
// The stablecoin set is not readable from chain: neither fee_config nor
// global_config carries a mint list, so the program holds it internally. USDC and
// USDT are used here because they are the only recognisable stablecoins with a
// meaningful pool count.
//
// Misclassifying an exotic mint is harmless for a QUOTE, since exotic_flat_fees
// and flat_fees both total 30 bps and only the total reaches the output.
// Misclassifying a stablecoin picks the wrong market-cap tier.
func ClassifyPumpQuoteMint(mint solana.PublicKey) PumpQuoteClass {
	switch {
	case mint.IsZero(), mint.Equals(pumpWSOLMint):
		// A decoded pool always has a quote mint (offset 75 is inside even the
		// shortest cohort), so a zero one means a caller left it unset. Read it as
		// SOL: it matches BondingCurve.IsSOLQuoted and is the conservative
		// direction, since exotic charges 30 bps against the tiers' 125 and
		// under-charging a fee over-states the output.
		return PumpQuoteSOL
	case mint.Equals(pumpUSDCMint), mint.Equals(pumpUSDTMint):
		return PumpQuoteStable
	default:
		return PumpQuoteExotic
	}
}

// PumpTotalFeeBps is the total swap fee (lp + protocol + creator) a Pump-AMM pool
// charges, mirroring the on-chain GetFees the swap CPIs.
//
// The schedule is chosen by GRADUATE STATUS and QUOTE MINT together, confirmed
// against live Buy/SellEvent logs.
//
//   - not a graduate          -> flat_fees
//   - graduate, SOL quote     -> fee_tiers by market cap
//   - graduate, stable quote  -> stable_fee_tiers (different thresholds entirely:
//     59e9 at tier 1 where the SOL schedule has 420e9)
//   - graduate, exotic quote  -> exotic_flat_fees, falling back to flat_fees
//
// The creator component is REPLACED, not topped up, by a pool's own CreatorFeeBps
// when the global config allows overrides; it is capped by the global maximum. A
// holder-reward pool REDIRECTS that same creator fee to token holders rather than
// charging anything extra, so it must not change the total.
//
// A cashback coin likewise still DEDUCTS the full creator fee in the swap — the
// cashback is rebated separately and claimable, so it does not raise the output.
func PumpTotalFeeBps(g *PumpGlobalConfig, fc *PumpFeeConfig, pool *PumpPool, baseReserve, quoteReserve, baseSupply uint64) uint64 {
	var f PumpFees
	switch {
	case fc == nil:
		// No fee_config: fall back to the global rates.
		f = PumpFees{LpBps: g.LpBps, ProtocolBps: g.ProtocolBps, CreatorBps: g.CoinCreatorBps}
	case IsPumpBondingPool(pool.BaseMint, pool.Creator):
		switch ClassifyPumpQuoteMint(pool.QuoteMint) {
		case PumpQuoteStable:
			tiers := fc.StableTiers
			if len(tiers) == 0 {
				tiers = fc.Tiers
			}
			f = tierFees(tiers, PumpMarketCap(quoteReserve, baseReserve, baseSupply))
		case PumpQuoteExotic:
			f = fc.ExoticFlat
			if f == (PumpFees{}) {
				f = fc.Flat
			}
		default:
			f = tierFees(fc.Tiers, PumpMarketCap(quoteReserve, baseReserve, baseSupply))
		}
	default:
		f = fc.Flat
	}

	total := f.LpBps + f.ProtocolBps
	total += pumpCreatorFeeBps(g, f, pool)
	return total
}

// pumpCreatorFeeBps returns the creator component of the swap fee. A per-pool
// override replaces the schedule's creator fee outright; otherwise the schedule
// applies only when the pool actually has a coin creator to pay.
func pumpCreatorFeeBps(g *PumpGlobalConfig, f PumpFees, pool *PumpPool) uint64 {
	if pool.CreatorFeeBps > 0 && g != nil && g.CreatorFeeConfigurable {
		override := pool.CreatorFeeBps
		if max := g.MaxConfigurableCreatorFeeBps; max > 0 && override > max {
			override = max
		}
		return override
	}
	if pool.CoinCreator.IsZero() {
		return 0
	}
	return f.CreatorBps
}

// tierFees selects the schedule for a market cap: the highest tier whose threshold
// the cap meets, or the first tier when the cap is below all of them.
func tierFees(tiers []PumpFeeTier, marketCap *big.Int) PumpFees {
	if len(tiers) == 0 {
		return PumpFees{}
	}
	if marketCap.Cmp(tiers[0].MarketCapThreshold) < 0 {
		return tiers[0].Fees
	}
	for i := len(tiers) - 1; i >= 0; i-- {
		if marketCap.Cmp(tiers[i].MarketCapThreshold) >= 0 {
			return tiers[i].Fees
		}
	}
	return tiers[0].Fees
}

// reverse returns a big-endian copy of a little-endian byte slice (for u128).
func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}
