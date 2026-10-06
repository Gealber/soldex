package soldex

import (
	"encoding/base64"
	"errors"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/orca"
)

// Fogo mainnet Vortex pool J7mxBLSz51Tcbog3XsiJTAXS64N46KqbpRGQmd3dQMKp (WSOL/USDC) at slot 793594098. A
// simulation of the deployed program on this state paid 18,201,442,236,295 for 100,000,000 USDC b-to-a.
const vortexVectorAccount = "gkmPVrR9i6sKSX0aXceFSnxZ+JNdYXEPNbwOQmTIrCqF8k4M7Y33ov9AAEAAuAvQBxcOwUJDAgAAAAAAAAAAAADbLPXztDSYAAAAAAAAAAAAIyb+/0pFkkAAAAAAHx8AAAAAAAAGm4hX/quBhPtof2NGGMA12sQ53BrrO1WYoPAAAAAAAT+2etF4d2TE7yF7Fm8Q3pMlAYolqwOS8J6ySnADvjIsmZDJD2BqgzM9AAAAAAAAAA1vLAmuTFWK7+R+sxcyP/gasjZY6otdfYHDctfP/P9PvEio6To0Mwryd9G2/StUo+dyry2GDJSNin6IwoPIvX7FvijpDe82AAAAAAAAAAAAhQ/FagAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAS0JTcf0wOZqWJHo8FnQN4fTG6mN+LbamnI91DCE2f10AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLQlNx/TA5mpYkejwWdA3h9MbqY34ttqacj3UMITZ/XQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEtCU3H9MDmaliR6PBZ0DeH0xupjfi22ppyPdQwhNn9dAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

const (
	vortexVectorAmountIn = uint64(100_000_000)
	vortexVectorExpected = uint64(18_201_442_236_295)
	// vortexVectorArrayStart holds the swap's whole walk; the next two swap arrays are never reached.
	vortexVectorArrayStart = int32(-123904)
)

// vortexVectorNets are the initialized ticks in the walked array, as the program read them.
var vortexVectorNets = map[int32]int64{
	-121920: 56057879903, -121088: 9634044737113, -120768: 201103030962, -120704: 68751911473,
	-120384: 919105793, -120128: -9634044737113, -120064: -269660602596, -119808: 77062008086,
	-119232: 1979755053, -119040: -56057879903, -118976: 301598519156, -118464: -378854867081,
}

func vortexVector(t *testing.T) ([]byte, orca.TickProvider) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(vortexVectorAccount)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var ticks orca.TickArray
	for tick, net := range vortexVectorNets {
		hi := uint64(0)
		if net < 0 {
			hi = ^uint64(0)
		}
		ticks[(tick-vortexVectorArrayStart)/64] = orca.ArrayTick{Initialized: true, LiquidityNet: bin.Int128{Lo: uint64(net), Hi: hi}}
	}
	loader := func(start int32) (orca.TickArray, bool, error) {
		return ticks, start == vortexVectorArrayStart, nil
	}
	walker := orca.NewTickArrayWalker(-121309, 64, false, loader)
	return raw, walker.Next
}

func TestVortexChainVector(t *testing.T) {
	raw, ticks := vortexVector(t)
	quoter, err := FromAccount(solana.MustPublicKeyFromBase58(models.ValiantVortexProgramID), raw, Aux{OrcaTicks: ticks, Now: 1})
	if err != nil {
		t.Fatalf("FromAccount: %v", err)
	}
	out, err := quoter.QuoteExactIn(vortexVectorAmountIn, false)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if out != vortexVectorExpected {
		t.Fatalf("out = %d, want %d", out, vortexVectorExpected)
	}

	// The swap crosses -121088, so a quote that never reads the ticks must come out different.
	noTicks, err := FromAccount(solana.MustPublicKeyFromBase58(models.ValiantVortexProgramID), raw, Aux{OrcaTicks: orca.NewTickArrayWalker(-121309, 64, false, func(int32) (orca.TickArray, bool, error) { return orca.TickArray{}, true, nil }).Next, Now: 1})
	if err != nil {
		t.Fatalf("FromAccount without ticks: %v", err)
	}
	if flat, _ := noTicks.QuoteExactIn(vortexVectorAmountIn, false); flat == out {
		t.Fatalf("the vector never crossed an initialized tick")
	}
}

// One byte changed from the live pool: a set extension, then an adaptive fee tier, each refused.
func TestFromVortexRefusesWhatWasNotVerified(t *testing.T) {
	raw, ticks := vortexVector(t)
	for name, offset := range map[string]int{"extension": 653, "fee tier index": 43} {
		changed := append([]byte(nil), raw...)
		changed[offset]++
		pool, err := models.DecodeVortex(changed, solana.PublicKey{})
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if _, err := FromVortex(pool, ticks, 1); !errors.Is(err, ErrPoolNotQuotable) {
			t.Fatalf("%s: err = %v, want ErrPoolNotQuotable", name, err)
		}
	}
}
