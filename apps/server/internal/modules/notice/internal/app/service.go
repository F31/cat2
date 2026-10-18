// Package app notice 应用服务：模板数据驱动的事件触达、订阅额度、同圈同日合并、站内兜底。
package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/modules/notice/contract"
	"catcube/internal/modules/notice/internal/domain"
	"catcube/internal/modules/notice/internal/infra"
	"catcube/pkg/ids"
)

// CircleFacade 圈成员（合并发送用；notice 依赖 circle contract，不读其表）。
type CircleFacade interface {
	Members(ctx context.Context, circleID int64) ([]int64, error)
}

// Repo 应用层仓储能力。
type Repo interface {
	BatchInsertNotice(ctx context.Context, items []infra.Notice) (int, error)
	UpsertTemplate(ctx context.Context, id int64, t domain.Template) error
	ListTemplates(ctx context.Context) ([]domain.Template, error)
	TemplateByEvent(ctx context.Context, eventCode string) (domain.Template, error)
	UpdateTemplate(ctx context.Context, eventCode, title, content string, channels []string, closeable bool) (int64, error)
	InsertNotice(ctx context.Context, q infra.Querier, n infra.Notice) error
	ListNotices(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]infra.Notice, error)
	UnreadCount(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, userID, noticeID int64) error
	Preference(ctx context.Context, userID int64, code string) (bool, error)
	SetPreference(ctx context.Context, userID int64, code string, enabled bool) error
	IncrQuota(ctx context.Context, userID int64, code string, n int) error
	ConsumeQuota(ctx context.Context, q infra.Querier, userID int64, code string) (bool, error)
	UpsertDigest(ctx context.Context, circleID int64) (int, error)
	DueDigests(ctx context.Context, limit int) ([]int64, error)
	DigestCount(ctx context.Context, circleID int64) (int, error)
	MarkDigestSent(ctx context.Context, circleID, noticeID int64) error
}

// Options 构造参数。
type Options struct {
	Repo   Repo
	Pool   *db.Pool
	Bus    *eventbus.Bus
	Circle CircleFacade
}

// Service notice 应用服务。
type Service struct {
	repo   Repo
	pool   *db.Pool
	bus    *eventbus.Bus
	circle CircleFacade

	mu        sync.RWMutex
	templates map[string]domain.Template
}

func New(o Options) *Service {
	s := &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, circle: o.Circle, templates: map[string]domain.Template{}}
	for _, t := range Templates {
		s.templates[t.EventCode] = t
	}
	return s
}

// Register 登记模板（幂等，后台可改文案/渠道）。
func (s *Service) Register(ctx context.Context) error {
	for _, t := range Templates {
		if err := s.repo.UpsertTemplate(ctx, int64(ids.New()), t); err != nil {
			return err
		}
	}
	return nil
}

// Topics 订阅的全部业务事件 Topic。
func Topics() []string {
	out := make([]string, 0, len(Templates))
	for _, t := range Templates {
		out = append(out, t.EventCode)
	}
	return out
}

// Handle 事件触达：查模板 → 接收人 → 偏好 → 渠道（订阅额度不足降级站内）→ 合并或写站内。
func (s *Service) Handle(ctx context.Context, e eventbus.Event) error {
	tpl, ok := s.template(ctx, e.Topic)
	if !ok {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return nil
	}
	userID, ok := domain.Receiver(tpl.ReceiverRule, payload)
	if !ok || userID == 0 {
		return nil
	}
	// 偏好：可关闭且已关闭 → 跳过（资金/审核类不可关闭）
	if tpl.Closeable {
		enabled, err := s.repo.Preference(ctx, userID, tpl.Code)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
	}
	title := domain.Render(tpl.TitleTpl, payload)
	content := domain.Render(tpl.ContentTpl, payload)

	// 同圈同日合并：仅累计，由日结 Job 汇总发送
	if tpl.MergeRule == domain.MergeSameCircleDay {
		if circleID := nestedID(payload, "circle_id"); circleID != 0 {
			_, err := s.repo.UpsertDigest(ctx, circleID)
			return err
		}
	}

	channel := domain.ChannelInApp
	if hasChannel(tpl.Channels, domain.ChannelSubscribe) {
		if ok, err := s.repo.ConsumeQuota(ctx, s.pool, userID, tpl.Code); err == nil && ok {
			channel = domain.ChannelSubscribe
		}
	}
	if hasChannel(tpl.Channels, domain.ChannelSMS) && channel == domain.ChannelSubscribe {
		channel = domain.ChannelSMS // 资金类优先短信（额度已消耗）
	}
	return s.repo.InsertNotice(ctx, s.pool, infra.Notice{
		ID: int64(ids.New()), UserID: userID, TemplateCode: tpl.Code,
		Title: title, Content: content, Ref: noticeRef(payload, e.ID), Channel: channel,
	})
}

// RunDigest 日结：同圈同日合并为一条（本人不收）。
// noticeRef 站内信落地引用：圈子/猫立方事件指向业务对象，其余保留事件 ID。
func noticeRef(payload map[string]any, eventID string) string {
	if id := nestedID(payload, "circle_id"); id != 0 {
		return "circle:" + itoa(id)
	}
	if id := nestedID(payload, "cube_id"); id != 0 {
		return "cube:" + itoa(id)
	}
	return eventID
}

func (s *Service) RunDigest(ctx context.Context) error {
	idsDue, err := s.repo.DueDigests(ctx, 200)
	if err != nil {
		return err
	}
	for _, circleID := range idsDue {
		count, err := s.repo.DigestCount(ctx, circleID)
		if err != nil || count == 0 {
			continue
		}
		var members []int64
		if s.circle != nil {
			members, _ = s.circle.Members(ctx, circleID)
		}
		var first int64
		for _, uid := range members {
			n := infra.Notice{
				ID: int64(ids.New()), UserID: uid, TemplateCode: "circle.DemandPublished.v1",
				Title: "圈内动态汇总", Content: "你所在的圈子今日有 " + itoa(int64(count)) + " 条新需求",
				Ref: "circle:" + itoa(circleID), Channel: domain.ChannelInApp,
			}
			if err := s.repo.InsertNotice(ctx, s.pool, n); err != nil {
				slog.Warn("notice 日结发送失败", "circle", circleID, "user", uid, "err", err)
				continue
			}
			if first == 0 {
				first = n.ID
			}
		}
		if first != 0 {
			_ = s.repo.MarkDigestSent(ctx, circleID, first)
		}
	}
	return nil
}

// Template 对外模板视图。
type Template struct {
	Code       string   `json:"code"`
	Module     string   `json:"module"`
	TitleTpl   string   `json:"title_tpl"`
	ContentTpl string   `json:"content_tpl"`
	Channels   []string `json:"channels"`
	MergeRule  string   `json:"merge_rule,omitempty"`
	Closeable  bool     `json:"closeable"`
}

// Templates 模板列表（DB 为准；空库先登记默认模板）。
func (s *Service) Templates(ctx context.Context) ([]Template, error) {
	rows, err := s.repo.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if err := s.Register(ctx); err != nil {
			return nil, err
		}
		rows, _ = s.repo.ListTemplates(ctx)
	}
	out := make([]Template, 0, len(rows))
	for _, t := range rows {
		out = append(out, Template{Code: t.Code, Module: t.Module, TitleTpl: t.TitleTpl,
			ContentTpl: t.ContentTpl, Channels: t.Channels, MergeRule: t.MergeRule, Closeable: t.Closeable})
	}
	if len(out) == 0 {
		for _, t := range Templates {
			out = append(out, Template{Code: t.Code, Module: t.Module, TitleTpl: t.TitleTpl,
				ContentTpl: t.ContentTpl, Channels: t.Channels, MergeRule: t.MergeRule, Closeable: t.Closeable})
		}
	}
	return out, nil
}

// UpdateTemplate 后台修改模板（文案/渠道/可关闭）；DB 为准，事件处理即时生效。
func (s *Service) UpdateTemplate(ctx context.Context, code, title, content string, channels []string, closeable bool) error {
	n, err := s.repo.UpdateTemplate(ctx, code, title, content, channels, closeable)
	if err != nil {
		return err
	}
	if n == 0 {
		if err := s.Register(ctx); err != nil {
			return err
		}
		if n, err = s.repo.UpdateTemplate(ctx, code, title, content, channels, closeable); err != nil {
			return err
		}
	}
	if n == 0 {
		return domain.ErrTemplateNotFound
	}
	return nil
}

// ---- 查询与偏好 ----

func (s *Service) List(ctx context.Context, userID int64, unreadOnly bool, limit int) ([]contract.Notice, error) {
	rows, err := s.repo.ListNotices(ctx, userID, unreadOnly, limit)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Notice, 0, len(rows))
	for _, n := range rows {
		out = append(out, contract.Notice{ID: n.ID, TemplateCode: n.TemplateCode, Title: n.Title,
			Content: n.Content, Ref: n.Ref, Channel: n.Channel, IsRead: n.IsRead, ReadAt: n.ReadAt, CreatedAt: n.CreatedAt})
	}
	return out, nil
}

func (s *Service) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.repo.UnreadCount(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, userID, noticeID int64) error {
	return s.repo.MarkRead(ctx, userID, noticeID)
}

func (s *Service) Preferences(ctx context.Context, userID int64) ([]contract.Preference, error) {
	out := make([]contract.Preference, 0, len(Templates))
	for _, t := range Templates {
		enabled := true
		if t.Closeable {
			if v, err := s.repo.Preference(ctx, userID, t.Code); err == nil {
				enabled = v
			}
		}
		out = append(out, contract.Preference{TemplateCode: t.Code, Enabled: enabled, Closeable: t.Closeable})
	}
	return out, nil
}

// Broadcast 定向公告：向指定用户站内信投递（不可关闭类；管理员代操作，审计由模块层写）。
func (s *Service) Broadcast(ctx context.Context, userIDs []int64, title, content string) (int, error) {
	if len(userIDs) == 0 || title == "" || content == "" {
		return 0, domain.ErrTemplateNotFound
	}
	if len(userIDs) > 500 {
		userIDs = userIDs[:500]
	}
	items := make([]infra.Notice, 0, len(userIDs))
	for _, uid := range userIDs {
		items = append(items, infra.Notice{
			ID: int64(ids.New()), UserID: uid, TemplateCode: "admin.announcement", Title: title, Content: content, Channel: "inapp",
		})
	}
	return s.repo.BatchInsertNotice(ctx, items)
}

func (s *Service) SetPreference(ctx context.Context, userID int64, code string, enabled bool) error {
	tpl, ok := s.template(ctx, code)
	if !ok {
		return domain.ErrTemplateNotFound
	}
	if !tpl.Closeable {
		return nil // 资金/审核类不可关闭
	}
	return s.repo.SetPreference(ctx, userID, code, enabled)
}

func (s *Service) ApplyQuota(ctx context.Context, userID int64, code string, n int) error {
	if _, ok := s.template(ctx, code); !ok {
		return domain.ErrTemplateNotFound
	}
	if n <= 0 || n > 10 {
		n = 1
	}
	return s.repo.IncrQuota(ctx, userID, code, n)
}

// ---- 内部 ----

func (s *Service) template(ctx context.Context, topic string) (domain.Template, bool) {
	if t, err := s.repo.TemplateByEvent(ctx, topic); err == nil {
		return t, true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[topic]
	return t, ok
}

func hasChannel(chs []string, ch string) bool {
	for _, c := range chs {
		if c == ch {
			return true
		}
	}
	return false
}

func nestedID(payload map[string]any, field string) int64 {
	v, ok := payload[field]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case string:
		var id int64
		for _, c := range strings.TrimSpace(t) {
			if c < '0' || c > '9' {
				return 0
			}
			id = id*10 + int64(c-'0')
		}
		return id
	case float64:
		return int64(t)
	}
	return 0
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var _ = time.Now
