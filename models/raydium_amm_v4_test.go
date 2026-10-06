package models

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// Mainnet SOL/USDC AmmInfo 58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2 at slot 453868631.
const raydiumAMMV4SOLUSDC = "BgAAAAAAAAD+AAAAAAAAAAcAAAAAAAAAAwAAAAAAAAAJAAAAAAAAAAYAAAAAAAAAAgAAAAAAAAAAAAAAAAAAAEBCDwAAAAAA9AEAAAAAAAAAAAAAAAAAAEBCDwAAAAAAQEIPAAAAAAABAAAAAAAAAADKmjsAAAAAAMqaOwAAAAAFAAAAAAAAABAnAAAAAAAAGQAAAAAAAAAQJwAAAAAAAAwAAAAAAAAAZAAAAAAAAAAZAAAAAAAAABAnAAAAAAAACgjwCgAAAACkmE4BAAAAAE64FyutAwAALkI4/Jo0AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACzqVos/TzgAAAAAAAAAAABzVLPxIFgFAAAAAAAAAAAAMWNjiHMDAACZN03ShGQFAAAAAAAAAAAAQEusDaXyOAAAAAAAAAAAAEiYZsIJJAAAuHDhLdN5iRVh0un6jyZDGDTrc28vJPwqKk3/H9XcpN/yy7m3YO3bGFcGMDBjrTPXtXKW6gLU4DNeMc6vpMxC3QabiFf+q4GE+2h/Y0YYwDXaxDncGus7VZig8AAAAAABxvp6877brTo9ZfNqq8l0MbG75MLS9uDkfKYCA0UvXWFsT5PYWOiP+v6gjENnRJfo5qkywMgxSCYqGuPMx4KexvkvOQ/5YJ6K1De7jkwfGqQ6wF0kMIzKd96FEsVQkpLTasTDzvqfGb9UyNwPXk0c7uUyfSZIKynSsTy6pDRHIY0NB1GoKC2mEwX+KZw3uZjlhHHbETUDcxD4vhBFpgr27qvkPHweIeqm+XyL01XiG9EnlnR1bByOEGxucSuhFtlwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAOW2K2XLO72m9WiI5m/ujmTcVWAZnA+IsR/ic70FnoqhVubWvz9dAABO2XAAAAAAABoEAAAAAAAAAAAAAAAAAAA="

func solUSDCAMMV4Data(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(raydiumAMMV4SOLUSDC)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodeRaydiumAMMV4PoolFromChain(t *testing.T) {
	addr := solana.MustPublicKeyFromBase58("58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2")
	pool, err := DecodeRaydiumAMMV4Pool(solUSDCAMMV4Data(t), addr)
	if err != nil {
		t.Fatal(err)
	}
	if pool.Status != RaydiumAMMV4StatusSwapOnly || pool.CoinDecimals != 9 || pool.PcDecimals != 6 {
		t.Fatalf("status %d decimals %d/%d, want 6 and 9/6", pool.Status, pool.CoinDecimals, pool.PcDecimals)
	}
	if pool.SwapFeeNumerator != 25 || pool.SwapFeeDenominator != 10_000 {
		t.Fatalf("swap fee %d/%d, want 25/10000", pool.SwapFeeNumerator, pool.SwapFeeDenominator)
	}
	if pool.NeedTakePnlCoin != 183_502_858 || pool.NeedTakePnlPc != 21_928_100 {
		t.Fatalf("need_take_pnl %d/%d, want 183502858/21928100", pool.NeedTakePnlCoin, pool.NeedTakePnlPc)
	}
	want := map[string][2]solana.PublicKey{
		"vaults": {solana.MustPublicKeyFromBase58("DQyrAcCrDXQ7NeoqGgDCZwBvWDcYmFCjSb9JtteuvPpz"), solana.MustPublicKeyFromBase58("HLmqeL62xR1QoZ1HKKbXRrdN1p3phKpxRMb2VVopvBBz")},
		"mints":  {solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112"), solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")},
	}
	if got := [2]solana.PublicKey{pool.CoinVault, pool.PcVault}; got != want["vaults"] {
		t.Fatalf("vaults = %v, want %v", got, want["vaults"])
	}
	if got := [2]solana.PublicKey{pool.CoinMint, pool.PcMint}; got != want["mints"] {
		t.Fatalf("mints = %v, want %v", got, want["mints"])
	}
}

func TestDecodeRaydiumAMMV4PoolRefusesOtherAccounts(t *testing.T) {
	data := solUSDCAMMV4Data(t)
	if _, err := DecodeRaydiumAMMV4Pool(data[:RaydiumAMMV4PoolSize-1], solana.PublicKey{}); !errors.Is(err, ErrInsufficientData) {
		t.Fatalf("short account: err = %v", err)
	}
	binary.LittleEndian.PutUint64(data[0:8], RaydiumAMMV4StatusWaitingTrade+1)
	if _, err := DecodeRaydiumAMMV4Pool(data, solana.PublicKey{}); !errors.Is(err, ErrInvalidDiscriminator) {
		t.Fatalf("status 8: err = %v", err)
	}
}

// Every live pool holds 25/10000 in both trade_fee @144 and swap_fee @176, so only a
// fixture where they differ can tell which one the decoder reads.
func TestDecodeRaydiumAMMV4PoolReadsSwapFeeNotTradeFee(t *testing.T) {
	data := solUSDCAMMV4Data(t)
	binary.LittleEndian.PutUint64(data[144:152], 30)
	pool, err := DecodeRaydiumAMMV4Pool(data, solana.PublicKey{})
	if err != nil {
		t.Fatal(err)
	}
	if pool.SwapFeeNumerator != 25 {
		t.Fatalf("swap fee numerator %d, want 25 from @176 not 30 from trade_fee @144", pool.SwapFeeNumerator)
	}
}

func TestRaydiumAMMV4NetReserves(t *testing.T) {
	pool := &RaydiumAMMV4Pool{NeedTakePnlCoin: 100, NeedTakePnlPc: 7}
	coin, pc, err := pool.NetReserves(1_000, 50)
	if err != nil || coin != 900 || pc != 43 {
		t.Fatalf("NetReserves = %d, %d, %v; want 900, 43", coin, pc, err)
	}
	if _, _, err := pool.NetReserves(99, 50); err == nil {
		t.Fatal("PnL above the coin vault accepted")
	}
	if _, _, err := pool.NetReserves(1_000, 6); err == nil {
		t.Fatal("PnL above the pc vault accepted")
	}
}

func TestRaydiumAMMV4CanSwapNoOrderbook(t *testing.T) {
	const openTime = 1_700_000_000
	cases := []struct {
		status uint64
		now    uint64
		want   bool
	}{
		{RaydiumAMMV4StatusSwapOnly, 0, true},
		{RaydiumAMMV4StatusWaitingTrade, openTime - 1, false},
		{RaydiumAMMV4StatusWaitingTrade, openTime, true},
		// orderbook-enabled: swap_base_in_v2 refuses these
		{RaydiumAMMV4StatusInitialized, openTime, false},
		{RaydiumAMMV4StatusOrderBookOnly, openTime, false},
		{RaydiumAMMV4StatusDisabled, openTime, false},
		{RaydiumAMMV4StatusWithdrawOnly, openTime, false},
		{RaydiumAMMV4StatusLiquidityOnly, openTime, false},
		{RaydiumAMMV4StatusUninitialized, openTime, false},
	}
	for _, tt := range cases {
		pool := &RaydiumAMMV4Pool{Status: tt.status, PoolOpenTime: openTime}
		if got := pool.CanSwapNoOrderbook(tt.now); got != tt.want {
			t.Fatalf("status %d at %d: CanSwapNoOrderbook = %v, want %v", tt.status, tt.now, got, tt.want)
		}
	}
}
