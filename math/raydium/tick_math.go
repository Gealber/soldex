// Package raydium ports the Raydium CLMM concentrated-liquidity swap math
// (canonical Uniswap-V3 Q64.64) faithfully from the on-chain program at
// raydium-clmm/programs/amm/src/libraries. It is a distinct port from the
// math/orca package: Raydium uses the canonical UniV3 hex-constant tick table
// and two-step-ceil delta rounding, which are NOT bit-compatible with Orca, so
// the two must not be shared. big.Int is used throughout so the 128/256-bit
// intermediates need no custom type.
package raydium

import "math/big"

// Tick index bounds and the corresponding Q64.64 sqrt-price bounds, matching the
// on-chain constants in tick_math.rs.
const (
	MinTick int32 = -443636
	MaxTick int32 = 443636

	bitPrecision = 16
)

var (
	MinSqrtPrice = big.NewInt(4295048016)
	MaxSqrtPrice = mustBig("79226673521066979257578248091")

	// 2^128 - 1, the U128::MAX used to invert the ratio for positive ticks.
	u128Max  = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	twoPow64 = new(big.Int).Lsh(big.NewInt(1), 64)

	// Change-of-base and rounding constants from get_tick_at_sqrt_price.
	logSqrt10001Mul = big.NewInt(59543866431248)
	tickLowSub      = big.NewInt(184467440737095516)
	tickHighAdd     = mustBig("15793534762490258745")
)

// sqrtPriceMagics are the per-bit Q64.64 multipliers from get_sqrt_price_at_tick.
// Index i applies when abs_tick has bit (1<<i) set; index 0 is also the seed when
// the tick is odd (else the seed is 2^64).
var sqrtPriceMagics = func() []*big.Int {
	hexes := []string{
		"fffcb933bd6fb800", // bit 0
		"fff97272373d4000", // bit 1
		"fff2e50f5f657000", // bit 2
		"ffe5caca7e10f000", // bit 3
		"ffcb9843d60f7000", // bit 4
		"ff973b41fa98e800", // bit 5
		"ff2ea16466c9b000", // bit 6
		"fe5dee046a9a3800", // bit 7
		"fcbe86c7900bb000", // bit 8
		"f987a7253ac65800", // bit 9
		"f3392b0822bb6000", // bit 10
		"e7159475a2caf000", // bit 11
		"d097f3bdfd2f2000", // bit 12
		"a9f746462d9f8000", // bit 13
		"70d869a156f31c00", // bit 14
		"31be135f97ed3200", // bit 15
		"9aa508b5b85a500",  // bit 16
		"5d6af8dedc582c",   // bit 17
		"2216e584f5fa",     // bit 18
	}
	out := make([]*big.Int, len(hexes))
	for i, h := range hexes {
		v, ok := new(big.Int).SetString(h, 16)
		if !ok {
			panic("raydium: bad sqrt-price magic " + h)
		}
		out[i] = v
	}
	return out
}()

func mustBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("raydium: bad big int " + s)
	}
	return v
}

// SqrtPriceFromTick returns the Q64.64 sqrt-price at tick, matching the on-chain
// get_sqrt_price_at_tick bit-for-bit. Callers must keep tick within [MinTick,
// MaxTick].
func SqrtPriceFromTick(tick int32) *big.Int {
	abs := tick
	if abs < 0 {
		abs = -abs
	}

	var ratio *big.Int
	if abs&0x1 != 0 {
		ratio = new(big.Int).Set(sqrtPriceMagics[0])
	} else {
		ratio = new(big.Int).Set(twoPow64) // 2^64
	}
	for i := 1; i < len(sqrtPriceMagics); i++ {
		if abs&(1<<uint(i)) != 0 {
			ratio.Mul(ratio, sqrtPriceMagics[i])
			ratio.Rsh(ratio, 64)
		}
	}

	// The table computes 1.0001^(-|tick|/2); invert for positive ticks.
	if tick > 0 {
		ratio = new(big.Int).Quo(u128Max, ratio)
	}
	return ratio
}

// TickFromSqrtPrice returns the greatest tick whose sqrt-price is <= sqrtPrice,
// matching get_tick_at_sqrt_price. sqrtPrice must be in [MinSqrtPrice,
// MaxSqrtPrice). All signed intermediates use big.Int so the Q64.64 fraction
// (whose top bit is 2^63) never overflows a machine int.
func TickFromSqrtPrice(sqrtPrice *big.Int) int32 {
	msb := sqrtPrice.BitLen() - 1 // 128 - leading_zeros - 1

	// Integer part of log2(sqrtPrice) as a Q32 value.
	log2pIntegerX32 := new(big.Int).Lsh(big.NewInt(int64(msb-64)), 32)

	var r *big.Int
	if msb >= 64 {
		r = new(big.Int).Rsh(sqrtPrice, uint(msb-63))
	} else {
		r = new(big.Int).Lsh(sqrtPrice, uint(63-msb))
	}

	bit := new(big.Int).Lsh(big.NewInt(1), 63) // 0.5 in Q64.64
	log2pFractionX64 := big.NewInt(0)
	for prec := 0; bit.Sign() > 0 && prec < bitPrecision; prec++ {
		r.Mul(r, r)
		isMoreThanTwo := new(big.Int).Rsh(r, 127) // 0 or 1
		r.Rsh(r, uint(63+isMoreThanTwo.Int64()))
		if isMoreThanTwo.Sign() > 0 {
			log2pFractionX64.Add(log2pFractionX64, bit)
		}
		bit.Rsh(bit, 1)
	}

	log2pFractionX32 := new(big.Int).Rsh(log2pFractionX64, 32)
	log2pX32 := new(big.Int).Add(log2pIntegerX32, log2pFractionX32)
	logSqrt10001 := new(big.Int).Mul(log2pX32, logSqrt10001Mul)

	// big.Int Rsh is an arithmetic (floor) shift for negatives, matching Rust i128 >>.
	tickLow := int32(new(big.Int).Rsh(new(big.Int).Sub(logSqrt10001, tickLowSub), 64).Int64())
	tickHigh := int32(new(big.Int).Rsh(new(big.Int).Add(logSqrt10001, tickHighAdd), 64).Int64())

	if tickLow == tickHigh {
		return tickLow
	}
	if SqrtPriceFromTick(tickHigh).Cmp(sqrtPrice) <= 0 {
		return tickHigh
	}
	return tickLow
}
