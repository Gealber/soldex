package models

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// Raydium CP-Swap (constant-product AMM, no orderbook) program. PoolState and
// AmmConfig accounts are owned by it, so one program-wide owner subscription
// streams pools and their shared fee configs.
const RaydiumCPMMProgramID = "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C"

// RaydiumCPMMPoolDiscriminator is sha256("account:PoolState")[:8]. NOTE: the
// Raydium CLMM pool account is also named PoolState, so it carries the SAME
// discriminator (RaydiumCLMMPoolDiscriminator). The two cannot be told apart by
// discriminator alone — dispatch on the owning program (RaydiumCPMMProgramID vs
// RaydiumCLMMProgramID) before decoding.
var RaydiumCPMMPoolDiscriminator = [8]byte{247, 237, 227, 245, 215, 195, 222, 70}

// RaydiumCPMMConfigDiscriminator is sha256("account:AmmConfig")[:8]. Like the
// pool account, this collides with the CLMM AmmConfig discriminator — gate on the
// owning program. The CP-Swap AmmConfig layout differs from CLMM's (trade_fee_rate
// is a u64 here, u32 there), so decode with DecodeRaydiumCPMMConfig specifically.
var RaydiumCPMMConfigDiscriminator = [8]byte{218, 244, 33, 104, 203, 203, 43, 111}

// RaydiumCPMMPool mirrors the fields of a Raydium CP-Swap PoolState needed to
// quote a swap. The swap needs no bin/tick arrays; the pool holds constant-product
// reserves in its two vault token accounts, but the SWAPPABLE reserve is the vault
// balance minus the protocol and fund fees the pool has accrued — use NetReserves.
// The trade fee lives in the linked AmmConfig, not the pool.
//
// Layout (Borsh, after the 8-byte discriminator): amm_config Pubkey@0,
// pool_creator Pubkey@32, token_0_vault Pubkey@64, token_1_vault Pubkey@96,
// lp_mint Pubkey@128, token_0_mint Pubkey@160, token_1_mint Pubkey@192,
// token_0_program Pubkey@224, token_1_program Pubkey@256, observation_key
// Pubkey@288, auth_bump u8@320, status u8@321, lp_mint_decimals u8@322,
// mint_0_decimals u8@323, mint_1_decimals u8@324, lp_supply u64@325,
// protocol_fees_token_0 u64@333, protocol_fees_token_1 u64@341,
// fund_fees_token_0 u64@349, fund_fees_token_1 u64@357, open_time u64@365, ...
type RaydiumCPMMPool struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey

	AmmConfig     solana.PublicKey
	Token0Vault   solana.PublicKey
	Token1Vault   solana.PublicKey
	Token0Mint    solana.PublicKey
	Token1Mint    solana.PublicKey
	Token0Program solana.PublicKey
	Token1Program solana.PublicKey
	Mint0Decimals uint8
	Mint1Decimals uint8

	// Fee accruals held in the vaults but NOT part of the swappable reserve.
	ProtocolFeesToken0 uint64
	ProtocolFeesToken1 uint64
	FundFeesToken0     uint64
	FundFeesToken1     uint64
}

// DecodeRaydiumCPMMPool decodes a Raydium CP-Swap PoolState from raw account bytes
// (with discriminator). The caller must have already confirmed the account is
// owned by RaydiumCPMMProgramID, since the discriminator collides with CLMM.
func DecodeRaydiumCPMMPool(data []byte, address solana.PublicKey) (*RaydiumCPMMPool, error) {
	// Through fund_fees_token_1 (ends at 8+365 = 373).
	const need = 8 + 365
	if len(data) < need {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != RaydiumCPMMPoolDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, disc, RaydiumCPMMPoolDiscriminator)
	}
	b := data[8:]
	return &RaydiumCPMMPool{
		Address:            address,
		AmmConfig:          solana.PublicKeyFromBytes(b[0:32]),
		Token0Vault:        solana.PublicKeyFromBytes(b[64:96]),
		Token1Vault:        solana.PublicKeyFromBytes(b[96:128]),
		Token0Mint:         solana.PublicKeyFromBytes(b[160:192]),
		Token1Mint:         solana.PublicKeyFromBytes(b[192:224]),
		Token0Program:      solana.PublicKeyFromBytes(b[224:256]),
		Token1Program:      solana.PublicKeyFromBytes(b[256:288]),
		Mint0Decimals:      b[323],
		Mint1Decimals:      b[324],
		ProtocolFeesToken0: binary.LittleEndian.Uint64(b[333:341]),
		ProtocolFeesToken1: binary.LittleEndian.Uint64(b[341:349]),
		FundFeesToken0:     binary.LittleEndian.Uint64(b[349:357]),
		FundFeesToken1:     binary.LittleEndian.Uint64(b[357:365]),
	}, nil
}

// NetReserves returns the swappable constant-product reserves given the pool's two
// raw vault token-account balances (read separately). It subtracts the protocol
// and fund fees the pool tracks, matching the on-chain vault_amount_without_fee.
// Subtraction saturates at zero rather than underflowing.
func (p *RaydiumCPMMPool) NetReserves(vault0Balance, vault1Balance uint64) (reserve0, reserve1 uint64) {
	return saturatingSub(vault0Balance, p.ProtocolFeesToken0+p.FundFeesToken0),
		saturatingSub(vault1Balance, p.ProtocolFeesToken1+p.FundFeesToken1)
}

func saturatingSub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// RaydiumCPMMConfig mirrors the Raydium CP-Swap AmmConfig fields needed for
// quoting. The shared config holds the trade fee for every pool that references
// it. Layout (after the 8-byte discriminator): bump u8@0, disable_create_pool
// bool@1, index u16@2, trade_fee_rate u64@4, protocol_fee_rate u64@12,
// fund_fee_rate u64@20, ...
type RaydiumCPMMConfig struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey

	// TradeFeeRate is the swap fee numerator, denominated in hundredths of a bip
	// (out of raycpmm.FeeRateDenominator = 1e6).
	TradeFeeRate uint64
}

// DecodeRaydiumCPMMConfig decodes a Raydium CP-Swap AmmConfig from raw account
// bytes (with discriminator). The caller must have confirmed RaydiumCPMMProgramID
// ownership, since the discriminator collides with CLMM's AmmConfig.
func DecodeRaydiumCPMMConfig(data []byte, address solana.PublicKey) (*RaydiumCPMMConfig, error) {
	// Through trade_fee_rate (ends at 8+12 = 20).
	const need = 8 + 12
	if len(data) < need {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != RaydiumCPMMConfigDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, disc, RaydiumCPMMConfigDiscriminator)
	}
	return &RaydiumCPMMConfig{
		Address:      address,
		TradeFeeRate: binary.LittleEndian.Uint64(data[8+4 : 8+12]),
	}, nil
}
