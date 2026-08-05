package models

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// pumpPoolAccount builds a Pool account with sentinels at each decoded offset, so a field
// read from the wrong place surfaces as the wrong sentinel rather than plausible garbage.
func pumpPoolAccount(virtualQuote int64, size int) []byte {
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
	if size >= pumpPoolVirtualQuoteEnd {
		putInt128(d[245:261], virtualQuote)
	}
	return d
}

// putInt128 writes v as a little-endian signed 128-bit integer, sign-extending the
// high word so negative values round-trip.
func putInt128(b []byte, v int64) {
	binary.LittleEndian.PutUint64(b[0:8], uint64(v))
	var hi uint64
	if v < 0 {
		hi = ^uint64(0)
	}
	binary.LittleEndian.PutUint64(b[8:16], hi)
}

func TestDecodePumpPoolVirtualQuoteReserves(t *testing.T) {
	const extra = 17_584_505_393
	addr := solana.MustPublicKeyFromBase58("GdN39oNzALYnGfBSyn6DF4sauTKpfWGyNiJiNYoDqV6w")

	p, err := DecodePumpPool(pumpPoolAccount(extra, 301), addr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.VirtualQuoteReserves != extra {
		t.Fatalf("VirtualQuoteReserves = %d, want %d", p.VirtualQuoteReserves, extra)
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
func TestDecodePumpPoolWithoutVirtualQuoteReserves(t *testing.T) {
	p, err := DecodePumpPool(pumpPoolAccount(0, 245), solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode of a pre-offset account: %v", err)
	}
	if p.VirtualQuoteReserves != 0 {
		t.Fatalf("VirtualQuoteReserves = %d, want 0", p.VirtualQuoteReserves)
	}
	if got := p.EffectiveQuoteReserve(1_234); got != 1_234 {
		t.Fatalf("EffectiveQuoteReserve = %d, want the vault balance unchanged", got)
	}
}

// Every account size live on chain must decode. Rejecting the two short cohorts dropped
// 110,317 of the 213,534 pools that exist, so the common case here is a SHORT account.
func TestDecodePumpPoolEveryLiveCohort(t *testing.T) {
	for _, tc := range []struct {
		size            int
		wantCoinCreator bool
		wantCashback    bool
		wantVirtual     int64
	}{
		{size: 211, wantCoinCreator: false, wantCashback: false, wantVirtual: 0},
		{size: 243, wantCoinCreator: true, wantCashback: false, wantVirtual: 0},
		{size: 245, wantCoinCreator: true, wantCashback: true, wantVirtual: 0},
		{size: 261, wantCoinCreator: true, wantCashback: true, wantVirtual: 17_584_505_393},
	} {
		p, err := DecodePumpPool(pumpPoolAccount(17_584_505_393, tc.size), solana.PublicKey{})
		if err != nil {
			t.Fatalf("size %d: decode: %v", tc.size, err)
		}
		if got := p.BaseMint[0]; got != 0x43 {
			t.Fatalf("size %d: BaseMint first byte %#x, want 0x43", tc.size, got)
		}
		if got := !p.CoinCreator.IsZero(); got != tc.wantCoinCreator {
			t.Fatalf("size %d: CoinCreator present = %v, want %v", tc.size, got, tc.wantCoinCreator)
		}
		if p.IsCashbackCoin != tc.wantCashback {
			t.Fatalf("size %d: IsCashbackCoin = %v, want %v", tc.size, p.IsCashbackCoin, tc.wantCashback)
		}
		if p.VirtualQuoteReserves != tc.wantVirtual {
			t.Fatalf("size %d: VirtualQuoteReserves = %d, want %d", tc.size, p.VirtualQuoteReserves, tc.wantVirtual)
		}
	}
}

// virtual_quote_reserves is an i128. Read as a u64 a negative value becomes ~1.8e19
// lamports of depth that is not there, so the sign has to survive the decode.
func TestDecodePumpPoolNegativeVirtualQuoteReserves(t *testing.T) {
	const virtual = -5_000_000_000
	p, err := DecodePumpPool(pumpPoolAccount(virtual, 261), solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.VirtualQuoteReserves != virtual {
		t.Fatalf("VirtualQuoteReserves = %d, want %d", p.VirtualQuoteReserves, virtual)
	}
	if got := p.EffectiveQuoteReserve(8_000_000_000); got != 3_000_000_000 {
		t.Fatalf("EffectiveQuoteReserve = %d, want 3000000000", got)
	}
	// A deficit larger than the vault saturates at 0 instead of wrapping.
	if got := p.EffectiveQuoteReserve(1_000); got != 0 {
		t.Fatalf("EffectiveQuoteReserve under-water = %d, want 0", got)
	}
}

// A value the int64 field cannot hold must be reported, not truncated into a
// plausible-looking reserve.
func TestDecodePumpPoolVirtualQuoteReservesOutOfRange(t *testing.T) {
	d := pumpPoolAccount(0, 261)
	binary.LittleEndian.PutUint64(d[253:261], 7) // high word far past int64
	if _, err := DecodePumpPool(d, solana.PublicKey{}); !errors.Is(err, ErrValueOutOfRange) {
		t.Fatalf("err = %v, want ErrValueOutOfRange", err)
	}
}

// Saturating up matters as much as saturating down: the sum must not wrap to a tiny
// reserve, which would read as a pool with almost no depth.
func TestPumpPoolEffectiveQuoteReserveSaturatesUp(t *testing.T) {
	p := &PumpPool{VirtualQuoteReserves: math.MaxInt64}
	if got := p.EffectiveQuoteReserve(math.MaxUint64); got != math.MaxUint64 {
		t.Fatalf("EffectiveQuoteReserve = %d, want MaxUint64", got)
	}
}

// A nil pool must not silently drop the caller's vault balance.
func TestPumpPoolEffectiveQuoteReserveNil(t *testing.T) {
	var p *PumpPool
	if got := p.EffectiveQuoteReserve(999); got != 999 {
		t.Fatalf("nil pool EffectiveQuoteReserve = %d, want 999", got)
	}
}
