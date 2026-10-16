// Package contract membership 对外契约：报价、开通、会员状态、子账号。
package contract

import (
	"context"
	"time"
)

// 会员等级。
const (
	LevelVIP        int16 = 1 // 普通 VIP
	LevelSVIP       int16 = 2 // 预留档位（一期不上架）
	LevelEnterprise int16 = 3 // 企业版
)

// Quote 报价（同时给原价与折后价）。
type Quote struct {
	Level        int16 `json:"level"`
	Years        int   `json:"years"`
	UnitFen      int64 `json:"unit_fen"`      // 年费单价
	OriginalFen  int64 `json:"original_fen"`  // 原价=单价×年限
	DiscountRate int   `json:"discount_rate"` // 折扣系数万分比（10000=无折扣）
	TotalFen     int64 `json:"total_fen"`     // 折后价
}

// Member 会员状态。
type Member struct {
	UserID    int64     `json:"user_id,string"`
	Level     int16     `json:"level"`
	Status    int16     `json:"status"` // 1生效中/2已过期
	ExpiresAt time.Time `json:"expires_at"`
}

// SubAccount 子账号。
type SubAccount struct {
	ID        int64     `json:"id,string"`
	MasterID  int64     `json:"master_id,string"`
	SubUserID int64     `json:"sub_user_id,string"`
	Status    int16     `json:"status"`
	InvitedAt time.Time `json:"invited_at"`
}

// Facade membership 对外接口。
type Facade interface {
	// Quote 报价（level,years）。
	Quote(ctx context.Context, level int16, years int) (Quote, error)
	// Open 开通/续费：按 Quote 计算并创建 trade 订单，返回订单参数。
	Open(ctx context.Context, userID int64, level int16, years int) (OrderResult, error)
	// Me 我的会员状态。
	Me(ctx context.Context, userID int64) (Member, bool, error)
	// LevelOf 会员等级与是否生效（供 discovery 同分排序/权限）。
	LevelOf(ctx context.Context, userID int64) (int16, bool, error)
	// UserIDsByLevel 按等级列出生效中会员（治理检索用；limit 上限 500）。
	UserIDsByLevel(ctx context.Context, level int16, limit int) ([]int64, error)
	// SubAccounts 子账号列表（企业版主账号）。
	SubAccounts(ctx context.Context, masterID int64) ([]SubAccount, error)
	// InviteSub 邀请子账号（须已注册已实名；企业版子账号上限 3）。
	InviteSub(ctx context.Context, masterID, subUserID int64) error
	// RemoveSub 移除子账号。
	RemoveSub(ctx context.Context, masterID, subUserID int64) error
	// RefundQuote 退款金额=剩余完整年数×年费×实付折扣系数（取订单快照）。
	RefundQuote(ctx context.Context, orderID int64) (int64, error)
}

// OrderResult 开通下单结果。
type OrderResult struct {
	OrderID     int64 `json:"order_id,string"`
	AmountFen   int64 `json:"amount_fen"`
	OriginalFen int64 `json:"original_fen"`
}
