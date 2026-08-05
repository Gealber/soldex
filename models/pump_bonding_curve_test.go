package models

import (
	"encoding/base64"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// Real mainnet BondingCurve account (v1 token 4kskvWho…, decoded 2026-07-08).
const bondingCurveV1B64 = "F7f4N2DYrGDojhdCDr0DAAonUx8HAAAA6PYE9ny+AgAKey8jAAAAAACAxqR+jQMAAFjs94LEDcnj3gIpOil+R58P8zsfIKB6go+2HIKfjm3oAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

func TestDecodeBondingCurve(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(bondingCurveV1B64)
	if err != nil {
		t.Fatal(err)
	}
	bc, err := DecodeBondingCurve(data, solana.PublicKey{})
	if err != nil {
		t.Fatal(err)
	}
	if bc.VirtualTokenReserves != 1052293866163944 {
		t.Errorf("virtual_token_reserves = %d", bc.VirtualTokenReserves)
	}
	if bc.VirtualSolReserves != 30590314250 {
		t.Errorf("virtual_sol_reserves = %d", bc.VirtualSolReserves)
	}
	if bc.RealTokenReserves != 772393866163944 {
		t.Errorf("real_token_reserves = %d", bc.RealTokenReserves)
	}
	if bc.RealSolReserves != 590314250 {
		t.Errorf("real_sol_reserves = %d", bc.RealSolReserves)
	}
	if bc.TokenTotalSupply != 1000000000000000 {
		t.Errorf("token_total_supply = %d", bc.TokenTotalSupply)
	}
	if bc.Complete {
		t.Error("complete should be false")
	}
	if bc.Creator.String() != "6z8TDnbgeCenxg3bYZMCYV3sd5jWgUotnDvEVLm4HF5R" {
		t.Errorf("creator = %s", bc.Creator)
	}
}

func TestDecodeBondingCurveBadDiscriminator(t *testing.T) {
	if _, err := DecodeBondingCurve(make([]byte, 81), solana.PublicKey{}); err == nil {
		t.Fatal("expected discriminator error on zeroed data")
	}
}

func TestDecodeBondingCurveShort(t *testing.T) {
	if _, err := DecodeBondingCurve(make([]byte, 10), solana.PublicKey{}); err == nil {
		t.Fatal("expected insufficient-data error")
	}
}

// bondingCurveAccount builds a curve account truncated to size, so each live cohort can be
// decoded from the same bytes.
func bondingCurveAccount(t *testing.T, size int, quoteMint solana.PublicKey) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(bondingCurveV1B64)
	if err != nil {
		t.Fatal(err)
	}
	if size > len(data) {
		t.Fatalf("size %d exceeds the golden account (%d bytes)", size, len(data))
	}
	if size >= bondingCurveQuoteMintEnd {
		copy(data[83:115], quoteMint[:])
	}
	return data[:size]
}

// The account has grown twice and all three cohorts are still live, so a curve too short to
// carry the newer fields must decode rather than fail.
func TestDecodeBondingCurveEveryLiveCohort(t *testing.T) {
	for _, size := range []int{81, 83, 115, 150} {
		bc, err := DecodeBondingCurve(bondingCurveAccount(t, size, solana.PublicKey{}), solana.PublicKey{})
		if err != nil {
			t.Fatalf("size %d: decode: %v", size, err)
		}
		if bc.VirtualSolReserves != 30590314250 {
			t.Fatalf("size %d: virtual_sol_reserves = %d", size, bc.VirtualSolReserves)
		}
		if !bc.IsSOLQuoted() {
			t.Fatalf("size %d: a curve with no quote_mint must read as SOL-quoted", size)
		}
	}
}

// A non-zero quote_mint means the *SolReserves fields are not lamports. Nothing else in the
// struct says so, so IsSOLQuoted is the only guard a caller has against pricing them as SOL.
func TestDecodeBondingCurveNonSOLQuoteMint(t *testing.T) {
	usdc := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	bc, err := DecodeBondingCurve(bondingCurveAccount(t, 115, usdc), solana.PublicKey{})
	if err != nil {
		t.Fatal(err)
	}
	if bc.QuoteMint != usdc {
		t.Fatalf("QuoteMint = %s, want %s", bc.QuoteMint, usdc)
	}
	if bc.IsSOLQuoted() {
		t.Fatal("a curve quoted in USDC must not report as SOL-quoted")
	}
}
