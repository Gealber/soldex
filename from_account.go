package soldex

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/Gealber/soldex/models"
	"github.com/Gealber/soldex/quote/dlmm"
	"github.com/Gealber/soldex/quote/orca"
	soldexray "github.com/Gealber/soldex/quote/raydium"
)

// Program IDs of the venues FromAccount dispatches on. Verified executable on
// mainnet.
const (
	MeteoraDLMMProgramID   = "LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo"
	MeteoraDAMMV2ProgramID = "cpamdpZCGKUy5JxQXB4dcpGPiikHawvSWAd6mEn1sGG"
	PumpAMMProgramID       = "pAMMBay6oceH9fJKBRHGP5D4bD4sWpmSwMn52FMfXEA"
	PumpBondingProgramID   = "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"
)

// ErrUnknownProgram is returned for an account owned by a program this package
// does not quote.
var ErrUnknownProgram = errors.New("soldex: unknown program")

// Aux carries the state a quote needs but an account alone does not hold:
// providers for the bin and tick windows, the linked config accounts, vault
// balances, and the clock.
//
// Every field is optional in the sense that Go will zero it, but a venue that
// needs one and does not get it is REFUSED rather than quoted on the zero value.
// Populate what the venue you are decoding requires; FromAccount says which when
// it refuses.
type Aux struct {
	// Now is the current unix time. DLMM, Orca and Raydium CLMM all decay a
	// volatility reference against it, so a stale value over-states their fees.
	Now uint64

	// CurrentPoint is the DAMM v2 base-fee schedule position, in the pool's OWN
	// activation unit: a slot when ActivationType is 0, a unix timestamp when it
	// is 1. The wrong unit silently picks the wrong period.
	CurrentPoint uint64

	// Liquidity windows. Each stops the swap at the edge of what it knows, so a
	// window too narrow silently truncates a large swap.
	Bins         dlmm.BinProvider
	OrcaTicks    orca.TickProvider
	RaydiumTicks soldexray.TickProvider

	// Linked accounts, fetched separately.
	Oracle        *models.WhirlpoolOracle   // Orca; nil for a static-fee pool
	RaydiumConfig *models.RaydiumAmmConfig  // Raydium CLMM
	CPMMConfig    *models.RaydiumCPMMConfig // Raydium CP-Swap
	PumpGlobal    *models.PumpGlobalConfig  // Pump AMM
	PumpFeeConfig *models.PumpFeeConfig     // Pump AMM

	// Vault token-account balances.
	Vault0, Vault1        uint64 // Raydium CP-Swap, token_0 / token_1
	BaseVault, QuoteVault uint64 // Pump AMM

	// BaseSupply is the Pump base mint supply, for the market-cap fee tier.
	BaseSupply uint64
	// CurveFeeBps is the total fee for a pump.fun bonding curve, whose schedule
	// this package does not model.
	CurveFeeBps uint64
}

// FromAccount decodes a pool account and returns a Quoter for it, dispatching on
// the OWNING PROGRAM rather than the discriminator.
//
// The owner is what makes this safe: Raydium CLMM and CP-Swap share an account
// discriminator, so discriminator-only dispatch picks the wrong decoder for one
// of them. An RPC gives the owner alongside the data, so use it.
//
// Adding a venue is a case here plus its From* constructor; callers do not
// change.
func FromAccount(owner solana.PublicKey, data []byte, aux Aux) (Quoter, error) {
	addr := solana.PublicKey{}
	switch owner.String() {
	case MeteoraDLMMProgramID:
		pool, err := models.DecodeDLMMPool(data, addr)
		if err != nil {
			return nil, err
		}
		return FromDLMMPool(pool, int64(aux.Now), aux.Bins)

	case MeteoraDAMMV2ProgramID:
		pool, err := models.DecodeDAMMPool(data, addr)
		if err != nil {
			return nil, err
		}
		return FromDAMMPool(pool, aux.CurrentPoint)

	case models.OrcaWhirlpoolProgramID:
		pool, err := models.DecodeWhirlpool(data, addr)
		if err != nil {
			return nil, err
		}
		return FromWhirlpool(pool, aux.Oracle, aux.OrcaTicks, aux.Now)

	case models.RaydiumCLMMProgramID:
		pool, err := models.DecodeRaydiumCLMMPool(data, addr)
		if err != nil {
			return nil, err
		}
		return FromRaydiumCLMM(pool, aux.RaydiumConfig, aux.RaydiumTicks, aux.Now)

	case models.RaydiumCPMMProgramID:
		pool, err := models.DecodeRaydiumCPMMPool(data, addr)
		if err != nil {
			return nil, err
		}
		return FromRaydiumCPMM(pool, aux.CPMMConfig, aux.Vault0, aux.Vault1)

	case PumpAMMProgramID:
		pool, err := models.DecodePumpPool(data, addr)
		if err != nil {
			return nil, err
		}
		return FromPumpPool(pool, aux.PumpGlobal, aux.PumpFeeConfig,
			aux.BaseVault, aux.QuoteVault, aux.BaseSupply)

	case PumpBondingProgramID:
		curve, err := models.DecodeBondingCurve(data, addr)
		if err != nil {
			return nil, err
		}
		return FromBondingCurve(curve, aux.CurveFeeBps)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProgram, owner)
	}
}
