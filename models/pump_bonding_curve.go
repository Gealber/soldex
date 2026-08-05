package models

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// BondingCurveDiscriminator is sha256("account:BondingCurve")[:8] for the pump.fun
// bonding-curve program (6EF8rrec…) — the PRE-graduation curve, distinct from the
// post-graduation PumpPool (Pump-AMM, pAMMBay…).
var BondingCurveDiscriminator = [8]byte{0x17, 0xb7, 0xf8, 0x37, 0x60, 0xd8, 0xac, 0x60}

// BondingCurve is a pump.fun bonding-curve account. Price and swap math run on the
// VIRTUAL reserves as a constant product; the real reserves and Complete flag track
// migration to the Pump-AMM. Only the fields swap/price/creator-routing need are
// decoded; the account has grown twice since and carries a further zeroed tail.
//
// Layout (Borsh, after the 8-byte discriminator): virtual_token_reserves u64,
// virtual_sol_reserves u64, real_token_reserves u64, real_sol_reserves u64,
// token_total_supply u64, complete bool, creator Pubkey, is_mayhem_mode bool,
// is_cashback_coin bool, quote_mint Pubkey.
type BondingCurve struct {
	Address solana.PublicKey
	// VirtualTokenReserves is the base-token side of the constant product (6 dp).
	VirtualTokenReserves uint64
	// VirtualSolReserves is the quote (SOL) side of the constant product (lamports).
	VirtualSolReserves uint64
	// RealTokenReserves is tokens still held by the curve (not yet sold).
	RealTokenReserves uint64
	// RealSolReserves is actual SOL accumulated in the curve (lamports).
	RealSolReserves uint64
	// TokenTotalSupply is the mint's total supply (base units).
	TokenTotalSupply uint64
	// Complete is true once the curve migrated to the Pump-AMM (no longer tradable here).
	Complete bool
	// Creator seeds the creator_vault PDA the buy/sell instruction must pass.
	Creator solana.PublicKey
	// IsMayhemMode and IsCashbackCoin (offsets 81, 82) are absent on the oldest cohort
	// and decode as false there.
	IsMayhemMode   bool
	IsCashbackCoin bool
	// QuoteMint (offset 83) is the mint the curve is priced in — the program added it
	// when curves stopped being SOL-only, and the IDL renamed the reserve fields
	// virtual_sol_reserves -> virtual_quote_reserves to match. The Go names above are
	// kept for compatibility, which makes this field the ONLY thing that says what
	// unit they are in.
	//
	// It is the zero pubkey on curves too old to carry it and on every curve sampled
	// 2026-08-05 (native SOL). A NON-ZERO QuoteMint means VirtualSolReserves and
	// RealSolReserves are NOT lamports but units of that mint, and any caller pricing
	// them as SOL is wrong. Check it before quoting.
	//
	// NOT MEASURED: how many live curves carry a non-zero QuoteMint. The count needs a
	// getProgramAccounts sweep the RPC refused (>10M accounts pre-filter), so treat
	// "all curves are SOL" as unverified rather than established.
	QuoteMint solana.PublicKey
}

// pump.fun BondingCurve account sizes live on chain (counted 2026-08-05). As with
// PumpPool every cohort is still live, so trailing fields decode as their zero value
// rather than failing the account.
const (
	// bondingCurveMinLen is the offset through Creator (8 disc + 5×u64 + bool + 32).
	bondingCurveMinLen = 81 // 415,273 curves
	// bondingCurveFlagsEnd is the offset past is_mayhem_mode and is_cashback_coin.
	bondingCurveFlagsEnd = 83 // 611,945 curves
	// bondingCurveQuoteMintEnd is the offset past quote_mint.
	bondingCurveQuoteMintEnd = 115 // 1,179,160 curves (a further 3,221,680 carry a
	// 35-byte tail that is all zeros today and holds nothing this decoder needs)
)

// DecodeBondingCurve decodes a pump.fun BondingCurve account.
func DecodeBondingCurve(data []byte, address solana.PublicKey) (*BondingCurve, error) {
	if len(data) < bondingCurveMinLen {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != BondingCurveDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, disc, BondingCurveDiscriminator)
	}
	curve := &BondingCurve{
		Address:              address,
		VirtualTokenReserves: binary.LittleEndian.Uint64(data[8:16]),
		VirtualSolReserves:   binary.LittleEndian.Uint64(data[16:24]),
		RealTokenReserves:    binary.LittleEndian.Uint64(data[24:32]),
		RealSolReserves:      binary.LittleEndian.Uint64(data[32:40]),
		TokenTotalSupply:     binary.LittleEndian.Uint64(data[40:48]),
		Complete:             data[48] != 0,
		Creator:              solana.PublicKeyFromBytes(data[49:81]),
	}
	if len(data) >= bondingCurveFlagsEnd {
		curve.IsMayhemMode = data[81] != 0
		curve.IsCashbackCoin = data[82] != 0
	}
	if len(data) >= bondingCurveQuoteMintEnd {
		curve.QuoteMint = solana.PublicKeyFromBytes(data[83:115])
	}
	return curve, nil
}

// IsSOLQuoted reports whether the curve prices in native SOL, i.e. whether its
// VirtualSolReserves and RealSolReserves are lamports. Curves too old to carry a
// QuoteMint predate non-SOL quoting and are SOL by construction.
func (c *BondingCurve) IsSOLQuoted() bool {
	return c != nil && c.QuoteMint.IsZero()
}
