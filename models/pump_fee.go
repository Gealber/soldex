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
}

// DecodePumpGlobalConfig reads global_config (ADyA8hde…). Layout: disc(8),
// admin(32)@8, lp_bps@40, protocol_bps@48, disable_flags(1)@56,
// protocol_fee_recipients(8*32)@57, coin_creator_bps@313.
func DecodePumpGlobalConfig(data []byte) (*PumpGlobalConfig, error) {
	if len(data) < 321 {
		return nil, ErrInsufficientData
	}
	return &PumpGlobalConfig{
		LpBps:          binary.LittleEndian.Uint64(data[40:]),
		ProtocolBps:    binary.LittleEndian.Uint64(data[48:]),
		CoinCreatorBps: binary.LittleEndian.Uint64(data[313:]),
	}, nil
}

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
}

// DecodePumpFeeConfig reads fee_config (under pfeeUxB…). Layout: disc(8), bump(1)@8,
// admin(32)@9, flat_fees(3*u64=24)@41, fee_tiers Vec: len u32@65, then entries of
// {market_cap_threshold u128(16), fees 3*u64(24)} = 40 bytes each at 69+i*40.
func DecodePumpFeeConfig(data []byte) (*PumpFeeConfig, error) {
	if len(data) < 69 {
		return nil, ErrInsufficientData
	}
	fees := func(o int) PumpFees {
		return PumpFees{
			LpBps:       binary.LittleEndian.Uint64(data[o:]),
			ProtocolBps: binary.LittleEndian.Uint64(data[o+8:]),
			CreatorBps:  binary.LittleEndian.Uint64(data[o+16:]),
		}
	}
	cfg := &PumpFeeConfig{Flat: fees(41)}
	n := int(binary.LittleEndian.Uint32(data[65:]))
	for i := range n {
		o := 69 + i*40
		if o+40 > len(data) {
			break
		}
		thr := new(big.Int).SetBytes(reverse(data[o : o+16])) // u128 little-endian
		cfg.Tiers = append(cfg.Tiers, PumpFeeTier{MarketCapThreshold: thr, Fees: fees(o + 16)})
	}
	return cfg, nil
}

// PumpMarketCap = quote_reserve * base_mint_supply / base_reserve (lamports).
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

// PumpTotalFeeBps is the total swap fee (lp + protocol + creator) a Pump-AMM pool
// charges, mirroring the on-chain GetFees the swap CPIs. For a pump graduate it
// picks the market-cap tier; otherwise the flat fees. The creator component only
// applies when the pool has a coin creator; a cashback coin uses the global rate.
func PumpTotalFeeBps(g *PumpGlobalConfig, fc *PumpFeeConfig, pool *PumpPool, baseReserve, quoteReserve, baseSupply uint64) uint64 {
	var f PumpFees
	switch {
	case fc == nil:
		// No fee_config: fall back to the global rates.
		f = PumpFees{LpBps: g.LpBps, ProtocolBps: g.ProtocolBps, CreatorBps: g.CoinCreatorBps}
	case IsPumpBondingPool(pool.BaseMint, pool.Creator):
		// Market-cap tier. NOTE: a cashback coin still DEDUCTS the full tier fee in
		// the swap (on-chain GetFees returns the tier creator fee); the cashback is
		// rebated separately (claimable), so it does NOT lower the swap output —
		// we must NOT override the creator fee here or we phantom-over-quote.
		f = tierFees(fc.Tiers, PumpMarketCap(quoteReserve, baseReserve, baseSupply))
	default:
		f = fc.Flat
	}
	total := f.LpBps + f.ProtocolBps
	if !pool.CoinCreator.IsZero() {
		total += f.CreatorBps
	}
	return total
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
