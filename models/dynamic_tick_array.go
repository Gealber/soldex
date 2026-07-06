package models

import (
	"encoding/binary"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// dynamicTickDataRest is the bytes of a DynamicTickData after liquidity_net:
// liquidity_gross(16) + fee_growth_outside_a(16) + fee_growth_outside_b(16) +
// reward_growths_outside[3](48). Only liquidity_net is needed for quoting.
const dynamicTickDataRest = 16 + 16 + 16 + 48

// DecodeDynamicTickArray decodes Orca's variable/dynamic TickArray into the same
// TickArray structure the quote engine reads. Layout (Borsh, after the 8-byte
// discriminator): start_tick_index i32, whirlpool Pubkey, tick_bitmap u128, then
// [DynamicTick; 88] where each DynamicTick is a 1-byte enum tag (0 = Uninitialized,
// 1 = Initialized) followed, when initialized, by DynamicTickData (liquidity_net
// i128 + the fields skipped above). This is what modern Orca pools (incl. most
// pump-graduated memecoins) use; the fixed decoder rejects them.
func DecodeDynamicTickArray(data []byte, address solana.PublicKey) (*TickArray, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	if disc != DynamicTickArrayDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, disc, DynamicTickArrayDiscriminator)
	}

	dec := bin.NewBinDecoder(data[8:])
	start, err := dec.ReadInt32(binary.LittleEndian)
	if err != nil {
		return nil, err
	}
	whirlpool, err := dec.ReadNBytes(32)
	if err != nil {
		return nil, err
	}
	if err := dec.SkipBytes(16); err != nil { // tick_bitmap u128
		return nil, err
	}

	ta := &TickArray{
		Address:        address,
		StartTickIndex: start,
		Whirlpool:      solana.PublicKeyFromBytes(whirlpool),
	}
	for i := 0; i < TicksPerArray; i++ {
		tag, err := dec.ReadByte()
		if err != nil {
			return nil, err
		}
		switch tag {
		case 0: // Uninitialized
		case 1: // Initialized(DynamicTickData)
			liquidityNet, err := dec.ReadInt128(binary.LittleEndian)
			if err != nil {
				return nil, err
			}
			if err := dec.SkipBytes(dynamicTickDataRest); err != nil {
				return nil, err
			}
			ta.Ticks[i] = Tick{Initialized: true, LiquidityNet: liquidityNet}
		default:
			return nil, fmt.Errorf("dynamic tick array %s: bad tick tag %d at %d", address, tag, i)
		}
	}
	return ta, nil
}

// DecodeAnyTickArray decodes a fixed or dynamic Orca tick array by discriminator.
func DecodeAnyTickArray(data []byte, address solana.PublicKey) (*TickArray, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}
	var disc [8]byte
	copy(disc[:], data[:8])
	switch disc {
	case FixedTickArrayDiscriminator:
		return DecodeTickArray(data, address)
	case DynamicTickArrayDiscriminator:
		return DecodeDynamicTickArray(data, address)
	default:
		return nil, fmt.Errorf("%w: %x", ErrInvalidDiscriminator, disc)
	}
}
