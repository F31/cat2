// Package trust R0 内容安全与审核模块（骨架）：
// 敏感词前置过滤、文本同步机检、统一审核任务队列、ReviewSubjectProvider 开口。
// 举报复核/记分/封禁闭环在 p2 补全。
package trust

import (
	"context"
	"strings"
	"sync"
	"time"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/module"
	"catcube/internal/modules/trust/contract"
	"catcube/internal/ports"
	"catcube/pkg/ids"
)

// Deps 装配依赖。
type Deps struct {
	Pool       *db.Pool
	Moderation ports.Moderation
}

// Module trust 模块。
type Module struct {
	deps Deps

	mu        sync.RWMutex
	providers map[string]contract.ReviewSubjectProvider
	words     []string // 敏感词/引流词库（后台可维护）
}

func New(deps Deps) *Module {
	return &Module{deps: deps, providers: map[string]contract.ReviewSubjectProvider{}}
}

func (m *Module) Name() string        { return "trust" }
func (m *Module) Rank() module.Rank   { return module.RankR0 }
func (m *Module) DependsOn() []string { return nil }

// Register 注册能力提供者与审核开口。
func (m *Module) Register(r *module.Registry) error {
	// 开口：ReviewSubjectProvider 注册表（扩展点 #1）
	r.Extensions["trust.ReviewSubjectProvider"] = func(p contract.ReviewSubjectProvider) {
		m.RegisterSubjectProvider(p)
	}
	r.CapProviders = append(r.CapProviders, capProvider{m})
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// RegisterSubjectProvider 高阶模块注册审核对象提供者（注册即生效，删除模块即失效）。
func (m *Module) RegisterSubjectProvider(p contract.ReviewSubjectProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[p.SubjectType()] = p
}

// LoadWords 加载敏感词库（trs_sensitive_word，后台可维护）。
func (m *Module) LoadWords(ctx context.Context) error {
	rows, err := m.deps.Pool.Query(ctx, `SELECT word FROM trs_sensitive_word WHERE status=1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var words []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return err
		}
		words = append(words, w)
	}
	m.mu.Lock()
	m.words = words
	m.mu.Unlock()
	return rows.Err()
}

// FilterSensitive 本地敏感词前置过滤（含引流词）；命中先提示再记分。
func (m *Module) FilterSensitive(text string) (hit string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.words {
		if w != "" && strings.Contains(text, w) {
			return w, true
		}
	}
	return "", false
}

// SubmitReview 提交审核：敏感词前置 → 文本同步机检（命中拦截并记违规）。
func (m *Module) SubmitReview(ctx context.Context, subjectType string, subjectID int64, text string, imageIDs []string) (bool, error) {
	if word, hit := m.FilterSensitive(text); hit {
		_ = m.createTask(ctx, 1, subjectType, subjectID, 2, "敏感词命中:"+word)
		return false, nil
	}
	if m.deps.Moderation == nil {
		return true, nil // 未配置适配器（开发环境）：默认通过
	}
	res, err := m.deps.Moderation.CheckText(ctx, ports.ModerationRequest{Text: text})
	if err != nil {
		// 机检超时：进「审核中」队列重试，不放行
		_ = m.createTask(ctx, 1, subjectType, subjectID, 3, "机检超时待重试")
		return false, nil
	}
	if !res.Pass {
		_ = m.createTask(ctx, 1, subjectType, subjectID, 2, "文本机检命中")
	}
	return res.Pass, nil
}

func (m *Module) Report(ctx context.Context, reporterID int64, targetRef, reasonCode, detail string) error {
	_, err := m.deps.Pool.Exec(ctx,
		`INSERT INTO trs_report (id, reporter_id, target_ref, reason_code, detail, status, created_at)
		 VALUES ($1,$2,$3,$4,$5,0,now())`,
		int64(ids.New()), reporterID, targetRef, reasonCode, detail)
	return err
}

func (m *Module) Restrict(ctx context.Context, userID int64, days int, reason string) error {
	_, err := m.deps.Pool.Exec(ctx,
		`INSERT INTO trs_restriction (id, user_id, type, days, reason, expires_at, created_at)
		 VALUES ($1,$2,1,$3,$4,$5,now())`,
		int64(ids.New()), userID, days, reason, time.Now().Add(time.Duration(days)*24*time.Hour))
	return err
}

func (m *Module) IsRestricted(ctx context.Context, userID int64) (bool, error) {
	var n int
	err := m.deps.Pool.QueryRow(ctx,
		`SELECT count(1) FROM trs_restriction WHERE user_id=$1 AND expires_at>now() AND lifted_at IS NULL`, userID).Scan(&n)
	return n > 0, err
}

func (m *Module) createTask(ctx context.Context, source int16, subjectType string, subjectID int64, risk int16, reason string) error {
	_, err := m.deps.Pool.Exec(ctx,
		`INSERT INTO trs_review_task (id, source, subject_type, subject_id, submitter_id, risk_level, status, reason, sla_due_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,0,$5,0,$6,now()+interval '24 hours',now(),now())`,
		int64(ids.New()), source, subjectType, subjectID, risk, reason)
	return err
}

// capProvider 「是否被限制发布」能力提供者。
type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "trust" }

func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	if capability != "publish.allowed" {
		return false, nil
	}
	restricted, err := p.m.IsRestricted(ctx, userID)
	if err != nil {
		return false, err
	}
	return !restricted, nil
}

var _ contract.Facade = (*Module)(nil)
