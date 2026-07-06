package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// TestDecodeWhirlpoolOracleLayout locks the Oracle field offsets against the
// values in the Orca program's own data_layout test (state/oracle.rs).
func TestDecodeWhirlpoolOracleLayout(t *testing.T) {
	data := make([]byte, oracleAccountLen)
	copy(data[:8], OracleDiscriminator[:])
	pool := solana.NewWallet().PublicKey()
	copy(data[8:40], pool[:])
	binary.LittleEndian.PutUint64(data[40:48], 0x1122334455667788) // trade_enable_timestamp

	// adaptive_fee_constants @ 48
	binary.LittleEndian.PutUint16(data[48:50], 0x1122)
	binary.LittleEndian.PutUint16(data[50:52], 0x3344)
	binary.LittleEndian.PutUint16(data[52:54], 0x5566)
	binary.LittleEndian.PutUint32(data[54:58], 0x778899aa)
	binary.LittleEndian.PutUint32(data[58:62], 0xaabbccdd)
	binary.LittleEndian.PutUint16(data[62:64], 0xeeff)
	binary.LittleEndian.PutUint16(data[64:66], 0x1122)

	// adaptive_fee_variables @ 82
	binary.LittleEndian.PutUint64(data[82:90], 0x1122334455667788)
	binary.LittleEndian.PutUint64(data[90:98], 0x2233445566778899)
	binary.LittleEndian.PutUint32(data[98:102], 0x99aabbcc)
	binary.LittleEndian.PutUint32(data[102:106], 0x00ddeeff)
	binary.LittleEndian.PutUint32(data[106:110], 0x11223344)

	o, err := DecodeWhirlpoolOracle(data, solana.NewWallet().PublicKey())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if o.Whirlpool != pool {
		t.Fatalf("whirlpool = %s, want %s", o.Whirlpool, pool)
	}
	checks := []struct {
		name     string
		got, exp uint64
	}{
		{"filter_period", uint64(o.FilterPeriod), 0x1122},
		{"decay_period", uint64(o.DecayPeriod), 0x3344},
		{"reduction_factor", uint64(o.ReductionFactor), 0x5566},
		{"adaptive_fee_control_factor", uint64(o.AdaptiveFeeControlFactor), 0x778899aa},
		{"max_volatility_accumulator", uint64(o.MaxVolatilityAccumulator), 0xaabbccdd},
		{"tick_group_size", uint64(o.TickGroupSize), 0xeeff},
		{"major_swap_threshold_ticks", uint64(o.MajorSwapThresholdTicks), 0x1122},
		{"last_reference_update_timestamp", o.LastReferenceUpdateTimestamp, 0x1122334455667788},
		{"last_major_swap_timestamp", o.LastMajorSwapTimestamp, 0x2233445566778899},
		{"volatility_reference", uint64(o.VolatilityReference), 0x99aabbcc},
		{"tick_group_index_reference", uint64(uint32(o.TickGroupIndexReference)), 0x00ddeeff},
		{"volatility_accumulator", uint64(o.VolatilityAccumulator), 0x11223344},
	}
	for _, c := range checks {
		if c.got != c.exp {
			t.Errorf("%s = %#x, want %#x", c.name, c.got, c.exp)
		}
	}
}
