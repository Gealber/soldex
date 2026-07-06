package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func sentinelKey(seed byte) solana.PublicKey {
	var key solana.PublicKey
	for i := range key {
		key[i] = seed
	}
	return key
}

// TestDecodeDLMMPoolLayout builds a full-size LbPair buffer with sentinels at
// known on-chain offsets and asserts the decoder maps them to the right fields.
// Offsets are relative to the start of the account data (including the 8-byte
// discriminator). Total LbPair account size is 904 bytes.
func TestDecodeDLMMPoolLayout(t *testing.T) {
	const accountSize = 904
	data := make([]byte, accountSize)
	copy(data[0:8], DLMMDiscriminator[:])

	binary.LittleEndian.PutUint16(data[8+0:], 7777)        // parameters.base_factor
	binary.LittleEndian.PutUint32(data[8+68:], 0xDEADBEEF) // active_id (i32)
	binary.LittleEndian.PutUint16(data[8+72:], 25)         // bin_step
	tokenX := sentinelKey(0xA1)
	tokenY := sentinelKey(0xB2)
	copy(data[8+80:], tokenX[:])  // token_x_mint
	copy(data[8+112:], tokenY[:]) // token_y_mint

	pool, err := DecodeDLMMPool(data, solana.PublicKey{})
	if err != nil {
		t.Fatalf("DecodeDLMMPool: %v", err)
	}
	if pool.Parameters.BaseFactor != 7777 {
		t.Errorf("BaseFactor = %d, want 7777", pool.Parameters.BaseFactor)
	}
	if uint32(pool.ActiveID) != 0xDEADBEEF {
		t.Errorf("ActiveID = %#x, want 0xDEADBEEF", uint32(pool.ActiveID))
	}
	if pool.BinStep != 25 {
		t.Errorf("BinStep = %d, want 25", pool.BinStep)
	}
	if !pool.TokenXMint.Equals(tokenX) {
		t.Errorf("TokenXMint = %s, want %s", pool.TokenXMint, tokenX)
	}
	if !pool.TokenYMint.Equals(tokenY) {
		t.Errorf("TokenYMint = %s, want %s", pool.TokenYMint, tokenY)
	}
}

// TestDecodeDAMMPoolLayout builds a full-size cp-amm Pool buffer with sentinels
// at known on-chain offsets and asserts the decoder maps them correctly.
// Total Pool account size is 1112 bytes.
func TestDecodeDAMMPoolLayout(t *testing.T) {
	const accountSize = 1112
	data := make([]byte, accountSize)
	copy(data[0:8], DAMMDiscriminator[:])

	binary.LittleEndian.PutUint64(data[8+0:], 2_500_000) // base_fee cliff_fee_numerator
	tokenA := sentinelKey(0xC3)
	tokenB := sentinelKey(0xD4)
	copy(data[8+160:], tokenA[:])                            // token_a_mint
	copy(data[8+192:], tokenB[:])                            // token_b_mint
	binary.LittleEndian.PutUint64(data[8+672:], 111_222_333) // token_a_amount
	binary.LittleEndian.PutUint64(data[8+680:], 444_555_666) // token_b_amount

	pool, err := DecodeDAMMPool(data, solana.PublicKey{})
	if err != nil {
		t.Fatalf("DecodeDAMMPool: %v", err)
	}
	if pool.TradingFeeNumerator != 2_500_000 {
		t.Errorf("TradingFeeNumerator = %d, want 2500000", pool.TradingFeeNumerator)
	}
	if !pool.TokenAMint.Equals(tokenA) {
		t.Errorf("TokenAMint = %s, want %s", pool.TokenAMint, tokenA)
	}
	if !pool.TokenBMint.Equals(tokenB) {
		t.Errorf("TokenBMint = %s, want %s", pool.TokenBMint, tokenB)
	}
	if pool.TokenAAmount != 111_222_333 {
		t.Errorf("TokenAAmount = %d, want 111222333", pool.TokenAAmount)
	}
	if pool.TokenBAmount != 444_555_666 {
		t.Errorf("TokenBAmount = %d, want 444555666", pool.TokenBAmount)
	}
}

func TestDecodeDLMMPoolInsufficientData(t *testing.T) {
	// Less than 8 bytes (no discriminator)
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	addr := solana.PublicKey{}

	_, err := DecodeDLMMPool(data, addr)
	if err != ErrInsufficientData {
		t.Errorf("expected ErrInsufficientData, got %v", err)
	}
}

func TestDecodeDLMMPoolInvalidDiscriminator(t *testing.T) {
	// Valid length but wrong discriminator
	data := make([]byte, 100)
	copy(data[0:8], []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	addr := solana.PublicKey{}

	_, err := DecodeDLMMPool(data, addr)
	if err == nil {
		t.Error("expected discriminator validation error")
	}
}

func TestDecodeDAMMPoolInsufficientData(t *testing.T) {
	// Less than 8 bytes (no discriminator)
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	addr := solana.PublicKey{}

	_, err := DecodeDAMMPool(data, addr)
	if err != ErrInsufficientData {
		t.Errorf("expected ErrInsufficientData, got %v", err)
	}
}

func TestPoolTypeString(t *testing.T) {
	tests := []struct {
		pt       PoolType
		expected string
	}{
		{PoolTypeDLMM, "DLMM"},
		{PoolTypeDAMM, "DAMM"},
		{PoolType(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.pt.String(); got != tt.expected {
			t.Errorf("PoolType(%d).String() = %s, want %s", tt.pt, got, tt.expected)
		}
	}
}
