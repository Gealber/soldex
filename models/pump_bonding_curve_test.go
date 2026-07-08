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
