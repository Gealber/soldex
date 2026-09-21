package soldex

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"

	"github.com/Gealber/soldex/models"
)

// Raydium CLMM and CP-Swap share an account discriminator, so a decoder chosen
// on the discriminator alone picks the wrong one for whichever it checks second.
// Dispatching on the OWNER is what makes this safe, and this pins it: the same
// bytes must decode as two different venues depending only on the owner.
func TestFromAccountDispatchesOnOwnerNotDiscriminator(t *testing.T) {
	cpmm := make([]byte, 8+637)
	copy(cpmm[0:8], models.RaydiumCPMMPoolDiscriminator[:])
	binary.LittleEndian.PutUint64(cpmm[8+333:], 0) // no accrued fees

	aux := Aux{
		CPMMConfig: &models.RaydiumCPMMConfig{TradeFeeRate: 2_500},
		Vault0:     1_000_000_000,
		Vault1:     1_000_000_000,
	}

	// As CP-Swap: quotable.
	q, err := FromAccount(solana.MustPublicKeyFromBase58(models.RaydiumCPMMProgramID), cpmm, aux)
	if err != nil {
		t.Fatalf("CP-Swap dispatch: %v", err)
	}
	if out, err := q.QuoteExactIn(1_000_000, true); err != nil || out == 0 {
		t.Fatalf("CP-Swap quote = %d, err = %v", out, err)
	}

	// The very same bytes under the CLMM program go to the CLMM decoder, which
	// reads them as a different account entirely.
	if _, err := FromAccount(solana.MustPublicKeyFromBase58(models.RaydiumCLMMProgramID), cpmm, aux); err == nil {
		t.Fatal("the same bytes must not quote as both venues — dispatch is not using the owner")
	}
}

func TestFromAccountRefusesUnknownProgram(t *testing.T) {
	_, err := FromAccount(solana.MustPublicKeyFromBase58("11111111111111111111111111111112"), []byte{1, 2, 3}, Aux{})
	if !errors.Is(err, ErrUnknownProgram) {
		t.Fatalf("err = %v, want ErrUnknownProgram", err)
	}
}

// A venue whose required Aux is missing must be refused, not quoted on the zero
// value — a zero fee rate over-states every output.
func TestFromAccountRefusesMissingAux(t *testing.T) {
	cpmm := make([]byte, 8+637)
	copy(cpmm[0:8], models.RaydiumCPMMPoolDiscriminator[:])

	_, err := FromAccount(solana.MustPublicKeyFromBase58(models.RaydiumCPMMProgramID), cpmm,
		Aux{Vault0: 1_000_000, Vault1: 1_000_000}) // no CPMMConfig
	if !errors.Is(err, ErrPoolNotQuotable) {
		t.Fatalf("err = %v, want ErrPoolNotQuotable for a missing AmmConfig", err)
	}
}

// Every program this package claims to dispatch on must actually be routed.
func TestFromAccountCoversEveryVenue(t *testing.T) {
	for _, pid := range []string{
		MeteoraDLMMProgramID, MeteoraDAMMV2ProgramID, models.OrcaWhirlpoolProgramID,
		models.RaydiumCLMMProgramID, models.RaydiumCPMMProgramID,
		PumpAMMProgramID, PumpBondingProgramID,
	} {
		// Empty data: every venue should fail to DECODE, never fall through to
		// ErrUnknownProgram, which would mean the case is missing.
		_, err := FromAccount(solana.MustPublicKeyFromBase58(pid), nil, Aux{})
		if errors.Is(err, ErrUnknownProgram) {
			t.Fatalf("%s is not routed", pid)
		}
		if err == nil {
			t.Fatalf("%s decoded empty data", pid)
		}
	}
}
