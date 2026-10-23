// Package app content 应用服务：发布/提交/查询/下架、状态机 #2、会员字段过滤、审核与投影。
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/kernel/statemachine"
	contentcontract "catcube/internal/modules/content/contract"
	"catcube/internal/modules/content/internal/domain"
	"catcube/internal/modules/content/internal/infra"
	"catcube/internal/modules/content/internal/kind"
	idcontract "catcube/internal/modules/identity/contract"
	trustcontract "catcube/internal/modules/trust/contract"
	dictcontract "catcube/internal/shared/dict/contract"
	mediacontract "catcube/internal/shared/media/contract"
	"catcube/pkg/ids"
)

// Repo 应用层仓储能力。
type Repo interface {
	InsertItem(ctx context.Context, tx db.Tx, it infra.Item) error
	GetItem(ctx context.Context, id int64) (infra.Item, error)
	ListByOwner(ctx context.Context, ownerID int64, kind string) ([]infra.Item, error)
	AdminList(ctx context.Context, kind string, status int16, limit int) ([]infra.Item, error)
	UpdateStatus(ctx context.Context, tx db.Tx, id int64, status int16, reason string, publishedAt *time.Time) error
	SaveSub(ctx context.Context, tx db.Tx, kind string, itemID int64, fields map[string]any) error
	GetSub(ctx context.Context, kind string, itemID int64) (map[string]any, error)
	SignupActive(ctx context.Context, itemID, userID int64) (bool, error)
	SetSignup(ctx context.Context, tx db.Tx, itemID, userID int64, active bool) (bool, error)
	CountActive(ctx context.Context, itemID int64) (int, error)
	EventCount(ctx context.Context, itemID int64) (int, error)
	BumpEventCount(ctx context.Context, tx db.Tx, itemID int64, delta int) error
	ListSignups(ctx context.Context, userID int64) ([]infra.SignupRecord, error)
	ListSignupUsers(ctx context.Context, itemID int64) ([]infra.SignupUser, error)
	LockSignup(ctx context.Context, tx db.Tx, itemID int64) error
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Kinds    *kind.Registry
	Dict     dictcontract.Facade
	Trust    trustcontract.Facade
	Media    mediacontract.Facade
	Policy   *policy.Engine
	Encrypt  *sensitive.Encryptor
	Identity idcontract.Facade
}

// Service 内容应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	kinds    *kind.Registry
	dict     dictcontract.Facade
	trust    trustcontract.Facade
	media    mediacontract.Facade
	policy   *policy.Engine
	enc      *sensitive.Encryptor
	identity idcontract.Facade
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, kinds: o.Kinds, dict: o.Dict,
		trust: o.Trust, media: o.Media, policy: o.Policy, enc: o.Encrypt, identity: o.Identity}
}

// Publish 创建内容（草稿=审核中）：校验 Kind 字段 + TagSet；联系方式加密后入库；发 ContentSubmitted。
func (s *Service) Publish(ctx context.Context, userID int64, in contentcontract.PublishInput) (contentcontract.Item, error) {
	k, ok := s.kinds.Get(in.Kind)
	if !ok {
		return contentcontract.Item{}, domain.ErrKindUnknown
	}
	if err := domain.ValidTitle(in.Title); err != nil {
		return contentcontract.Item{}, err
	}
	if err := kind.Validate(k, in.Fields); err != nil {
		return contentcontract.Item{}, err
	}
	// 限制期禁止发布（trust 提供 publish.allowed 能力）
	if ok, err := s.hasCap(ctx, userID, "publish.allowed"); err == nil && !ok {
		return contentcontract.Item{}, domain.ErrRestricted
	}
	if s.dict != nil {
		if err := s.dict.ValidateTagSet(ctx, in.TagSet); err != nil {
			return contentcontract.Item{}, fmt.Errorf("content: tagset 非法: %w", err)
		}
	}
	fields, err := s.prepareFields(in.Fields)
	if err != nil {
		return contentcontract.Item{}, err
	}
	it := infra.Item{
		ID: int64(ids.New()), Kind: in.Kind,
		OwnerRef: contentcontract.OwnerRef{Type: "user", ID: userID},
		Title:    in.Title, CoverFileID: in.CoverFileID, RegionCode: in.RegionCode,
		TagSet: in.TagSet, Status: contentcontract.StatusReviewing,
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.InsertItem(ctx, tx, it); err != nil {
			return err
		}
		if err := s.repo.SaveSub(ctx, tx, in.Kind, it.ID, fields); err != nil {
			return err
		}
		return s.publish(ctx, tx, it, "", contentcontract.EventContentSubmitted)
	}); err != nil {
		return contentcontract.Item{}, err
	}
	return s.toItem(ctx, it, true), nil
}

// CreateFromCube 由猫立方引入创建平台草稿（项目类；仅标题+正文，字段后续在平台补全）。
func (s *Service) CreateFromCube(ctx context.Context, ownerID int64, title, body string) (int64, error) {
	if err := domain.ValidTitle(title); err != nil {
		return 0, err
	}
	it := infra.Item{
		ID: int64(ids.New()), Kind: contentcontract.KindProject,
		OwnerRef: contentcontract.OwnerRef{Type: "user", ID: ownerID},
		Title:    title, Status: contentcontract.StatusReviewing,
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.InsertItem(ctx, tx, it); err != nil {
			return err
		}
		if err := s.repo.SaveSub(ctx, tx, contentcontract.KindProject, it.ID, map[string]any{"intro": body}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return it.ID, nil
}

// Submit 提交审核：状态机 #2 审核中→展示中/已下架（trust 审核结论驱动）。
func (s *Service) Submit(ctx context.Context, userID, itemID int64) (contentcontract.Item, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.Item{}, err
	}
	if it.OwnerRef.ID != userID {
		return contentcontract.Item{}, domain.ErrForbidden
	}
	// 发布门槛：已实名 + 非限制期（trust 提供 publish.allowed 能力）
	if ok, err := s.hasCap(ctx, userID, "realname"); err != nil {
		return contentcontract.Item{}, err
	} else if !ok {
		return contentcontract.Item{}, domain.ErrNotRealname
	}
	if ok, err := s.hasCap(ctx, userID, "publish.allowed"); err == nil && !ok {
		return contentcontract.Item{}, domain.ErrRestricted
	}
	if it.Status != contentcontract.StatusReviewing {
		return contentcontract.Item{}, fmt.Errorf("content: 当前状态不可提交")
	}
	k, _ := s.kinds.Get(it.Kind)
	fields, _ := s.repo.GetSub(ctx, it.Kind, itemID)
	outcome := trustcontract.OutcomePassed
	if s.trust != nil {
		outcome, err = s.trust.Review(ctx, "content."+it.Kind, itemID, k.ReviewText(it.Title, fields), k.ReviewImages(it.CoverFileID, fields))
		if err != nil {
			return contentcontract.Item{}, err
		}
	}
	if outcome == trustcontract.OutcomePending {
		// 机检不可用：保持「审核中」（已入机检重试队列），不放行（p6 §4.3）
		return s.toItem(ctx, it, true), nil
	}
	pass := outcome == trustcontract.OutcomePassed
	now := time.Now().UTC()
	newStatus := contentcontract.StatusPublished
	reason := ""
	evt := domain.EventPublish
	topic := contentcontract.EventContentPublished
	if !pass {
		newStatus = contentcontract.StatusTakenDown
		reason = "内容审核未通过"
		evt = domain.EventReject
		topic = contentcontract.EventContentTakenDown
	}
	if err := s.transition(ctx, it, newStatus, reason, &now, evt, topic, "system"); err != nil {
		return contentcontract.Item{}, err
	}
	it.Status = newStatus
	it.RejectReason = reason
	it.PublishedAt = &now
	return s.toItem(ctx, it, true), nil
}

// Close 本人关闭（展示中→已下架）。
func (s *Service) Close(ctx context.Context, userID, itemID int64) (contentcontract.Item, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.Item{}, err
	}
	if it.OwnerRef.ID != userID {
		return contentcontract.Item{}, domain.ErrForbidden
	}
	if err := s.transition(ctx, it, contentcontract.StatusTakenDown, "本人关闭", nil, domain.EventClose, contentcontract.EventContentClosed, "user"); err != nil {
		return contentcontract.Item{}, err
	}
	it.Status = contentcontract.StatusTakenDown
	it.RejectReason = "本人关闭"
	return s.toItem(ctx, it, true), nil
}

// Get 详情：未展示内容非本人不可见；非会员过滤会员可见字段。
func (s *Service) Get(ctx context.Context, viewerID, itemID int64) (contentcontract.Item, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.Item{}, err
	}
	if it.Status != contentcontract.StatusPublished && it.OwnerRef.ID != viewerID {
		return contentcontract.Item{}, domain.ErrNotFound
	}
	member, err := s.hasCap(ctx, viewerID, "member.active")
	if err != nil {
		return contentcontract.Item{}, err
	}
	out := s.toItem(ctx, it, true)
	fields, hidden, reason := domain.FilterMember(out.Fields, memberFields(out.Kind), member)
	out.Fields = fields
	out.MemberGate = hidden
	out.UnlockReason = reason
	return out, nil
}

// ListByOwner 我的发布。
func (s *Service) ListByOwner(ctx context.Context, ownerID int64, kindName string) ([]contentcontract.Item, error) {
	rows, err := s.repo.ListByOwner(ctx, ownerID, kindName)
	if err != nil {
		return nil, err
	}
	out := make([]contentcontract.Item, 0, len(rows))
	for _, it := range rows {
		out = append(out, s.toItem(ctx, it, true))
	}
	return out, nil
}

// Signup 报名活动/会议：仅展示中内容；重复报名幂等；满员拒绝。
func (s *Service) Signup(ctx context.Context, userID, itemID int64) (contentcontract.SignupState, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	if !signupKind(it.Kind) {
		return contentcontract.SignupState{}, domain.ErrNotSignupable
	}
	if it.Status != contentcontract.StatusPublished {
		return contentcontract.SignupState{}, domain.ErrNotFound
	}
	fields, _ := s.repo.GetSub(ctx, it.Kind, itemID)
	if signupClosed(fields) {
		return contentcontract.SignupState{}, domain.ErrSignupClosed
	}
	state, err := s.SignupState(ctx, userID, itemID)
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	if state.SignedUp {
		return state, nil
	}
	if state.Limit > 0 && state.Count >= state.Limit {
		return contentcontract.SignupState{}, domain.ErrSignupFull
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		// 同一内容串行化报名，避免并发下超卖/重复计数。
		if err := s.repo.LockSignup(ctx, tx, itemID); err != nil {
			return err
		}
		st, err := s.SignupState(ctx, userID, itemID)
		if err != nil {
			return err
		}
		if st.SignedUp {
			return nil
		}
		if st.Limit > 0 && st.Count >= st.Limit {
			return domain.ErrSignupFull
		}
		changed, err := s.repo.SetSignup(ctx, tx, itemID, userID, true)
		if err != nil {
			return err
		}
		if changed && it.Kind == contentcontract.KindEvent {
			return s.repo.BumpEventCount(ctx, tx, itemID, 1)
		}
		return nil
	}); err != nil {
		return contentcontract.SignupState{}, err
	}
	return s.SignupState(ctx, userID, itemID)
}

// CancelSignup 取消报名；未报名幂等；已下架内容仍允许取消。
func (s *Service) CancelSignup(ctx context.Context, userID, itemID int64) (contentcontract.SignupState, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	if !signupKind(it.Kind) {
		return contentcontract.SignupState{}, domain.ErrNotSignupable
	}
	state, err := s.SignupState(ctx, userID, itemID)
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	if !state.SignedUp {
		return state, nil
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.LockSignup(ctx, tx, itemID); err != nil {
			return err
		}
		changed, err := s.repo.SetSignup(ctx, tx, itemID, userID, false)
		if err != nil {
			return err
		}
		if changed && it.Kind == contentcontract.KindEvent {
			return s.repo.BumpEventCount(ctx, tx, itemID, -1)
		}
		return nil
	}); err != nil {
		return contentcontract.SignupState{}, err
	}
	return s.SignupState(ctx, userID, itemID)
}

// SignupState 报名状态；count 活动取展示总数，会议取有效报名数。
func (s *Service) SignupState(ctx context.Context, userID, itemID int64) (contentcontract.SignupState, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	if !signupKind(it.Kind) {
		return contentcontract.SignupState{}, domain.ErrNotSignupable
	}
	signed, err := s.repo.SignupActive(ctx, itemID, userID)
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	var count int
	if it.Kind == contentcontract.KindEvent {
		count, err = s.repo.EventCount(ctx, itemID)
	} else {
		count, err = s.repo.CountActive(ctx, itemID)
	}
	if err != nil {
		return contentcontract.SignupState{}, err
	}
	fields, _ := s.repo.GetSub(ctx, it.Kind, itemID)
	return contentcontract.SignupState{SignedUp: signed, Count: count, Limit: intField(fields, "participant_limit")}, nil
}

// ListSignups 我的活动：当前用户已报名的活动/会议。
func (s *Service) ListSignups(ctx context.Context, userID int64) ([]contentcontract.SignupItem, error) {
	rows, err := s.repo.ListSignups(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contentcontract.SignupItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, contentcontract.SignupItem{Item: s.toItem(ctx, r.Item, true), SignedUpAt: r.SignedUpAt})
	}
	return out, nil
}

// ListSignupUsers 主办方查看报名名单（仅内容所有者）。
func (s *Service) ListSignupUsers(ctx context.Context, ownerID, itemID int64) ([]contentcontract.SignupUser, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if it.OwnerRef.ID != ownerID {
		return nil, domain.ErrForbidden
	}
	if !signupKind(it.Kind) {
		return nil, domain.ErrNotSignupable
	}
	rows, err := s.repo.ListSignupUsers(ctx, itemID)
	if err != nil {
		return nil, err
	}
	out := make([]contentcontract.SignupUser, 0, len(rows))
	for _, r := range rows {
		u := contentcontract.SignupUser{UserID: r.UserID, SignedUpAt: r.SignedUpAt}
		if s.identity != nil {
			if b, err := s.identity.UserBrief(ctx, r.UserID); err == nil {
				u.Nickname = b.Nickname
				u.AvatarFileID = b.AvatarID
			}
		}
		out = append(out, u)
	}
	return out, nil
}

func signupClosed(fields map[string]any) bool {
	v, ok := fields["end_at"].(string)
	if !ok || v == "" {
		return false
	}
	end, err := time.Parse(time.RFC3339, v)
	return err == nil && time.Now().UTC().After(end)
}

func signupKind(kind string) bool {
	return kind == contentcontract.KindEvent || kind == contentcontract.KindMeeting
}

func intField(f map[string]any, key string) int {
	switch v := f[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// HandleReportConfirmed trust.ReportConfirmed 订阅：举报核实后下架内容（trust 不直接改内容表）。
func (s *Service) HandleReportConfirmed(ctx context.Context, e eventbus.Event) error {
	var p struct {
		TargetType string `json:"target_type"`
		TargetID   int64  `json:"target_id,string"`
		ReasonCode string `json:"reason_code"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	if !strings.HasPrefix(p.TargetType, "content.") || p.TargetID == 0 {
		return nil
	}
	it, err := s.repo.GetItem(ctx, p.TargetID)
	if err != nil {
		return nil // 已删除/不存在，忽略
	}
	if it.Status != contentcontract.StatusPublished {
		return nil
	}
	return s.transition(ctx, it, contentcontract.StatusTakenDown, "举报核实下架", nil,
		domain.EventTakeDown, contentcontract.EventContentTakenDown, "admin")
}

// AdminList 后台内容列表。
func (s *Service) AdminList(ctx context.Context, kind string, status int16, limit int) ([]contentcontract.Item, error) {
	rows, err := s.repo.AdminList(ctx, kind, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contentcontract.Item, 0, len(rows))
	for _, it := range rows {
		out = append(out, s.toItem(ctx, it, false))
	}
	return out, nil
}

// AdminTakedown 后台下架（展示中→已下架；非法态由状态机拒绝）。
func (s *Service) AdminTakedown(ctx context.Context, itemID int64, reason string) (contentcontract.Item, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return contentcontract.Item{}, err
	}
	if reason == "" {
		reason = "后台下架"
	}
	if err := s.transition(ctx, it, contentcontract.StatusTakenDown, reason, nil,
		domain.EventTakeDown, contentcontract.EventContentTakenDown, "admin"); err != nil {
		return contentcontract.Item{}, err
	}
	it.Status = contentcontract.StatusTakenDown
	it.RejectReason = reason
	return s.toItem(ctx, it, false), nil
}

// OwnerOf 内容归属人（供 trust 违规记分/限制）。
func (s *Service) OwnerOf(ctx context.Context, itemID int64) (int64, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return 0, err
	}
	return it.OwnerRef.ID, nil
}

// ReviewText 取审核素材（供 review provider）。
func (s *Service) ReviewText(ctx context.Context, itemID int64) (string, []string, error) {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return "", nil, err
	}
	k, ok := s.kinds.Get(it.Kind)
	if !ok {
		return "", nil, domain.ErrKindUnknown
	}
	fields, _ := s.repo.GetSub(ctx, it.Kind, itemID)
	return k.ReviewText(it.Title, fields), k.ReviewImages(it.CoverFileID, fields), nil
}

// ---- 内部 ----

func (s *Service) transition(ctx context.Context, it infra.Item, to int16, reason string, publishedAt *time.Time, event, topic, actor string) error {
	from := statusState(it.Status)
	if _, err := statemachine.NewEngine(nil).Apply(ctx, domain.Machine, it.ID, from, event, actor); err != nil {
		return err
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		if err := s.repo.UpdateStatus(ctx, tx, it.ID, to, reason, publishedAt); err != nil {
			return err
		}
		it.Status = to
		it.RejectReason = reason
		return s.publish(ctx, tx, it, reason, topic)
	})
}

func (s *Service) publish(ctx context.Context, tx db.Tx, it infra.Item, reason, topic string) error {
	if s.bus == nil {
		return nil
	}
	return s.bus.Publish(ctx, tx, contentcontract.NewEvent(ids.New().String(), topic, contentcontract.ContentPayload{
		ItemID: it.ID, Kind: it.Kind, OwnerRef: it.OwnerRef, Title: it.Title,
		CoverFileID: it.CoverFileID, RegionCode: it.RegionCode, TagSet: it.TagSet,
		Status: it.Status, SearchDocVersion: it.SearchDocVersion + 1, Reason: reason,
	}))
}

func (s *Service) prepareFields(in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	if phone, ok := out["contact_phone"].(string); ok && phone != "" {
		delete(out, "contact_phone")
		if s.enc != nil {
			enc, err := s.enc.Encrypt([]byte(phone))
			if err != nil {
				return nil, err
			}
			out["contact_phone_enc"] = enc
		} else {
			out["contact_phone_enc"] = []byte(phone)
		}
	}
	return out, nil
}

func (s *Service) toItem(ctx context.Context, it infra.Item, includeFields bool) contentcontract.Item {
	out := contentcontract.Item{
		ID: it.ID, Kind: it.Kind, OwnerRef: it.OwnerRef, Title: it.Title,
		CoverFileID: it.CoverFileID, RegionCode: it.RegionCode, TagSet: it.TagSet,
		Status: it.Status, RejectReason: it.RejectReason, PublishedAt: it.PublishedAt,
		Fields: map[string]any{},
	}
	if includeFields {
		f, err := s.repo.GetSub(ctx, it.Kind, it.ID)
		if err == nil && f != nil {
			delete(f, "contact_phone_enc")
			out.Fields = f
		}
	}
	return out
}

func (s *Service) hasCap(ctx context.Context, userID int64, capability string) (bool, error) {
	if s.policy == nil {
		return false, nil
	}
	return s.policy.Has(ctx, s.policy.NewActor(userID, 1), capability)
}

func statusState(status int16) string {
	switch status {
	case contentcontract.StatusPublished:
		return domain.StatePublished
	case contentcontract.StatusTakenDown:
		return domain.StateTakenDown
	default:
		return domain.StateReviewing
	}
}

func memberFields(kindName string) []string {
	switch kindName {
	case contentcontract.KindProject, contentcontract.KindProduct:
		return []string{"core_strength", "contact_phone"}
	}
	return nil
}
