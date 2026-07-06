package models

import (
	"encoding/binary"

	"github.com/gagliardetto/solana-go"
)

// OracleDiscriminator is the Anchor account discriminator for the Whirlpool
// Oracle account (sha256("account:Oracle")[:8]), which holds a pool's
// adaptive-fee constants and variables. Only adaptive-fee pools have an
// initialized (whirlpool-program-owned) Oracle, so its presence on the
// program-wide owner subscription marks the pool as adaptive-fee.
var OracleDiscriminator = [8]byte{139, 194, 131, 179, 140, 179, 229, 244}

// oracleAccountLen is the full serialized Oracle account size (with discriminator):
// 8 + whirlpool(32) + trade_enable(8) + constants(34) + variables(44) + reserved(128).
const oracleAccountLen = 254

// WhirlpoolOracle mirrors the on-chain Oracle account's adaptive-fee state. Field
// values map directly to the quote package's AdaptiveFeeConstants/Variables; the
// model stays free of any quote-package dependency.
type WhirlpoolOracle struct {
	// Whirlpool is the pool this oracle belongs to (the storage key).
	Whirlpool solana.PublicKey `bin:"-"`

	FilterPeriod             uint16
	DecayPeriod              uint16
	ReductionFactor          uint16
	AdaptiveFeeControlFactor uint32
	MaxVolatilityAccumulator uint32
	TickGroupSize            uint16
	MajorSwapThresholdTicks  uint16

	LastReferenceUpdateTimestamp uint64
	LastMajorSwapTimestamp       uint64
	VolatilityReference          uint32
	TickGroupIndexReference      int32
	VolatilityAccumulator        uint32
}

// DecodeWhirlpoolOracle decodes a Whirlpool Oracle account from raw bytes (with
// discriminator). The layout is packed (repr(C, packed)), so fields are read at
// fixed little-endian offsets.
func DecodeWhirlpoolOracle(data []byte, address solana.PublicKey) (*WhirlpoolOracle, error) {
	if len(data) < oracleAccountLen {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != OracleDiscriminator {
		return nil, ErrInvalidDiscriminator
	}

	// The Oracle stores its own whirlpool key at [8:40]; prefer it over the account
	// address so the store is keyed by the pool, not the oracle PDA.
	pool := solana.PublicKeyFromBytes(data[8:40])

	o := &WhirlpoolOracle{
		Whirlpool: pool,
		// adaptive_fee_constants @ 48
		FilterPeriod:             binary.LittleEndian.Uint16(data[48:50]),
		DecayPeriod:              binary.LittleEndian.Uint16(data[50:52]),
		ReductionFactor:          binary.LittleEndian.Uint16(data[52:54]),
		AdaptiveFeeControlFactor: binary.LittleEndian.Uint32(data[54:58]),
		MaxVolatilityAccumulator: binary.LittleEndian.Uint32(data[58:62]),
		TickGroupSize:            binary.LittleEndian.Uint16(data[62:64]),
		MajorSwapThresholdTicks:  binary.LittleEndian.Uint16(data[64:66]),
		// adaptive_fee_variables @ 82
		LastReferenceUpdateTimestamp: binary.LittleEndian.Uint64(data[82:90]),
		LastMajorSwapTimestamp:       binary.LittleEndian.Uint64(data[90:98]),
		VolatilityReference:          binary.LittleEndian.Uint32(data[98:102]),
		TickGroupIndexReference:      int32(binary.LittleEndian.Uint32(data[102:106])),
		VolatilityAccumulator:        binary.LittleEndian.Uint32(data[106:110]),
	}
	return o, nil
}
