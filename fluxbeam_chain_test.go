package soldex

import (
	"encoding/base64"
	"testing"

	"github.com/gagliardetto/solana-go"

	"github.com/Gealber/soldex/models"
)

// A real FluxBeam swap, checked against what the program actually paid out.
//
// Transaction 5oNv1nHLuSZPBZjA3hAwjF7tqUoXCX9Xcc4sgABodAc9mgeyK8WtGegCn7MZ67Eqc9PGkXajxfEB8pSWtEZ2YqRx
// on pool 93FMRPyhLMSGsDLviY1uL6ea8fVTtbBtKX8EaNcJndhc: 273,000,000 WSOL in, and the pool's
// token-B vault fell by exactly the amount asserted below. Reserves are the
// vault balances immediately before the swap, taken from the transaction's own
// pre-token-balances, so nothing here depends on a simulation.
//
// The pool charges trade 20/10000 AND owner 99/100. The owner fee alone takes
// 270,270,000 of the 273,000,000 input, leaving 2,184,000 to reach the curve —
// which is why TestFluxBeamChainVectorOwnerFeeIsLoadBearing exists.
const (
	fluxBeamVectorAccount = "AQH/Bt324e51j94YQl285GzN2rYa/E2DuQ0n/r35KNihi/zlkw0DFPv1/lAWj0vjZLcHxz5lBR8hfd1IaYH615D1vfvvUdIY7MRZHy2QjX5lFnfeOkt+DxhhhvyjLIQKh2i2kjWtuLfqhRSVkc1/QbaokY0JX6uKRRikfLKAl4K7V9sGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAAcf7oeKfhyUwjIby2PBE7YmNampXjesv9/vT9+yVjAqqs5SNS1PlPWI0cP1UMfRffitKEYRCIVdFqxp9G+TrALsUAAAAAAAAABAnAAAAAAAAYwAAAAAAAABkAAAAAAAAAGMAAAAAAAAAZAAAAAAAAAAAAAAAAAAAABAnAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	fluxBeamVectorReserveIn  = uint64(63974000000)
	fluxBeamVectorReserveOut = uint64(985161102423463)
	fluxBeamVectorAmountIn   = uint64(273000000)
	fluxBeamVectorExpected   = uint64(33631137607)
)

func fluxBeamVectorPool(t *testing.T) *models.FluxBeamPool {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(fluxBeamVectorAccount)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	pool, err := models.DecodeFluxBeamPool(raw, solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	return pool
}

func TestFluxBeamQuoteMatchesChain(t *testing.T) {
	q, err := FromFluxBeamPool(fluxBeamVectorPool(t), FluxBeamSide{Reserve: fluxBeamVectorReserveIn}, FluxBeamSide{Reserve: fluxBeamVectorReserveOut}, 0)
	if err != nil {
		t.Fatalf("%v", err)
	}

	got, err := q.QuoteExactIn(fluxBeamVectorAmountIn, true)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got != fluxBeamVectorExpected {
		t.Fatalf("quote = %d, the pool paid out %d (off by %+d)",
			got, fluxBeamVectorExpected, int64(got)-int64(fluxBeamVectorExpected))
	}
}

// The match only means something because the owner fee dominates this swap.
// Dropping it leaves 272,454,000 reaching the curve instead of 2,184,000, so the
// quote comes back around a hundred times too high. If a future change makes the
// owner fee inert on this vector, the match above stops proving anything.
func TestFluxBeamChainVectorOwnerFeeIsLoadBearing(t *testing.T) {
	pool := fluxBeamVectorPool(t)

	full, err := FromFluxBeamPool(pool, FluxBeamSide{Reserve: fluxBeamVectorReserveIn}, FluxBeamSide{Reserve: fluxBeamVectorReserveOut}, 0)
	if err != nil {
		t.Fatalf("%v", err)
	}
	withOwner, err := full.QuoteExactIn(fluxBeamVectorAmountIn, true)
	if err != nil {
		t.Fatalf("%v", err)
	}

	noOwner := *pool
	noOwner.Fees.OwnerTradeFeeNumerator = 0
	q, err := FromFluxBeamPool(&noOwner, FluxBeamSide{Reserve: fluxBeamVectorReserveIn}, FluxBeamSide{Reserve: fluxBeamVectorReserveOut}, 0)
	if err != nil {
		t.Fatalf("%v", err)
	}
	withoutOwner, err := q.QuoteExactIn(fluxBeamVectorAmountIn, true)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if withoutOwner <= withOwner {
		t.Fatal("owner fee is inert on this vector — the chain match proves nothing about it")
	}
	if ratio := float64(withoutOwner) / float64(withOwner); ratio < 50 {
		t.Fatalf("owner fee moves the quote only %.1fx; expected ~100x on this pool", ratio)
	}
}
