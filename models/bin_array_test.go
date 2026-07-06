package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func TestDecodeBinArrayLayout(t *testing.T) {
	// 8 disc + 48 header + 70 bins * 144 bytes each.
	const accountSize = 8 + 48 + MaxBinPerArray*144
	data := make([]byte, accountSize)
	copy(data[0:8], BinArrayDiscriminator[:])

	// index = 3 at offset 8 (i64 LE).
	binary.LittleEndian.PutUint64(data[8:], 3)
	lbPair := sentinelKey(0x55)
	copy(data[8+16:], lbPair[:]) // lb_pair at struct offset 16

	// bins start at struct offset 48 (account offset 56), stride 144.
	// Put amount_x/amount_y into bin index 2.
	const binsStart = 8 + 48
	bin2 := binsStart + 2*144
	binary.LittleEndian.PutUint64(data[bin2+0:], 123_456) // amount_x
	binary.LittleEndian.PutUint64(data[bin2+8:], 789_012) // amount_y

	ba, err := DecodeBinArray(data, solana.PublicKey{})
	if err != nil {
		t.Fatalf("DecodeBinArray: %v", err)
	}
	if ba.Index != 3 {
		t.Errorf("Index = %d, want 3", ba.Index)
	}
	if !ba.LbPair.Equals(lbPair) {
		t.Errorf("LbPair = %s, want %s", ba.LbPair, lbPair)
	}

	// array index 3 covers bin ids [210, 279]; bin slot 2 is bin id 212.
	got, ok := ba.BinAt(212)
	if !ok {
		t.Fatal("BinAt(212) not found")
	}
	if got.AmountX != 123_456 || got.AmountY != 789_012 {
		t.Errorf("bin 212 = (%d, %d), want (123456, 789012)", got.AmountX, got.AmountY)
	}
}

func TestBinIDToArrayIndex(t *testing.T) {
	cases := []struct {
		binID int32
		want  int64
	}{
		{0, 0}, {69, 0}, {70, 1}, {-1, -1}, {-70, -1}, {-71, -2}, {140, 2},
	}
	for _, c := range cases {
		if got := BinIDToArrayIndex(c.binID); got != c.want {
			t.Errorf("BinIDToArrayIndex(%d) = %d, want %d", c.binID, got, c.want)
		}
	}
}
