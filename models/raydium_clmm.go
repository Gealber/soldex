package models

import (
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// Raydium CLMM (concentrated liquidity) program. PoolState, TickArrayState, and
// AmmConfig accounts are all owned by it, so one program-wide owner subscription
// streams pools, their tick arrays, and the shared fee configs.
const RaydiumCLMMProgramID = "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK"

// RaydiumTicksPerArray is the number of ticks stored in one Raydium CLMM
// TickArray account.
const RaydiumTicksPerArray = 60

// Anchor account discriminators, verified against mainnet accounts (pool
// 3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv and its tick array / config).
var (
	RaydiumCLMMPoolDiscriminator  = [8]byte{247, 237, 227, 245, 215, 195, 222, 70}
	RaydiumTickArrayDiscriminator = [8]byte{192, 155, 85, 205, 49, 249, 129, 42}
	RaydiumAmmConfigDiscriminator = [8]byte{218, 244, 33, 104, 203, 203, 43, 111}
)

// RaydiumCLMMPool mirrors the Raydium CLMM PoolState up to TickCurrent; the
// trailing fee-growth/reward/padding fields past it are not needed for quoting,
// so the decoder stops there (newer fields like fee_on/dynamic_fee_info are
// carved from former padding, leaving these offsets stable). The trade fee lives
// in a separate AmmConfig account, not the pool, so TradeFeeRate is filled in
// post-decode by the caller from the linked AmmConfig.
type RaydiumCLMMPool struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey `bin:"-"`

	Bump           [1]uint8
	AmmConfig      solana.PublicKey
	Owner          solana.PublicKey
	TokenMint0     solana.PublicKey
	TokenMint1     solana.PublicKey
	TokenVault0    solana.PublicKey
	TokenVault1    solana.PublicKey
	ObservationKey solana.PublicKey
	MintDecimals0  uint8
	MintDecimals1  uint8
	TickSpacing    uint16
	Liquidity      bin.Uint128
	// Q64.64 current price as sqrt(token_1/token_0).
	SqrtPriceX64 bin.Uint128
	TickCurrent  int32

	// TradeFeeRate is the fee numerator (out of 1e6) read from the linked
	// AmmConfig account. Not part of the pool's serialized data; set post-decode.
	TradeFeeRate uint32 `bin:"-"`
}

// DecodeRaydiumCLMMPool decodes a Raydium CLMM PoolState from raw account bytes
// (with discriminator).
func DecodeRaydiumCLMMPool(data []byte, address solana.PublicKey) (*RaydiumCLMMPool, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != RaydiumCLMMPoolDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, RaydiumCLMMPoolDiscriminator)
	}

	pool := &RaydiumCLMMPool{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(pool); err != nil {
		return nil, fmt.Errorf("failed to decode raydium clmm pool: %w", err)
	}

	return pool, nil
}

// RaydiumTick mirrors one Raydium CLMM TickState (168 bytes). Only Tick,
// LiquidityNet and LiquidityGross are read for quoting; the trailing 132 bytes
// (fee/reward growths, limit-order fields, padding) are present so the fixed
// array stride decodes correctly.
type RaydiumTick struct {
	Tick           int32
	LiquidityNet   bin.Int128
	LiquidityGross bin.Uint128
	Rest           [132]uint8
}

// Initialized reports whether the tick holds liquidity. In Uniswap-V3 a tick is
// initialized exactly when its gross liquidity is non-zero.
func (t RaydiumTick) Initialized() bool {
	return t.LiquidityGross.Lo != 0 || t.LiquidityGross.Hi != 0
}

// RaydiumTickArray mirrors the Raydium CLMM TickArrayState: PoolID and
// StartTickIndex precede a contiguous run of 60 ticks spaced by tick_spacing.
type RaydiumTickArray struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey `bin:"-"`

	PoolID               solana.PublicKey
	StartTickIndex       int32
	Ticks                [RaydiumTicksPerArray]RaydiumTick
	InitializedTickCount uint8
}

// DecodeRaydiumTickArray decodes a Raydium CLMM TickArrayState from raw account
// bytes (with discriminator).
func DecodeRaydiumTickArray(data []byte, address solana.PublicKey) (*RaydiumTickArray, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != RaydiumTickArrayDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, RaydiumTickArrayDiscriminator)
	}

	tickArray := &RaydiumTickArray{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(tickArray); err != nil {
		return nil, fmt.Errorf("failed to decode raydium tick array: %w", err)
	}

	return tickArray, nil
}

// RaydiumAmmConfig mirrors the Raydium CLMM AmmConfig up to FundFeeRate. The
// shared config holds the trade fee for every pool that references it.
type RaydiumAmmConfig struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey `bin:"-"`

	Bump            uint8
	Index           uint16
	Owner           solana.PublicKey
	ProtocolFeeRate uint32
	// TradeFeeRate is the swap fee numerator, denominated in hundredths of a bip
	// (out of 1e6).
	TradeFeeRate uint32
	TickSpacing  uint16
	FundFeeRate  uint32
}

// DecodeRaydiumAmmConfig decodes a Raydium CLMM AmmConfig from raw account bytes
// (with discriminator).
func DecodeRaydiumAmmConfig(data []byte, address solana.PublicKey) (*RaydiumAmmConfig, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != RaydiumAmmConfigDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, RaydiumAmmConfigDiscriminator)
	}

	config := &RaydiumAmmConfig{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to decode raydium amm config: %w", err)
	}

	return config, nil
}

// RaydiumTickArrayStartIndex returns the start_tick_index of the TickArray that
// contains tickIndex for the given tickSpacing, using floor division so negative
// ticks map correctly. Mirrors the on-chain tick-array PDA derivation.
func RaydiumTickArrayStartIndex(tickIndex int32, tickSpacing uint16) int32 {
	span := int32(tickSpacing) * RaydiumTicksPerArray
	start := tickIndex / span
	if tickIndex < 0 && tickIndex%span != 0 {
		start--
	}
	return start * span
}

// TickAt returns the tick at tickIndex from this array, or (zero, false) if the
// index falls outside the array's [start, start + span) range or is unaligned to
// tickSpacing.
func (ta *RaydiumTickArray) TickAt(tickIndex int32, tickSpacing uint16) (RaydiumTick, bool) {
	span := int32(tickSpacing) * RaydiumTicksPerArray
	if tickIndex < ta.StartTickIndex || tickIndex >= ta.StartTickIndex+span {
		return RaydiumTick{}, false
	}
	if tickSpacing == 0 || (tickIndex-ta.StartTickIndex)%int32(tickSpacing) != 0 {
		return RaydiumTick{}, false
	}
	offset := (tickIndex - ta.StartTickIndex) / int32(tickSpacing)
	return ta.Ticks[offset], true
}
