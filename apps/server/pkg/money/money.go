// Package money 金额计算：int64 分；折扣系数 numeric(6,4) 以万分比整数表示；
// 统一四舍五入到分。
package money

// Fen 金额，单位分。
type Fen int64

// Rate 折扣系数，万分比（9000 = 0.9000），对应 numeric(6,4)。
type Rate int64

// Mul 金额 × 系数，四舍五入到分。
func (f Fen) Mul(r Rate) Fen {
	v := int64(f) * int64(r)
	return Fen((v + 5000) / 10000)
}

// Years 年费 × 年限 × 折扣系数。
func Years(annual Fen, years int, discount Rate) Fen {
	return Fen(int64(annual) * int64(years)).Mul(discount)
}

// Div 金额均分 N 份，四舍五入到分（分摊每人=档价÷N）。
func (f Fen) Div(n int) Fen {
	if n <= 0 {
		return 0
	}
	v := int64(f)
	return Fen((v + int64(n)/2) / int64(n))
}

// ProrataRefund 退款金额 = 剩余完整年数 × 年费 × 实付折扣系数（取订单快照）。
func ProrataRefund(annual Fen, remainingYears int, paidDiscount Rate) Fen {
	if remainingYears <= 0 {
		return 0
	}
	return Fen(int64(annual) * int64(remainingYears)).Mul(paidDiscount)
}
