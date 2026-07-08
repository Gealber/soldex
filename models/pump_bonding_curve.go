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
// decoded — the on-chain account carries ~70 trailing bytes of newer fields we
// don't use.
//
// Layout (Borsh, after the 8-byte discriminator): virtual_token_reserves u64,
// virtual_sol_reserves u64, real_token_reserves u64, real_sol_reserves u64,
// token_total_supply u64, complete bool, creator Pubkey.
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
}

// bondingCurveMinLen is the offset through Creator (8 disc + 5×u64 + bool + 32).
const bondingCurveMinLen = 81

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
	return &BondingCurve{
		Address:              address,
		VirtualTokenReserves: binary.LittleEndian.Uint64(data[8:16]),
		VirtualSolReserves:   binary.LittleEndian.Uint64(data[16:24]),
		RealTokenReserves:    binary.LittleEndian.Uint64(data[24:32]),
		RealSolReserves:      binary.LittleEndian.Uint64(data[32:40]),
		TokenTotalSupply:     binary.LittleEndian.Uint64(data[40:48]),
		Complete:             data[48] != 0,
		Creator:              solana.PublicKeyFromBytes(data[49:81]),
	}, nil
}
