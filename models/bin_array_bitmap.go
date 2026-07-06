package models

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// BinArrayBitmapExtensionDiscriminator is the anchor account discriminator for
// the lb_clmm BinArrayBitmapExtension account.
var BinArrayBitmapExtensionDiscriminator = [8]byte{80, 111, 124, 113, 55, 237, 18, 5}

// BinArrayBitmapExtension is the per-pool extension bitmap, present only for
// pools whose liquidity reaches bin arrays outside the LbPair's internal bitmap
// range (|bin_array_index| > 512). Only the address and owning LbPair are needed
// to supply it as the swap's bin_array_bitmap_extension account; the packed
// bitmaps themselves are read on-chain.
type BinArrayBitmapExtension struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey
	// LbPair is the pool this extension belongs to (data offset 0, after the disc).
	LbPair solana.PublicKey
}

// DecodeBinArrayBitmapExtension extracts the owning LbPair from raw account bytes
// (with discriminator). The packed bitmaps are not decoded.
func DecodeBinArrayBitmapExtension(data []byte, address solana.PublicKey) (*BinArrayBitmapExtension, error) {
	if len(data) < 8+32 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != BinArrayBitmapExtensionDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, BinArrayBitmapExtensionDiscriminator)
	}

	return &BinArrayBitmapExtension{
		Address: address,
		LbPair:  solana.PublicKeyFromBytes(data[8 : 8+32]),
	}, nil
}
