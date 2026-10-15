// Package infra msg 仓储（PG，msg_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"catcube/internal/kernel/db"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 记录不存在")

// Querier pgx 查询接口（Pool 与 Tx 均满足）。
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SessionRow 会话行（含未读与单聊对端）。
type SessionRow struct {
	ID            int64
	Kind          int16
	LastSeq       int64
	LastMessageAt *time.Time
	Unread        int
	PeerID        int64
}

// MessageRow 消息行。
type MessageRow struct {
	ID        int64
	SessionID int64
	Seq       int64
	SenderID  int64
	MsgType   int16
	Content   string
	CreatedAt time.Time
}

// Store msg 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// EnsureSession 幂等创建单聊会话，返回 (id, 是否新建)。
func (s *Store) EnsureSession(ctx context.Context, tx pgx.Tx, id, min, max int64) (int64, bool, error) {
	var sid int64
	err := tx.QueryRow(ctx, `
		INSERT INTO msg_session (id, kind, min_uid, max_uid, created_at, updated_at)
		VALUES ($1, 1, $2, $3, now(), now())
		ON CONFLICT (min_uid, max_uid) WHERE kind = 1 DO NOTHING
		RETURNING id`, id, min, max).Scan(&sid)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM msg_session WHERE kind=1 AND min_uid=$1 AND max_uid=$2`, min, max).Scan(&sid)
		return sid, false, err
	}
	if err != nil {
		return 0, false, err
	}
	return sid, true, nil
}

// AddMember 加入会话成员（幂等）。
func (s *Store) AddMember(ctx context.Context, q Querier, sessionID, userID int64, role int16) error {
	_, err := q.Exec(ctx, `
		INSERT INTO msg_session_member (session_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (session_id, user_id) DO NOTHING`, sessionID, userID, role)
	return err
}

// IsMember 是否会话成员。
func (s *Store) IsMember(ctx context.Context, q Querier, sessionID, userID int64) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM msg_session_member WHERE session_id=$1 AND user_id=$2)`,
		sessionID, userID).Scan(&ok)
	return ok, err
}

// ListSessions 我的会话列表（未读=最大序号−已读游标）。
func (s *Store) ListSessions(ctx context.Context, userID int64) ([]SessionRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.kind, s.last_seq, s.last_message_at,
		       GREATEST(COALESCE(s.last_seq - c.read_seq, s.last_seq), 0) AS unread,
		       COALESCE((SELECT pm.user_id FROM msg_session_member pm
		                  WHERE pm.session_id=s.id AND pm.user_id<>$1 LIMIT 1), 0) AS peer_id
		  FROM msg_session s
		  JOIN msg_session_member m ON m.session_id=s.id AND m.user_id=$1
		  LEFT JOIN msg_read_cursor c ON c.session_id=s.id AND c.user_id=$1
		 ORDER BY s.last_message_at DESC NULLS LAST, s.id DESC
		 LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.LastSeq, &r.LastMessageAt, &r.Unread, &r.PeerID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LockSession 行锁取最大序号（并发发送时分配 seq）。
func (s *Store) LockSession(ctx context.Context, tx pgx.Tx, sessionID int64) (int64, error) {
	var last int64
	err := tx.QueryRow(ctx, `SELECT last_seq FROM msg_session WHERE id=$1 FOR UPDATE`, sessionID).Scan(&last)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return last, err
}

// InsertMessage 落库消息。
func (s *Store) InsertMessage(ctx context.Context, tx pgx.Tx, m MessageRow) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO msg_message (id, session_id, seq, sender_id, msg_type, content, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		m.ID, m.SessionID, m.Seq, m.SenderID, m.MsgType, m.Content, m.CreatedAt)
	return err
}

// BumpSession 推进会话最大序号。
func (s *Store) BumpSession(ctx context.Context, tx pgx.Tx, sessionID, seq int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE msg_session SET last_seq=$2, last_message_at=now(), updated_at=now() WHERE id=$1`, sessionID, seq)
	return err
}

// IncrOthersUnread 其它成员未读 +1。
func (s *Store) IncrOthersUnread(ctx context.Context, tx pgx.Tx, sessionID, exceptUserID int64) error {
	_, err := tx.Exec(ctx,
		`UPDATE msg_session_member SET unread_count=unread_count+1 WHERE session_id=$1 AND user_id<>$2`, sessionID, exceptUserID)
	return err
}

// SetRead 提交某用户已读游标（取较大值）并清零其未读。
func (s *Store) SetRead(ctx context.Context, tx pgx.Tx, sessionID, userID, seq int64) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO msg_read_cursor (session_id, user_id, read_seq, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (session_id, user_id) DO UPDATE
		SET read_seq=GREATEST(msg_read_cursor.read_seq, EXCLUDED.read_seq), updated_at=now()`,
		sessionID, userID, seq); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE msg_session_member SET unread_count=0 WHERE session_id=$1 AND user_id=$2`, sessionID, userID)
	return err
}

// MarkRead 提交已读游标并按最新序号重算未读（幂等）。
func (s *Store) MarkRead(ctx context.Context, tx pgx.Tx, sessionID, userID, readSeq int64) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO msg_read_cursor (session_id, user_id, read_seq, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (session_id, user_id) DO UPDATE
		SET read_seq=GREATEST(msg_read_cursor.read_seq, EXCLUDED.read_seq), updated_at=now()`,
		sessionID, userID, readSeq); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE msg_session_member m
		   SET unread_count=GREATEST(s.last_seq - c.read_seq, 0)
		  FROM msg_session s, msg_read_cursor c
		 WHERE m.session_id=$1 AND m.user_id=$2 AND s.id=$1 AND c.session_id=$1 AND c.user_id=$2`,
		sessionID, userID)
	return err
}

// Messages 增量拉取（seq > cursor 升序）。
func (s *Store) Messages(ctx context.Context, q Querier, sessionID, afterSeq int64, limit int) ([]MessageRow, error) {
	rows, err := q.Query(ctx, `
		SELECT id, session_id, seq, sender_id, msg_type, content, created_at
		  FROM msg_message WHERE session_id=$1 AND seq>$2 ORDER BY seq ASC LIMIT $3`,
		sessionID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageRow
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &m.SenderID, &m.MsgType, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- 分区与归档 ----

var partitionRe = regexp.MustCompile(`^msg_message_[0-9]{6}$`)

// EnsureMonthPartition 幂等创建某月分区。
func (s *Store) EnsureMonthPartition(ctx context.Context, month time.Time) (string, error) {
	from := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	name := "msg_message_" + from.Format("200601")
	_, err := s.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF msg_message FOR VALUES FROM ('%s') TO ('%s')`,
		name, from.Format("2006-01-02"), to.Format("2006-01-02")))
	return name, err
}

// ListMonthPartitions 列出按月分区名（排除 default）。
func (s *Store) ListMonthPartitions(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.relname
		  FROM pg_class c
		  JOIN pg_inherits i ON i.inhrelid = c.oid
		  JOIN pg_class p ON p.oid = i.inhparent
		 WHERE p.relname = 'msg_message' AND c.relname LIKE 'msg_message\_%'
		 ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		if partitionRe.MatchString(n) {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

// CountPartition 统计分区行数。
func (s *Store) CountPartition(ctx context.Context, name string) (int64, error) {
	if !partitionRe.MatchString(name) {
		return 0, fmt.Errorf("infra: 非法分区名 %q", name)
	}
	var n int64
	err := s.pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s`, name)).Scan(&n)
	return n, err
}

// ExportPartition 导出分区为 JSON Lines（每行一条消息），返回导出行数。
func (s *Store) ExportPartition(ctx context.Context, name string, w io.Writer) (int64, error) {
	if !partitionRe.MatchString(name) {
		return 0, fmt.Errorf("infra: 非法分区名 %q", name)
	}
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT id, session_id, seq, sender_id, msg_type, content, created_at FROM %s ORDER BY seq, id`, name))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	enc := json.NewEncoder(w)
	var n int64
	for rows.Next() {
		var (
			id, sessionID, seq, senderID int64
			msgType                      int16
			content                      string
			createdAt                    time.Time
		)
		if err := rows.Scan(&id, &sessionID, &seq, &senderID, &msgType, &content, &createdAt); err != nil {
			return n, err
		}
		if err := enc.Encode(map[string]any{
			"id": strconv.FormatInt(id, 10), "session_id": strconv.FormatInt(sessionID, 10),
			"seq": seq, "sender_id": strconv.FormatInt(senderID, 10),
			"msg_type": msgType, "content": content, "created_at": createdAt,
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// MarkArchived 标记分区已归档（记录压缩对象 key）。
func (s *Store) MarkArchived(ctx context.Context, name, objectKey string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE msg_archive_meta SET object_key=$2, status=1 WHERE partition_name=$1`, name, objectKey)
	return err
}

// DetachPartition 分离分区（归档成功后从主表摘除，保留表数据供对象存储检索）。
func (s *Store) DetachPartition(ctx context.Context, name string) error {
	if !partitionRe.MatchString(name) {
		return fmt.Errorf("infra: 非法分区名 %q", name)
	}
	_, err := s.pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE msg_message DETACH PARTITION %s`, name))
	return err
}

// UpsertArchiveMeta 记录归档元数据（幂等）。
func (s *Store) UpsertArchiveMeta(ctx context.Context, name string, from, to time.Time, rowCount int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO msg_archive_meta (partition_name, range_from, range_to, row_count, status, created_at)
		VALUES ($1,$2,$3,$4,0,now())
		ON CONFLICT (partition_name) DO UPDATE SET row_count=EXCLUDED.row_count`,
		name, from, to, rowCount)
	return err
}
