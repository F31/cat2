// Package trade R1 交易模块：只管钱——订单、支付（回调验签/幂等）、退款（幂等）、每日对账、
// 履约注册表。不认识「会员/订阅」，权益由各业务模块 OrderFulfiller 履约。
package trade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catcube/internal/kernel/audit"
	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	idcontract "catcube/internal/modules/identity/contract"
	"catcube/internal/modules/trade/contract"
	"catcube/internal/modules/trade/internal/app"
	"catcube/internal/modules/trade/internal/domain"
	"catcube/internal/modules/trade/internal/infra"
	"catcube/internal/ports"
)

// Deps 装配依赖。
type Deps struct {
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Pay      ports.Pay
	Identity idcontract.Facade
	Audit    *audit.Recorder
}

// Module trade 模块。
type Module struct {
	deps Deps
	svc  *app.Service
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "trade" }
func (m *Module) Rank() module.Rank   { return module.RankR1 }
func (m *Module) DependsOn() []string { return []string{"identity"} }

func (m *Module) Register(r *module.Registry) error {
	m.svc = app.New(app.Options{Repo: infra.NewStore(m.deps.Pool), Pool: m.deps.Pool, Bus: m.deps.Bus, Pay: m.deps.Pay})
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/orders", Handler: m.auth(m.handleCreate)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/orders/{id}/pay", Handler: m.auth(m.handlePay)},
		module.Route{Method: http.MethodGet, Path: "/api/mp/v1/orders/{id}", Handler: m.auth(m.handleGet)},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/pay/notify", Handler: m.handleNotify},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/refunds", Handler: m.auth(m.handleRefund), Admin: true},
		module.Route{Method: http.MethodPost, Path: "/api/admin/v1/fulfillments/{orderID}/retry", Handler: m.auth(m.handleRetryFulfill), Admin: true},
		module.Route{Method: http.MethodGet, Path: "/api/admin/v1/orders", Handler: m.auth(m.handleAdminOrders), Admin: true},
	)
	r.Jobs = append(r.Jobs,
		module.JobDef{Name: "trade.close_expired", Interval: time.Minute, Run: m.svc.CloseExpired},
		module.JobDef{Name: "trade.fulfill", Interval: 30 * time.Second, Run: func(ctx context.Context) error {
			_, err := m.svc.RunFulfillment(ctx)
			return err
		}},
		module.JobDef{Name: "trade.recon", Interval: time.Hour, Run: m.svc.RunRecon},
		module.JobDef{Name: "trade.apportion_expire", Interval: 5 * time.Minute, Run: m.svc.ExpireApportions},
	)
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

func (m *Module) CreateOrder(ctx context.Context, buyerID int64, in contract.CreateOrderInput, idempotencyKey string) (contract.Order, error) {
	return m.svc.CreateOrder(ctx, buyerID, in, idempotencyKey)
}
func (m *Module) Pay(ctx context.Context, buyerID, orderID int64) (contract.PayParams, error) {
	return m.svc.Pay(ctx, buyerID, orderID)
}
func (m *Module) GetOrder(ctx context.Context, orderID int64) (contract.Order, error) {
	return m.svc.GetOrder(ctx, orderID)
}
func (m *Module) Refund(ctx context.Context, operatorID, orderID, amountFen int64, reason string) (contract.Refund, error) {
	return m.svc.Refund(ctx, operatorID, orderID, amountFen, reason)
}
func (m *Module) RegisterFulfiller(f contract.OrderFulfiller) { m.svc.RegisterFulfiller(f) }

// ---- 分摊认筹 Facade ----

func (m *Module) CreateApportion(ctx context.Context, in contract.ApportionInput) (contract.ApportionGroup, error) {
	return m.svc.CreateApportion(ctx, in)
}
func (m *Module) PledgeOrder(ctx context.Context, groupID, buyerID int64, idemKey string) (contract.Order, error) {
	return m.svc.PledgeOrder(ctx, groupID, buyerID, idemKey)
}
func (m *Module) ExitPledge(ctx context.Context, groupID, buyerID int64) (contract.Refund, error) {
	return m.svc.ExitPledge(ctx, groupID, buyerID)
}
func (m *Module) ApportionGroupOf(ctx context.Context, groupID int64) (contract.ApportionGroup, error) {
	return m.svc.ApportionGroupOf(ctx, groupID)
}

// ExpireApportions 手动触发未成团退款（运维入口；定时由 trade.apportion_expire Job 调用同一实现）。
func (m *Module) ExpireApportions(ctx context.Context) error { return m.svc.ExpireApportions(ctx) }

// ---- HTTP 层 ----

// trade 错误码（contracts/errors/README.md 08 段）。
const (
	codeParamInvalid  = 1     // 000001
	codeUnauthorized  = 2     // 000002
	codeInternal      = 9     // 000009
	codeOrderNotFound = 80001 // 080001
	codeForbidden     = 80002 // 080002
	codeAmountInvalid = 80003 // 080003
	codeNotPayable    = 80004 // 080004
	codeNotifyInvalid = 80005 // 080005
	codeRefundInvalid = 80006 // 080006
	codeApportionBad  = 80007 // 080007 分摊已结束/已成团
)

func (m *Module) handleCreate(w http.ResponseWriter, req *http.Request) {
	idem := req.Header.Get("Idempotency-Key")
	if idem == "" {
		writeErr(w, codeParamInvalid, "缺少 Idempotency-Key")
		return
	}
	var in contract.CreateOrderInput
	if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	order, err := m.svc.CreateOrder(req.Context(), userIDFrom(req.Context()), in, idem)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	writeData(w, order)
}

func (m *Module) handlePay(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	params, err := m.svc.Pay(req.Context(), userIDFrom(req.Context()), id)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	writeData(w, params)
}

func (m *Module) handleGet(w http.ResponseWriter, req *http.Request) {
	id, err := pathID(req)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	o, err := m.svc.GetOrder(req.Context(), id)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	if o.BuyerID != userIDFrom(req.Context()) {
		writeErr(w, codeForbidden, "无权操作该订单")
		return
	}
	writeData(w, o)
}

// handleNotify 支付回调（无鉴权，验签在 service）。
func (m *Module) handleNotify(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	headers := map[string]string{}
	for _, k := range []string{"Wechatpay-Signature", "Wechatpay-Timestamp", "Wechatpay-Nonce"} {
		headers[k] = req.Header.Get(k)
	}
	res, err := m.svc.HandleNotify(req.Context(), headers, body)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	writeData(w, res)
}

func (m *Module) handleRefund(w http.ResponseWriter, req *http.Request) {
	var body struct {
		OrderID   int64  `json:"order_id,string"`
		AmountFen int64  `json:"amount_fen"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.OrderID == 0 {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	r, err := m.svc.Refund(req.Context(), userIDFrom(req.Context()), body.OrderID, body.AmountFen, body.Reason)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	m.auditAdmin(req, "trade:refund:execute", fmt.Sprintf(`{"type":"order","id":"%d"}`, body.OrderID),
		map[string]any{"reason": body.Reason, "amount_fen": body.AmountFen, "refund_id": strconv.FormatInt(r.ID, 10)})
	writeData(w, r)
}

// auditAdmin 后台操作审计（退款/履约补偿）。
func (m *Module) auditAdmin(req *http.Request, action, targetRef string, detail any) {
	if m.deps.Audit == nil {
		return
	}
	ip := req.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	_ = m.deps.Audit.Record(req.Context(), nil, audit.Entry{
		ActorID: userIDFrom(req.Context()), ActorType: audit.ActorAdmin, Channel: "admin",
		Action: action, TargetRef: targetRef, Detail: detail,
		Result: audit.ResultOK, IP: ip, UserAgent: req.UserAgent(),
	})
}

func (m *Module) handleRetryFulfill(w http.ResponseWriter, req *http.Request) {
	orderID, err := strconv.ParseInt(req.PathValue("orderID"), 10, 64)
	if err != nil {
		writeErr(w, codeParamInvalid, "参数错误")
		return
	}
	ok, err := m.svc.RetryFulfillment(req.Context(), orderID)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	m.auditAdmin(req, "trade:fulfillment:retry", fmt.Sprintf(`{"type":"order","id":"%d"}`, orderID),
		map[string]any{"reset": ok})
	writeData(w, map[string]any{"reset": ok})
}

func (m *Module) handleAdminOrders(w http.ResponseWriter, req *http.Request) {
	status := int16(-1)
	if s := req.URL.Query().Get("status"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			status = int16(v)
		}
	}
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	items, err := m.svc.AdminListOrders(req.Context(), status, limit)
	if err != nil {
		writeTradeErr(w, err)
		return
	}
	writeData(w, map[string]any{"items": items})
}

func (m *Module) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, codeUnauthorized, "未登录")
			return
		}
		uid, ver, err := m.deps.Identity.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, codeUnauthorized, "登录已失效")
			return
		}
		brief, err := m.deps.Identity.UserBrief(req.Context(), uid)
		if err != nil || brief.AuthzVer != ver || (brief.Status != 0 && brief.Status != 1) {
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

func pathID(req *http.Request) (int64, error) { return strconv.ParseInt(req.PathValue("id"), 10, 64) }

func writeTradeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrOrderNotFound), errors.Is(err, infra.ErrNotFound):
		writeErr(w, codeOrderNotFound, "订单不存在")
	case errors.Is(err, domain.ErrForbidden):
		writeErr(w, codeForbidden, "无权操作该订单")
	case errors.Is(err, domain.ErrAmountInvalid), errors.Is(err, domain.ErrBizTypeRequired), errors.Is(err, domain.ErrRefundAmount):
		writeErr(w, codeAmountInvalid, err.Error())
	case errors.Is(err, domain.ErrNotPayable):
		writeErr(w, codeNotPayable, "订单当前状态不可支付")
	case errors.Is(err, domain.ErrNotifyInvalid):
		writeErr(w, codeNotifyInvalid, "支付回调验签失败")
	case errors.Is(err, domain.ErrPaymentNotFound):
		writeErr(w, codeOrderNotFound, "支付单不存在")
	case errors.Is(err, domain.ErrApportionClosed):
		writeErr(w, codeApportionBad, err.Error())
	case errors.Is(err, domain.ErrApportionInvalid):
		writeErr(w, codeAmountInvalid, err.Error())
	default:
		writeErr(w, codeInternal, "服务错误")
	}
}

func httpStatus(code int) int {
	switch code {
	case codeUnauthorized:
		return http.StatusUnauthorized
	case codeForbidden:
		return http.StatusForbidden
	case codeOrderNotFound:
		return http.StatusNotFound
	case codeParamInvalid, codeAmountInvalid, codeNotifyInvalid:
		return http.StatusBadRequest
	case codeNotPayable, codeApportionBad:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

var _ contract.Facade = (*Module)(nil)
