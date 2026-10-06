package models

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// Fogo mainnet Vortex pool J7mxBLSz51Tcbog3XsiJTAXS64N46KqbpRGQmd3dQMKp (WSOL/USDC) at slot 793594098.
const vortexMainnetAccount = "gkmPVrR9i6sKSX0aXceFSnxZ+JNdYXEPNbwOQmTIrCqF8k4M7Y33ov9AAEAAuAvQBxcOwUJDAgAAAAAAAAAAAADbLPXztDSYAAAAAAAAAAAAIyb+/0pFkkAAAAAAHx8AAAAAAAAGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAAT+2etF4d2TE7yF7Fm8Q3pMlAYolqwOS8J6ySnADvjIsmZDJD2BqgzM9AAAAAAAAAA1vLAmuTFWK7+R+sxcyP/gasjZY6otdfYHDctfP/P9PvEio6To0Mwryd9G2/StUo+dyry2GDJSNin6IwoPIvX7FvijpDe82AAAAAAAAAAAAhQ/FagAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAS0JTcf0wOZqWJHo8FnQN4fTG6mN+LbamnI91DCE2f10AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLQlNx/TA5mpYkejwWdA3h9MbqY34ttqacj3UMITZ/XQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEtCU3H9MDmaliR6PBZ0DeH0xupjfi22ppyPdQwhNn9dAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

// Fogo testnet Vortex pool D4npggh8hqZoyVcfNvSEbK6hKHusY1fHiMmks8oFVyZ at slot 1191316908, a 653-byte pool
// from before the extension; both vaults are token accounts of the decoded mints owned by the pool.
const vortexTestnetBaseAccount = "gkmPVrR9i6tb0ZwCLDFGIIc39cBENcu9w2lPT9EAFjDYpio+oglyDf+AAIAAECcyAAAAAAAAAAAAAAAAAAAAAAAAAACWmh/5UwIJAAAAAAAA+1wCAAAAAAAAAAAAAAAAAAAAAAAGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAAY/hlxPYFswF+Dt+UOdO+IeCbnDb1SYz0pthdlvg78vcAAAAAAAAAAAAAAAAAAAAAFsBQ/fVgZO1t+Q4eMpaeYsQhK66bkKmc5LPBfNDUiqsC+H7Db56CQzJrUj125kpgauwFYx/gTcPqoajrJwkTVsAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAoz85EHHmkmq1D4FHUymGHsPdAtBt4Ze8O9f9B016DYMAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACjPzkQceaSarUPgUdTKYYew90C0G3hl7w71/0HTXoNgwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAKM/ORBx5pJqtQ+BR1Mphh7D3QLQbeGXvDvX/QdNeg2DAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func decodeFixture(t *testing.T, encoded string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return raw
}

// The vaults are the ones transaction 5MuasdJjuY1iZMdqBXUtqiQQ9sXqnAgDyan5Uw57yCCpDMWwVbVmgBvWTkoWUbVfQmPvWq7HiJ3o4uXcZVoE45ig
// passed to swap_v2 for this pool, so the offsets are checked against the program, not this decoder.
func TestDecodeVortexMainnetPool(t *testing.T) {
	pool, err := DecodeVortex(decodeFixture(t, vortexMainnetAccount), solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	checks := []struct {
		name      string
		got, want solana.PublicKey
	}{
		{"mint a", pool.TokenMintA, solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")},
		{"mint b", pool.TokenMintB, solana.MustPublicKeyFromBase58("uSd2czE61Evaf76RNbq4KPpXnkiL3irdzgLFUMe3NoG")},
		{"vault a", pool.TokenVaultA, solana.MustPublicKeyFromBase58("5Hi57na7wCbQ2b7D3QXRPAy9b4tsT1S5WWeXJ7WcDga7")},
		{"vault b", pool.TokenVaultB, solana.MustPublicKeyFromBase58("Dfyuf7jjpZ1xSKSBTYLc8i6HGBnrEn8429b9ziDDgNBo")},
	}
	for _, check := range checks {
		if !check.got.Equals(check.want) {
			t.Fatalf("%s = %s, want %s", check.name, check.got, check.want)
		}
	}
	if pool.TickSpacing != 64 || pool.FeeRate != 3000 || pool.TickCurrentIndex != -121309 {
		t.Fatalf("spacing %d fee %d tick %d, want 64 3000 -121309", pool.TickSpacing, pool.FeeRate, pool.TickCurrentIndex)
	}
	if len(pool.Extension) != 50 {
		t.Fatalf("extension is %d bytes, want 50", len(pool.Extension))
	}
}

func TestDecodeVortexBaseCohort(t *testing.T) {
	pool, err := DecodeVortex(decodeFixture(t, vortexTestnetBaseAccount), solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(pool.Extension) != 0 || pool.TickSpacing != 128 || pool.FeeRate != 10000 {
		t.Fatalf("extension %d bytes, spacing %d, fee %d; want 0, 128, 10000", len(pool.Extension), pool.TickSpacing, pool.FeeRate)
	}
	vaultA := solana.MustPublicKeyFromBase58("AgepCnii5UuzR9Z6Ky6MAm6D5owEwMBEjrENwwP8BR23")
	if !pool.TokenVaultA.Equals(vaultA) {
		t.Fatalf("vault a = %s, want %s", pool.TokenVaultA, vaultA)
	}
}

// The two programs' pools share a layout, so only the discriminator keeps one from passing as the other.
func TestDecodeVortexAndWhirlpoolRefuseEachOther(t *testing.T) {
	vortex := decodeFixture(t, vortexMainnetAccount)
	if _, err := DecodeWhirlpool(vortex, solana.PublicKey{}); !errors.Is(err, ErrInvalidDiscriminator) {
		t.Fatalf("DecodeWhirlpool on a Vortex pool: err = %v, want ErrInvalidDiscriminator", err)
	}
	asWhirlpool := append([]byte(nil), vortex...)
	copy(asWhirlpool, WhirlpoolDiscriminator[:])
	if _, err := DecodeVortex(asWhirlpool, solana.PublicKey{}); !errors.Is(err, ErrInvalidDiscriminator) {
		t.Fatalf("DecodeVortex on a Whirlpool: err = %v, want ErrInvalidDiscriminator", err)
	}
	if _, err := DecodeVortex(vortex[:vortexBaseSize-1], solana.PublicKey{}); !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("short account: err = %v, want ErrInsufficientData", err)
	}
}
