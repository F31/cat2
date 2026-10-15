// Package infra trade 仓储（PG，trd_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"catcube/internal/kernel/db"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Querier pgx 查询接口（Pool 与 Tx 均满足）。
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Order 订单记录。
type Order struct {
	ID            int64
	BizType       string
	BizRef        string
	BuyerID       int64
	AmountFen     int64
	PriceSnapshot []byte
	Status        int16
	PayExpireAt   time.Time
	PaidAt        *time.Time
	ClosedAt      *time.Time
}

// Payment 支付单。
type Payment struct {
	ID         int64
	OrderID    int64
	OutTradeNo string
	Channel    string
	AmountFen  int64
	Status     int16
	PaidAt     *time.Time
}

// Refund 退款单。
type Refund struct {
	ID         int64
	PaymentID  int64
	Reason     string
	AmountFen  int64
	Status     int16
	RefundNo   string
	OperatorID int64
	IdemKey    string
}

// Fulfillment 履约待办。
type Fulfillment struct {
	OrderID    int64
	BizType    string
	Status     int16
	RetryCount int
	LastError  string
}

// Store trade 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// InsertOrder 写订单。
func (s *Store) InsertOrder(ctx context.Context, tx db.Tx, o Order) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trd_order (id, biz_type, biz_ref, buyer_id, amount_fen, price_snapshot, status, pay_expire_at, idempotency_key, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now())`,
		o.ID, o.BizType, nullStr(o.BizRef), o.BuyerID, o.AmountFen, snap(o.PriceSnapshot), o.Status, o.PayExpireAt, nil)
	return err
}

// InsertOrderIdem 写订单（带幂等键）。
func (s *Store) InsertOrderIdem(ctx context.Context, tx db.Tx, o Order, idem string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trd_order (id, biz_type, biz_ref, buyer_id, amount_fen, price_snapshot, status, pay_expire_at, idempotency_key, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now())`,
		o.ID, o.BizType, nullStr(o.BizRef), o.BuyerID, o.AmountFen, snap(o.PriceSnapshot), o.Status, o.PayExpireAt, nullStr(idem))
	return err
}

const orderCols = `id, biz_type, COALESCE(biz_ref,''), buyer_id, amount_fen, price_snapshot, status, pay_expire_at, paid_at, closed_at`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.BizType, &o.BizRef, &o.BuyerID, &o.AmountFen, &o.PriceSnapshot, &o.Status, &o.PayExpireAt, &o.PaidAt, &o.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	return o, err
}

// GetOrder 按 id。
func (s *Store) GetOrder(ctx context.Context, id int64) (Order, error) {
	return scanOrder(s.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM trd_order WHERE id=$1`, id))
}

// GetOrderByIdem 按幂等键。
// AdminListOrders 后台订单列表（status<0 表示全部）。
func (s *Store) AdminListOrders(ctx context.Context, status int16, limit int) ([]Order, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+orderCols+` FROM trd_order
		 WHERE ($1 < 0 OR status=$1) ORDER BY created_at DESC, id DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) GetOrderByIdem(ctx context.Context, idem string) (Order, error) {
	return scanOrder(s.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM trd_order WHERE idempotency_key=$1`, idem))
}

// ListExpired 待支付且已超时的订单 id。
func (s *Store) ListExpired(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM trd_order WHERE status=0 AND pay_expire_at < now() ORDER BY pay_expire_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MarkOrderPaid 置已支付。
func (s *Store) MarkOrderPaid(ctx context.Context, tx db.Tx, id int64) error {
	_, err := tx.Exec(ctx, `UPDATE trd_order SET status=1, paid_at=now(), version=version+1, updated_at=now() WHERE id=$1 AND status=0`, id)
	return err
}

// CloseOrder 关闭订单。
func (s *Store) CloseOrder(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE trd_order SET status=2, closed_at=now(), version=version+1, updated_at=now() WHERE id=$1 AND status=0`, id)
	return err
}

// InsertPayment 写支付单。
func (s *Store) InsertPayment(ctx context.Context, tx db.Tx, p Payment) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trd_payment (id, order_id, out_trade_no, channel, amount_fen, status, created_at)
		VALUES ($1,$2,$3,$4,$5,0,now())`,
		p.ID, p.OrderID, p.OutTradeNo, p.Channel, p.AmountFen)
	return err
}

// GetPaymentByOutTradeNo 按商户单号。
func (s *Store) GetPaymentByOutTradeNo(ctx context.Context, no string) (Payment, error) {
	var p Payment
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_id, out_trade_no, channel, amount_fen, status, paid_at FROM trd_payment WHERE out_trade_no=$1`, no).
		Scan(&p.ID, &p.OrderID, &p.OutTradeNo, &p.Channel, &p.AmountFen, &p.Status, &p.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// MarkPaymentPaid 置支付单已支付（幂等：仅 status=0 生效）。
func (s *Store) MarkPaymentPaid(ctx context.Context, tx db.Tx, id int64, rawNotify []byte) error {
	_, err := tx.Exec(ctx, `UPDATE trd_payment SET status=1, paid_at=now(), raw_notify=$2 WHERE id=$1 AND status=0`, id, rawNotify)
	return err
}

// LatestPaidPayment 最近一笔已支付单。
func (s *Store) LatestPaidPayment(ctx context.Context, orderID int64) (Payment, error) {
	var p Payment
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_id, out_trade_no, channel, amount_fen, status, paid_at FROM trd_payment WHERE order_id=$1 AND status=1 ORDER BY paid_at DESC LIMIT 1`, orderID).
		Scan(&p.ID, &p.OrderID, &p.OutTradeNo, &p.Channel, &p.AmountFen, &p.Status, &p.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// InsertRefund 写退款单（幂等键唯一）。
func (s *Store) InsertRefund(ctx context.Context, tx db.Tx, r Refund) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trd_refund (id, payment_id, reason, amount_fen, status, refund_no, operator_id, idem_key, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())`,
		r.ID, r.PaymentID, r.Reason, r.AmountFen, r.Status, r.RefundNo, r.OperatorID, r.IdemKey)
	return err
}

// GetRefundByIdem 按幂等键。
func (s *Store) GetRefundByIdem(ctx context.Context, idem string) (Refund, error) {
	var r Refund
	err := s.pool.QueryRow(ctx,
		`SELECT id, payment_id, reason, amount_fen, status, refund_no, operator_id, idem_key FROM trd_refund WHERE idem_key=$1`, idem).
		Scan(&r.ID, &r.PaymentID, &r.Reason, &r.AmountFen, &r.Status, &r.RefundNo, &r.OperatorID, &r.IdemKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// MarkRefundStatus 更新退款状态。
func (s *Store) MarkRefundStatus(ctx context.Context, tx db.Tx, id int64, status int16) error {
	_, err := tx.Exec(ctx, `UPDATE trd_refund SET status=$2 WHERE id=$1`, id, status)
	return err
}

// InsertFulfillment 写履约待办。
func (s *Store) InsertFulfillment(ctx context.Context, tx db.Tx, orderID int64, bizType string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO trd_fulfillment (order_id, biz_type, status, retry_count, created_at, updated_at)
		VALUES ($1,$2,0,0,now(),now()) ON CONFLICT (order_id) DO NOTHING`, orderID, bizType)
	return err
}

// PendingFulfillments 待履约（含可重试）。
func (s *Store) PendingFulfillments(ctx context.Context, limit int) ([]Fulfillment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT order_id, biz_type, status, retry_count, COALESCE(last_error,'') FROM trd_fulfillment WHERE status=0 AND retry_count < 8 ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Fulfillment
	for rows.Next() {
		var f Fulfillment
		if err := rows.Scan(&f.OrderID, &f.BizType, &f.Status, &f.RetryCount, &f.LastError); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// UpdateFulfillment 更新履约状态（成功=1；失败 retry+1，≥8 置死信 9）。
func (s *Store) UpdateFulfillment(ctx context.Context, orderID int64, ok bool, lastErr string) error {
	if ok {
		_, err := s.pool.Exec(ctx, `UPDATE trd_fulfillment SET status=1, updated_at=now() WHERE order_id=$1`, orderID)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE trd_fulfillment SET retry_count=retry_count+1,
			status=CASE WHEN retry_count+1>=8 THEN 9 ELSE 0 END,
			last_error=$2, updated_at=now() WHERE order_id=$1`, orderID, lastErr)
	return err
}

// ResetFulfillment 死信手动补偿：重置为待履约。
func (s *Store) ResetFulfillment(ctx context.Context, orderID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE trd_fulfillment SET status=0, retry_count=0, last_error='人工补偿', updated_at=now() WHERE order_id=$1 AND status=9`, orderID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UpsertRecon 写对账记录。
func (s *Store) UpsertRecon(ctx context.Context, date string, channel string, orderCount int, amountFen int64, diff bool, detail []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO trd_recon (recon_date, channel, order_count, amount_fen, diff_status, diff_detail)
		VALUES ($1::date,$2,$3,$4,$5,$6)
		ON CONFLICT (recon_date, channel) DO UPDATE SET order_count=EXCLUDED.order_count,
			amount_fen=EXCLUDED.amount_fen, diff_status=EXCLUDED.diff_status, diff_detail=EXCLUDED.diff_detail`,
		date, channel, orderCount, amountFen, boolToStatus(diff), detail)
	return err
}

func snap(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func boolToStatus(b bool) int16 {
	if b {
		return 1
	}
	return 0
}

// MarshalSnapshot 序列化价格快照。
func MarshalSnapshot(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ---- 分摊认筹 ----

// ApportionGroup 分摊组行。
type ApportionGroup struct {
	ID           int64
	CubePlanRef  string
	InitiatorID  int64
	TotalFen     int64
	PerFen       int64
	TargetN      int
	PledgedCount int
	Deadline     time.Time
	Status       int16
	Snapshot     []byte
}

const apportionCols = `id, COALESCE(cube_plan_ref,''), initiator_id, total_fen, per_fen, target_n,
	pledged_count, deadline, status, price_snapshot`

func scanApportion(row pgx.Row) (ApportionGroup, error) {
	var g ApportionGroup
	err := row.Scan(&g.ID, &g.CubePlanRef, &g.InitiatorID, &g.TotalFen, &g.PerFen, &g.TargetN,
		&g.PledgedCount, &g.Deadline, &g.Status, &g.Snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApportionGroup{}, ErrNotFound
	}
	return g, err
}

// InsertApportion 新建分摊组。
func (s *Store) InsertApportion(ctx context.Context, q Querier, g ApportionGroup) error {
	_, err := q.Exec(ctx, `
		INSERT INTO trd_apportion_group (id, cube_plan_ref, initiator_id, total_fen, per_fen, target_n, deadline, status, price_snapshot)
		VALUES ($1,$2,$3,$4,$5,$6,$7,0,$8)`,
		g.ID, g.CubePlanRef, g.InitiatorID, g.TotalFen, g.PerFen, g.TargetN, g.Deadline, g.Snapshot)
	return err
}

// GetApportion 取分摊组。
func (s *Store) GetApportion(ctx context.Context, id int64) (ApportionGroup, error) {
	return scanApportion(s.pool.QueryRow(ctx, `SELECT `+apportionCols+` FROM trd_apportion_group WHERE id=$1`, id))
}

// LockApportion 锁分摊组行（成团强一致）。
func (s *Store) LockApportion(ctx context.Context, tx db.Tx, id int64) (ApportionGroup, error) {
	return scanApportion(tx.QueryRow(ctx, `SELECT `+apportionCols+` FROM trd_apportion_group WHERE id=$1 FOR UPDATE`, id))
}

// SetApportionStatus 变更加分摊组状态。
func (s *Store) SetApportionStatus(ctx context.Context, q Querier, id int64, status int16) error {
	_, err := q.Exec(ctx, `UPDATE trd_apportion_group SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// IncrPledged 认筹计数（强一致，行已锁）。
func (s *Store) IncrPledged(ctx context.Context, q Querier, id int64) error {
	_, err := q.Exec(ctx, `UPDATE trd_apportion_group SET pledged_count=pledged_count+1, updated_at=now() WHERE id=$1`, id)
	return err
}

// InsertPledge 认筹记录（支付成功后写入）；返回是否新插入（重复认筹不计数）。
func (s *Store) InsertPledge(ctx context.Context, q Querier, id, groupID, userID, amountFen, paymentID int64) (bool, error) {
	tag, err := q.Exec(ctx, `INSERT INTO trd_pledge (id, group_id, user_id, amount_fen, payment_id, status)
		VALUES ($1,$2,$3,$4,$5,0) ON CONFLICT (group_id, user_id) DO NOTHING`,
		id, groupID, userID, amountFen, nullID(paymentID))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListPendingRefunds 未成团组中待退款的认筹（幂等逐笔）。
func (s *Store) ListPendingRefunds(ctx context.Context, groupID int64) ([]Pledge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, group_id, user_id, amount_fen, COALESCE(payment_id,0), status FROM trd_pledge
		 WHERE group_id=$1 AND status=0 ORDER BY id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pledge
	for rows.Next() {
		var p Pledge
		if err := rows.Scan(&p.ID, &p.GroupID, &p.UserID, &p.AmountFen, &p.PaymentID, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Pledge 认筹行。
type Pledge struct {
	ID        int64
	GroupID   int64
	UserID    int64
	AmountFen int64
	PaymentID int64
	Status    int16
}

// GetPledge 取认筹。
func (s *Store) GetPledge(ctx context.Context, groupID, userID int64) (Pledge, error) {
	var p Pledge
	err := s.pool.QueryRow(ctx,
		`SELECT id, group_id, user_id, amount_fen, COALESCE(payment_id,0), status
		 FROM trd_pledge WHERE group_id=$1 AND user_id=$2`, groupID, userID).
		Scan(&p.ID, &p.GroupID, &p.UserID, &p.AmountFen, &p.PaymentID, &p.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pledge{}, ErrNotFound
	}
	return p, err
}

// MarkPledgeRefunded 标记认筹已退款。
func (s *Store) MarkPledgeRefunded(ctx context.Context, q Querier, id int64) error {
	_, err := q.Exec(ctx, `UPDATE trd_pledge SET status=1, refunded_at=now() WHERE id=$1 AND status=0`, id)
	return err
}

// ListExpiredApportions 到期未成团的分摊组。
func (s *Store) ListExpiredApportions(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM trd_apportion_group WHERE status=0 AND deadline<now() ORDER BY deadline LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CountUnsettledPledges 未退款认筹数（全部退款完成才终结）。
func (s *Store) CountUnsettledPledges(ctx context.Context, groupID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(1) FROM trd_pledge WHERE group_id=$1 AND status=0`, groupID).Scan(&n)
	return n, err
}

// GetPaymentByID 按支付单 ID。
func (s *Store) GetPaymentByID(ctx context.Context, id int64) (Payment, error) {
	var p Payment
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_id, out_trade_no, channel, amount_fen, status, paid_at FROM trd_payment WHERE id=$1`, id).
		Scan(&p.ID, &p.OrderID, &p.OutTradeNo, &p.Channel, &p.AmountFen, &p.Status, &p.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func nullID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
