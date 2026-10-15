// Package app msg 应用服务：单聊发起、发送、增量同步、已读、分区与归档。
package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/policy"
	"catcube/internal/modules/msg/contract"
	"catcube/internal/modules/msg/internal/domain"
	"catcube/internal/modules/msg/internal/infra"
	"catcube/internal/shared/entityref"
	"catcube/pkg/ids"
)

// Repo 应用层仓储能力。
type Repo interface {
	EnsureSession(ctx context.Context, tx db.Tx, id, min, max int64) (int64, bool, error)
	AddMember(ctx context.Context, q infra.Querier, sessionID, userID int64, role int16) error
	IsMember(ctx context.Context, q infra.Querier, sessionID, userID int64) (bool, error)
	ListSessions(ctx context.Context, userID int64) ([]infra.SessionRow, error)
	LockSession(ctx context.Context, tx db.Tx, sessionID int64) (int64, error)
	InsertMessage(ctx context.Context, tx db.Tx, m infra.MessageRow) error
	BumpSession(ctx context.Context, tx db.Tx, sessionID, seq int64) error
	IncrOthersUnread(ctx context.Context, tx db.Tx, sessionID, exceptUserID int64) error
	SetRead(ctx context.Context, tx db.Tx, sessionID, userID, seq int64) error
	MarkRead(ctx context.Context, tx db.Tx, sessionID, userID, readSeq int64) error
	Messages(ctx context.Context, q infra.Querier, sessionID, afterSeq int64, limit int) ([]infra.MessageRow, error)
	EnsureMonthPartition(ctx context.Context, month time.Time) (string, error)
	ListMonthPartitions(ctx context.Context) ([]string, error)
	CountPartition(ctx context.Context, name string) (int64, error)
	UpsertArchiveMeta(ctx context.Context, name string, from, to time.Time, rowCount int64) error
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Policy   *policy.Engine
	Resolver *entityref.Registry
}

// Service msg 应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	policy   *policy.Engine
	resolver *entityref.Registry
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, policy: o.Policy, resolver: o.Resolver}
}

// EnsureSession 发起单聊（幂等；发起=会员权益）。
func (s *Service) EnsureSession(ctx context.Context, userID int64, targetRef string) (contract.EnsureResult, error) {
	peerID, err := parseUserRef(targetRef)
	if err != nil {
		return contract.EnsureResult{}, err
	}
	if peerID == userID {
		return contract.EnsureResult{}, domain.ErrSelfSession
	}
	if s.policy != nil {
		ok, err := s.policy.Has(ctx, s.policy.NewActor(userID, 1), "member.active")
		if err != nil {
			return contract.EnsureResult{}, err
		}
		if !ok {
			return contract.EnsureResult{}, domain.ErrMemberRequired
		}
	}
	min, max := domain.DMKey(userID, peerID)
	var sid int64
	var created bool
	err = s.pool.InTx(ctx, func(tx db.Tx) error {
		id, c, err := s.repo.EnsureSession(ctx, tx, int64(ids.New()), min, max)
		if err != nil {
			return err
		}
		sid, created = id, c
		if c {
			if err := s.repo.AddMember(ctx, tx, sid, userID, 1); err != nil {
				return err
			}
			if err := s.repo.AddMember(ctx, tx, sid, peerID, 1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return contract.EnsureResult{}, err
	}
	return contract.EnsureResult{
		Session: contract.Session{ID: sid, Kind: contract.KindDM, Peer: s.peer(ctx, peerID)},
		Created: created,
	}, nil
}

// Sessions 我的会话列表。
func (s *Service) Sessions(ctx context.Context, userID int64) ([]contract.Session, error) {
	rows, err := s.repo.ListSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Session, 0, len(rows))
	for _, r := range rows {
		item := contract.Session{ID: r.ID, Kind: r.Kind, LastSeq: r.LastSeq, Unread: r.Unread, LastMessageAt: r.LastMessageAt}
		if r.Kind == contract.KindDM && r.PeerID > 0 {
			item.Peer = s.peer(ctx, r.PeerID)
		}
		out = append(out, item)
	}
	return out, nil
}

// Send 发送消息（成员校验；seq 由会话行锁分配）。
func (s *Service) Send(ctx context.Context, userID, sessionID int64, msgType int16, content string) (contract.Message, error) {
	if err := domain.ValidateContent(msgType, content); err != nil {
		return contract.Message{}, err
	}
	var m infra.MessageRow
	err := s.pool.InTx(ctx, func(tx db.Tx) error {
		last, err := s.repo.LockSession(ctx, tx, sessionID)
		if errors.Is(err, infra.ErrNotFound) {
			return domain.ErrSessionNotFound
		}
		if err != nil {
			return err
		}
		ok, err := s.repo.IsMember(ctx, tx, sessionID, userID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrNotParticipant
		}
		m = infra.MessageRow{
			ID: int64(ids.New()), SessionID: sessionID, Seq: last + 1,
			SenderID: userID, MsgType: msgType, Content: content, CreatedAt: time.Now().UTC(),
		}
		if err := s.repo.InsertMessage(ctx, tx, m); err != nil {
			return err
		}
		if err := s.repo.BumpSession(ctx, tx, sessionID, m.Seq); err != nil {
			return err
		}
		if err := s.repo.IncrOthersUnread(ctx, tx, sessionID, userID); err != nil {
			return err
		}
		if err := s.repo.SetRead(ctx, tx, sessionID, userID, m.Seq); err != nil {
			return err
		}
		if s.bus != nil {
			return s.bus.Publish(ctx, tx, contract.NewMessageSent(ids.New().String(), contract.MessageSentPayload{
				MessageID: m.ID, SessionID: sessionID, SessionKind: contract.KindDM,
				SenderID: userID, MsgType: msgType, Seq: m.Seq, Content: content,
			}))
		}
		return nil
	})
	if err != nil {
		return contract.Message{}, err
	}
	return toMessage(m), nil
}

// Sync 增量拉取（cursor=已拉取最大 seq；非成员拒绝）。
func (s *Service) Sync(ctx context.Context, userID, sessionID, cursor int64, limit int) (contract.SyncResult, error) {
	ok, err := s.repo.IsMember(ctx, s.pool, sessionID, userID)
	if err != nil {
		return contract.SyncResult{}, err
	}
	if !ok {
		// denied 记审计（审计存储适配器落地后接通）
		return contract.SyncResult{}, domain.ErrNotParticipant
	}
	if cursor < 0 {
		cursor = 0
	}
	limit = domain.ClampLimit(limit)
	rows, err := s.repo.Messages(ctx, s.pool, sessionID, cursor, limit+1)
	if err != nil {
		return contract.SyncResult{}, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]contract.Message, 0, len(rows))
	next := cursor
	for _, r := range rows {
		items = append(items, toMessage(r))
		next = r.Seq
	}
	return contract.SyncResult{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// MarkRead 提交已读游标（幂等；越权拒绝）。
func (s *Service) MarkRead(ctx context.Context, userID, sessionID, readSeq int64) error {
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		last, err := s.repo.LockSession(ctx, tx, sessionID)
		if errors.Is(err, infra.ErrNotFound) {
			return domain.ErrSessionNotFound
		}
		if err != nil {
			return err
		}
		ok, err := s.repo.IsMember(ctx, tx, sessionID, userID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrNotParticipant
		}
		if readSeq > last {
			readSeq = last
		}
		if readSeq < 0 {
			readSeq = 0
		}
		return s.repo.MarkRead(ctx, tx, sessionID, userID, readSeq)
	})
}

// EnsurePartitions 保证当月与下月分区存在（SchedulerJob）。
func (s *Service) EnsurePartitions(ctx context.Context) error {
	now := time.Now().UTC()
	months := []time.Time{time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)}
	next := now.AddDate(0, 1, 0)
	months = append(months, time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, time.UTC))
	for _, m := range months {
		if _, err := s.repo.EnsureMonthPartition(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Archive 标记超期（>6 个月）分区为待归档并记录可检索元数据（SchedulerJob）。
// 物理压缩上传对象存储与 detach 在对象存储适配器落地后接通，此处只登记元数据、不丢数据。
func (s *Service) Archive(ctx context.Context) error {
	cutoff := time.Now().UTC().AddDate(0, -6, 0)
	names, err := s.repo.ListMonthPartitions(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		from, err := partitionMonth(name)
		if err != nil {
			continue
		}
		to := from.AddDate(0, 1, 0)
		if !to.Before(cutoff) && !to.Equal(cutoff) {
			continue
		}
		n, err := s.repo.CountPartition(ctx, name)
		if err != nil {
			return err
		}
		if err := s.repo.UpsertArchiveMeta(ctx, name, from, to, n); err != nil {
			return err
		}
	}
	return nil
}

// ---- 内部 ----

func (s *Service) peer(ctx context.Context, userID int64) *contract.Peer {
	p := &contract.Peer{UserID: userID}
	if s.resolver != nil {
		if info, err := s.resolver.Resolve(ctx, entityref.Ref{Type: "user", ID: userID}); err == nil {
			p.Title, p.Cover = info.Title, info.Cover
		}
	}
	return p
}

func toMessage(m infra.MessageRow) contract.Message {
	return contract.Message{ID: m.ID, SessionID: m.SessionID, Seq: m.Seq, SenderID: m.SenderID,
		MsgType: m.MsgType, Content: m.Content, CreatedAt: m.CreatedAt}
}

func parseUserRef(ref string) (int64, error) {
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 || parts[0] != "user" {
		return 0, domain.ErrInvalidTarget
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.ErrInvalidTarget
	}
	return id, nil
}

func partitionMonth(name string) (time.Time, error) {
	if len(name) != len("msg_message_")+6 {
		return time.Time{}, domain.ErrInvalidTarget
	}
	return time.ParseInLocation("200601", name[len(name)-6:], time.UTC)
}
