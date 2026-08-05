package models

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// buildRaydiumPoolData lays out a Raydium CLMM PoolState at the absolute offsets
// verified against mainnet (pool 3ucNos4..., len 1544), so the test locks the
// byte layout rather than just struct self-consistency.
func buildRaydiumPoolData(config, mint0, mint1, vault0, vault1 solana.PublicKey) []byte {
	data := make([]byte, 1544)
	copy(data[0:8], RaydiumCLMMPoolDiscriminator[:])
	copy(data[9:41], config[:])                                       // amm_config @1
	copy(data[73:105], mint0[:])                                      // token_mint_0 @65
	copy(data[105:137], mint1[:])                                     // token_mint_1 @97
	copy(data[137:169], vault0[:])                                    // token_vault_0 @129
	copy(data[169:201], vault1[:])                                    // token_vault_1 @161
	data[233] = 9                                                     // mint_decimals_0 @225
	data[234] = 6                                                     // mint_decimals_1 @226
	binary.LittleEndian.PutUint16(data[235:237], 1)                   // tick_spacing @227
	binary.LittleEndian.PutUint64(data[237:245], 98132489249010)      // liquidity lo @229
	binary.LittleEndian.PutUint64(data[253:261], 4907934225356241358) // sqrt_price lo @245
	tickCurrent := int32(-26483)
	binary.LittleEndian.PutUint32(data[269:273], uint32(tickCurrent)) // tick_current @261
	return data
}

func TestDecodeRaydiumCLMMPoolLayout(t *testing.T) {
	config := solana.MustPublicKeyFromBase58("3h2e43PunVA5K34vwKCLHWhZF4aZpyaC9RmxvshGAQpL")
	mint0 := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
	mint1 := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	vault0 := solana.MustPublicKeyFromBase58("4ct7br2vTPzfdmY3S5HLtTxcGSBfn6pnw98hsS6v359A")
	vault1 := solana.MustPublicKeyFromBase58("5it83u57VRrVgc51oNV19TTmAJuffPx5GtGwQr7gQNUo")
	addr := solana.MustPublicKeyFromBase58("3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv")

	pool, err := DecodeRaydiumCLMMPool(buildRaydiumPoolData(config, mint0, mint1, vault0, vault1), addr)
	if err != nil {
		t.Fatalf("DecodeRaydiumCLMMPool: %v", err)
	}
	if !pool.AmmConfig.Equals(config) {
		t.Fatalf("AmmConfig = %s, want %s", pool.AmmConfig, config)
	}
	if !pool.TokenMint0.Equals(mint0) || !pool.TokenMint1.Equals(mint1) {
		t.Fatalf("mints = %s/%s, want %s/%s", pool.TokenMint0, pool.TokenMint1, mint0, mint1)
	}
	if !pool.TokenVault0.Equals(vault0) || !pool.TokenVault1.Equals(vault1) {
		t.Fatalf("vaults = %s/%s, want %s/%s", pool.TokenVault0, pool.TokenVault1, vault0, vault1)
	}
	if pool.MintDecimals0 != 9 || pool.MintDecimals1 != 6 {
		t.Fatalf("decimals = %d/%d, want 9/6", pool.MintDecimals0, pool.MintDecimals1)
	}
	if pool.TickSpacing != 1 {
		t.Fatalf("TickSpacing = %d, want 1", pool.TickSpacing)
	}
	if pool.Liquidity.Lo != 98132489249010 {
		t.Fatalf("Liquidity.Lo = %d, want 98132489249010", pool.Liquidity.Lo)
	}
	if pool.SqrtPriceX64.Lo != 4907934225356241358 {
		t.Fatalf("SqrtPriceX64.Lo = %d, want 4907934225356241358", pool.SqrtPriceX64.Lo)
	}
	if pool.TickCurrent != -26483 {
		t.Fatalf("TickCurrent = %d, want -26483", pool.TickCurrent)
	}
	if !pool.Address.Equals(addr) {
		t.Fatalf("Address = %s, want %s", pool.Address, addr)
	}
}

// buildRaydiumTickArrayData lays out a TickArrayState at the verified offsets
// (len 10240, tick stride 168) with two initialized ticks to lock the stride.
func buildRaydiumTickArrayData(pool solana.PublicKey, start int32) []byte {
	data := make([]byte, 10240)
	copy(data[0:8], RaydiumTickArrayDiscriminator[:])
	copy(data[8:40], pool[:])                                 // pool_id @0
	binary.LittleEndian.PutUint32(data[40:44], uint32(start)) // start_tick_index @32
	const base, stride = 44, 168
	// tick[0]
	binary.LittleEndian.PutUint32(data[base:base+4], uint32(start)) // tick @0
	binary.LittleEndian.PutUint64(data[base+4:base+12], 3083712)    // liquidity_net lo @4
	binary.LittleEndian.PutUint64(data[base+20:base+28], 3083712)   // liquidity_gross lo @20
	// tick[1] (one stride further)
	o := base + stride
	binary.LittleEndian.PutUint32(data[o:o+4], uint32(start+1)) // tick @0
	binary.LittleEndian.PutUint64(data[o+20:o+28], 999)         // liquidity_gross lo
	data[10124] = 17                                            // initialized_tick_count @10116
	return data
}

func TestDecodeRaydiumTickArrayLayout(t *testing.T) {
	pool := solana.MustPublicKeyFromBase58("3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv")
	addr := solana.MustPublicKeyFromBase58("B549oMiBzTTMcnNZ9ZvCa1LMzV1xrNLf4JCLaTZ6DpaV")
	const start = int32(-26520)

	ta, err := DecodeRaydiumTickArray(buildRaydiumTickArrayData(pool, start), addr)
	if err != nil {
		t.Fatalf("DecodeRaydiumTickArray: %v", err)
	}
	if !ta.PoolID.Equals(pool) {
		t.Fatalf("PoolID = %s, want %s", ta.PoolID, pool)
	}
	if ta.StartTickIndex != start {
		t.Fatalf("StartTickIndex = %d, want %d", ta.StartTickIndex, start)
	}
	if ta.InitializedTickCount != 17 {
		t.Fatalf("InitializedTickCount = %d, want 17", ta.InitializedTickCount)
	}
	if ta.Ticks[0].Tick != start || ta.Ticks[0].LiquidityNet.Lo != 3083712 || ta.Ticks[0].LiquidityGross.Lo != 3083712 {
		t.Fatalf("tick[0] = {%d, net %d, gross %d}, want {%d, 3083712, 3083712}",
			ta.Ticks[0].Tick, ta.Ticks[0].LiquidityNet.Lo, ta.Ticks[0].LiquidityGross.Lo, start)
	}
	// The stride decoded correctly iff tick[1] reads its own sentinel, not a
	// shifted byte from tick[0].
	if ta.Ticks[1].Tick != start+1 || ta.Ticks[1].LiquidityGross.Lo != 999 {
		t.Fatalf("tick[1] = {%d, gross %d}, want {%d, 999} (stride 168 wrong)",
			ta.Ticks[1].Tick, ta.Ticks[1].LiquidityGross.Lo, start+1)
	}
	if !ta.Ticks[0].Initialized() || ta.Ticks[2].Initialized() {
		t.Fatal("Initialized() should be true for tick[0], false for empty tick[2]")
	}

	tick, ok := ta.TickAt(start+1, 1)
	if !ok || tick.Tick != start+1 {
		t.Fatalf("TickAt(start+1) = {%d, ok %v}, want {%d, true}", tick.Tick, ok, start+1)
	}
	if _, ok := ta.TickAt(start+RaydiumTicksPerArray, 1); ok {
		t.Fatal("TickAt past array span should be false")
	}
}

func buildRaydiumAmmConfigData(owner solana.PublicKey) []byte {
	data := make([]byte, 117)
	copy(data[0:8], RaydiumAmmConfigDiscriminator[:])
	data[8] = 251                                      // bump @0
	binary.LittleEndian.PutUint16(data[9:11], 8)       // index @1
	copy(data[11:43], owner[:])                        // owner @3
	binary.LittleEndian.PutUint32(data[43:47], 120000) // protocol_fee_rate @35
	binary.LittleEndian.PutUint32(data[47:51], 400)    // trade_fee_rate @39
	binary.LittleEndian.PutUint16(data[51:53], 1)      // tick_spacing @43
	binary.LittleEndian.PutUint32(data[53:57], 40000)  // fund_fee_rate @45
	return data
}

func TestDecodeRaydiumAmmConfigLayout(t *testing.T) {
	owner := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	addr := solana.MustPublicKeyFromBase58("3h2e43PunVA5K34vwKCLHWhZF4aZpyaC9RmxvshGAQpL")

	cfg, err := DecodeRaydiumAmmConfig(buildRaydiumAmmConfigData(owner), addr)
	if err != nil {
		t.Fatalf("DecodeRaydiumAmmConfig: %v", err)
	}
	if cfg.Index != 8 {
		t.Fatalf("Index = %d, want 8", cfg.Index)
	}
	if cfg.TradeFeeRate != 400 {
		t.Fatalf("TradeFeeRate = %d, want 400", cfg.TradeFeeRate)
	}
	if cfg.TickSpacing != 1 {
		t.Fatalf("TickSpacing = %d, want 1", cfg.TickSpacing)
	}
}

func TestRaydiumTickArrayStartIndex(t *testing.T) {
	// Verified anchor: tick_current -26483, spacing 1 -> array start -26520.
	if got := RaydiumTickArrayStartIndex(-26483, 1); got != -26520 {
		t.Fatalf("start(-26483,1) = %d, want -26520", got)
	}
	if got := RaydiumTickArrayStartIndex(0, 1); got != 0 {
		t.Fatalf("start(0,1) = %d, want 0", got)
	}
	// Positive, multi-spacing: span = 10*60 = 600.
	if got := RaydiumTickArrayStartIndex(1234, 10); got != 1200 {
		t.Fatalf("start(1234,10) = %d, want 1200", got)
	}
}

// status, fee_on and dynamic_fee_info sit past the sequentially decoded prefix, so a
// wrong offset here reads plausible garbage rather than failing — pin them.
func TestDecodeRaydiumCLMMPoolLateFields(t *testing.T) {
	data := buildRaydiumPoolData(solana.PublicKey{}, solana.PublicKey{},
		solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{})
	data[raydiumPoolStatusOffset] = 1 << 4 // swap disabled
	data[raydiumPoolFeeOnOffset] = 2       // Token1Only

	const o = raydiumPoolDynamicFeeOffset
	binary.LittleEndian.PutUint16(data[o+0:o+2], 30)        // filter_period
	binary.LittleEndian.PutUint16(data[o+2:o+4], 600)       // decay_period
	binary.LittleEndian.PutUint16(data[o+4:o+6], 5_000)     // reduction_factor
	binary.LittleEndian.PutUint32(data[o+6:o+10], 40_000)   // dynamic_fee_control
	binary.LittleEndian.PutUint32(data[o+10:o+14], 350_000) // max_volatility_accumulator
	ref := int32(-26483)
	binary.LittleEndian.PutUint32(data[o+14:o+18], uint32(ref)) // tick_spacing_index_reference
	binary.LittleEndian.PutUint32(data[o+18:o+22], 111_111)     // volatility_reference
	binary.LittleEndian.PutUint32(data[o+22:o+26], 222_222)     // volatility_accumulator
	binary.LittleEndian.PutUint64(data[o+26:o+34], 1_700_000_000)

	pool, err := DecodeRaydiumCLMMPool(data, solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !pool.SwapDisabled() {
		t.Fatal("status bit4 must read as swap disabled")
	}
	if pool.FeeOn != 2 {
		t.Fatalf("FeeOn = %d, want 2", pool.FeeOn)
	}
	// Token1Only: the fee is on token1, which is the input only when buying token0.
	if pool.IsFeeOnInput(true) {
		t.Fatal("Token1Only selling token0 must be fee-on-output")
	}
	if !pool.IsFeeOnInput(false) {
		t.Fatal("Token1Only buying token0 must be fee-on-input")
	}
	want := RaydiumDynamicFee{
		FilterPeriod: 30, DecayPeriod: 600, ReductionFactor: 5_000,
		DynamicFeeControl: 40_000, MaxVolatilityAccumulator: 350_000,
		TickSpacingIndexReference: -26483, VolatilityReference: 111_111,
		VolatilityAccumulator: 222_222, LastUpdateTimestamp: 1_700_000_000,
	}
	if pool.DynamicFee != want {
		t.Fatalf("DynamicFee = %+v, want %+v", pool.DynamicFee, want)
	}
	if !pool.DynamicFee.Enabled() {
		t.Fatal("a populated DynamicFeeInfo must read as enabled")
	}
}

// A pool with no dynamic fee must read as disabled, not as a fee of zero-ish
// constants that the quote would then try to apply.
func TestDecodeRaydiumCLMMPoolNoDynamicFee(t *testing.T) {
	data := buildRaydiumPoolData(solana.PublicKey{}, solana.PublicKey{},
		solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{})
	pool, err := DecodeRaydiumCLMMPool(data, solana.PublicKey{})
	if err != nil {
		t.Fatal(err)
	}
	if pool.DynamicFee.Enabled() {
		t.Fatal("an all-zero DynamicFeeInfo must read as absent")
	}
	if !pool.IsFeeOnInput(true) || !pool.IsFeeOnInput(false) {
		t.Fatal("fee_on = 0 is fee-on-input in both directions")
	}
	if pool.SwapDisabled() {
		t.Fatal("status 0 must not read as disabled")
	}
}

// The limit-order fields were carved out of TickState's old padding, so the 168-byte
// stride is unchanged — but a tick can now be initialized on orders ALONE, and a
// quote that checks gross liquidity walks straight past it.
func TestDecodeRaydiumTickLimitOrders(t *testing.T) {
	pool := solana.MustPublicKeyFromBase58("3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv")
	const start = int32(-26520)
	data := buildRaydiumTickArrayData(pool, start)

	// tick[2]: no liquidity at all, only resting orders.
	const o = 44 + 2*168
	tickIndex := start + 2
	binary.LittleEndian.PutUint32(data[o:o+4], uint32(tickIndex))
	binary.LittleEndian.PutUint64(data[o+116:o+124], 7)         // order_phase
	binary.LittleEndian.PutUint64(data[o+124:o+132], 4_000_000) // orders_amount
	binary.LittleEndian.PutUint64(data[o+132:o+140], 1_500_000) // part_filled_orders_remaining

	array, err := DecodeRaydiumTickArray(data, solana.PublicKey{})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tick := array.Ticks[2]
	if tick.Tick != tickIndex {
		t.Fatalf("stride broken: tick = %d, want %d", tick.Tick, tickIndex)
	}
	if tick.OrderPhase != 7 || tick.OrdersAmount != 4_000_000 || tick.PartFilledOrdersLeft != 1_500_000 {
		t.Fatalf("limit-order fields decoded wrong: %+v", tick)
	}
	if got := tick.LimitOrderUnfilled(); got != 5_500_000 {
		t.Fatalf("LimitOrderUnfilled = %d, want 5500000", got)
	}
	if tick.HasLiquidity() {
		t.Fatal("this tick has no gross liquidity")
	}
	if !tick.Initialized() {
		t.Fatal("a tick holding only limit orders must still read as initialized")
	}
	// The liquidity-bearing tick built by the fixture must be unaffected.
	if !array.Ticks[0].HasLiquidity() || array.Ticks[0].HasLimitOrders() {
		t.Fatalf("liquidity tick misdecoded: %+v", array.Ticks[0])
	}
}
