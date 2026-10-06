package models

import (
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// Valiant's Vortex program on Fogo, an Orca Whirlpool fork: same swap and tick arrays, the pool under
// its own discriminator.
const ValiantVortexProgramID = "vnt1u7PzorND5JjweFWmDawKe2hLWoTwHU6QKz6XX98"

var VortexDiscriminator = [8]byte{130, 73, 143, 86, 180, 125, 139, 171}

// vortexBaseSize is the Whirlpool layout every Vortex pool starts with; newer pools append 50 bytes.
const vortexBaseSize = 653

// Vortex is a Whirlpool behind the Vortex discriminator. Extension is what follows the Whirlpool
// layout, empty on the 653-byte cohort.
type Vortex struct {
	Whirlpool
	Extension []byte
}

// DecodeVortex decodes a Vortex pool from raw account bytes (with discriminator).
func DecodeVortex(data []byte, address solana.PublicKey) (*Vortex, error) {
	if len(data) < vortexBaseSize {
		return nil, ErrInsufficientData
	}

	var discoveredDiscriminator [8]byte
	copy(discoveredDiscriminator[:], data[:8])
	if discoveredDiscriminator != VortexDiscriminator {
		return nil, fmt.Errorf("%w: got %x, expected %x", ErrInvalidDiscriminator, discoveredDiscriminator, VortexDiscriminator)
	}

	pool := &Vortex{Whirlpool: Whirlpool{Address: address}}
	if err := bin.NewBinDecoder(data[8:]).Decode(&pool.Whirlpool); err != nil {
		return nil, fmt.Errorf("failed to decode vortex: %w", err)
	}
	pool.Extension = append([]byte(nil), data[vortexBaseSize:]...)
	return pool, nil
}
