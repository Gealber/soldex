// NOTE: Most of this file was AI-generated and may contain errors. Please review carefully.
package damm

type TradeDirection uint8

const (
	TradeDirectionAtoB TradeDirection = iota
	TradeDirectionBtoA
)

type SplitFees struct {
	ClaimingFee    uint64
	CompoundingFee uint64
	ProtocolFee    uint64
	ReferralFee    uint64
}

type FeeOnAmountResult struct {
	Amount         uint64
	ClaimingFee    uint64
	CompoundingFee uint64
	ProtocolFee    uint64
	ReferralFee    uint64
}

type SwapResult struct {
	IncludedFeeInputAmount uint64
	ExcludedFeeInputAmount uint64
	OutputAmount           uint64
	AmountLeft             uint64
	SplitFees
}
