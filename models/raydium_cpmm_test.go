package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// buildRaydiumCPMMPoolData lays out a CP-Swap PoolState at the CP-swap Borsh
// offsets (all fields fixed-size and contiguous after the 8-byte discriminator),
// locking the byte layout rather than just struct self-consistency.
func buildRaydiumCPMMPoolData(config, mint0, mint1, vault0, vault1, prog0, prog1 solana.PublicKey) []byte {
	data := make([]byte, 8+637) // through the padding tail; only <=373 is read
	copy(data[0:8], RaydiumCPMMPoolDiscriminator[:])
	b := data[8:]
	copy(b[0:32], config[:])                       // amm_config @0
	copy(b[64:96], vault0[:])                      // token_0_vault @64
	copy(b[96:128], vault1[:])                     // token_1_vault @96
	copy(b[160:192], mint0[:])                     // token_0_mint @160
	copy(b[192:224], mint1[:])                     // token_1_mint @192
	copy(b[224:256], prog0[:])                     // token_0_program @224
	copy(b[256:288], prog1[:])                     // token_1_program @256
	b[323] = 6                                     // mint_0_decimals @323
	b[324] = 9                                     // mint_1_decimals @324
	binary.LittleEndian.PutUint64(b[333:341], 111) // protocol_fees_token_0 @333
	binary.LittleEndian.PutUint64(b[341:349], 222) // protocol_fees_token_1 @341
	binary.LittleEndian.PutUint64(b[349:357], 333) // fund_fees_token_0 @349
	binary.LittleEndian.PutUint64(b[357:365], 444) // fund_fees_token_1 @357
	return data
}

func TestDecodeRaydiumCPMMPoolLayout(t *testing.T) {
	config := solana.MustPublicKeyFromBase58("3h2e43PunVA5K34vwKCLHWhZF4aZpyaC9RmxvshGAQpL")
	mint0 := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	mint1 := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	vault0 := solana.MustPublicKeyFromBase58("4ct7br2vTPzfdmY3S5HLtTxcGSBfn6pnw98hsS6v359A")
	vault1 := solana.MustPublicKeyFromBase58("5it83u57VRrVgc51oNV19TTmAJuffPx5GtGwQr7gQNUo")
	prog := solana.MustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	addr := solana.MustPublicKeyFromBase58("3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv")

	data := buildRaydiumCPMMPoolData(config, mint0, mint1, vault0, vault1, prog, prog)
	pool, err := DecodeRaydiumCPMMPool(data, addr)
	if err != nil {
		t.Fatalf("DecodeRaydiumCPMMPool: %v", err)
	}
	if !pool.AmmConfig.Equals(config) {
		t.Fatalf("AmmConfig = %s, want %s", pool.AmmConfig, config)
	}
	if !pool.Token0Mint.Equals(mint0) || !pool.Token1Mint.Equals(mint1) {
		t.Fatalf("mints = %s/%s, want %s/%s", pool.Token0Mint, pool.Token1Mint, mint0, mint1)
	}
	if !pool.Token0Vault.Equals(vault0) || !pool.Token1Vault.Equals(vault1) {
		t.Fatalf("vaults = %s/%s, want %s/%s", pool.Token0Vault, pool.Token1Vault, vault0, vault1)
	}
	if !pool.Token0Program.Equals(prog) || !pool.Token1Program.Equals(prog) {
		t.Fatalf("programs = %s/%s, want %s", pool.Token0Program, pool.Token1Program, prog)
	}
	if pool.Mint0Decimals != 6 || pool.Mint1Decimals != 9 {
		t.Fatalf("decimals = %d/%d, want 6/9", pool.Mint0Decimals, pool.Mint1Decimals)
	}
	if pool.ProtocolFeesToken0 != 111 || pool.ProtocolFeesToken1 != 222 ||
		pool.FundFeesToken0 != 333 || pool.FundFeesToken1 != 444 {
		t.Fatalf("fees = %d/%d/%d/%d, want 111/222/333/444",
			pool.ProtocolFeesToken0, pool.ProtocolFeesToken1, pool.FundFeesToken0, pool.FundFeesToken1)
	}
	if !pool.Address.Equals(addr) {
		t.Fatalf("Address = %s, want %s", pool.Address, addr)
	}

	// NetReserves subtracts protocol+fund fees per side, saturating at zero.
	r0, r1 := pool.NetReserves(1_000_000, 2_000_000)
	if r0 != 1_000_000-(111+333) || r1 != 2_000_000-(222+444) {
		t.Fatalf("NetReserves = %d/%d, want %d/%d", r0, r1, 1_000_000-444, 2_000_000-666)
	}
	if r0, _ := pool.NetReserves(10, 2_000_000); r0 != 0 {
		t.Fatalf("NetReserves underflow should saturate to 0, got %d", r0)
	}

	// TokenXY reads the same mints straight from bytes.
	tx, ty, err := TokenXY(PoolTypeRaydiumCPMM, data)
	if err != nil {
		t.Fatalf("TokenXY: %v", err)
	}
	if !tx.Equals(mint0) || !ty.Equals(mint1) {
		t.Fatalf("TokenXY = %s/%s, want %s/%s", tx, ty, mint0, mint1)
	}
}

func buildRaydiumCPMMConfigData(feeRate uint64) []byte {
	data := make([]byte, 8+108)
	copy(data[0:8], RaydiumCPMMConfigDiscriminator[:])
	data[8] = 251                                       // bump @0
	binary.LittleEndian.PutUint16(data[10:12], 4)       // index @2
	binary.LittleEndian.PutUint64(data[12:20], feeRate) // trade_fee_rate @4
	binary.LittleEndian.PutUint64(data[20:28], 120000)  // protocol_fee_rate @12
	binary.LittleEndian.PutUint64(data[28:36], 40000)   // fund_fee_rate @20
	return data
}

func TestDecodeRaydiumCPMMConfigLayout(t *testing.T) {
	addr := solana.MustPublicKeyFromBase58("3h2e43PunVA5K34vwKCLHWhZF4aZpyaC9RmxvshGAQpL")
	cfg, err := DecodeRaydiumCPMMConfig(buildRaydiumCPMMConfigData(2500), addr)
	if err != nil {
		t.Fatalf("DecodeRaydiumCPMMConfig: %v", err)
	}
	if cfg.TradeFeeRate != 2500 {
		t.Fatalf("TradeFeeRate = %d, want 2500", cfg.TradeFeeRate)
	}
	if !cfg.Address.Equals(addr) {
		t.Fatalf("Address = %s, want %s", cfg.Address, addr)
	}
}

func TestDecodeRaydiumCPMMWrongDiscriminator(t *testing.T) {
	data := make([]byte, 8+365)
	// Left as zero discriminator — must be rejected, not silently decoded.
	if _, err := DecodeRaydiumCPMMPool(data, solana.PublicKey{}); err == nil {
		t.Fatal("expected discriminator error on zeroed data")
	}
	short := make([]byte, 100)
	copy(short[0:8], RaydiumCPMMPoolDiscriminator[:])
	if _, err := DecodeRaydiumCPMMPool(short, solana.PublicKey{}); err == nil {
		t.Fatal("expected insufficient-data error on short buffer")
	}
}
