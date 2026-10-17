// Package channel R4 渠道模块：运营商身份（移动端）、归属关系、佣金/结算（p4 §1–§2）。
// 红线：归属单层（referrer_operator_id 单列，无上级/下级集合，不自建下级）；一地一家。
package channel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/policy"
	"catcube/internal/kernel/settings"
	"catcube/internal/kernel/statemachine"
	"catcube/internal/modules/channel/contract"
	"catcube/internal/modules/channel/domain"
	cubecontract "catcube/internal/modules/cube/contract"
	dictcontract "catcube/internal/shared/dict/contract"
	"catcube/pkg/ids"
)

// channel 错误码（contracts/errors/README.md 15 段）。
const (
	codeParamInvalid      = 1
	codeUnauthorized      = 2
	codeInternal          = 9
	codeRegionTaken       = 150001
	codeAlreadyApplied    = 150002
	codeInvalidState      = 150003
	codeBadReferrer       = 150004
	codeAlreadyAttributed = 150005
	codeNoOperator        = 150006
	codeBadInput          = 150007
)

// AuthFunc 认证函数（装配根注入，避免同阶依赖 identity 之外的耦合）。
type AuthFunc func(ctx context.Context, accessToken string) (userID int64, err error)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Auth     AuthFunc
	Audit    *audit.Recorder
	Dict     dictcontract.Facade
	Cube     cubecontract.Facade
	Settings *settings.Registry
	Policy   *policy.Engine
}

// Module channel 模块。
type Module struct {
	deps Deps
	dict dictcontract.Facade
}

func New(deps Deps) *Module { return &Module{deps: deps, dict: deps.Dict} }

func (m *Module) Name() string        { return "channel" }
func (m *Module) Rank() module.Rank   { return module.RankR4 }
func (m *Module) DependsOn() []string { return []string{"identity", "cube"} }

const migrations = `
CREATE TABLE IF NOT EXISTS chn_operator (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL UNIQUE,
	region_code varchar(16) NOT NULL,
	name varchar(64) NOT NULL,
	status smallint NOT NULL DEFAULT 0,             -- 0申请中/1已开通/2已停用
	referrer_operator_id bigint NOT NULL DEFAULT 0, -- 单层展示，无上级链（红线）
	agency_fee_paid_fen bigint NOT NULL DEFAULT 0,
	agency_fee_year int NOT NULL DEFAULT 0,
	guarantee_fen bigint NOT NULL DEFAULT 0,
	opened_at timestamptz,
	suspended_at timestamptz,
	reason varchar(200),
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	version int NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_chn_operator_region_open ON chn_operator(region_code) WHERE status=1;

CREATE TABLE IF NOT EXISTS chn_guarantee (
	id bigint PRIMARY KEY,
	operator_id bigint NOT NULL,
	amount_fen bigint NOT NULL,
	paid_at timestamptz,
	refunded_at timestamptz,
	refund_reason varchar(200),
	voucher_ref text
);

CREATE TABLE IF NOT EXISTS chn_commission (
	id bigint PRIMARY KEY,
	operator_id bigint NOT NULL,
	user_id bigint NOT NULL,
	order_id bigint NOT NULL,
	biz_type varchar(32) NOT NULL,
	base_fen bigint NOT NULL,
	ratio_bp int NOT NULL,
	ratio_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	amount_fen bigint NOT NULL,
	month char(7) NOT NULL,
	status smallint NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chn_commission_operator ON chn_commission(operator_id, month);

CREATE TABLE IF NOT EXISTS chn_settlement (
	id bigint PRIMARY KEY,
	operator_id bigint NOT NULL,
	month char(7) NOT NULL,
	commission_fen bigint NOT NULL DEFAULT 0,
	adjust_fen bigint NOT NULL DEFAULT 0,
	payable_fen bigint NOT NULL DEFAULT 0,
	status smallint NOT NULL DEFAULT 0, -- 0待结算/1已复核/2已打款/3已驳回
	reviewed_by bigint NOT NULL DEFAULT 0,
	paid_by bigint NOT NULL DEFAULT 0,
	paid_at timestamptz,
	voucher_ref text,
	reason varchar(200),
	created_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE(operator_id, month)
);

CREATE TABLE IF NOT EXISTS chn_annual_target (
	operator_id bigint NOT NULL,
	year int NOT NULL,
	paid_member_count int NOT NULL DEFAULT 0,
	landing_project_count int NOT NULL DEFAULT 0,
	warn_flags jsonb NOT NULL DEFAULT '{}'::jsonb,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (operator_id, year)
);

CREATE TABLE IF NOT EXISTS chn_attribution (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL,
	operator_id bigint NOT NULL,
	region_code varchar(16) NOT NULL,
	source smallint NOT NULL DEFAULT 1, -- 1链接/2二维码/3邀请码
	source_ref varchar(64),
	bound_at timestamptz NOT NULL DEFAULT now(),
	unbind_at timestamptz,
	correct_approval jsonb
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_chn_attr_active ON chn_attribution(user_id) WHERE unbind_at IS NULL;

CREATE TABLE IF NOT EXISTS chn_link (
	id bigint PRIMARY KEY,
	operator_id bigint NOT NULL,
	channel varchar(16) NOT NULL, -- link/qrcode/invite_code
	code varchar(64) NOT NULL UNIQUE,
	region_code varchar(16) NOT NULL,
	status smallint NOT NULL DEFAULT 0, -- 0有效/1停用
	created_at timestamptz NOT NULL DEFAULT now(),
	disabled_at timestamptz
);
`

func (m *Module) Migrations() string { return migrations }

func (m *Module) Facade() contract.Facade { return m }

func (m *Module) Register(r *module.Registry) error {
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/channel/operator", Handler: m.auth(m.handleMyOperator)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/channel/operator/apply", Handler: m.auth(m.handleApply)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/channel/attribution", Handler: m.auth(m.handleMyAttribution)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/channel/attribution", Handler: m.auth(m.handleBindAttribution)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/channel/overview", Handler: m.auth(m.handleOverview)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/channel/commissions", Handler: m.auth(m.handleCommissions)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/channel/link", Handler: m.auth(m.handleMyLink)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/channel/operators", Handler: m.auth(m.handleListOperators), Admin: true, Perm: "channel:operator:review"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/operators/{id}/open", Handler: m.auth(m.handleOpen), Admin: true, Perm: "channel:operator:review"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/operators/{id}/suspend", Handler: m.auth(m.handleSuspend), Admin: true, Perm: "channel:operator:review"},
		// 提现申请（运营商）/ 结算（后台，复核≠执行）
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/channel/withdraw", Handler: m.auth(m.handleWithdraw)},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/channel/settlements", Handler: m.auth(m.handleListSettlements), Admin: true, Perm: "channel:settlement:review"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/settlements/generate", Handler: m.auth(m.handleGenerateSettlement), Admin: true, Perm: "channel:settlement:create"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/settlements/{id}/review", Handler: m.auth(m.handleReviewSettlement), Admin: true, Perm: "channel:settlement:review"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/settlements/{id}/reject", Handler: m.auth(m.handleRejectSettlement), Admin: true, Perm: "channel:settlement:review"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/settlements/{id}/pay", Handler: m.auth(m.handlePaySettlement), Admin: true, Perm: "channel:settlement:pay"},
		// 代理费/保证金台账（2.1）与归属纠错（2.6）
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/channel/operators/{id}/fees", Handler: m.auth(m.handleOperatorFees), Admin: true, Perm: "channel:operator_fee:set"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/operators/{id}/fees", Handler: m.auth(m.handleRecordFees), Admin: true, Perm: "channel:operator_fee:set"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/operators/{id}/guarantee/refund", Handler: m.auth(m.handleRefundGuarantee), Admin: true, Perm: "channel:operator_fee:set"},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/channel/attributions", Handler: m.auth(m.handleListAttributions), Admin: true, Perm: "governance:user:read"},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/channel/attributions/{id}/correct", Handler: m.auth(m.handleCorrectAttribution), Admin: true, Perm: "channel:operator:review"},
	)
	r.AuditPoints = append(r.AuditPoints,
		audit.Point{Action: "channel:operator:open", Path: "/api/admin/v1/channel/operators/{id}/open", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:operator:suspend", Path: "/api/admin/v1/channel/operators/{id}/suspend", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:settlement:review", Path: "/api/admin/v1/channel/settlements/{id}/review", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:settlement:reject", Path: "/api/admin/v1/channel/settlements/{id}/reject", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:settlement:pay", Path: "/api/admin/v1/channel/settlements/{id}/pay", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:operator_fee:record", Path: "/api/admin/v1/channel/operators/{id}/fees", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:guarantee:refund", Path: "/api/admin/v1/channel/operators/{id}/guarantee/refund", Required: true, LogFailed: true, Module: "channel"},
		audit.Point{Action: "channel:attribution:correct", Path: "/api/admin/v1/channel/attributions/{id}/correct", Required: true, LogFailed: true, Module: "channel"},
	)
	r.AdminSections = append(r.AdminSections,
		module.AdminSection{Key: "channel.operators", Label: "运营商", Path: "/channel-operators", Module: "channel"},
		module.AdminSection{Key: "channel.settlements", Label: "渠道结算", Path: "/channel-settlements", Module: "channel"})
	r.StateMachines = append(r.StateMachines, domain.MachineOperator, domain.MachineSettlement)
	r.Settings = append(r.Settings, Schemas...)
	r.Jobs = append(r.Jobs,
		module.JobDef{Name: "channel.target_monitor", Interval: 24 * time.Hour, Run: m.monitorTargets},
	)
	// 计佣：订阅订阅制订单支付成功（首单/续费按配置比例，写快照）
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: "trade.OrderPaid.v1", Handler: m.accrueOnOrderPaid,
	})
	// 保底任务：立方项目落地（按立方猫王归属运营商）计数
	r.EventSubs = append(r.EventSubs, eventbus.Subscription{
		Topic: "cube.ProjectCreated.v1", Handler: m.countLandingProject,
	})
	return nil
}

// ---- 契约实现 ----

// OperatorOf 用户运营商身份。
func (m *Module) OperatorOf(ctx context.Context, userID int64) (contract.Operator, bool, error) {
	return m.operatorOf(ctx, userID)
}

func (m *Module) operatorOf(ctx context.Context, userID int64) (contract.Operator, bool, error) {
	var o contract.Operator
	var opened *time.Time
	err := m.deps.Pool.QueryRow(ctx, `
		SELECT id, user_id, region_code, name, status, referrer_operator_id, opened_at
		FROM chn_operator WHERE user_id=$1`, userID).
		Scan(&o.ID, &o.UserID, &o.RegionCode, &o.Name, &o.Status, &o.ReferrerOperatorID, &opened)
	if err != nil {
		return contract.Operator{}, false, nil
	}
	if opened != nil {
		o.OpenedAt = opened.Unix()
	}
	o.RegionName = m.regionName(ctx, o.RegionCode)
	return o, true, nil
}

// Apply 申请/重新申请（区域查重一地一家；referrer 单层展示）。
func (m *Module) Apply(ctx context.Context, userID int64, regionCode, name string, referrerOperatorID int64) (contract.Operator, error) {
	regionCode = strings.TrimSpace(regionCode)
	name = strings.TrimSpace(name)
	if regionCode == "" || name == "" || len([]rune(name)) > 64 {
		return contract.Operator{}, errBadInput
	}
	if m.dict != nil {
		if o, err := m.dict.Resolve(ctx, regionCode); err != nil || !o.Enabled {
			return contract.Operator{}, errBadInput
		}
	}
	// 上级仅单层展示：必须已开通且不同区域（不得自建下级/多级）
	if referrerOperatorID != 0 {
		var refRegion string
		var refStatus int16
		if err := m.deps.Pool.QueryRow(ctx, `SELECT region_code, status FROM chn_operator WHERE id=$1`, referrerOperatorID).Scan(&refRegion, &refStatus); err != nil {
			return contract.Operator{}, errBadReferrer
		}
		if refStatus != contract.StatusOpened || refRegion == regionCode {
			return contract.Operator{}, errBadReferrer
		}
	}
	var id int64
	if err := m.deps.Pool.QueryRow(ctx, `SELECT id FROM chn_operator WHERE user_id=$1`, userID).Scan(&id); err == nil {
		var status int16
		_ = m.deps.Pool.QueryRow(ctx, `SELECT status FROM chn_operator WHERE id=$1`, id).Scan(&status)
		if status != contract.StatusSuspended {
			return contract.Operator{}, errAlreadyApplied
		}
		// 2→0 重新申请
		if _, err := m.deps.Pool.Exec(ctx, `
			UPDATE chn_operator SET region_code=$2, name=$3, status=0, reason='', updated_at=now(), version=version+1
			WHERE id=$1`, id, regionCode, name); err != nil {
			return contract.Operator{}, err
		}
	} else {
		id = int64(ids.New())
		if _, err := m.deps.Pool.Exec(ctx, `
			INSERT INTO chn_operator (id, user_id, region_code, name, status, referrer_operator_id)
			VALUES ($1,$2,$3,$4,0,$5)`, id, userID, regionCode, name, referrerOperatorID); err != nil {
			return contract.Operator{}, err
		}
	}
	o, _, err := m.operatorOf(ctx, userID)
	return o, err
}

// OpenOperator 开通（后台审核；0→1；一地一家）。
func (m *Module) OpenOperator(ctx context.Context, operatorID int64) error {
	var region string
	var status int16
	if err := m.deps.Pool.QueryRow(ctx, `SELECT region_code, status FROM chn_operator WHERE id=$1`, operatorID).Scan(&region, &status); err != nil {
		return errNoOperator
	}
	var occupied int
	if err := m.deps.Pool.QueryRow(ctx, `SELECT count(1) FROM chn_operator WHERE region_code=$1 AND status=1 AND id<>$2`, region, operatorID).Scan(&occupied); err != nil {
		return err
	}
	if occupied > 0 {
		return errRegionTaken
	}
	to, err := m.transition(ctx, operatorID, status, "open")
	if err != nil {
		return err
	}
	if _, err := m.deps.Pool.Exec(ctx, `
		UPDATE chn_operator SET status=$2, opened_at=now(), suspended_at=NULL, updated_at=now(), version=version+1 WHERE id=$1`,
		operatorID, to); err != nil {
		return err
	}
	m.publishStatus(ctx, operatorID, status, to)
	return nil
}

// SuspendOperator 停用（1→2）。
func (m *Module) SuspendOperator(ctx context.Context, operatorID int64) error {
	var status int16
	if err := m.deps.Pool.QueryRow(ctx, `SELECT status FROM chn_operator WHERE id=$1`, operatorID).Scan(&status); err != nil {
		return errNoOperator
	}
	to, err := m.transition(ctx, operatorID, status, "suspend")
	if err != nil {
		return err
	}
	if _, err := m.deps.Pool.Exec(ctx, `
		UPDATE chn_operator SET status=$2, suspended_at=now(), updated_at=now(), version=version+1 WHERE id=$1`,
		operatorID, to); err != nil {
		return err
	}
	m.publishStatus(ctx, operatorID, status, to)
	return nil
}

func (m *Module) transition(ctx context.Context, id int64, from int16, event string) (int16, error) {
	to, err := statemachine.NewEngine(nil).Apply(ctx, domain.MachineOperator, id, strconv.Itoa(int(from)), event, "*")
	if err != nil {
		return 0, errInvalidState
	}
	n, _ := strconv.Atoi(to)
	return int16(n), nil
}

func (m *Module) publishStatus(ctx context.Context, operatorID int64, from, to int16) {
	if m.deps.Bus == nil {
		return
	}
	payload, _ := json.Marshal(map[string]any{"operator_id": operatorID, "from": from, "to": to})
	_ = m.deps.Bus.Publish(ctx, nil, eventbus.Event{
		ID: ids.New().String(), Topic: "channel.OperatorStatusChanged.v1", Payload: payload, SpaceID: 1,
	})
}

// AttributionOf 有效归属。
func (m *Module) AttributionOf(ctx context.Context, userID int64) (contract.Attribution, bool, error) {
	var a contract.Attribution
	err := m.deps.Pool.QueryRow(ctx, `
		SELECT id, user_id, operator_id, region_code, source, EXTRACT(EPOCH FROM bound_at)::bigint
		FROM chn_attribution WHERE user_id=$1 AND unbind_at IS NULL`, userID).
		Scan(&a.ID, &a.UserID, &a.OperatorID, &a.RegionCode, &a.Source, &a.BoundAt)
	if err != nil {
		return contract.Attribution{}, false, nil
	}
	return a, true, nil
}

// BindAttribution 绑定归属（按推广码；幂等，单一有效）。
func (m *Module) BindAttribution(ctx context.Context, userID int64, code string) (contract.Attribution, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return contract.Attribution{}, errBadInput
	}
	var operatorID int64
	var region string
	if err := m.deps.Pool.QueryRow(ctx, `
		SELECT operator_id, region_code FROM chn_link WHERE code=$1 AND status=0`, code).Scan(&operatorID, &region); err != nil {
		return contract.Attribution{}, errBadInput
	}
	if cur, ok, _ := m.attributionOf(ctx, userID); ok {
		if cur.OperatorID == operatorID {
			return cur, nil // 幂等
		}
		return contract.Attribution{}, errAlreadyAttributed
	}
	if _, err := m.deps.Pool.Exec(ctx, `
		INSERT INTO chn_attribution (id, user_id, operator_id, region_code, source, source_ref)
		VALUES ($1,$2,$3,$4,3,$5)`, int64(ids.New()), userID, operatorID, region, code); err != nil {
		return contract.Attribution{}, err
	}
	a, _, err := m.attributionOf(ctx, userID)
	return a, err
}

func (m *Module) attributionOf(ctx context.Context, userID int64) (contract.Attribution, bool, error) {
	return m.AttributionOf(ctx, userID)
}

// Overview 业绩概览。
func (m *Module) Overview(ctx context.Context, userID int64, month string) (contract.Overview, error) {
	o, ok, err := m.operatorOf(ctx, userID)
	if err != nil {
		return contract.Overview{}, err
	}
	if !ok {
		return contract.Overview{}, errNoOperator
	}
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	var monthFen, totalFen int64
	_ = m.deps.Pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_fen),0) FROM chn_commission WHERE operator_id=$1 AND month=$2`, o.ID, month).Scan(&monthFen)
	_ = m.deps.Pool.QueryRow(ctx, `SELECT COALESCE(sum(amount_fen),0) FROM chn_commission WHERE operator_id=$1`, o.ID).Scan(&totalFen)
	t := contract.Target{Year: time.Now().Year()}
	_ = m.deps.Pool.QueryRow(ctx, `
		SELECT paid_member_count, landing_project_count FROM chn_annual_target WHERE operator_id=$1 AND year=$2`,
		o.ID, t.Year).Scan(&t.PaidMemberCount, &t.LandingProjectCount)
	cs, _ := m.commissions(ctx, o.ID, month, 20)
	return contract.Overview{Operator: o, Month: month, MonthFen: monthFen, TotalFen: totalFen, Target: t, Commissions: cs}, nil
}

func (m *Module) commissions(ctx context.Context, operatorID int64, month string, limit int) ([]contract.Commission, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `SELECT id, order_id, biz_type, base_fen, ratio_bp, amount_fen, month, status FROM chn_commission WHERE operator_id=$1`
	args := []any{operatorID}
	if month != "" {
		q += ` AND month=$2`
		args = append(args, month)
	}
	q += ` ORDER BY created_at DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := m.deps.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contract.Commission{}
	for rows.Next() {
		var c contract.Commission
		if err := rows.Scan(&c.ID, &c.OrderID, &c.BizType, &c.BaseFen, &c.RatioBp, &c.AmountFen, &c.Month, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Commissions 佣金明细（契约开放）。
func (m *Module) Commissions(ctx context.Context, operatorID int64, month string, limit int) ([]contract.Commission, error) {
	return m.commissions(ctx, operatorID, month, limit)
}

// ListOperators 后台列表。
func (m *Module) ListOperators(ctx context.Context, status int16, regionCode string, limit int) ([]contract.Operator, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, user_id, region_code, name, status, referrer_operator_id, opened_at FROM chn_operator WHERE 1=1`
	args := []any{}
	if status >= 0 {
		args = append(args, status)
		q += ` AND status=$` + strconv.Itoa(len(args))
	}
	if regionCode != "" {
		args = append(args, regionCode)
		q += ` AND region_code=$` + strconv.Itoa(len(args))
	}
	q += ` ORDER BY created_at DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := m.deps.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contract.Operator{}
	for rows.Next() {
		var o contract.Operator
		var opened *time.Time
		if err := rows.Scan(&o.ID, &o.UserID, &o.RegionCode, &o.Name, &o.Status, &o.ReferrerOperatorID, &opened); err != nil {
			return nil, err
		}
		if opened != nil {
			o.OpenedAt = opened.Unix()
		}
		o.RegionName = m.regionName(ctx, o.RegionCode)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (m *Module) regionName(ctx context.Context, code string) string {
	if m.dict == nil || code == "" {
		return ""
	}
	if o, err := m.dict.Resolve(ctx, code); err == nil {
		return o.Label
	}
	return ""
}

// ---- 错误 ----

var (
	errBadInput          = errors.New("channel: 输入非法")
	errAlreadyApplied    = errors.New("channel: 已提交过申请")
	errRegionTaken       = errors.New("channel: 该区域已有运营商（一地一家）")
	errInvalidState      = errors.New("channel: 状态不允许该操作")
	errBadReferrer       = errors.New("channel: 推荐运营商非法")
	errAlreadyAttributed = errors.New("channel: 已绑定其它运营商归属")
	errNoOperator        = errors.New("channel: 非运营商或不存在")
)

// ---- HTTP ----

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, codeUnauthorized, "未登录")
			return
		}
		uid, err := m.deps.Auth(req.Context(), token)
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		next(w, req.WithContext(context.WithValue(req.Context(), ctxUserID{}, uid)))
	}
}

type ctxUserID struct{}

func userIDFrom(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxUserID{}).(int64); ok {
		return v
	}
	return 0
}

func bearer(req *http.Request) string {
	if h := req.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func (m *Module) handleMyOperator(w http.ResponseWriter, req *http.Request) {
	o, ok, err := m.operatorOf(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"operator": o, "is_operator": ok})
}

func (m *Module) handleApply(w http.ResponseWriter, req *http.Request) {
	var body struct {
		RegionCode         string `json:"region_code"`
		Name               string `json:"name"`
		ReferrerOperatorID string `json:"referrer_operator_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	ref, _ := strconv.ParseInt(body.ReferrerOperatorID, 10, 64)
	o, err := m.Apply(req.Context(), userIDFrom(req.Context()), body.RegionCode, body.Name, ref)
	if err != nil {
		writeChannelErr(w, err)
		return
	}
	writeData(w, map[string]any{"operator": o})
}

func (m *Module) handleMyAttribution(w http.ResponseWriter, req *http.Request) {
	a, ok, err := m.attributionOf(req.Context(), userIDFrom(req.Context()))
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"attribution": a, "bound": ok})
}

func (m *Module) handleBindAttribution(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	a, err := m.BindAttribution(req.Context(), userIDFrom(req.Context()), body.Code)
	if err != nil {
		writeChannelErr(w, err)
		return
	}
	writeData(w, map[string]any{"attribution": a})
}

func (m *Module) handleOverview(w http.ResponseWriter, req *http.Request) {
	o, err := m.Overview(req.Context(), userIDFrom(req.Context()), req.URL.Query().Get("month"))
	if err != nil {
		writeChannelErr(w, err)
		return
	}
	writeData(w, o)
}

func (m *Module) handleCommissions(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	op, ok, _ := m.operatorOf(req.Context(), uid)
	if !ok {
		writeErr(w, codeNoOperator, "非运营商")
		return
	}
	cs, err := m.commissions(req.Context(), op.ID, req.URL.Query().Get("month"), 50)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"items": cs})
}

// handleMyLink 推广链接/邀请码（已开通运营商；幂等复用）。
func (m *Module) handleMyLink(w http.ResponseWriter, req *http.Request) {
	op, ok, _ := m.operatorOf(req.Context(), userIDFrom(req.Context()))
	if !ok || op.Status != contract.StatusOpened {
		writeErr(w, codeNoOperator, "非已开通运营商")
		return
	}
	var code string
	if err := m.deps.Pool.QueryRow(req.Context(), `
		SELECT code FROM chn_link WHERE operator_id=$1 AND channel='invite_code' AND status=0
		ORDER BY created_at DESC LIMIT 1`, op.ID).Scan(&code); err != nil {
		code = "inv" + strconv.FormatInt(int64(ids.New()), 36)
		if _, err := m.deps.Pool.Exec(req.Context(), `
			INSERT INTO chn_link (id, operator_id, channel, code, region_code, status)
			VALUES ($1,$2,'invite_code',$3,$4,0)`, int64(ids.New()), op.ID, code, op.RegionCode); err != nil {
			writeErr(w, codeInternal, "服务错误")
			return
		}
	}
	writeData(w, map[string]any{
		"code": code, "path": "/pages/channel/join?code=" + code, "region_code": op.RegionCode,
	})
}

func (m *Module) handleListOperators(w http.ResponseWriter, req *http.Request) {
	status := int16(-1)
	if v := req.URL.Query().Get("status"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			status = int16(n)
		}
	}
	items, err := m.ListOperators(req.Context(), status, req.URL.Query().Get("region_code"), 50)
	if err != nil {
		writeErr(w, codeInternal, "服务错误")
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) handleOpen(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err := m.OpenOperator(req.Context(), id); err != nil {
		m.auditAdmin(req, "channel:operator:open", id, err, audit.ResultDenied)
		writeChannelErr(w, err)
		return
	}
	m.auditAdmin(req, "channel:operator:open", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

func (m *Module) handleSuspend(w http.ResponseWriter, req *http.Request) {
	id, _ := strconv.ParseInt(req.PathValue("id"), 10, 64)
	if err := m.SuspendOperator(req.Context(), id); err != nil {
		m.auditAdmin(req, "channel:operator:suspend", id, err, audit.ResultDenied)
		writeChannelErr(w, err)
		return
	}
	m.auditAdmin(req, "channel:operator:suspend", id, nil, audit.ResultOK)
	writeData(w, map[string]any{"ok": true})
}

func (m *Module) auditAdmin(req *http.Request, action string, operatorID int64, cause error, result audit.Result) {
	if m.deps.Audit == nil {
		return
	}
	detail := map[string]any{"operator_id": operatorID}
	if cause != nil {
		detail["err"] = cause.Error()
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
		Action: action, Detail: detail, Result: result,
	})
}

func writeChannelErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBadInput):
		writeErr(w, codeBadInput, "参数非法")
	case errors.Is(err, errAlreadyApplied):
		writeErr(w, codeAlreadyApplied, "已提交过申请")
	case errors.Is(err, errRegionTaken):
		writeErr(w, codeRegionTaken, "该区域已有运营商（一地一家）")
	case errors.Is(err, errInvalidState):
		writeErr(w, codeInvalidState, "状态不允许该操作")
	case errors.Is(err, errBadReferrer):
		writeErr(w, codeBadReferrer, "推荐运营商非法")
	case errors.Is(err, errAlreadyAttributed):
		writeErr(w, codeAlreadyAttributed, "已绑定其它运营商归属")
	case errors.Is(err, errNoOperator):
		writeErr(w, codeNoOperator, "非运营商或不存在")
	case errors.Is(err, errDualControl):
		writeErr(w, codeBadInput, "复核≠执行：同一人不得复核并打款")
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": msg})
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeParamInvalid, codeBadInput, codeAlreadyApplied, codeRegionTaken, codeInvalidState, codeBadReferrer, codeAlreadyAttributed:
		return http.StatusBadRequest
	case codeNoOperator:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}
