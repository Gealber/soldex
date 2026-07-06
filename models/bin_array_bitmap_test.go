package models

import (
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestDecodeBinArrayBitmapExtension(t *testing.T) {
	// 8 disc + 32 lb_pair + 2 * (12 * 8 * 8) packed bitmaps.
	const accountSize = 8 + 32 + 2*(12*8*8)
	data := make([]byte, accountSize)
	copy(data[0:8], BinArrayBitmapExtensionDiscriminator[:])
	lbPair := sentinelKey(0x77)
	copy(data[8:], lbPair[:]) // lb_pair at data offset 0 (account offset 8)

	addr := sentinelKey(0x11)
	ext, err := DecodeBinArrayBitmapExtension(data, addr)
	if err != nil {
		t.Fatalf("DecodeBinArrayBitmapExtension: %v", err)
	}
	if !ext.Address.Equals(addr) {
		t.Errorf("Address = %s, want %s", ext.Address, addr)
	}
	if !ext.LbPair.Equals(lbPair) {
		t.Errorf("LbPair = %s, want %s", ext.LbPair, lbPair)
	}
}

func TestDecodeBinArrayBitmapExtensionInvalidDiscriminator(t *testing.T) {
	data := make([]byte, 8+32)
	copy(data[0:8], BinArrayDiscriminator[:]) // wrong disc
	if _, err := DecodeBinArrayBitmapExtension(data, solana.PublicKey{}); err == nil {
		t.Fatal("expected error on wrong discriminator")
	}
}

func TestDecodeBinArrayBitmapExtensionInsufficientData(t *testing.T) {
	data := make([]byte, 8+10)
	copy(data[0:8], BinArrayBitmapExtensionDiscriminator[:])
	if _, err := DecodeBinArrayBitmapExtension(data, solana.PublicKey{}); err == nil {
		t.Fatal("expected error on short data")
	}
}
