package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// buildWhirlpoolData lays out a Whirlpool account at the documented absolute
// offsets so the test locks the byte layout, not just struct self-consistency.
func buildWhirlpoolData(mintA, mintB solana.PublicKey) []byte {
	data := make([]byte, 256)
	copy(data[0:8], WhirlpoolDiscriminator[:])
	binary.LittleEndian.PutUint16(data[41:43], 64)                 // tick_spacing
	binary.LittleEndian.PutUint16(data[45:47], 3000)               // fee_rate
	binary.LittleEndian.PutUint64(data[49:57], 123456789)          // liquidity (lo)
	binary.LittleEndian.PutUint64(data[65:73], 0x0123456789ABCDEF) // sqrt_price (lo)
	binary.LittleEndian.PutUint32(data[81:85], uint32(-123&0xFFFFFFFF))
	copy(data[101:133], mintA[:]) // token_mint_a
	copy(data[181:213], mintB[:]) // token_mint_b
	return data
}

func TestDecodeWhirlpool(t *testing.T) {
	mintA := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	mintB := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	addr := solana.MustPublicKeyFromBase58("11111111111111111111111111111112")

	pool, err := DecodeWhirlpool(buildWhirlpoolData(mintA, mintB), addr)
	if err != nil {
		t.Fatalf("DecodeWhirlpool: %v", err)
	}
	if pool.TickSpacing != 64 {
		t.Fatalf("TickSpacing = %d, want 64", pool.TickSpacing)
	}
	if pool.FeeRate != 3000 {
		t.Fatalf("FeeRate = %d, want 3000", pool.FeeRate)
	}
	if pool.Liquidity.Lo != 123456789 {
		t.Fatalf("Liquidity.Lo = %d, want 123456789", pool.Liquidity.Lo)
	}
	if pool.SqrtPrice.Lo != 0x0123456789ABCDEF {
		t.Fatalf("SqrtPrice.Lo = %x, want 0123456789ABCDEF", pool.SqrtPrice.Lo)
	}
	if pool.TickCurrentIndex != -123 {
		t.Fatalf("TickCurrentIndex = %d, want -123", pool.TickCurrentIndex)
	}
	if !pool.TokenMintA.Equals(mintA) {
		t.Fatalf("TokenMintA = %s, want %s", pool.TokenMintA, mintA)
	}
	if !pool.TokenMintB.Equals(mintB) {
		t.Fatalf("TokenMintB = %s, want %s", pool.TokenMintB, mintB)
	}
	if !pool.Address.Equals(addr) {
		t.Fatalf("Address = %s, want %s", pool.Address, addr)
	}
}

func TestTokenXYOrca(t *testing.T) {
	mintA := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	mintB := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

	x, y, err := TokenXY(PoolTypeOrca, buildWhirlpoolData(mintA, mintB))
	if err != nil {
		t.Fatalf("TokenXY: %v", err)
	}
	if !x.Equals(mintA) || !y.Equals(mintB) {
		t.Fatalf("TokenXY = (%s, %s), want (%s, %s)", x, y, mintA, mintB)
	}
}

func TestDecodeWhirlpoolRejectsBadDiscriminator(t *testing.T) {
	data := make([]byte, 256)
	copy(data[0:8], FixedTickArrayDiscriminator[:]) // wrong account type
	if _, err := DecodeWhirlpool(data, solana.PublicKey{}); err == nil {
		t.Fatal("expected discriminator mismatch error")
	}
}

func TestDecodeTickArrayRejectsDynamic(t *testing.T) {
	data := make([]byte, 16)
	copy(data[0:8], DynamicTickArrayDiscriminator[:])
	if _, err := DecodeTickArray(data, solana.PublicKey{}); err == nil {
		t.Fatal("dynamic tick array must be rejected by the fixed decoder")
	}
}

func TestTickArrayStartIndex(t *testing.T) {
	// span = 64 * 88 = 5632.
	cases := []struct {
		tick int32
		want int32
	}{
		{0, 0},
		{5631, 0},
		{5632, 5632},
		{-1, -5632},
		{-5632, -5632},
		{-5633, -11264},
	}
	for _, c := range cases {
		if got := TickArrayStartIndex(c.tick, 64); got != c.want {
			t.Fatalf("TickArrayStartIndex(%d) = %d, want %d", c.tick, got, c.want)
		}
	}
}

func TestTickAt(t *testing.T) {
	ta := &TickArray{StartTickIndex: 0}
	ta.Ticks[2].Initialized = true

	if tk, ok := ta.TickAt(128, 64); !ok || !tk.Initialized { // offset (128-0)/64 = 2
		t.Fatalf("TickAt(128) ok=%v initialized=%v, want true/true", ok, tk.Initialized)
	}
	if _, ok := ta.TickAt(130, 64); ok { // unaligned to spacing
		t.Fatal("TickAt(130) should be false (unaligned)")
	}
	if _, ok := ta.TickAt(-1, 64); ok { // below range
		t.Fatal("TickAt(-1) should be false (out of range)")
	}
	if _, ok := ta.TickAt(5632, 64); ok { // == start+span, exclusive
		t.Fatal("TickAt(5632) should be false (out of range)")
	}
}
