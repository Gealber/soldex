package raycpmm

import "testing"

// Mainnet SwapEvents: reserves are the event's own pre-swap net vaults, rates the pool's AmmConfig.
// One of each mode and direction, each sized so fee-on-input and fee-on-output give different outputs.
var chainVectors = []struct {
	name                       string
	creatorFeeOn               uint8
	zeroForOne                 bool
	tradeFeeRate, creatorRate  uint64
	reserveIn, reserveOut, in  uint64
	out                        uint64
	eventCreatorFeeOnInputFlag bool
}{
	// 58t8hNJYCJgVDmG4g4g3XZBPTXFAAQatBcL5EB1jdZyrXzL5JL3vkawA72siKKpr5Rt153f1s99jCKxyPajpxVh5
	{"both/0to1", CreatorFeeOnBothToken, true, 3_000, 12_000, 841_298_616_389, 721_291_230_647_310, 604_033_161, 509_742_050_393, true},
	// 5v9nd9Zp7R1ZYjtcNPE43j4LPmHzciMjQLjiUmV4mfqchfrYBDJwzxKRrosCVn29ZqSn9Z1GYSChbQdEsmgYXzzU
	{"only0/0to1", CreatorFeeOnOnlyToken0, true, 2_500, 10_000, 2_862_786_131, 400_790_698_630_024, 71_473_302, 9_643_447_583_452, true},
	// 42LetUBwq1iwBDPrt36q7Q13fjReCesJeiYULt7JGdsSaDnTecgj1AvHpWiDW3uRFYLeR93RhCmpBQfcUKmQRu9u
	{"only0/1to0", CreatorFeeOnOnlyToken0, false, 2_500, 10_000, 448_378_014_144_768, 29_250_433_340, 22_301_536_139_072, 1_368_804_104, false},
	// 4FHe9ewrAPVbq1sSfz4VGzo2yTJabWS875s2oVJ21DhPNB2e6DnbPZRv2cQSsmYDwdVfAm2z14sT1TUc2mbKZiZj
	{"only1/1to0", CreatorFeeOnOnlyToken1, false, 2_500, 10_000, 86_564_434, 812_878_963_394_729, 1_541_350, 14_046_068_494_690, true},
	// 2F8bZdvLABkdsKamt4tPgP3yPPrAyKxjZFgYe18HfMKrWPTVmnmmBk59jEQYJwq7iR5Ym7pCVszkeZSq7yhChU2S
	{"only1/0to1", CreatorFeeOnOnlyToken1, true, 2_500, 10_000, 443_687_259_845_800, 974_360_261_566, 18_323_326_366_986, 38_164_810_911, false},
}

func TestSwapBaseInputMatchesChain(t *testing.T) {
	for _, vec := range chainVectors {
		t.Run(vec.name, func(t *testing.T) {
			onInput, err := CreatorFeeOnInput(vec.creatorFeeOn, vec.zeroForOne)
			if err != nil {
				t.Fatal(err)
			}
			if onInput != vec.eventCreatorFeeOnInputFlag {
				t.Fatalf("CreatorFeeOnInput = %v, the program's event says %v", onInput, vec.eventCreatorFeeOnInputFlag)
			}
			if got := SwapBaseInput(vec.reserveIn, vec.reserveOut, vec.in, vec.tradeFeeRate, vec.creatorRate, onInput); got != vec.out {
				t.Fatalf("out = %d, chain paid %d", got, vec.out)
			}
			if other := SwapBaseInput(vec.reserveIn, vec.reserveOut, vec.in, vec.tradeFeeRate, vec.creatorRate, !onInput); other == vec.out {
				t.Fatalf("the other fee side also gives %d, so this vector cannot tell them apart", other)
			}
		})
	}
}

func TestCreatorFeeOnInputRefusesUnknownMode(t *testing.T) {
	if _, err := CreatorFeeOnInput(3, true); err == nil {
		t.Fatal("mode 3 accepted")
	}
}
