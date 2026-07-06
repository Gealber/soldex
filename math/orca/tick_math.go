// Package orca ports the Orca Whirlpool concentrated-liquidity swap math
// (canonical Uniswap-V3 Q64.64) faithfully from the on-chain program, using
// big.Int throughout so the 256-bit intermediate products need no custom type.
package orca

import "math/big"

// Sqrt-price bounds derived from the min/max tick index (Q64.64).
var (
	MinSqrtPrice = big.NewInt(4295048016)
	MaxSqrtPrice = mustBig("79226673515401279992447579055")
)

// Tick index bounds (sqrt(1.0001) limits at the 2^64 price ceiling).
const (
	MinTickIndex int32 = -443636
	MaxTickIndex int32 = 443636
)

const logBTwoX32 = int64(59543866431248)

var (
	logBPErrMarginLowerX64 = big.NewInt(184467440737095516)  // 0.01
	logBPErrMarginUpperX64 = mustBig("15793534762490258745") // 2^-precision/log_2_b + 0.01
)

// positiveTickMultipliers are the per-bit Q96 ratios for tick >= 0; each set bit
// folds in via mul-shift-96. Index i corresponds to bit (1<<i).
var positiveTickMultipliers = []*big.Int{
	mustBig("79232123823359799118286999567"), // bit 0 (seed-odd factor)
	mustBig("79236085330515764027303304731"),
	mustBig("79244008939048815603706035061"),
	mustBig("79259858533276714757314932305"),
	mustBig("79291567232598584799939703904"),
	mustBig("79355022692464371645785046466"),
	mustBig("79482085999252804386437311141"),
	mustBig("79736823300114093921829183326"),
	mustBig("80248749790819932309965073892"),
	mustBig("81282483887344747381513967011"),
	mustBig("83390072131320151908154831281"),
	mustBig("87770609709833776024991924138"),
	mustBig("97234110755111693312479820773"),
	mustBig("119332217159966728226237229890"),
	mustBig("179736315981702064433883588727"),
	mustBig("407748233172238350107850275304"),
	mustBig("2098478828474011932436660412517"),
	mustBig("55581415166113811149459800483533"),
	mustBig("38992368544603139932233054999993551"),
}

// negativeTickMultipliers are the per-bit Q64 ratios for tick < 0; each set bit
// folds in via mul-shift-64. Index i corresponds to bit (1<<i).
var negativeTickMultipliers = []*big.Int{
	mustBig("18445821805675392311"), // bit 0 (seed-odd factor)
	mustBig("18444899583751176498"),
	mustBig("18443055278223354162"),
	mustBig("18439367220385604838"),
	mustBig("18431993317065449817"),
	mustBig("18417254355718160513"),
	mustBig("18387811781193591352"),
	mustBig("18329067761203520168"),
	mustBig("18212142134806087854"),
	mustBig("17980523815641551639"),
	mustBig("17526086738831147013"),
	mustBig("16651378430235024244"),
	mustBig("15030750278693429944"),
	mustBig("12247334978882834399"),
	mustBig("8131365268884726200"),
	mustBig("3584323654723342297"),
	mustBig("696457651847595233"),
	mustBig("26294789957452057"),
	mustBig("37481735321082"),
}

var (
	twoPow96  = mustBig("79228162514264337593543950336") // 2^96
	twoPow64  = mustBig("18446744073709551616")          // 2^64
	oddSeed96 = twoPow96                                 // tick>=0 even seed
	oddSeed64 = twoPow64                                 // tick<0 even seed
)

// SqrtPriceFromTickIndex returns the Q64.64 sqrt-price for a tick index, matching
// the on-chain sqrt_price_from_tick_index bit-for-bit.
func SqrtPriceFromTickIndex(tick int32) *big.Int {
	if tick >= 0 {
		return sqrtPricePositiveTick(tick)
	}
	return sqrtPriceNegativeTick(tick)
}

func sqrtPricePositiveTick(tick int32) *big.Int {
	ratio := new(big.Int)
	if tick&1 != 0 {
		ratio.Set(positiveTickMultipliers[0])
	} else {
		ratio.Set(oddSeed96)
	}
	for i := 1; i < len(positiveTickMultipliers); i++ {
		if tick&(1<<uint(i)) != 0 {
			ratio = mulShift(ratio, positiveTickMultipliers[i], 96)
		}
	}
	return new(big.Int).Rsh(ratio, 32)
}

func sqrtPriceNegativeTick(tick int32) *big.Int {
	abs := tick
	if abs < 0 {
		abs = -abs
	}
	ratio := new(big.Int)
	if abs&1 != 0 {
		ratio.Set(negativeTickMultipliers[0])
	} else {
		ratio.Set(oddSeed64)
	}
	for i := 1; i < len(negativeTickMultipliers); i++ {
		if abs&(1<<uint(i)) != 0 {
			ratio = mulShift(ratio, negativeTickMultipliers[i], 64)
		}
	}
	return ratio
}

// TickIndexFromSqrtPrice returns the floor tick index for a Q64.64 sqrt-price,
// matching the on-chain log-approximation and its tick_low/tick_high correction.
func TickIndexFromSqrtPrice(sqrtPrice *big.Int) int32 {
	// All of this runs in i128-width in the on-chain code; big.Int avoids the
	// int64 overflow of the 2^63 fraction bit and the i128 logbp product.
	msb := uint(sqrtPrice.BitLen() - 1)
	log2pIntegerX32 := new(big.Int).Lsh(big.NewInt(int64(msb)-64), 32)

	var r *big.Int
	if msb >= 64 {
		r = new(big.Int).Rsh(sqrtPrice, msb-63)
	} else {
		r = new(big.Int).Lsh(sqrtPrice, 63-msb)
	}

	bit := new(big.Int).Lsh(big.NewInt(1), 63) // 2^63 (0.5 in Q64.64)
	log2pFractionX64 := new(big.Int)
	for precision := 0; bit.Sign() > 0 && precision < 14; precision++ {
		r.Mul(r, r)
		isMoreThanTwo := new(big.Int).Rsh(r, 127) // 0 or 1
		r.Rsh(r, uint(63+isMoreThanTwo.Int64()))
		if isMoreThanTwo.Sign() > 0 {
			log2pFractionX64.Add(log2pFractionX64, bit)
		}
		bit.Rsh(bit, 1)
	}

	log2pX32 := new(big.Int).Add(log2pIntegerX32, new(big.Int).Rsh(log2pFractionX64, 32))
	logbpX64 := new(big.Int).Mul(log2pX32, big.NewInt(logBTwoX32))

	tickLow := int32(arithRsh(new(big.Int).Sub(logbpX64, logBPErrMarginLowerX64), 64).Int64())
	tickHigh := int32(arithRsh(new(big.Int).Add(logbpX64, logBPErrMarginUpperX64), 64).Int64())

	if tickLow == tickHigh {
		return tickLow
	}
	if SqrtPriceFromTickIndex(tickHigh).Cmp(sqrtPrice) <= 0 {
		return tickHigh
	}
	return tickLow
}

// mulShift returns (a*b) >> shift.
func mulShift(a, b *big.Int, shift uint) *big.Int {
	return new(big.Int).Rsh(new(big.Int).Mul(a, b), shift)
}

// arithRsh performs an arithmetic (sign-preserving) right shift, which big.Int's
// Rsh already does for negative values via two's-complement semantics.
func arithRsh(x *big.Int, shift uint) *big.Int {
	return new(big.Int).Rsh(x, shift)
}

func mustBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("orca: bad bigint constant " + s)
	}
	return v
}
