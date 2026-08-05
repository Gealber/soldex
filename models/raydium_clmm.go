package models

import (
	"encoding/binary"
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

// RaydiumCLMMPool mirrors the Raydium CLMM PoolState up to TickCurrent, then picks
// up the three later fields a quote needs (Status, FeeOn, DynamicFee) by explicit
// offset — the fee-growth and reward blocks between them are not needed. The trade
// fee lives in a separate AmmConfig account, not the pool, so TradeFeeRate is
// filled in post-decode by the caller from the linked AmmConfig.
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

	// Status (offset 389) is the pool's disable bitmap; bit4 disables swap and bit5
	// disables limit orders. Read past the sequential decode, not by it.
	Status uint8 `bin:"-"`
	// FeeOn (offset 390) selects which token the swap fee is taken in: 0 FromInput,
	// 1 Token0Only, 2 Token1Only. Anything but 0 moves the fee to the OUTPUT side
	// for one of the two directions, which changes the amount out.
	FeeOn uint8 `bin:"-"`
	// DynamicFee (offset 1096) is the pool's volatility-driven fee state. Zero when
	// the pool has no dynamic fee, which is the overwhelming majority — 547 of
	// 178,353 pools carried a non-zero one on 2026-08-05.
	DynamicFee RaydiumDynamicFee `bin:"-"`
}

// Byte offsets of the PoolState fields that sit past the sequentially decoded
// prefix. They are stable: every field added since launch was carved out of
// existing padding.
const (
	raydiumPoolStatusOffset     = 389
	raydiumPoolFeeOnOffset      = 390
	raydiumPoolDynamicFeeOffset = 1096
	raydiumPoolDynamicFeeLen    = 80
	// raydiumPoolLen is PoolState::LEN — an account shorter than this predates the
	// fields above and reads them as zero.
	raydiumPoolLen = 1544
)

// RaydiumDynamicFee mirrors the on-chain DynamicFeeInfo: a volatility accumulator
// that adds to the AmmConfig fee. The first five fields are the pool's configured
// constants; the rest is live state the swap advances.
type RaydiumDynamicFee struct {
	// FilterPeriod is the high-frequency window (seconds) inside which the
	// reference is not updated at all.
	FilterPeriod uint16
	// DecayPeriod is the window (seconds) past which the volatility reference
	// resets to zero instead of decaying.
	DecayPeriod uint16
	// ReductionFactor is the decay applied to the accumulator, out of 10_000.
	ReductionFactor uint16
	// DynamicFeeControl scales volatility into fee rate, out of 100_000.
	DynamicFeeControl uint32
	// MaxVolatilityAccumulator caps the accumulator, and so caps the fee.
	MaxVolatilityAccumulator uint32
	// TickSpacingIndexReference is the tick-spacing group at the last reference update.
	TickSpacingIndexReference int32
	// VolatilityReference is the decayed accumulator carried into this swap.
	VolatilityReference uint32
	// VolatilityAccumulator is the live accumulator, scaled by 10_000.
	VolatilityAccumulator uint32
	// LastUpdateTimestamp is when the reference was last updated (unix seconds).
	LastUpdateTimestamp uint64
}

// Enabled reports whether the pool charges a dynamic fee on top of the config fee.
// The program treats an all-zero DynamicFeeInfo as absent.
func (d RaydiumDynamicFee) Enabled() bool {
	return d != RaydiumDynamicFee{}
}

// IsFeeOnInput reports whether the swap fee is taken from the input token for this
// direction. When false the fee comes out of the OUTPUT, which the quote must
// deduct from what the trader receives rather than from what they pay.
func (p *RaydiumCLMMPool) IsFeeOnInput(zeroForOne bool) bool {
	switch p.FeeOn {
	case 1:
		return zeroForOne
	case 2:
		return !zeroForOne
	default:
		return true
	}
}

// SwapDisabled reports whether bit4 of Status disables swapping on this pool.
func (p *RaydiumCLMMPool) SwapDisabled() bool {
	return p.Status&(1<<4) != 0
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
	if len(data) >= raydiumPoolLen {
		pool.Status = data[raydiumPoolStatusOffset]
		pool.FeeOn = data[raydiumPoolFeeOnOffset]
		pool.DynamicFee = decodeRaydiumDynamicFee(
			data[raydiumPoolDynamicFeeOffset : raydiumPoolDynamicFeeOffset+raydiumPoolDynamicFeeLen])
	}

	return pool, nil
}

// decodeRaydiumDynamicFee reads the 80-byte DynamicFeeInfo blob.
func decodeRaydiumDynamicFee(b []byte) RaydiumDynamicFee {
	return RaydiumDynamicFee{
		FilterPeriod:              binary.LittleEndian.Uint16(b[0:2]),
		DecayPeriod:               binary.LittleEndian.Uint16(b[2:4]),
		ReductionFactor:           binary.LittleEndian.Uint16(b[4:6]),
		DynamicFeeControl:         binary.LittleEndian.Uint32(b[6:10]),
		MaxVolatilityAccumulator:  binary.LittleEndian.Uint32(b[10:14]),
		TickSpacingIndexReference: int32(binary.LittleEndian.Uint32(b[14:18])),
		VolatilityReference:       binary.LittleEndian.Uint32(b[18:22]),
		VolatilityAccumulator:     binary.LittleEndian.Uint32(b[22:26]),
		LastUpdateTimestamp:       binary.LittleEndian.Uint64(b[26:34]),
	}
}

// RaydiumTick mirrors one Raydium CLMM TickState (168 bytes). The limit-order
// fields were carved out of the old trailing padding, so the stride is unchanged;
// they matter because a swap MATCHES resting orders as it crosses the tick, and a
// tick can be initialized on orders alone with no liquidity at all.
type RaydiumTick struct {
	Tick           int32
	LiquidityNet   bin.Int128
	LiquidityGross bin.Uint128

	FeeGrowthOutside0X64 bin.Uint128
	FeeGrowthOutside1X64 bin.Uint128
	RewardGrowthsOutside [3]bin.Uint128
	OrderPhase           uint64
	OrdersAmount         uint64
	PartFilledOrdersLeft uint64
	UnfilledRatioX64     bin.Uint128
	Padding              [3]uint32
}

// HasLiquidity reports whether the tick holds concentrated liquidity.
func (t RaydiumTick) HasLiquidity() bool {
	return t.LiquidityGross.Lo != 0 || t.LiquidityGross.Hi != 0
}

// HasLimitOrders reports whether the tick holds unfilled limit orders.
func (t RaydiumTick) HasLimitOrders() bool {
	return t.OrdersAmount > 0 || t.PartFilledOrdersLeft > 0
}

// LimitOrderUnfilled is the order size still resting at this tick, which a swap
// crossing the tick fills before (and instead of) moving further along the curve.
func (t RaydiumTick) LimitOrderUnfilled() uint64 {
	return t.OrdersAmount + t.PartFilledOrdersLeft
}

// Initialized reports whether a swap must stop at this tick. Mirrors the on-chain
// is_initialized: liquidity OR resting limit orders — checking gross liquidity
// alone walks straight past an orders-only tick and quotes a fill that the pool
// would not give.
func (t RaydiumTick) Initialized() bool {
	return t.HasLiquidity() || t.HasLimitOrders()
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
