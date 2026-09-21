package models

import (
	"encoding/base64"
	"testing"
)

// Two live devnet mints. The first stages a fee change — the older setting caps
// the fee at 5000 and the newer one at 0, from a later epoch — which is what
// makes the epoch selection testable on one account.
// 4ELbGX34jnVEvgtLYBdHfjRvsQUtWjXCiHQCn5EAfa48
const stagedFeeMint = "AQAAAMbii4WazZmSZGgAu8ofKKuNZYoACYzw9uuK5tTIEWOsAABkp7O24A0JAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQEAbADG4ouFms2ZkmRoALvKHyirjWWKAAmM8PbriubUyBFjrMbii4WazZmSZGgAu8ofKKuNZYoACYzw9uuK5tTIEWOsECcAAAAAAABoAgAAAAAAAIgTAAAAAAAAZABqAgAAAAAAAAAAAAAAAAAAZAA="

// 3bi4J88gijfpseoECPyzFS6V1nA49FDyX4n5FKpeSX1g, 200 bps and effectively uncapped.
const flatFeeMint = "AQAAADvVudm5VVdRk5uG0s0yqYfdqMMFJ83xHV2coHcxAXeXAADoiQQjx4oJAQEAAAA71bnZuVVXUZObhtLNMqmH3ajDBSfN8R1dnKB3MQF3lwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQEAbAA71bnZuVVXUZObhtLNMqmH3ajDBSfN8R1dnKB3MQF3lzvVudm5VVdRk5uG0s0yqYfdqMMFJ83xHV2coHcxAXeXAAAAAAAAAABZAgAAAAAAAAAA6IkEI8eKyABZAgAAAAAAAAAA6IkEI8eKyAA="

func decodeMint(t *testing.T, fixture string) *TransferFeeConfig {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(fixture)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}

	config, err := DecodeMintTransferFee(raw)
	if err != nil {
		t.Fatalf("DecodeMintTransferFee: %v", err)
	}
	if config == nil {
		t.Fatal("mint carries a transfer fee and none was decoded")
	}

	return config
}

func TestDecodeMintTransferFee(t *testing.T) {
	staged := decodeMint(t, stagedFeeMint)
	if staged.Older != (TransferFee{Epoch: 616, MaximumFee: 5_000, BasisPoints: 100}) {
		t.Fatalf("Older = %+v", staged.Older)
	}
	if staged.Newer != (TransferFee{Epoch: 618, MaximumFee: 0, BasisPoints: 100}) {
		t.Fatalf("Newer = %+v", staged.Newer)
	}

	flat := decodeMint(t, flatFeeMint)
	if flat.Newer.BasisPoints != 200 {
		t.Fatalf("BasisPoints = %d, want 200", flat.Newer.BasisPoints)
	}
}

// A mint with no extension is not an error; it charges nothing. Same for a
// classic mint, which is 82 bytes and has no extension region at all.
func TestDecodeMintTransferFeeAbsent(t *testing.T) {
	config, err := DecodeMintTransferFee(make([]byte, 82))
	if err != nil || config != nil {
		t.Fatalf("classic mint: config = %v, err = %v", config, err)
	}

	if config, err = DecodeMintTransferFee(nil); err != nil || config != nil {
		t.Fatalf("empty: config = %v, err = %v", config, err)
	}
}

// The epoch is the only thing that differs between these two calls, and it moves
// the fee from its cap to nothing.
func TestTransferFeeEpochSelectsTheSetting(t *testing.T) {
	config := decodeMint(t, stagedFeeMint)

	const amount = 1_000_000
	if fee := config.EpochFee(617).Fee(amount); fee != 5_000 {
		t.Fatalf("epoch 617 fee = %d, want 5000 (100 bps of %d, capped)", fee, amount)
	}
	if fee := config.EpochFee(618).Fee(amount); fee != 0 {
		t.Fatalf("epoch 618 fee = %d, want 0 (the newer setting caps at 0)", fee)
	}
}

// The program rounds the fee UP, so the smallest possible transfer still pays.
func TestTransferFeeRoundsUp(t *testing.T) {
	fee := TransferFee{MaximumFee: 5_000, BasisPoints: 100}

	// 100 bps of 1 is 0.01; rounded down that is nothing.
	if got := fee.Fee(1); got != 1 {
		t.Fatalf("Fee(1) = %d, want 1", got)
	}
	if got := fee.Fee(0); got != 0 {
		t.Fatalf("Fee(0) = %d, want 0", got)
	}
	if got := fee.PostFeeAmount(1_000); got != 990 {
		t.Fatalf("PostFeeAmount(1000) = %d, want 990", got)
	}
}

// The fee is charged on the whole balance without overflowing 64 bits.
func TestTransferFeeDoesNotOverflow(t *testing.T) {
	fee := TransferFee{MaximumFee: ^uint64(0), BasisPoints: 10_000}
	if got := fee.Fee(^uint64(0)); got != ^uint64(0) {
		t.Fatalf("a 100%% fee on the maximum amount = %d, want the whole amount", got)
	}
}
