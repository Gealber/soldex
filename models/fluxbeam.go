package models

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// FluxBeamProgramID owns the pools this file decodes.
const FluxBeamProgramID = "FLUXubRmkEi2q6K3Y9kBPg9248ggaZVsoSFhtJHSrm1X"

// FluxBeam curve types, from the program's CurveType enum.
const (
	FluxBeamCurveConstantProduct uint8 = 0
	FluxBeamCurveConstantPrice   uint8 = 1
	FluxBeamCurveOffset          uint8 = 2
)

// fluxBeamAccountLen is a version byte followed by a packed SwapV1 (323 bytes).
// This is an SPL token-swap fork, so there is no 8-byte Anchor discriminator —
// the version byte and is_initialized flag are what identify the account.
const fluxBeamAccountLen = 324

// FluxBeamFees is the pool's four fee pairs, each a numerator over its own
// denominator rather than a shared one.
//
// Only TradeFee and OwnerTradeFee come off a swap. OwnerWithdrawFee applies to
// withdrawals and HostFee is carved out of the owner fee for a referrer, so
// neither changes what a swap returns.
type FluxBeamFees struct {
	TradeFeeNumerator           uint64
	TradeFeeDenominator         uint64
	OwnerTradeFeeNumerator      uint64
	OwnerTradeFeeDenominator    uint64
	OwnerWithdrawFeeNumerator   uint64
	OwnerWithdrawFeeDenominator uint64
	HostFeeNumerator            uint64
	HostFeeDenominator          uint64
}

// FluxBeamPool is a FluxBeam swap pool. The reserves are the two vault token
// account balances, read separately.
//
// Layout, verified against 809,369 live accounts: version u8@0,
// is_initialized u8@1, bump_seed u8@2, token_program@3, token_a (vault)@35,
// token_b (vault)@67, pool_mint@99, token_a_mint@131, token_b_mint@163,
// pool_fee_account@195, fees (8 x u64)@227, curve_type u8@291,
// calculator@292.
type FluxBeamPool struct {
	Address solana.PublicKey

	TokenProgram solana.PublicKey
	VaultA       solana.PublicKey
	VaultB       solana.PublicKey
	PoolMint     solana.PublicKey
	MintA        solana.PublicKey
	MintB        solana.PublicKey

	Fees FluxBeamFees

	// CurveType selects the pricing curve. All but a handful of live pools are
	// constant product.
	CurveType uint8
	// Calculator is the curve's packed parameters, meaningful only for the
	// non-constant-product curves.
	Calculator [32]uint8
}

// DecodeFluxBeamPool decodes a FluxBeam SwapV1 account.
func DecodeFluxBeamPool(data []byte, address solana.PublicKey) (*FluxBeamPool, error) {
	if len(data) < fluxBeamAccountLen {
		return nil, ErrInsufficientData
	}
	if data[0] != 1 {
		return nil, fmt.Errorf("%w: fluxbeam swap version %d", ErrInvalidDiscriminator, data[0])
	}
	if data[1] != 1 {
		return nil, fmt.Errorf("%w: fluxbeam pool is not initialized", ErrInvalidDiscriminator)
	}

	pool := &FluxBeamPool{
		Address:      address,
		TokenProgram: solana.PublicKeyFromBytes(data[3:35]),
		VaultA:       solana.PublicKeyFromBytes(data[35:67]),
		VaultB:       solana.PublicKeyFromBytes(data[67:99]),
		PoolMint:     solana.PublicKeyFromBytes(data[99:131]),
		MintA:        solana.PublicKeyFromBytes(data[131:163]),
		MintB:        solana.PublicKeyFromBytes(data[163:195]),
		Fees: FluxBeamFees{
			TradeFeeNumerator:           binary.LittleEndian.Uint64(data[227:235]),
			TradeFeeDenominator:         binary.LittleEndian.Uint64(data[235:243]),
			OwnerTradeFeeNumerator:      binary.LittleEndian.Uint64(data[243:251]),
			OwnerTradeFeeDenominator:    binary.LittleEndian.Uint64(data[251:259]),
			OwnerWithdrawFeeNumerator:   binary.LittleEndian.Uint64(data[259:267]),
			OwnerWithdrawFeeDenominator: binary.LittleEndian.Uint64(data[267:275]),
			HostFeeNumerator:            binary.LittleEndian.Uint64(data[275:283]),
			HostFeeDenominator:          binary.LittleEndian.Uint64(data[283:291]),
		},
		CurveType: data[291],
	}
	copy(pool.Calculator[:], data[292:324])

	return pool, nil
}
