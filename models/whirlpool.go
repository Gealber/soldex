package models

import (
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// Orca Whirlpool program (concentrated liquidity). Both Whirlpool and TickArray
// accounts are owned by it, so a program-wide owner subscription streams both.
const OrcaWhirlpoolProgramID = "whirLbMiicVdio4qvUfM5KAg6Ct8VwpYzGff3uctyCc"

// TicksPerArray is the number of ticks stored in one fixed TickArray account.
const TicksPerArray = 88

// Anchor account discriminators (verified against the Orca rust-sdk generated
// client). The dynamic tick array variant is not decoded in this version.
var (
	WhirlpoolDiscriminator        = [8]byte{63, 149, 209, 12, 225, 128, 99, 9}
	FixedTickArrayDiscriminator   = [8]byte{69, 97, 189, 190, 110, 7, 66, 187}
	DynamicTickArrayDiscriminator = [8]byte{17, 216, 246, 142, 225, 199, 218, 56}
)

// Whirlpool mirrors the Orca Whirlpool account up to TokenVaultB; the trailing
// reward/fee-growth fields past it are not needed for quoting, so the decoder
// stops there. token_mint_a and token_mint_b are NOT contiguous (vault_a and
// fee_growth_global_a sit between them), so TokenXY needs Orca-specific offsets.
type Whirlpool struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey `bin:"-"`

	WhirlpoolsConfig solana.PublicKey
	WhirlpoolBump    [1]uint8
	TickSpacing      uint16
	FeeTierIndexSeed [2]uint8
	// fee_rate is stored as hundredths of a basis point (1e-6).
	FeeRate         uint16
	ProtocolFeeRate uint16
	Liquidity       bin.Uint128
	// Q64.64 current price.
	SqrtPrice        bin.Uint128
	TickCurrentIndex int32
	ProtocolFeeOwedA uint64
	ProtocolFeeOwedB uint64
	TokenMintA       solana.PublicKey
	TokenVaultA      solana.PublicKey
	FeeGrowthGlobalA bin.Uint128
	TokenMintB       solana.PublicKey
	TokenVaultB      solana.PublicKey
}

// DecodeWhirlpool decodes a Whirlpool from raw account bytes (with discriminator).
func DecodeWhirlpool(data []byte, address solana.PublicKey) (*Whirlpool, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != WhirlpoolDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, WhirlpoolDiscriminator)
	}

	pool := &Whirlpool{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(pool); err != nil {
		return nil, fmt.Errorf("failed to decode whirlpool: %w", err)
	}

	return pool, nil
}

// Tick mirrors one Orca Tick (113 bytes). Only liquidity_net is read for quoting;
// the rest is present so the fixed layout decodes in order.
type Tick struct {
	Initialized          bool
	LiquidityNet         bin.Int128
	LiquidityGross       bin.Uint128
	FeeGrowthOutsideA    bin.Uint128
	FeeGrowthOutsideB    bin.Uint128
	RewardGrowthsOutside [3]bin.Uint128
}

// TickArray mirrors the Orca fixed TickArray account: a contiguous run of 88 ticks
// starting at StartTickIndex, spaced by the pool's tick_spacing.
type TickArray struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey `bin:"-"`

	StartTickIndex int32
	Ticks          [TicksPerArray]Tick
	Whirlpool      solana.PublicKey
}

// DecodeTickArray decodes a fixed Orca TickArray from raw account bytes (with
// discriminator). The dynamic tick array variant is rejected as unknown.
func DecodeTickArray(data []byte, address solana.PublicKey) (*TickArray, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != FixedTickArrayDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, FixedTickArrayDiscriminator)
	}

	tickArray := &TickArray{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(tickArray); err != nil {
		return nil, fmt.Errorf("failed to decode tick array: %w", err)
	}

	return tickArray, nil
}

// TickArrayStartIndex returns the start_tick_index of the fixed TickArray that
// contains tickIndex for the given tickSpacing, using floor division so negative
// ticks map correctly. Mirrors the on-chain tick-array PDA derivation.
func TickArrayStartIndex(tickIndex int32, tickSpacing uint16) int32 {
	span := int32(tickSpacing) * TicksPerArray
	start := tickIndex / span
	if tickIndex < 0 && tickIndex%span != 0 {
		start--
	}
	return start * span
}

// TickAt returns the tick at tickIndex from this array, or (zero, false) if the
// index falls outside the array's [start, start + span) range or is unaligned to
// tickSpacing.
func (ta *TickArray) TickAt(tickIndex int32, tickSpacing uint16) (Tick, bool) {
	span := int32(tickSpacing) * TicksPerArray
	if tickIndex < ta.StartTickIndex || tickIndex >= ta.StartTickIndex+span {
		return Tick{}, false
	}
	if tickSpacing == 0 || (tickIndex-ta.StartTickIndex)%int32(tickSpacing) != 0 {
		return Tick{}, false
	}
	offset := (tickIndex - ta.StartTickIndex) / int32(tickSpacing)
	return ta.Ticks[offset], true
}
