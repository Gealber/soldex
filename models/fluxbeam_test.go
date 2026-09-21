package models

import (
	"encoding/base64"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// A real account, fetched from mainnet. The byte-for-byte fixture is what makes
// this a layout check rather than a restatement of the decoder.
// 118mWJaGmZismRBndEyEgRY7nRvQywZLtzgkBkvg2u4
const fluxBeamLiveAccount = "AQH/Bt324e51j94YQl285GzN2rYa/E2DuQ0n/r35KNihi/x5nc2S5APTvSHsjh+vovR3JIkOi11/Kl9W9Lu1uNpF+dzigSl2uIyhIOdm+ozD/NPTh0lq0anWKuvdsxAZfIMLkvjDQo7SunJK80J9m0+9rqUMRqTSoVkM3HQ+GX8ZgbgGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAAX9Ule9HruGBhb4zNxqu4K69mtZFdZsMc5Xh6wwhIH12e1/BNBO4yuH9q+8K5pomM5R8wlxEbTvzQ9HUo3zQZk0CAAAAAAAAAOgDAAAAAAAAWgAAAAAAAABkAAAAAAAAAGIAAAAAAAAAZAAAAAAAAAAAAAAAAAAAABAnAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestDecodeFluxBeamPoolRejectsBadHeader(t *testing.T) {
	data := make([]byte, fluxBeamAccountLen)
	data[0] = 1
	data[1] = 1
	if _, err := DecodeFluxBeamPool(data, solana.PublicKey{}); err != nil {
		t.Fatalf("a well-formed header should decode: %v", err)
	}

	// Version 0 is an account the program itself refuses to unpack.
	data[0] = 0
	if _, err := DecodeFluxBeamPool(data, solana.PublicKey{}); err == nil {
		t.Fatal("version 0 must be rejected")
	}

	// 91 of the live accounts carry is_initialized = 0.
	data[0], data[1] = 1, 0
	if _, err := DecodeFluxBeamPool(data, solana.PublicKey{}); err == nil {
		t.Fatal("an uninitialized pool must be rejected")
	}

	if _, err := DecodeFluxBeamPool(data[:fluxBeamAccountLen-1], solana.PublicKey{}); err != ErrInsufficientData {
		t.Fatal("a short account must be rejected")
	}
}

// Decoding a real account pins every offset at once: a shift anywhere turns the
// mints and vaults into noise.
func TestDecodeFluxBeamPoolLiveAccount(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(fluxBeamLiveAccount)
	if err != nil {
		t.Skip("fixture not populated")
	}

	pool, err := DecodeFluxBeamPool(raw, solana.PublicKey{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if pool.MintA.String() != "So11111111111111111111111111111111111111112" {
		t.Fatalf("MintA = %s, want WSOL", pool.MintA)
	}
	if pool.TokenProgram.String() != "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb" {
		t.Fatalf("TokenProgram = %s, want Token-2022", pool.TokenProgram)
	}
	if pool.CurveType != FluxBeamCurveConstantProduct {
		t.Fatalf("CurveType = %d, want constant product", pool.CurveType)
	}
	if pool.Fees.TradeFeeDenominator == 0 {
		t.Fatal("trade fee denominator must not be zero")
	}
	if pool.VaultA.IsZero() || pool.VaultB.IsZero() || pool.PoolMint.IsZero() {
		t.Fatal("vaults and pool mint must decode to real keys")
	}
}

// A devnet pool, with its authority and its two vault programs taken from the
// vault accounts rather than from this one. That makes the bump a real check:
// the authority only comes out right if bump_seed is read from offset 2.
// 77KQdRPrmzR4731cvDuM1sSPN6DcDaxsSH6aEyYwXjjC
const (
	fluxBeamDevnetAccount   = "AQH/Bt324e51j94YQl285GzN2rYa/E2DuQ0n/r35KNihi/xJRW6ssubS+Vc0vBTSh/t7O1q1Lod0MlHiXdaVS1wabCCOGqCLFfoz1hzYuIASOzGtdAURILZPuv/b+YgOZwSJOxtCJDVPsqNHbZEwIuu/Ue5xRhueYM8YxKKk83ZkQawGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAASabi2EDsz70kWj9ok7Rum0RxrR15mLnfCooftOKQOl3dLrvMHt+8GfUy04d9PINhwsExefOGlrdRGrWa8LXiacUAAAAAAAAAOgDAAAAAAAABQAAAAAAAADoAwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAUAAAAAAAAAOgDAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	fluxBeamDevnetPool      = "77KQdRPrmzR4731cvDuM1sSPN6DcDaxsSH6aEyYwXjjC"
	fluxBeamDevnetAuthority = "3ifZ3jLupGUVjAJdUPdqePwSMGTcH8CSswo7oTrg3Lsp"
	fluxBeamDevnetFeeAcc    = "8rfct1j8pLf11XcMzuHnaKajiHMVVwiMPatxV6VhEEM4"
)

func TestFluxBeamPoolAuthorityAndFeeAccount(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(fluxBeamDevnetAccount)
	if err != nil {
		t.Fatalf("%v", err)
	}

	pool, err := DecodeFluxBeamPool(raw, solana.MPK(fluxBeamDevnetPool))
	if err != nil {
		t.Fatalf("%v", err)
	}

	authority, err := pool.Authority()
	if err != nil {
		t.Fatalf("authority: %v", err)
	}
	if authority.String() != fluxBeamDevnetAuthority {
		t.Fatalf("Authority() = %s, want %s (the owner of both vaults)", authority, fluxBeamDevnetAuthority)
	}
	if pool.PoolFeeAccount.String() != fluxBeamDevnetFeeAcc {
		t.Fatalf("PoolFeeAccount = %s, want %s", pool.PoolFeeAccount, fluxBeamDevnetFeeAcc)
	}
}

// This pool holds one classic vault and one Token-2022 vault while its own
// TokenProgram is Token-2022, so a caller that takes the sides from
// TokenProgram builds the swap with the wrong program on one of them.
func TestFluxBeamPoolTokenProgramIsNotTheSides(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(fluxBeamDevnetAccount)
	if err != nil {
		t.Fatalf("%v", err)
	}

	pool, err := DecodeFluxBeamPool(raw, solana.MPK(fluxBeamDevnetPool))
	if err != nil {
		t.Fatalf("%v", err)
	}

	if !pool.MintA.Equals(solana.WrappedSol) {
		t.Fatalf("MintA = %s, want WSOL", pool.MintA)
	}
	if pool.TokenProgram.Equals(solana.TokenProgramID) {
		t.Fatalf("TokenProgram = %s, but WSOL is a classic-Token mint on side A", pool.TokenProgram)
	}
}
