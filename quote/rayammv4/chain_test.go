package rayammv4

import (
	"math/big"
	"testing"
)

// Mainnet ray_log SwapBaseIn events: reserves are the log's own pre-swap net
// pool_coin / pool_pc, every pool charging 25/10000. direction 1 = pc in, 2 = coin in.
var chainVectors = []struct {
	name                 string
	direction            uint64
	in, poolCoin, poolPc uint64
	out                  uint64
}{
	// SOL/USDC 58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2
	// 4HPrvUBgjPRUKnj6WYqyETHaodPgT9CNSnQNA5MTc2PD6sWNpfXJvZfz1QiP6kkCN9QBVjF3agrDxUdmR3kDfmpa
	{"sol-usdc/coin-in", 2, 113_312_371, 112_590_054_272_241, 13_447_575_830_488, 13_500_000},
	// 64gT4vJUFZ9TM779WytbBBLMNfdX57ejPqErBLf8wK9CheDkT6YFnbgMbZs4BJ2WWKDQHjK9YHYC3d77oR9oRkiC
	{"sol-usdc/coin-in-large", 2, 10_000_000_000, 112_580_040_032_478, 13_448_769_031_168, 1_191_504_158},
	// 4uEGwZTxKLWe5zDsMzXMUShjECfWYBMh9Rhq74P75y9UsmMKyo6pzQxcnn2skrBn6uUVsnLSVFHh6RLGQV6B3uYZ
	{"sol-usdc/pc-in", 1, 183_778_937, 112_571_574_353_996, 13_449_776_967_815, 1_534_321_518},
	// JUP/SOL EYErUp5muPYEEkeaUCY22JibeZX7E9UuMcJFZkmNAN7c
	// 2nY4Zms8FGPEbMpQ6kt6P1V89Xtdk2f57AdWi2J5AzNGmxNhkWwBW4Le7LD5cFeCqRBTYgDL9P42hGzhNR7otoeQ
	{"jup-sol/coin-in-small", 2, 10_148, 17_056_394_746, 49_877_161_967, 29_599},
	// 5H8vdf2r44YYf3YeCax6N1PSotG5h1yr7MujTHLDW54n35t6ASEqULB2iJLsh2B8XYpLPPFq7B5d8NaZt45obYp
	{"jup-sol/coin-in", 2, 2_696_892, 17_051_982_791, 49_890_034_687, 7_869_492},
}

func TestSwapBaseInMatchesChain(t *testing.T) {
	for _, vec := range chainVectors {
		t.Run(vec.name, func(t *testing.T) {
			reserveIn, reserveOut := vec.poolCoin, vec.poolPc
			if vec.direction == 1 {
				reserveIn, reserveOut = vec.poolPc, vec.poolCoin
			}
			if got := SwapBaseIn(reserveIn, reserveOut, vec.in, 25, 10_000); got != vec.out {
				t.Fatalf("out = %d, chain paid %d", got, vec.out)
			}
		})
	}
}

// A floor-rounded fee pays 29,602 on jup-sol/coin-in-small instead of 29,599, so
// these vectors pin the ceiling.
func TestChainVectorsSeparateFeeRounding(t *testing.T) {
	separating := 0
	for _, vec := range chainVectors {
		reserveIn, reserveOut := vec.poolCoin, vec.poolPc
		if vec.direction == 1 {
			reserveIn, reserveOut = vec.poolPc, vec.poolCoin
		}
		floorFee := vec.in * 25 / 10_000
		netIn := new(big.Int).SetUint64(vec.in - floorFee)
		out := new(big.Int).Mul(new(big.Int).SetUint64(reserveOut), netIn)
		out.Div(out, new(big.Int).Add(new(big.Int).SetUint64(reserveIn), netIn))
		if out.Uint64() != vec.out {
			separating++
		}
	}
	if separating == 0 {
		t.Fatal("no vector tells a ceiling fee from a floor fee")
	}
}

func TestSwapBaseInRefusesZeroDenominator(t *testing.T) {
	if got := SwapBaseIn(1_000_000, 1_000_000, 1_000, 25, 0); got != 0 {
		t.Fatalf("out = %d with a zero fee denominator", got)
	}
}
