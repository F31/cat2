package pg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
		`SELECT btrim(hash) FROM sys_audit ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&h)
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

// ListChain 按链序读取审计记录（断链检测，p6 §1.6）。
func (a *Audit) ListChain(ctx context.Context, limit int) ([]audit.ChainEntry, error) {
	return a.list(ctx, time.Time{}, limit)
}

func (a *Audit) list(ctx context.Context, since time.Time, limit int) ([]audit.ChainEntry, error) {
	cond := ""
	args := []any{limit}
	if !since.IsZero() {
		cond = "WHERE created_at >= $2"
		args = append(args, since)
	}
	rows, err := a.pool.Query(ctx, `
		SELECT id, actor_id, actor_type, channel, action, target_ref, detail, result,
			COALESCE(host(ip),''), COALESCE(user_agent,''), COALESCE(request_id,''), btrim(prev_hash), btrim(hash),
			EXTRACT(EPOCH FROM created_at)::bigint
		FROM sys_audit `+cond+` ORDER BY created_at, id LIMIT $1`, args...)
	if err != nil {
		return nil, err
	}
	return a.scanChainRows(rows)
}

func (a *Audit) scanChainRows(rows pgx.Rows) ([]audit.ChainEntry, error) {
	defer rows.Close()
	var out []audit.ChainEntry
	for rows.Next() {
		var ce audit.ChainEntry
		var e audit.Entry
		var target, detail []byte
		if err := rows.Scan(&ce.ID, &e.ActorID, &e.ActorType, &e.Channel, &e.Action, &target, &detail,
			&e.Result, &e.IP, &e.UserAgent, &e.RequestID, &ce.PrevHash, &ce.Hash, &e.CreatedAt); err != nil {
			return nil, err
		}
		// 归一为紧凑 JSON：与写入时 canonical 用的 marshal 形式一致（jsonb 文本带空格）
		e.TargetRef = compactJSON(target)
		if len(detail) > 0 {
			var v any
			_ = json.Unmarshal(detail, &v)
			e.Detail = v
		}
		ce.Entry = e
		out = append(out, ce)
	}
	return out, rows.Err()
}

// Cursor 读取增量校验游标（sys_settings: platform/audit.verify.cursor，值 "id:hash"）。
func (a *Audit) Cursor(ctx context.Context) (int64, string, error) {
	var v string
	err := a.pool.QueryRow(ctx,
		`SELECT value FROM sys_settings WHERE scope='platform' AND key='audit.verify.cursor'`).Scan(&v)
	if err != nil {
		return 0, "", nil // 无游标：从头/空链开始
	}
	parts := strings.SplitN(v, ":", 2)
	id, _ := strconv.ParseInt(parts[0], 10, 64)
	if len(parts) == 2 {
		return id, parts[1], nil
	}
	return id, "", nil
}

// Latest 链上最新一条（建立校验基线）。
func (a *Audit) Latest(ctx context.Context) (int64, string, error) {
	var id int64
	var h string
	err := a.pool.QueryRow(ctx,
		`SELECT id, btrim(hash) FROM sys_audit ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&id, &h)
	if err != nil {
		return 0, "", nil
	}
	return id, h, nil
}

// SetCursor 推进增量校验游标。
func (a *Audit) SetCursor(ctx context.Context, id int64, hash string) error {
	_, err := a.pool.Exec(ctx, `
		INSERT INTO sys_settings (scope, key, value, type, updated_at)
		VALUES ('platform','audit.verify.cursor',$1,'string',now())
		ON CONFLICT (scope, key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		fmt.Sprintf("%d:%s", id, hash))
	return err
}

// ListChainAfter 读取 id 之后的审计记录（增量断链监控）。
func (a *Audit) ListChainAfter(ctx context.Context, afterID int64, limit int) ([]audit.ChainEntry, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, actor_id, actor_type, channel, action, target_ref, detail, result,
			COALESCE(host(ip),''), COALESCE(user_agent,''), COALESCE(request_id,''), btrim(prev_hash), btrim(hash),
			EXTRACT(EPOCH FROM created_at)::bigint
		FROM sys_audit WHERE id > $1 ORDER BY created_at, id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, err
	}
	return a.scanChainRows(rows)
}

// ListChainFrom 读取时间窗内审计记录（断链监控用）。
func (a *Audit) ListChainFrom(ctx context.Context, sinceUnix int64, limit int) ([]audit.ChainEntry, error) {
	since := time.Unix(sinceUnix, 0)
	return a.list(ctx, since, limit)
}

// compactJSON 去除 JSON 文本中的空白（保持键序与值不变）。
func compactJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

var (
	_ audit.Store             = (*Audit)(nil)
	_ audit.ChainLister       = (*Audit)(nil)
	_ audit.ChainListerWindow = (*Audit)(nil)
	_ audit.ChainListerAfter  = (*Audit)(nil)
)
