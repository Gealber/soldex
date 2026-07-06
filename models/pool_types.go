package models

import (
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// DLMMStaticParameters mirrors lb_clmm StaticParameters (fee config).
type DLMMStaticParameters struct {
	BaseFactor               uint16
	FilterPeriod             uint16
	DecayPeriod              uint16
	ReductionFactor          uint16
	VariableFeeControl       uint32
	MaxVolatilityAccumulator uint32
	MinBinID                 int32
	MaxBinID                 int32
	ProtocolShare            uint16
	BaseFeePowerFactor       uint8
	FunctionType             uint8
	CollectFeeMode           uint8
	Padding                  [3]uint8
}

// DLMMVariableParameters mirrors lb_clmm VariableParameters (volatility state).
type DLMMVariableParameters struct {
	VolatilityAccumulator uint32
	VolatilityReference   uint32
	IndexReference        int32
	Padding               [4]uint8
	LastUpdateTimestamp   int64
	Padding1              [8]uint8
}

// DLMMProtocolFee mirrors lb_clmm ProtocolFee (uncollected protocol fee).
type DLMMProtocolFee struct {
	AmountX uint64
	AmountY uint64
}

// DLMMRewardInfo mirrors lb_clmm RewardInfo (farming reward config).
type DLMMRewardInfo struct {
	Mint                                      solana.PublicKey
	Vault                                     solana.PublicKey
	Funder                                    solana.PublicKey
	RewardDuration                            uint64
	RewardDurationEnd                         uint64
	RewardRate                                bin.Uint128
	LastUpdateTime                            uint64
	CumulativeSecondsWithEmptyLiquidityReward uint64
}

// DLMMPool represents the on-chain LbPair account of a Meteora DLMM pool.
// Field order and padding mirror lb_clmm::state::lb_pair::LbPair exactly so the
// flat (no-implicit-padding) zero-copy layout decodes correctly.
type DLMMPool struct {
	// Account address of the pool (not part of serialized data)
	Address solana.PublicKey `bin:"-"`

	Parameters  DLMMStaticParameters
	VParameters DLMMVariableParameters

	BumpSeed                [1]uint8
	BinStepSeed             [2]uint8
	PairType                uint8
	ActiveID                int32
	BinStep                 uint16
	Status                  uint8
	RequireBaseFactorSeed   uint8
	BaseFactorSeed          [2]uint8
	ActivationType          uint8
	CreatorPoolOnOffControl uint8

	TokenXMint solana.PublicKey
	TokenYMint solana.PublicKey
	ReserveX   solana.PublicKey
	ReserveY   solana.PublicKey

	ProtocolFee DLMMProtocolFee
	Padding1    [32]uint8

	RewardInfos [2]DLMMRewardInfo

	Oracle         solana.PublicKey
	BinArrayBitmap [16]uint64
	LastUpdatedAt  int64
	Padding2       [32]uint8

	PreActivationSwapAddress solana.PublicKey
	BaseKey                  solana.PublicKey
	ActivationPoint          uint64
	PreActivationDuration    uint64
	Padding3                 [8]uint8
	Padding4                 uint64
	Creator                  solana.PublicKey
	TokenMintXProgramFlag    uint8
	TokenMintYProgramFlag    uint8
	Version                  uint8
	Reserved                 [21]uint8
}

// DAMMBaseFeeStruct mirrors cp-amm BaseFeeStruct. BaseFeeInfo is an opaque
// 32-byte blob whose first u64 (LE) is the cliff_fee_numerator for every base
// fee mode.
type DAMMBaseFeeStruct struct {
	BaseFeeInfo [32]uint8
	Padding1    uint64
}

// DAMMDynamicFeeStruct mirrors cp-amm DynamicFeeStruct.
type DAMMDynamicFeeStruct struct {
	Initialized              uint8
	Padding                  [7]uint8
	MaxVolatilityAccumulator uint32
	VariableFeeControl       uint32
	BinStep                  uint16
	FilterPeriod             uint16
	DecayPeriod              uint16
	ReductionFactor          uint16
	LastUpdateTimestamp      uint64
	BinStepU128              bin.Uint128
	SqrtPriceReference       bin.Uint128
	VolatilityAccumulator    bin.Uint128
	VolatilityReference      bin.Uint128
}

// DAMMPoolFeesStruct mirrors cp-amm PoolFeesStruct.
type DAMMPoolFeesStruct struct {
	BaseFee            DAMMBaseFeeStruct
	ProtocolFeePercent uint8
	Padding0           uint8
	ReferralFeePercent uint8
	Padding1           [3]uint8
	CompoundingFeeBps  uint16
	DynamicFee         DAMMDynamicFeeStruct
	InitSqrtPrice      bin.Uint128
}

// DAMMPoolMetrics mirrors cp-amm PoolMetrics.
type DAMMPoolMetrics struct {
	TotalLpAFee       bin.Uint128
	TotalLpBFee       bin.Uint128
	TotalProtocolAFee uint64
	TotalProtocolBFee uint64
	Padding0          [2]uint64
	TotalPosition     uint64
	Padding           uint64
}

// DAMMRewardInfo mirrors cp-amm RewardInfo (farming reward config).
type DAMMRewardInfo struct {
	Initialized                               uint8
	RewardTokenFlag                           uint8
	Padding0                                  [6]uint8
	DeadLiquidityRewardCheckpoint             uint64
	Mint                                      solana.PublicKey
	Vault                                     solana.PublicKey
	Funder                                    solana.PublicKey
	RewardDuration                            uint64
	RewardDurationEnd                         uint64
	RewardRate                                bin.Uint128
	RewardPerTokenStored                      [32]uint8
	LastUpdateTime                            uint64
	CumulativeSecondsWithEmptyLiquidityReward uint64
}

// DAMMPool represents the on-chain Pool account of a Meteora DAMM v2 (cp-amm)
// pool. Field order and padding mirror cp_amm::state::pool::Pool exactly so the
// flat zero-copy layout decodes correctly.
type DAMMPool struct {
	// Account address of the pool (not part of serialized data)
	Address solana.PublicKey `bin:"-"`

	PoolFees DAMMPoolFeesStruct

	TokenAMint       solana.PublicKey
	TokenBMint       solana.PublicKey
	TokenAVault      solana.PublicKey
	TokenBVault      solana.PublicKey
	WhitelistedVault solana.PublicKey
	Padding0         [32]uint8

	Liquidity    bin.Uint128
	Padding1     bin.Uint128
	ProtocolAFee uint64
	ProtocolBFee uint64
	Padding2     bin.Uint128

	SqrtMinPrice bin.Uint128
	SqrtMaxPrice bin.Uint128
	SqrtPrice    bin.Uint128

	ActivationPoint uint64
	ActivationType  uint8
	PoolStatus      uint8
	TokenAFlag      uint8
	TokenBFlag      uint8
	CollectFeeMode  uint8
	PoolTypeFlag    uint8
	FeeVersion      uint8
	Padding3        uint8

	FeeAPerLiquidity [32]uint8
	FeeBPerLiquidity [32]uint8

	PermanentLockLiquidity bin.Uint128
	Metrics                DAMMPoolMetrics
	Creator                solana.PublicKey

	// Live reserve amounts (layout version 1 tracks these on-chain).
	TokenAAmount uint64
	TokenBAmount uint64

	LayoutVersion uint8
	Padding4      [7]uint8
	Padding5      [3]uint64
	RewardInfos   [2]DAMMRewardInfo

	// TradingFeeNumerator is the static base trading fee numerator (out of
	// FEE_DENOMINATOR = 1e9), extracted from PoolFees.BaseFee.BaseFeeInfo[0:8].
	// Not part of the serialized layout; populated during decode.
	TradingFeeNumerator uint64 `bin:"-"`
}

// PoolUpdate represents a Yellowstone account update for a pool.
// This is a wrapper for any pool update containing the account data and slot information.
type PoolUpdate struct {
	// Solana slot where this update occurred
	Slot uint64

	// Account data in bytes (binary serialized)
	Data []byte

	// Whether this is a DLMM or DAMM pool
	PoolType PoolType

	// Pool address
	Address solana.PublicKey

	// Timestamp of when this update was received
	UpdatedAt int64
}

// PoolType indicates the type of pool
type PoolType uint8

const (
	PoolTypeDLMM PoolType = iota
	PoolTypeDAMM
	PoolTypeOrca
	PoolTypeRaydiumCLMM
)

// String returns the string representation of the pool type
func (pt PoolType) String() string {
	switch pt {
	case PoolTypeDLMM:
		return "DLMM"
	case PoolTypeDAMM:
		return "DAMM"
	case PoolTypeOrca:
		return "ORCA"
	case PoolTypeRaydiumCLMM:
		return "RAYDIUM_CLMM"
	default:
		return "UNKNOWN"
	}
}
