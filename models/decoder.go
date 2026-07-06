package models

import (
	"encoding/binary"
	"errors"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// Discriminators for different account types. These are the anchor account
// discriminators (sha256("account:<Name>")[:8]) as published in the program
// IDLs and emitted on-chain.
var (
	// DLMM LbPair discriminator (lb_clmm program).
	DLMMDiscriminator = [8]byte{33, 11, 49, 98, 181, 101, 177, 13}

	// DAMM Pool discriminator (cp-amm program).
	DAMMDiscriminator = [8]byte{241, 154, 109, 4, 17, 177, 109, 188}

	ErrInsufficientData     = errors.New("insufficient data to decode pool")
	ErrInvalidDiscriminator = errors.New("invalid account discriminator")
	ErrUnknownPoolType      = errors.New("unknown pool type")
)

// DecodeDLMMPool decodes a DLMM pool from raw account bytes received from Yellowstone.
// The data should include the 8-byte discriminator at the start.
func DecodeDLMMPool(data []byte, address solana.PublicKey) (*DLMMPool, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	// Verify discriminator
	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != DLMMDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, DLMMDiscriminator)
	}

	// Unmarshal using binary decoder (skips discriminator)
	pool := &DLMMPool{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(pool); err != nil {
		return nil, fmt.Errorf("failed to decode DLMM pool: %w", err)
	}

	return pool, nil
}

// DecodeDAMMPool decodes a DAMM pool from raw account bytes received from Yellowstone.
func DecodeDAMMPool(data []byte, address solana.PublicKey) (*DAMMPool, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	// Verify discriminator
	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != DAMMDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, DAMMDiscriminator)
	}

	// Unmarshal using binary decoder (skips discriminator)
	pool := &DAMMPool{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(pool); err != nil {
		return nil, fmt.Errorf("failed to decode DAMM pool: %w", err)
	}

	// cliff_fee_numerator is the first u64 (LE) of the base fee data blob for
	// every base fee mode; it is the static base trading fee numerator.
	pool.TradingFeeNumerator = binary.LittleEndian.Uint64(pool.PoolFees.BaseFee.BaseFeeInfo[0:8])

	return pool, nil
}

func TokenXY(t PoolType, data []byte) (solana.PublicKey, solana.PublicKey, error) {
	var (
		tokenX, tokenY solana.PublicKey
	)

	switch t {
	case PoolTypeDLMM:
		start := 8 + 80
		if len(data) < start+64 {
			return tokenX, tokenY, ErrInsufficientData
		}
		tokenX = solana.PublicKeyFromBytes(data[start : start+32])
		tokenY = solana.PublicKeyFromBytes(data[start+32 : start+64])
	case PoolTypeDAMM:
		start := 8 + 160
		if len(data) < start+64 {
			return tokenX, tokenY, ErrInsufficientData
		}
		tokenX = solana.PublicKeyFromBytes(data[start : start+32])
		tokenY = solana.PublicKeyFromBytes(data[start+32 : start+64])
	case PoolTypeOrca:
		// token_mint_a@101 and token_mint_b@181 are non-contiguous (vault_a and
		// fee_growth_global_a sit between them), so they are read separately.
		const mintA, mintB = 101, 181
		if len(data) < mintB+32 {
			return tokenX, tokenY, ErrInsufficientData
		}
		tokenX = solana.PublicKeyFromBytes(data[mintA : mintA+32])
		tokenY = solana.PublicKeyFromBytes(data[mintB : mintB+32])
	case PoolTypeRaydiumCLMM:
		// token_mint_0@65 and token_mint_1@97 (relative to the data after the
		// 8-byte discriminator) are contiguous.
		start := 8 + 65
		if len(data) < start+64 {
			return tokenX, tokenY, ErrInsufficientData
		}
		tokenX = solana.PublicKeyFromBytes(data[start : start+32])
		tokenY = solana.PublicKeyFromBytes(data[start+32 : start+64])
	default:
		return tokenX, tokenY, ErrUnknownPoolType
	}

	return tokenX, tokenY, nil
}
