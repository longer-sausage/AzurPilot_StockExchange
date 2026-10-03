package exchange

import "math/bits"

func startingCash(p *Player) int64 {
	if p.InitialCash > 0 {
		return p.InitialCash
	}
	return InitialCash
}

// 使用 128 位中间值，避免小本金、大收益时收益率乘法溢出。
func returnPPM(equity, principal int64) int64 {
	delta := equity - principal
	amount := uint64(delta)
	if delta < 0 {
		amount = uint64(-delta)
	}
	hi, lo := bits.Mul64(amount, 1_000_000)
	var result uint64
	if hi >= uint64(principal) {
		result = 1<<63 - 1
	} else {
		result, _ = bits.Div64(hi, lo, uint64(principal))
		result = min(result, uint64(1<<63-1))
	}
	if delta < 0 {
		return -int64(result)
	}
	return int64(result)
}
