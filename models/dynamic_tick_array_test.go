package models

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func putI128(buf *bytes.Buffer, lo, hi uint64) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:8], lo)
	binary.LittleEndian.PutUint64(b[8:16], hi)
	buf.Write(b)
}

// TestDecodeDynamicTickArray builds a dynamic tick array with two initialized
// ticks (a positive and a negative liquidity_net) and checks the decode.
func TestDecodeDynamicTickArray(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(DynamicTickArrayDiscriminator[:])
	_ = binary.Write(&buf, binary.LittleEndian, int32(-14080)) // start_tick_index
	whirlpool := solana.NewWallet().PublicKey()
	buf.Write(whirlpool[:])     // whirlpool
	buf.Write(make([]byte, 16)) // tick_bitmap u128

	for i := 0; i < TicksPerArray; i++ {
		switch i {
		case 5: // liquidity_net = 12345
			buf.WriteByte(1)
			putI128(&buf, 12345, 0)
			buf.Write(make([]byte, dynamicTickDataRest))
		case 10: // liquidity_net = -7
			buf.WriteByte(1)
			putI128(&buf, ^uint64(6), ^uint64(0))
			buf.Write(make([]byte, dynamicTickDataRest))
		default:
			buf.WriteByte(0) // uninitialized
		}
	}

	addr := solana.NewWallet().PublicKey()
	ta, err := DecodeDynamicTickArray(buf.Bytes(), addr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ta.StartTickIndex != -14080 || !ta.Whirlpool.Equals(whirlpool) {
		t.Fatalf("header wrong: start=%d whirlpool=%s", ta.StartTickIndex, ta.Whirlpool)
	}
	if !ta.Ticks[5].Initialized || ta.Ticks[5].LiquidityNet.BigInt().Int64() != 12345 {
		t.Fatalf("tick 5 = %+v", ta.Ticks[5])
	}
	if !ta.Ticks[10].Initialized || ta.Ticks[10].LiquidityNet.BigInt().Int64() != -7 {
		t.Fatalf("tick 10 = %+v", ta.Ticks[10])
	}
	if ta.Ticks[0].Initialized || ta.Ticks[87].Initialized {
		t.Fatal("uninitialized ticks must stay zero")
	}

	// DecodeAnyTickArray must route by discriminator.
	if _, err := DecodeAnyTickArray(buf.Bytes(), addr); err != nil {
		t.Fatalf("DecodeAnyTickArray: %v", err)
	}
}
