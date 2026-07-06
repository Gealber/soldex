package models

import (
	"math/big"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestPumpTotalFeeBps(t *testing.T) {
	g := &PumpGlobalConfig{LpBps: 20, ProtocolBps: 5, CoinCreatorBps: 5}
	fc := &PumpFeeConfig{
		Flat: PumpFees{LpBps: 25, ProtocolBps: 5, CreatorBps: 0}, // non-pump pools
		Tiers: []PumpFeeTier{
			{MarketCapThreshold: big.NewInt(0), Fees: PumpFees{LpBps: 2, ProtocolBps: 93, CreatorBps: 30}},
			{MarketCapThreshold: big.NewInt(1000), Fees: PumpFees{LpBps: 20, ProtocolBps: 5, CreatorBps: 90}},
			{MarketCapThreshold: big.NewInt(5000), Fees: PumpFees{LpBps: 20, ProtocolBps: 5, CreatorBps: 50}},
		},
	}

	base := solana.MustPublicKeyFromBase58("48oT2QgpyPFUiJYXbvZTS1tVDp98R9uYAJX5ojNrpump")
	authority, _, _ := solana.FindProgramAddress([][]byte{[]byte("pool-authority"), base.Bytes()}, pumpBondingProgram)

	// market cap = quote*supply/base. Pick cap = 2000 -> tier index 1 (creator 90).
	const q, b, sup = 2000, 1, 1

	// Graduated pump pool, has coin creator, cashback ON -> STILL the full tier fee
	// (cashback is rebated separately, does not lower the swap fee) = 20+5+90 = 115.
	cashback := &PumpPool{BaseMint: base, Creator: authority, CoinCreator: base, IsCashbackCoin: true}
	if got := PumpTotalFeeBps(g, fc, cashback, b, q, sup); got != 115 {
		t.Fatalf("cashback graduate: got %d, want 115", got)
	}

	// No coin creator -> creator component dropped = 20+5 = 25.
	noCreator := &PumpPool{BaseMint: base, Creator: authority, CoinCreator: solana.PublicKey{}}
	if got := PumpTotalFeeBps(g, fc, noCreator, b, q, sup); got != 25 {
		t.Fatalf("no-creator graduate: got %d, want 25", got)
	}

	// Not a graduated pool (creator != pool-authority PDA) -> flat fees = 25+5 = 30.
	notGraduate := &PumpPool{BaseMint: base, Creator: base, CoinCreator: base}
	if got := PumpTotalFeeBps(g, fc, notGraduate, b, q, sup); got != 30 {
		t.Fatalf("non-graduate flat: got %d, want 30", got)
	}

	// Below the first tier threshold uses tier 0 (2+93+30 = 125).
	tiny := PumpTotalFeeBps(g, &PumpFeeConfig{Tiers: fc.Tiers}, cashback, 1_000_000, 1, 0) // cap 0
	if tiny != 125 {
		t.Fatalf("below-tier0: got %d, want 125", tiny)
	}
}
