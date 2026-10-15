package pg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/pkg/ids"
)

// Audit 审计存储（sys_audit，哈希链追加写）。
type Audit struct{ pool *db.Pool }

func NewAudit(pool *db.Pool) *Audit { return &Audit{pool: pool} }

// LastHash 取链上最新 hash（空串表示链空）。
func (a *Audit) LastHash(ctx context.Context) (string, error) {
	var h string
	err := a.pool.QueryRow(ctx,
		`SELECT hash FROM sys_audit ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&h)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return h, err
}

// Append 追加写（tx 可为 nil：独立写入；否则复用业务事务）。
func (a *Audit) Append(ctx context.Context, tx any, e audit.Entry) error {
	var detail, target any
	if e.Detail != nil {
		raw, err := json.Marshal(e.Detail)
		if err != nil {
			return err
		}
		detail = raw
	}
	if e.TargetRef != "" {
		target = json.RawMessage(e.TargetRef)
	}
	var ip any
	if e.IP != "" {
		ip = e.IP
	}
	at := time.Unix(e.CreatedAt, 0)

	const q = `INSERT INTO sys_audit
		(id, actor_id, actor_type, channel, action, target_ref, detail, result, ip, user_agent, request_id, prev_hash, hash, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	args := []any{int64(ids.New()), e.ActorID, e.ActorType, e.Channel, e.Action, target, detail,
		e.Result, ip, e.UserAgent, e.RequestID, e.PrevHash, e.Hash, at}

	if tx != nil {
		if t, ok := tx.(db.Tx); ok {
			_, err := t.Exec(ctx, q, args...)
			return err
		}
	}
	_, err := a.pool.Exec(ctx, q, args...)
	return err
}

var _ audit.Store = (*Audit)(nil)
