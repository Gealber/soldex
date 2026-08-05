package soldex

import (
	"testing"

	"github.com/Gealber/soldex/models"
)

// Golden vectors captured 2026-07-25 from simulateTransaction against live Pump-AMM pools.
// Each one pairs the pool state the PROGRAM reported in its own BuyEvent (pre-swap reserves and
// user_quote_amount_in, the amount that actually reaches the curve) with the base it paid out.
// Because the input is already net of fees, feeBps is 0 here and the vector isolates the reserves.
//
// The pools differ only in models.PumpPool.VirtualQuoteReserves: a quote-side reserve the program
// prices with that is NOT held in the quote vault. Quoting on the vault balance alone reads the
// pool as far shallower than it is, and over-predicts the base a buy returns by an amount that
// grows as the pool shrinks — +5.5% at 317 SOL of quote, +775% at 2.2 SOL.
var pumpChainFills = []struct {
	name              string
	base, quote       uint64
	extraQuoteReserve uint64
	curveIn           uint64
	chainOut          uint64
	uncorrectedErr    string
}{
	{
		name: "GdN39 317 SOL quote", base: 99_597_894_357_464, quote: 317_089_814_891,
		extraQuoteReserve: 17_584_505_393, curveIn: 98_911_968, chainOut: 29_427_154_535,
		uncorrectedErr: "+5.54%",
	},
	{
		name: "3FPMt 2.2 SOL quote", base: 958_225_176_246_517, quote: 2_168_623_052,
		extraQuoteReserve: 17_584_505_290, curveIn: 98_765_430, chainOut: 4_767_279_217_907,
		uncorrectedErr: "+775.54%",
	},
	{
		// The older cohort carries no offset; it must keep quoting exactly as before.
		name: "9Hoqo no offset", base: 656_938_473_146_389, quote: 38_683_627_238,
		extraQuoteReserve: 0, curveIn: 98_765_430, chainOut: 1_672_996_575_637,
		uncorrectedErr: "+0.00%",
	},
}

// tolerance is generous next to the defect (5.5%-775%) but tight enough to catch it. Pump's own
// curve leaves a ~0.001% residual against this formula even on a zero-offset pool, so exact
// integer equality is not achievable and would make the test flap.
const pumpQuoteTolerance = 0.0005

func TestPumpQuoteMatchesChainWithVirtualQuoteReserves(t *testing.T) {
	for _, v := range pumpChainFills {
		t.Run(v.name, func(t *testing.T) {
			pool := &models.PumpPool{VirtualQuoteReserves: int64(v.extraQuoteReserve)}
			out, err := Pump(v.base, pool.EffectiveQuoteReserve(v.quote), 0).QuoteExactIn(v.curveIn, false)
			if err != nil {
				t.Fatalf("quote: %v", err)
			}
			relErr := float64(out)/float64(v.chainOut) - 1
			if relErr > pumpQuoteTolerance || relErr < -pumpQuoteTolerance {
				t.Fatalf("quote %d vs chain %d: off %+.4f%%, want within %+.4f%% "+
					"(quoting the vault balance alone is off %s)",
					out, v.chainOut, relErr*100, pumpQuoteTolerance*100, v.uncorrectedErr)
			}
		})
	}
}

// The offset only ever adds quote-side depth, so omitting it can only ever over-predict a buy.
// This pins the direction: a silent regression that drops the field manufactures profit rather
// than hiding it, which is why it produced phantom arbitrage rather than missed trades.
func TestPumpVirtualQuoteReservesOnlyReduceBuyOutput(t *testing.T) {
	v := pumpChainFills[1] // the 2.2 SOL pool, where the offset dominates
	pool := &models.PumpPool{VirtualQuoteReserves: int64(v.extraQuoteReserve)}

	withOffset, _ := Pump(v.base, pool.EffectiveQuoteReserve(v.quote), 0).QuoteExactIn(v.curveIn, false)
	vaultOnly, _ := Pump(v.base, v.quote, 0).QuoteExactIn(v.curveIn, false)
	if vaultOnly <= withOffset {
		t.Fatalf("vault-only quote %d must exceed the corrected quote %d", vaultOnly, withOffset)
	}
	if vaultOnly < v.chainOut*2 {
		t.Fatalf("vault-only quote %d should be wildly optimistic against chain %d", vaultOnly, v.chainOut)
	}
}
