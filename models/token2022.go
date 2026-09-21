package models

import (
	"encoding/binary"
	"math/big"
)

// Token-2022 packs a mint's extensions after the base layout: 82 bytes of mint,
// padding to 165, an account type byte, then type-length-value entries.
const (
	t2022AccountTypeOffset = 165
	t2022TLVOffset         = 166
	t2022AccountTypeMint   = 1
)

// transferFeeConfigExtension is the extension type of a mint's transfer fee.
const transferFeeConfigExtension uint16 = 1

// transferFeeOffset is where older_transfer_fee starts in a TransferFeeConfig:
// after the two authorities and the withheld amount.
const transferFeeOffset = 32 + 32 + 8

// transferFeeLen is epoch, maximum_fee and basis points packed little-endian.
const transferFeeLen = 8 + 8 + 2

// maxFeeBasisPoints is a 100% transfer fee.
const maxFeeBasisPoints = 10_000

// TransferFee is one of a mint's two fee settings.
type TransferFee struct {
	// Epoch is the first epoch the fee applies in.
	Epoch       uint64
	MaximumFee  uint64
	BasisPoints uint16
}

// Fee is what a transfer of amount is charged, rounded up and capped.
func (f TransferFee) Fee(amount uint64) uint64 {
	if f.BasisPoints == 0 || amount == 0 {
		return 0
	}

	// amount * basis points overflows 64 bits, and the division rounds up.
	fee := new(big.Int).Mul(new(big.Int).SetUint64(amount), big.NewInt(int64(f.BasisPoints)))
	fee.Add(fee, big.NewInt(maxFeeBasisPoints-1))
	fee.Div(fee, big.NewInt(maxFeeBasisPoints))

	if !fee.IsUint64() || fee.Uint64() > f.MaximumFee {
		return f.MaximumFee
	}

	return fee.Uint64()
}

// PostFeeAmount is what arrives when amount is sent.
func (f TransferFee) PostFeeAmount(amount uint64) uint64 {
	return amount - f.Fee(amount)
}

// TransferFeeConfig is a mint's transfer fee extension. A fee change is staged:
// the newer setting names the epoch it starts in and the older one applies
// until then.
type TransferFeeConfig struct {
	Older TransferFee
	Newer TransferFee
}

// EpochFee is the setting in force in epoch.
func (c *TransferFeeConfig) EpochFee(epoch uint64) TransferFee {
	if epoch >= c.Newer.Epoch {
		return c.Newer
	}

	return c.Older
}

// DecodeMintTransferFee reads a mint's transfer fee extension, returning nil for
// a mint that has none — a classic mint, or a Token-2022 mint without the
// extension. A nil config is not an error; it means transfers are not charged.
func DecodeMintTransferFee(data []byte) (*TransferFeeConfig, error) {
	if len(data) <= t2022TLVOffset || data[t2022AccountTypeOffset] != t2022AccountTypeMint {
		return nil, nil
	}

	for offset := t2022TLVOffset; offset+4 <= len(data); {
		extension := binary.LittleEndian.Uint16(data[offset : offset+2])
		length := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		offset += 4

		if offset+length > len(data) {
			return nil, ErrInsufficientData
		}
		if extension != transferFeeConfigExtension {
			offset += length
			continue
		}
		if length < transferFeeOffset+2*transferFeeLen {
			return nil, ErrInsufficientData
		}

		body := data[offset : offset+length]
		return &TransferFeeConfig{
			Older: decodeTransferFee(body[transferFeeOffset:]),
			Newer: decodeTransferFee(body[transferFeeOffset+transferFeeLen:]),
		}, nil
	}

	return nil, nil
}

func decodeTransferFee(data []byte) TransferFee {
	return TransferFee{
		Epoch:       binary.LittleEndian.Uint64(data[0:8]),
		MaximumFee:  binary.LittleEndian.Uint64(data[8:16]),
		BasisPoints: binary.LittleEndian.Uint16(data[16:18]),
	}
}
