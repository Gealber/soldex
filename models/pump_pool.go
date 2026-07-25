package models

import (
	"encoding/binary"
	"fmt"

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
	// ExtraQuoteReserve (u64 at offset 245) is quote-side reserve the program prices
	// with that is NOT held in the quote vault. A quote taken on the vault balance
	// alone therefore reads the pool as shallower than it is and over-predicts what a
	// buy returns — measured 2026-07-25 against simulateTransaction at +5.5% on a
	// 317 SOL pool and +775% on a 2.2 SOL one, since the offset is absolute and the
	// relative error grows as the pool shrinks. Always price through
	// EffectiveQuoteReserve, never the raw vault balance.
	//
	// Its provenance is NOT established: it is absent (zero) on older pools, present
	// on newer ones at ~17.58 SOL, near-identical across unrelated pools at any given
	// moment, and it drifts upward over time. Because it is neither a constant nor a
	// per-pool invariant it must be read live from the account, not cached or assumed.
	ExtraQuoteReserve uint64
}

// EffectiveQuoteReserve is the quote reserve a swap must be quoted against: the pool's
// quote vault balance plus the offset the program prices with. Mirrors
// RaydiumCPMMPool.NetReserves — the vault balance alone is never the right input.
func (p *PumpPool) EffectiveQuoteReserve(vaultQuoteBalance uint64) uint64 {
	if p == nil {
		return vaultQuoteBalance
	}
	return vaultQuoteBalance + p.ExtraQuoteReserve
}

// DecodePumpPool decodes a Pump-AMM Pool account.
func DecodePumpPool(data []byte, address solana.PublicKey) (*PumpPool, error) {
	// Through is_cashback_coin (offset 244): coin_creator@211, is_mayhem@243,
	// is_cashback_coin@244 — all the swap and fee math need.
	if len(data) < 245 {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != PumpPoolDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, disc, PumpPoolDiscriminator)
	}
	// The offset postdates the original layout, so an account too short to carry it is
	// decoded as zero rather than rejected — that is exactly how the older cohort reads,
	// and those pools quote correctly on the vault balance alone.
	var extraQuote uint64
	if len(data) >= 253 {
		extraQuote = binary.LittleEndian.Uint64(data[245:253])
	}
	return &PumpPool{
		Address:               address,
		BaseMint:              solana.PublicKeyFromBytes(data[43:75]),
		QuoteMint:             solana.PublicKeyFromBytes(data[75:107]),
		PoolBaseTokenAccount:  solana.PublicKeyFromBytes(data[139:171]),
		PoolQuoteTokenAccount: solana.PublicKeyFromBytes(data[171:203]),
		CoinCreator:           solana.PublicKeyFromBytes(data[211:243]),
		Creator:               solana.PublicKeyFromBytes(data[11:43]),
		IsCashbackCoin:        data[244] != 0,
		ExtraQuoteReserve:     extraQuote,
	}, nil
}
