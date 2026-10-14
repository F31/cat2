// Package identity R0 账号模块：微信登录、会话与令牌、authz_ver、账号状态、注销。
// 本阶段为骨架：登录/令牌/注销链路；实名核验属 verify（p2）。
package identity

import (
	"context"
	"encoding/json"
	"net/http"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/module"
	"catcube/internal/kernel/sensitive"
	"catcube/internal/modules/identity/contract"
	"catcube/internal/modules/identity/internal/app"
	"catcube/internal/ports"
	"catcube/pkg/ids"
)

// Deps 装配依赖。
type Deps struct {
	Pool      *db.Pool
	WeChat    ports.WeChatAuth
	Bus       *eventbus.Bus
	Encryptor *sensitive.Encryptor // 手机号/证件号字段加密
	BlindKey  []byte               // phone_hmac 独立密钥
}

// Module identity 模块。
type Module struct {
	deps  Deps
	token *app.TokenService
}

func New(deps Deps) *Module { return &Module{deps: deps} }

func (m *Module) Name() string        { return "identity" }
func (m *Module) Rank() module.Rank   { return module.RankR0 }
func (m *Module) DependsOn() []string { return []string{"dict"} }

// Register 注册路由与能力提供者。
func (m *Module) Register(r *module.Registry) error {
	ts, err := app.NewTokenService()
	if err != nil {
		return err
	}
	m.token = ts
	r.Routes = append(r.Routes,
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/auth/wechat", Handler: m.handleWechatLogin},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/auth/refresh", Handler: m.handleRefresh},
		module.Route{Method: http.MethodPost, Path: "/api/mp/v1/users/me/deactivate", Handler: m.handleDeactivate},
	)
	// CapabilityProvider：账号状态（实名能力在 verify p2 注册，此处恒 false）
	r.CapProviders = append(r.CapProviders, capProvider{m})
	return nil
}

func (m *Module) Migrations() string { return migrations }

// Facade 对外契约。
func (m *Module) Facade() contract.Facade { return m }

// Authenticate 校验 Access Token；网关比对 authz_ver 消除陈旧窗口。
func (m *Module) Authenticate(_ context.Context, accessToken string) (int64, int, error) {
	claims, err := m.token.Verify(accessToken)
	if err != nil {
		return 0, 0, err
	}
	return claims.Sub, claims.AuthzVer, nil
}

func (m *Module) SessionOf(ctx context.Context, userID int64) (int, error) {
	var n int
	err := m.deps.Pool.QueryRow(ctx,
		`SELECT count(1) FROM idn_session WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>now()`, userID).Scan(&n)
	return n, err
}

func (m *Module) UserBrief(ctx context.Context, userID int64) (contract.UserBrief, error) {
	var b contract.UserBrief
	err := m.deps.Pool.QueryRow(ctx,
		`SELECT id, COALESCE(nickname,''), COALESCE(avatar_file_id,0), status FROM idn_user WHERE id=$1`, userID).
		Scan(&b.ID, &b.Nickname, &b.AvatarID, &b.Status)
	return b, err
}

// AuthzBump 权限变更递增授权版本。
func (m *Module) AuthzBump(ctx context.Context, userID int64) error {
	_, err := m.deps.Pool.Exec(ctx,
		`UPDATE idn_user SET authz_ver=authz_ver+1, updated_at=now() WHERE id=$1`, userID)
	return err
}

// Deactivate 注销：匿名化本模块数据并发布事件（其他模块订阅后各自处理）。
func (m *Module) Deactivate(ctx context.Context, userID int64) error {
	return m.deps.Pool.InTx(ctx, func(tx db.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE idn_user SET status=3, nickname='已注销用户', avatar_file_id=NULL,
				wechat_openid=NULL, phone_enc=NULL, phone_hmac=NULL,
				id_card_name_enc=NULL, id_card_no_enc=NULL, updated_at=now()
			WHERE id=$1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE idn_session SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
			return err
		}
		if m.deps.Bus != nil {
			return m.deps.Bus.Publish(ctx, tx,
				contract.NewEvent(ids.New().String(), contract.EventUserDeactivated, userID))
		}
		return nil
	})
}

type loginReq struct {
	Code          string `json:"code"`
	EncryptedData string `json:"encrypted_data"`
	IV            string `json:"iv"`
}

type loginResp struct {
	UserID int64           `json:"user_id,string"`
	New    bool            `json:"new"`
	Tokens contract.Tokens `json:"tokens"`
}

func (m *Module) handleWechatLogin(w http.ResponseWriter, req *http.Request) {
	var body loginReq
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Code == "" {
		http.Error(w, `{"message":"参数错误"}`, http.StatusBadRequest)
		return
	}
	openID, sessionKey, err := m.deps.WeChat.Code2Session(req.Context(), body.Code)
	if err != nil {
		http.Error(w, `{"message":"微信登录失败"}`, http.StatusBadGateway)
		return
	}
	_ = sessionKey // session_key 只在服务端短期使用，不下发不入日志
	_ = openID
	// TODO(p2): 经 GetPhoneNumber 取手机号 → phone_hmac 查是否已注册 → 注册/登录一体化
	http.Error(w, `{"message":"登录链路待 p2 联通（GetPhoneNumber/注册登录一体化）"}`, http.StatusNotImplemented)
}

func (m *Module) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"message":"refresh 待实现"}`, http.StatusNotImplemented)
}

func (m *Module) handleDeactivate(w http.ResponseWriter, req *http.Request) {
	http.Error(w, `{"message":"deactivate 待接入鉴权中间件"}`, http.StatusNotImplemented)
}

// capProvider 账号状态能力提供者。
type capProvider struct{ m *Module }

func (p capProvider) Name() string { return "identity" }

func (p capProvider) Has(ctx context.Context, userID int64, capability string) (bool, error) {
	switch capability {
	case "account.active":
		b, err := p.m.UserBrief(ctx, userID)
		if err != nil {
			return false, err
		}
		return b.Status == 0, nil
	case "realname": // 实名能力在 verify（p2）注册；此处恒 false
		return false, nil
	default:
		return false, nil
	}
}

var _ contract.Facade = (*Module)(nil)
