package models

import (
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
// coin_creator Pubkey, is_mayhem_mode bool, is_cashback_coin bool.
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
	return &PumpPool{
		Address:               address,
		BaseMint:              solana.PublicKeyFromBytes(data[43:75]),
		QuoteMint:             solana.PublicKeyFromBytes(data[75:107]),
		PoolBaseTokenAccount:  solana.PublicKeyFromBytes(data[139:171]),
		PoolQuoteTokenAccount: solana.PublicKeyFromBytes(data[171:203]),
		CoinCreator:           solana.PublicKeyFromBytes(data[211:243]),
		Creator:               solana.PublicKeyFromBytes(data[11:43]),
		IsCashbackCoin:        data[244] != 0,
	}, nil
}
