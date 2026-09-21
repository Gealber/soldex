package fluxbeam

import (
	"math/big"
	"testing"

	"github.com/Gealber/soldex/models"
)

// The commonest fee shape on chain: trade 2/1000, owner 90/100.
func extremeFees() models.FluxBeamFees {
	return models.FluxBeamFees{
		TradeFeeNumerator: 2, TradeFeeDenominator: 1000,
		OwnerTradeFeeNumerator: 90, OwnerTradeFeeDenominator: 100,
		OwnerWithdrawFeeNumerator: 98, OwnerWithdrawFeeDenominator: 100,
		HostFeeNumerator: 0, HostFeeDenominator: 10_000,
	}
}

// A sane pool: trade 20/10000, owner 5/10000.
func normalFees() models.FluxBeamFees {
	return models.FluxBeamFees{
		TradeFeeNumerator: 20, TradeFeeDenominator: 10_000,
		OwnerTradeFeeNumerator: 5, OwnerTradeFeeDenominator: 10_000,
		HostFeeNumerator: 0, HostFeeDenominator: 10_000,
	}
}

// The owner trade fee comes off the input just like the trade fee. Skipping it
// is not a rounding error — on the commonest live fee shape it is 90% of the
// input.
func TestOwnerTradeFeeComesOffTheInput(t *testing.T) {
	const r0, r1, in = uint64(1_000_000_000), uint64(1_000_000_000), uint64(10_000_000)

	both, err := SwapExactIn(r0, r1, in, extremeFees())
	if err != nil {
		t.Fatalf("%v", err)
	}

	tradeOnly := extremeFees()
	tradeOnly.OwnerTradeFeeNumerator = 0
	without, err := SwapExactIn(r0, r1, in, tradeOnly)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if both >= without {
		t.Fatalf("owner fee not applied: %d with, %d without", both, without)
	}
	// Roughly a tenth gets through, since 90.2% of the input is taken.
	if ratio := float64(both) / float64(without); ratio > 0.15 {
		t.Fatalf("owner fee barely moved the quote (ratio %.3f); expected ~0.1", ratio)
	}
}

// Neither the withdraw fee nor the host fee touches a swap.
func TestWithdrawAndHostFeesDoNotAffectSwaps(t *testing.T) {
	const r0, r1, in = uint64(1_000_000_000), uint64(1_000_000_000), uint64(10_000_000)

	base, err := SwapExactIn(r0, r1, in, normalFees())
	if err != nil {
		t.Fatalf("%v", err)
	}

	loaded := normalFees()
	loaded.OwnerWithdrawFeeNumerator, loaded.OwnerWithdrawFeeDenominator = 99, 100
	loaded.HostFeeNumerator, loaded.HostFeeDenominator = 50, 100

	got, err := SwapExactIn(r0, r1, in, loaded)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got != base {
		t.Fatalf("withdraw/host fees changed the quote: %d vs %d", got, base)
	}
}

// A fee that rounds down to zero is charged as one token.
func TestFeeMinimumIsOneToken(t *testing.T) {
	if got := calculateFee(1, 20, 10_000); got.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("tiny fee = %s, want the 1-token minimum", got)
	}
	if got := calculateFee(1_000_000, 0, 10_000); got.Sign() != 0 {
		t.Fatalf("zero numerator = %s, want no fee", got)
	}
	if got := calculateFee(0, 20, 10_000); got.Sign() != 0 {
		t.Fatalf("zero amount = %s, want no fee", got)
	}
}

func TestSwapExactInBasics(t *testing.T) {
	const r0, r1 = uint64(1_000_000_000), uint64(2_000_000_000)

	out, err := SwapExactIn(r0, r1, 1_000_000, normalFees())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if out == 0 {
		t.Fatal("expected a positive output")
	}

	// Price impact: a bigger swap returns a worse rate.
	big1, err := SwapExactIn(r0, r1, 100_000_000, normalFees())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if float64(big1)/100_000_000 >= float64(out)/1_000_000 {
		t.Fatal("expected the larger swap to price worse")
	}

	// A swap too small to move the destination token is an error, not a zero.
	if _, err := SwapExactIn(r0, r1, 1, extremeFees()); err != ErrZeroOutput {
		t.Fatalf("err = %v, want ErrZeroOutput", err)
	}
}
