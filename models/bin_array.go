package models

import (
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// MaxBinPerArray is the number of bins stored in one DLMM BinArray account.
const MaxBinPerArray = 70

// BinArrayDiscriminator is the anchor account discriminator for lb_clmm BinArray.
var BinArrayDiscriminator = [8]byte{92, 142, 92, 220, 5, 148, 70, 181}

// Bin mirrors lb_clmm Bin. Only amount_x/amount_y are read for quoting; the
// remaining fields are present so the fixed 144-byte layout decodes in order.
type Bin struct {
	AmountX                       uint64
	AmountY                       uint64
	Price                         bin.Uint128
	LiquiditySupply               bin.Uint128
	FulfilledOrderAmountX         uint64
	FulfilledOrderAmountY         uint64
	LimitOrderFeeAskSide          uint64
	LimitOrderFeeBidSide          uint64
	FeeAmountXPerTokenStored      bin.Uint128
	FeeAmountYPerTokenStored      bin.Uint128
	OpenOrderAmount               uint64
	TotalProcessingOrderAmount    uint64
	ProcessedOrderRemainingAmount uint64
	OrderAge                      uint32
	LimitOrderAskSide             uint8
	Padding1                      [3]uint8
}

// BinArray mirrors the on-chain lb_clmm BinArray account.
type BinArray struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey `bin:"-"`

	Index    int64
	Version  uint8
	Padding1 [7]uint8
	LbPair   solana.PublicKey
	Bins     [MaxBinPerArray]Bin
}

// DecodeBinArray decodes a BinArray from raw account bytes (with discriminator).
func DecodeBinArray(data []byte, address solana.PublicKey) (*BinArray, error) {
	if len(data) < 8 {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != BinArrayDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, BinArrayDiscriminator)
	}

	binArray := &BinArray{Address: address}
	decoder := bin.NewBinDecoder(data[8:])
	if err := decoder.Decode(binArray); err != nil {
		return nil, fmt.Errorf("failed to decode bin array: %w", err)
	}

	return binArray, nil
}

// BinIDToArrayIndex returns the BinArray index that contains binID, using floor
// division so negative ids map correctly. Mirrors BinArray::bin_id_to_bin_array_index.
func BinIDToArrayIndex(binID int32) int64 {
	idx := binID / MaxBinPerArray
	if binID < 0 && binID%MaxBinPerArray != 0 {
		idx--
	}
	return int64(idx)
}

// ArrayLowerBinID returns the lowest bin id stored in the array at arrayIndex.
func ArrayLowerBinID(arrayIndex int64) int32 {
	return int32(arrayIndex) * MaxBinPerArray
}

// BinAt returns the bin with the given id from this array, or (zero, false) if
// the id falls outside the array's range.
func (ba *BinArray) BinAt(binID int32) (Bin, bool) {
	lower := ArrayLowerBinID(ba.Index)
	upper := lower + MaxBinPerArray - 1
	if binID < lower || binID > upper {
		return Bin{}, false
	}
	return ba.Bins[binID-lower], true
}
