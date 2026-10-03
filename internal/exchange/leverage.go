package exchange

import (
	"encoding/json"
	"math/bits"
)

func maxLeverage(r Rules) int64 { return max(int64(1), min(int64(10), 1_000_000/r.IMRPPM)) }

func validateLeverage(side string, leverage int64, r Rules) error {
	if leverage < 0 || leverage > 10 || ((side == "sell" || side == "cover") && leverage > 1) {
		return fail("INVALID_LEVERAGE", "杠杆须为 1–10 的整数，平仓不能新增杠杆")
	}
	if leverage > maxLeverage(r) {
		return fail("INVALID_LEVERAGE", "所选杠杆超过当前初始保证金规则允许的上限")
	}
	return nil
}

func orderMargin(o *Order, r Rules) int64 {
	// 旧版本未传杠杆的空单继续沿用 IMR；旧买单为现金买入。
	if o.Side == "short" && o.Leverage == 0 {
		return r.IMRPPM
	}
	ppm := (1_000_000 + max(int64(1), o.Leverage) - 1) / max(int64(1), o.Leverage)
	if o.Side == "short" {
		return max(ppm, r.IMRPPM)
	}
	return ppm
}

func positionMargin(pos *Position, r Rules) int64 {
	if pos.MarginPPM > 0 {
		return max(pos.MarginPPM, r.IMRPPM)
	}
	if pos.Quantity < 0 {
		return r.IMRPPM
	}
	return 1_000_000
}

func positionUnits(pos *Position, r Rules) int64 {
	if pos.MarginUnits > 0 {
		return pos.MarginUnits
	}
	qty := pos.Quantity
	if qty < 0 {
		qty = -qty
	}
	return qty * positionMargin(pos, r)
}

func addMargin(pos *Position, qty, ppm int64, r Rules) {
	oldQty := pos.Quantity
	if oldQty < 0 {
		oldQty = -oldQty
	}
	pos.MarginUnits = positionUnits(pos, r) + qty*ppm
	pos.MarginPPM = (pos.MarginUnits + oldQty + qty - 1) / (oldQty + qty)
}

func reduceMargin(pos *Position, qty int64, r Rules) {
	oldQty := pos.Quantity
	if oldQty < 0 {
		oldQty = -oldQty
	}
	units := positionUnits(pos, r)
	pos.MarginUnits = units - proportional(units, qty, oldQty)
}

func capitalMargin(pos *Position, price int64, r Rules) int64 {
	// 保存逐笔股数 × 保证金率，避免混合现金/融资持仓的平均 ppm 取整放大冻结。
	hi, lo := bits.Mul64(uint64(price), uint64(positionUnits(pos, r)))
	q, rem := bits.Div64(hi, lo, 1_000_000)
	if rem > 0 {
		q++
	}
	qty := pos.Quantity
	if qty < 0 {
		qty = -qty
	}
	return max(int64(q), rate(qty*price, r.IMRPPM))
}

func proportional(total, part, whole int64) int64 {
	return total/whole*part + total%whole*part/whole
}

// 128 位乘积避免大额本金的计息溢出，分以下余数结转；无额外数据库查询。
func carriedInterest(principal, annual, elapsed, remainder int64) (int64, int64) {
	hi, lo := bits.Mul64(uint64(principal), uint64(annual)*uint64(elapsed))
	lo, carry := bits.Add64(lo, uint64(remainder), 0)
	hi += carry
	q, rem := bits.Div64(hi, lo, 365*86400*1_000_000)
	return int64(q), int64(rem)
}

func (e *Engine) financing(pos *Position, now int64) (int64, int64) {
	if pos.Loan <= 0 || pos.FinancingAt == 0 || now <= pos.FinancingAt {
		return 0, pos.FinancingRemainder
	}
	return carriedInterest(pos.Loan, e.state.Settings.Active.FinancingAnnualPPM, now-pos.FinancingAt, pos.FinancingRemainder)
}

// 仅为缺少新字段的旧配置补默认值，显式设置的零利率保留。
func migrateFinancingRules(raw string, settings *Settings) {
	var previous struct {
		Settings struct {
			InitialCash json.RawMessage
			Active      map[string]json.RawMessage
			Presets     []map[string]json.RawMessage
		}
	}
	if json.Unmarshal([]byte(raw), &previous) != nil {
		return
	}
	if previous.Settings.InitialCash == nil {
		settings.InitialCash = InitialCash
	}
	if _, ok := previous.Settings.Active["financingAnnualPPM"]; !ok {
		settings.Active.FinancingAnnualPPM = 80_000
	}
	for i, preset := range previous.Settings.Presets {
		if _, ok := preset["financingAnnualPPM"]; !ok && i < len(settings.Presets) {
			settings.Presets[i].FinancingAnnualPPM = 80_000
		}
	}
}
