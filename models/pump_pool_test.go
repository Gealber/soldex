package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// pumpPoolAccount builds a Pool account with sentinels at each decoded offset, so a field
// read from the wrong place surfaces as the wrong sentinel rather than plausible garbage.
func pumpPoolAccount(extraQuote uint64, size int) []byte {
	d := make([]byte, size)
	copy(d, PumpPoolDiscriminator[:])
	fill := func(off int, b byte) {
		for i := off; i < off+32 && i < len(d); i++ {
			d[i] = b
		}
	}
	fill(11, 0x11)  // creator
	fill(43, 0x43)  // base_mint
	fill(75, 0x75)  // quote_mint
	fill(139, 0x8b) // pool_base_token_account
	fill(171, 0xab) // pool_quote_token_account
	fill(211, 0xd3) // coin_creator
	if size > 244 {
		d[244] = 1 // is_cashback_coin
	}
	if size >= 253 {
		binary.LittleEndian.PutUint64(d[245:253], extraQuote)
	}
	return d
}

func TestDecodePumpPoolExtraQuoteReserve(t *testing.T) {
	const extra = 17_584_505_393
	addr := solana.MustPublicKeyFromBase58("GdN39oNzALYnGfBSyn6DF4sauTKpfWGyNiJiNYoDqV6w")

	p, err := DecodePumpPool(pumpPoolAccount(extra, 301), addr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.ExtraQuoteReserve != extra {
		t.Fatalf("ExtraQuoteReserve = %d, want %d", p.ExtraQuoteReserve, extra)
	}
	if !p.IsCashbackCoin {
		t.Fatal("IsCashbackCoin should still decode from offset 244")
	}
	if got := p.BaseMint[0]; got != 0x43 {
		t.Fatalf("BaseMint read from the wrong offset: first byte %#x", got)
	}
	if got := p.EffectiveQuoteReserve(1_000); got != 1_000+extra {
		t.Fatalf("EffectiveQuoteReserve = %d, want %d", got, 1_000+extra)
	}
}

// The offset postdates the original layout. An account too short to carry it must decode as
// zero rather than error: that is the older cohort, which quotes correctly on the vault balance.
func TestDecodePumpPoolWithoutExtraQuoteReserve(t *testing.T) {
	p, err := DecodePumpPool(pumpPoolAccount(0, 245), solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode of a pre-offset account: %v", err)
	}
	if p.ExtraQuoteReserve != 0 {
		t.Fatalf("ExtraQuoteReserve = %d, want 0", p.ExtraQuoteReserve)
	}
	if got := p.EffectiveQuoteReserve(1_234); got != 1_234 {
		t.Fatalf("EffectiveQuoteReserve = %d, want the vault balance unchanged", got)
	}
}

// A nil pool must not silently drop the caller's vault balance.
func TestPumpPoolEffectiveQuoteReserveNil(t *testing.T) {
	var p *PumpPool
	if got := p.EffectiveQuoteReserve(999); got != 999 {
		t.Fatalf("nil pool EffectiveQuoteReserve = %d, want 999", got)
	}
}
