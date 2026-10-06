package models

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
)

// RaydiumAMMV4ProgramID is the Raydium Liquidity Pool v4 program. Its AmmInfo has
// no discriminator, so pools are told apart by owner and RaydiumAMMV4PoolSize.
const RaydiumAMMV4ProgramID = "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8"

// RaydiumAMMV4PoolSize is the packed AmmInfo length.
const RaydiumAMMV4PoolSize = 752

// AmmStatus values, as in the program's state.rs.
const (
	RaydiumAMMV4StatusUninitialized uint64 = 0
	RaydiumAMMV4StatusInitialized   uint64 = 1
	RaydiumAMMV4StatusDisabled      uint64 = 2
	RaydiumAMMV4StatusWithdrawOnly  uint64 = 3
	RaydiumAMMV4StatusLiquidityOnly uint64 = 4
	RaydiumAMMV4StatusOrderBookOnly uint64 = 5
	RaydiumAMMV4StatusSwapOnly      uint64 = 6
	RaydiumAMMV4StatusWaitingTrade  uint64 = 7
)

// RaydiumAMMV4Pool mirrors the AmmInfo fields a swap needs. The swappable
// reserve is the vault balance minus the PnL owed to the protocol, so use
// NetReserves.
//
// Layout (repr(C, packed), no discriminator; vault, mint and decimal offsets
// verified on 669 live pools of 705,986): status u64@0, coin_decimals u64@32,
// pc_decimals u64@40, swap_fee_numerator u64@176, swap_fee_denominator u64@184,
// need_take_pnl_coin u64@192, need_take_pnl_pc u64@200, pool_open_time u64@224,
// coin_vault Pubkey@336, pc_vault Pubkey@368, coin_vault_mint Pubkey@400,
// pc_vault_mint Pubkey@432.
type RaydiumAMMV4Pool struct {
	// Account address (not part of serialized data).
	Address solana.PublicKey

	Status       uint64
	CoinDecimals uint64
	PcDecimals   uint64

	// SwapFeeNumerator / SwapFeeDenominator is the fee charged on the input.
	SwapFeeNumerator   uint64
	SwapFeeDenominator uint64

	// PnL held in the vaults but NOT part of the swappable reserve.
	NeedTakePnlCoin uint64
	NeedTakePnlPc   uint64

	// PoolOpenTime gates a WaitingTrade pool: swaps fail before this unix time.
	PoolOpenTime uint64

	CoinVault solana.PublicKey
	PcVault   solana.PublicKey
	CoinMint  solana.PublicKey
	PcMint    solana.PublicKey
}

// DecodeRaydiumAMMV4Pool decodes an AmmInfo. With no discriminator to check, it
// requires the exact account size and a status the program defines.
func DecodeRaydiumAMMV4Pool(data []byte, address solana.PublicKey) (*RaydiumAMMV4Pool, error) {
	if len(data) != RaydiumAMMV4PoolSize {
		return nil, fmt.Errorf("%w: AmmInfo is %d bytes, got %d", ErrInsufficientData, RaydiumAMMV4PoolSize, len(data))
	}
	status := binary.LittleEndian.Uint64(data[0:8])
	if status > RaydiumAMMV4StatusWaitingTrade {
		return nil, fmt.Errorf("%w: AmmInfo status %d", ErrInvalidDiscriminator, status)
	}
	return &RaydiumAMMV4Pool{
		Address:            address,
		Status:             status,
		CoinDecimals:       binary.LittleEndian.Uint64(data[32:40]),
		PcDecimals:         binary.LittleEndian.Uint64(data[40:48]),
		SwapFeeNumerator:   binary.LittleEndian.Uint64(data[176:184]),
		SwapFeeDenominator: binary.LittleEndian.Uint64(data[184:192]),
		NeedTakePnlCoin:    binary.LittleEndian.Uint64(data[192:200]),
		NeedTakePnlPc:      binary.LittleEndian.Uint64(data[200:208]),
		PoolOpenTime:       binary.LittleEndian.Uint64(data[224:232]),
		CoinVault:          solana.PublicKeyFromBytes(data[336:368]),
		PcVault:            solana.PublicKeyFromBytes(data[368:400]),
		CoinMint:           solana.PublicKeyFromBytes(data[400:432]),
		PcMint:             solana.PublicKeyFromBytes(data[432:464]),
	}, nil
}

// NetReserves subtracts the owed PnL from the raw vault balances. The program
// fails the swap when PnL exceeds a vault, so this errors rather than saturating.
func (p *RaydiumAMMV4Pool) NetReserves(coinVaultBalance, pcVaultBalance uint64) (coinReserve, pcReserve uint64, err error) {
	if coinVaultBalance < p.NeedTakePnlCoin || pcVaultBalance < p.NeedTakePnlPc {
		return 0, 0, fmt.Errorf("AmmInfo owes more PnL (%d coin, %d pc) than its vaults hold (%d, %d)",
			p.NeedTakePnlCoin, p.NeedTakePnlPc, coinVaultBalance, pcVaultBalance)
	}
	return coinVaultBalance - p.NeedTakePnlCoin, pcVaultBalance - p.NeedTakePnlPc, nil
}

// CanSwapNoOrderbook reports whether swap_base_in_v2 accepts the pool at unix time
// now. That instruction refuses orderbook-enabled pools and pre-open WaitingTrade ones.
func (p *RaydiumAMMV4Pool) CanSwapNoOrderbook(now uint64) bool {
	switch p.Status {
	case RaydiumAMMV4StatusSwapOnly:
		return true
	case RaydiumAMMV4StatusWaitingTrade:
		return now >= p.PoolOpenTime
	}
	return false
}
