package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func cpmmPoolWithCreatorFee(feeOn uint8, enabled bool, fees0, fees1 uint64) []byte {
	d := make([]byte, 8+637)
	copy(d[0:8], RaydiumCPMMPoolDiscriminator[:])
	d[389] = feeOn
	if enabled {
		d[390] = 1
	}
	binary.LittleEndian.PutUint64(d[397:405], fees0)
	binary.LittleEndian.PutUint64(d[405:413], fees1)
	return d
}

func TestDecodeCPMMCreatorFeeFields(t *testing.T) {
	pool, err := DecodeRaydiumCPMMPool(cpmmPoolWithCreatorFee(1, true, 4_000, 7_000), solana.PublicKey{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if pool.CreatorFeeOn != 1 || !pool.EnableCreatorFee {
		t.Fatalf("feeOn=%d enabled=%v", pool.CreatorFeeOn, pool.EnableCreatorFee)
	}
	if pool.CreatorFeesToken0 != 4_000 || pool.CreatorFeesToken1 != 7_000 {
		t.Fatalf("accruals = %d/%d", pool.CreatorFeesToken0, pool.CreatorFeesToken1)
	}
}

// Creator fees sit in the vault but are not swappable. Counting them as reserve
// over-states the pool's depth and so over-states the output.
func TestCPMMNetReservesSubtractsCreatorFees(t *testing.T) {
	d := cpmmPoolWithCreatorFee(1, true, 4_000, 7_000)
	b := d[8:]
	binary.LittleEndian.PutUint64(b[333:341], 100) // protocol_fees_token_0
	binary.LittleEndian.PutUint64(b[341:349], 200) // protocol_fees_token_1
	binary.LittleEndian.PutUint64(b[349:357], 300) // fund_fees_token_0
	binary.LittleEndian.PutUint64(b[357:365], 400) // fund_fees_token_1

	pool, err := DecodeRaydiumCPMMPool(d, solana.PublicKey{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	r0, r1 := pool.NetReserves(1_000_000, 2_000_000)
	if want := uint64(1_000_000 - 100 - 300 - 4_000); r0 != want {
		t.Fatalf("reserve0 = %d, want %d (protocol+fund+creator removed)", r0, want)
	}
	if want := uint64(2_000_000 - 200 - 400 - 7_000); r1 != want {
		t.Fatalf("reserve1 = %d, want %d", r1, want)
	}
	// Saturation still holds when the accruals exceed the balance.
	if got, _ := pool.NetReserves(10, 2_000_000); got != 0 {
		t.Fatalf("underflow should saturate to 0, got %d", got)
	}
}

func TestCPMMEffectiveCreatorFeeRate(t *testing.T) {
	cfg := &RaydiumCPMMConfig{TradeFeeRate: 2_500, CreatorFeeRate: 7_500}

	on, _ := DecodeRaydiumCPMMPool(cpmmPoolWithCreatorFee(1, true, 0, 0), solana.PublicKey{})
	if got := on.EffectiveCreatorFeeRate(cfg); got != 7_500 {
		t.Fatalf("enabled pool rate = %d, want 7500", got)
	}
	off, _ := DecodeRaydiumCPMMPool(cpmmPoolWithCreatorFee(0, false, 0, 0), solana.PublicKey{})
	if got := off.EffectiveCreatorFeeRate(cfg); got != 0 {
		t.Fatalf("disabled pool rate = %d, want 0", got)
	}
	if got := on.EffectiveCreatorFeeRate(nil); got != 0 {
		t.Fatalf("nil config rate = %d, want 0", got)
	}
}

func TestDecodeCPMMConfigCreatorFeeRate(t *testing.T) {
	d := make([]byte, 8+237)
	copy(d[0:8], RaydiumCPMMConfigDiscriminator[:])
	binary.LittleEndian.PutUint64(d[12:20], 2_500)   // trade_fee_rate
	binary.LittleEndian.PutUint64(d[108:116], 7_500) // creator_fee_rate
	cfg, err := DecodeRaydiumCPMMConfig(d, solana.PublicKey{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if cfg.TradeFeeRate != 2_500 || cfg.CreatorFeeRate != 7_500 {
		t.Fatalf("trade=%d creator=%d, want 2500/7500", cfg.TradeFeeRate, cfg.CreatorFeeRate)
	}
}
