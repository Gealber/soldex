package models

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
)

// PumpPoolDiscriminator is sha256("account:Pool")[:8] for the Pump-AMM (pAMMBay)
// program.
var PumpPoolDiscriminator = [8]byte{241, 154, 109, 4, 17, 177, 109, 188}

// PumpPool is a Pump-AMM constant-product pool — where pump.fun tokens live after
// they graduate. Only the fields a swap leg needs are decoded: the base/quote
// mints, the pool's vault token accounts, and the coin creator (which seeds the
// creator-fee vault the swap must pass). The swap needs no bin/tick arrays; the
// reserves are the vault token-account balances (read separately for a quote).
//
// Layout (Borsh, after the 8-byte discriminator): pool_bump u8, index u16,
// creator Pubkey, base_mint Pubkey, quote_mint Pubkey, lp_mint Pubkey,
// pool_base_token_account Pubkey, pool_quote_token_account Pubkey, lp_supply u64,
// coin_creator Pubkey, is_mayhem_mode bool, is_cashback_coin bool, then a u64 at
// offset 245 (see ExtraQuoteReserve).
type PumpPool struct {
	Address               solana.PublicKey
	BaseMint              solana.PublicKey
	QuoteMint             solana.PublicKey
	PoolBaseTokenAccount  solana.PublicKey
	PoolQuoteTokenAccount solana.PublicKey
	CoinCreator           solana.PublicKey
	// Creator is the pool creator (offset 11); when it equals the bonding-curve
	// pool-authority PDA the pool is a pump graduate and uses market-cap fee tiers.
	Creator solana.PublicKey
	// IsCashbackCoin (offset 244) flips the creator fee to the global rate.
	IsCashbackCoin bool
	// VirtualQuoteReserves (i128 at offset 245, named virtual_quote_reserves in the
	// program IDL) is quote-side reserve the program prices with that is NOT held in
	// the quote vault. A quote taken on the vault balance alone therefore reads the
	// pool as shallower than it is and over-predicts what a buy returns — measured
	// 2026-07-25 against simulateTransaction at +5.5% on a 317 SOL pool and +775% on
	// a 2.2 SOL one, since the offset is absolute and the relative error grows as the
	// pool shrinks. Always price through EffectiveQuoteReserve, never the raw vault
	// balance.
	//
	// It is absent (zero) on every cohort below 261 bytes, present on the newest at
	// ~17.58 SOL, near-identical across unrelated pools at any given moment, and it
	// drifts upward over time — neither a constant nor a per-pool invariant, so it
	// must be read live from the account, not cached or assumed.
	//
	// The on-chain type is SIGNED, so the pool can in principle price against LESS
	// than its vault balance. No live pool held a negative value when this was
	// written (4,982 of 4,982 positive on 2026-08-05); the sign is honoured anyway
	// rather than assumed away, because reading it unsigned turns the first negative
	// value into ~1.8e19 lamports of imaginary depth.
	VirtualQuoteReserves int64
}

// EffectiveQuoteReserve is the quote reserve a swap must be quoted against: the pool's
// quote vault balance plus the virtual reserve the program prices with. Mirrors
// RaydiumCPMMPool.NetReserves — the vault balance alone is never the right input.
// A negative virtual reserve nets the pool DOWN; the result saturates at 0 and at
// MaxUint64 rather than wrapping.
func (p *PumpPool) EffectiveQuoteReserve(vaultQuoteBalance uint64) uint64 {
	if p == nil || p.VirtualQuoteReserves == 0 {
		return vaultQuoteBalance
	}
	if p.VirtualQuoteReserves < 0 {
		// uint64(-MinInt64) is 1<<63, which exceeds any real balance and saturates to 0.
		deficit := uint64(-p.VirtualQuoteReserves)
		if deficit > vaultQuoteBalance {
			return 0
		}
		return vaultQuoteBalance - deficit
	}
	virtual := uint64(p.VirtualQuoteReserves)
	if vaultQuoteBalance > math.MaxUint64-virtual {
		return math.MaxUint64
	}
	return vaultQuoteBalance + virtual
}

// Pump-AMM Pool account sizes live on chain (counted 2026-08-05). The layout has been
// appended to three times and EVERY cohort is still live, so a field the account is
// too short to carry decodes as its zero value instead of failing the whole account —
// rejecting short accounts dropped 110,317 of the 213,534 pools on chain.
const (
	// pumpPoolMinLen covers through lp_supply — every mint and vault a swap needs.
	pumpPoolMinLen = 211 // 37,096 pools: no coin_creator
	// pumpPoolCoinCreatorEnd is the offset past coin_creator.
	pumpPoolCoinCreatorEnd = 243 // 73,221 pools: coin_creator, no flags
	// pumpPoolFlagsEnd is the offset past is_mayhem_mode and is_cashback_coin.
	pumpPoolFlagsEnd = 245 // 98,235 pools: flags, no virtual reserve
	// pumpPoolVirtualQuoteEnd is the offset past virtual_quote_reserves (i128).
	pumpPoolVirtualQuoteEnd = 261 // 4,982 pools: the full layout
)

// DecodePumpPool decodes a Pump-AMM Pool account.
func DecodePumpPool(data []byte, address solana.PublicKey) (*PumpPool, error) {
	if len(data) < pumpPoolMinLen {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != PumpPoolDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, disc, PumpPoolDiscriminator)
	}
	pool := &PumpPool{
		Address:               address,
		Creator:               solana.PublicKeyFromBytes(data[11:43]),
		BaseMint:              solana.PublicKeyFromBytes(data[43:75]),
		QuoteMint:             solana.PublicKeyFromBytes(data[75:107]),
		PoolBaseTokenAccount:  solana.PublicKeyFromBytes(data[139:171]),
		PoolQuoteTokenAccount: solana.PublicKeyFromBytes(data[171:203]),
	}
	if len(data) >= pumpPoolCoinCreatorEnd {
		pool.CoinCreator = solana.PublicKeyFromBytes(data[211:243])
	}
	if len(data) >= pumpPoolFlagsEnd {
		pool.IsCashbackCoin = data[244] != 0
	}
	if len(data) >= pumpPoolVirtualQuoteEnd {
		virtual, err := int128AsInt64(data[245:261])
		if err != nil {
			return nil, fmt.Errorf("pump pool %s virtual_quote_reserves: %w", address, err)
		}
		pool.VirtualQuoteReserves = virtual
	}
	return pool, nil
}

// int128AsInt64 reads a little-endian signed 128-bit integer and narrows it to int64.
// A quote reserve cannot legitimately need more than 63 bits — 2^63 lamports is ~9.2
// billion SOL, more than exists — so a value outside the range is reported rather than
// silently truncated to a plausible-looking number.
func int128AsInt64(b []byte) (int64, error) {
	lo := binary.LittleEndian.Uint64(b[0:8])
	hi := int64(binary.LittleEndian.Uint64(b[8:16]))
	if (hi == 0 && lo <= math.MaxInt64) || (hi == -1 && lo > math.MaxInt64) {
		return int64(lo), nil
	}
	return 0, fmt.Errorf("%w: i128 hi=%d lo=%d", ErrValueOutOfRange, hi, lo)
}
