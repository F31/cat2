// Package bff 只读聚合层：组合各模块 contract（不 import 模块根包），
// 为小程序提供首屏单次聚合（feed）与详情页聚合（detail）。
package bff

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	circlecontract "catcube/internal/modules/circle/contract"
	contentcontract "catcube/internal/modules/content/contract"
	cubecontract "catcube/internal/modules/cube/contract"
	discoverycontract "catcube/internal/modules/discovery/contract"
	idcontract "catcube/internal/modules/identity/contract"
	membershipcontract "catcube/internal/modules/membership/contract"
	msgcontract "catcube/internal/modules/msg/contract"
	profilecontract "catcube/internal/modules/profile/contract"
	socialcontract "catcube/internal/modules/social/contract"
	verifycontract "catcube/internal/modules/verify/contract"
	"catcube/internal/shared/entityref"
)

// Deps 聚合依赖（全部为模块 contract）。
type Deps struct {
	Identity   idcontract.Facade
	Discovery  discoverycontract.Facade
	Profile    profilecontract.Facade
	Social     socialcontract.Facade
	Msg        msgcontract.Facade
	Verify     verifycontract.Facade
	Membership membershipcontract.Facade
	Content    contentcontract.Facade
	Cube       cubecontract.Facade
	Circle     circlecontract.Facade
}

// Aggregator 只读聚合器。
type Aggregator struct{ deps Deps }

func New(deps Deps) *Aggregator { return &Aggregator{deps: deps} }

// Register 注册聚合路由（bff 不属于依赖阶梯，直接挂 mux）。
func (a *Aggregator) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mp/v1/feed", a.auth(a.handleFeed))
	mux.HandleFunc("POST /api/mp/v1/feed/refresh", a.auth(a.handleFeed))
	// 详情页游客可看（浏览/搜索/详情=游客可），登录态可选：已登录回关注/会员态，未登录 uid=0。
	mux.HandleFunc("GET /api/mp/v1/aggregate/detail", a.optionalAuth(a.handleDetail))
	// 联系方式揭示：会员权益（手机号），非会员 120007 由前端引导开通。
	mux.HandleFunc("GET /api/mp/v1/aggregate/contact", a.auth(a.handleContact))
	// 猫立方首页聚合（立方列表+待办数+未读单次返回）
	mux.HandleFunc("GET /api/mp/v1/aggregate/cube-home", a.auth(a.handleCubeHome))
	// 圈详情聚合（非成员裁剪视图：圈信息+成员数+圈主+加入状态）
	mux.HandleFunc("GET /api/mp/v1/aggregate/circle-detail", a.optionalAuth(a.handleCircleDetail))
}

// CubeHomeData 猫立方首页聚合。
type CubeHomeData struct {
	Cubes     []cubecontract.CubeBrief `json:"cubes"`
	TodoCount int                      `json:"todo_count"`
	Unread    int                      `json:"unread"`
}

// handleCubeHome 猫立方首页：立方列表 + 待办数 + 未读（单次聚合）。
func (a *Aggregator) handleCubeHome(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	cubes, err := a.deps.Cube.ListMine(req.Context(), uid)
	if err != nil {
		writeErrCode(w, http.StatusInternalServerError, 9, "服务错误")
		return
	}
	todo := 0
	for _, c := range cubes {
		if ts, err := a.deps.Cube.ListTasks(req.Context(), c.ID, 0); err == nil {
			for _, t := range ts {
				if t.Status == 0 || t.Status == 1 {
					todo++ // 待办/进行中
				}
			}
		}
	}
	unread := 0
	if sessions, err := a.deps.Msg.Sessions(req.Context(), uid); err == nil {
		for _, s := range sessions {
			unread += s.Unread
		}
	}
	writeData(w, CubeHomeData{Cubes: cubes, TodoCount: todo, Unread: unread})
}

// CircleDetailData 圈详情聚合（非成员裁剪）。
type CircleDetailData struct {
	Circle      circlecontract.CircleDetail `json:"circle"`
	Owner       *profilecontract.Card       `json:"owner,omitempty"`
	MemberCount int                         `json:"member_count"`
	Joined      bool                        `json:"joined"`
}

// handleCircleDetail 圈详情聚合：圈信息 + 圈主名片 + 成员数 + 加入状态。
func (a *Aggregator) handleCircleDetail(w http.ResponseWriter, req *http.Request) {
	idStr := strings.TrimPrefix(req.URL.Query().Get("ref"), "circle:")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErr(w, 1, "参数错误")
		return
	}
	uid := userIDFrom(req.Context())
	d, err := a.deps.Circle.Detail(req.Context(), uid, id)
	if err != nil {
		writeErrCode(w, http.StatusNotFound, 140001, "圈子不存在或不可见")
		return
	}
	out := CircleDetailData{Circle: d, MemberCount: d.MemberCount, Joined: d.Joined}
	// 非成员裁剪：仅「关于本圈」+加入状态，不返回动态/需求/成员明细
	if card, err := a.deps.Profile.CardOf(req.Context(), d.OwnerUserID); err == nil {
		out.Owner = &card
	}
	writeData(w, out)
}

// FeedData 首屏聚合：卡片流 + 画像进度 + 未读数。
type FeedData struct {
	Items           []discoverycontract.Card   `json:"items"`
	ProfileProgress profilecontract.Completion `json:"profile_progress"`
	Unread          UnreadData                 `json:"unread"`
}

// UnreadData 未读汇总（关注 + 消息）。
type UnreadData struct {
	Follow int `json:"follow"`
	Msg    int `json:"msg"`
	Total  int `json:"total"`
}

// DetailData 详情页聚合。
type DetailData struct {
	Ref       string                     `json:"ref"`
	Type      string                     `json:"type"`
	Card      *profilecontract.Card      `json:"card,omitempty"`
	Content   *contentcontract.Item      `json:"content,omitempty"`
	Owner     *profilecontract.Card      `json:"owner,omitempty"`
	Badges    []verifycontract.Badge     `json:"badges,omitempty"`
	Member    *membershipcontract.Member `json:"member,omitempty"`
	Following bool                       `json:"following"`
}

func (a *Aggregator) handleFeed(w http.ResponseWriter, req *http.Request) {
	uid := userIDFrom(req.Context())
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))

	cards, err := a.deps.Discovery.Feed(req.Context(), uid, limit, offset)
	if err != nil {
		writeErr(w, 9, "服务错误")
		return
	}
	if cards == nil {
		cards = []discoverycontract.Card{}
	}
	progress, _ := a.deps.Profile.CompletionOf(req.Context(), uid)
	follow, _ := a.deps.Social.Unread(req.Context(), uid)
	msgUnread := 0
	if sessions, err := a.deps.Msg.Sessions(req.Context(), uid); err == nil {
		for _, s := range sessions {
			msgUnread += s.Unread
		}
	}
	writeData(w, FeedData{
		Items:           cards,
		ProfileProgress: progress,
		Unread:          UnreadData{Follow: follow, Msg: msgUnread, Total: follow + msgUnread},
	})
}

func (a *Aggregator) handleDetail(w http.ResponseWriter, req *http.Request) {
	ref := req.URL.Query().Get("ref")
	typ, id, ok := splitRef(ref)
	if !ok {
		writeErr(w, 1, "参数错误")
		return
	}
	uid := userIDFrom(req.Context())
	out := DetailData{Ref: ref, Type: typ}

	switch {
	case typ == "user" || typ == "org":
		if card, err := a.deps.Profile.CardOf(req.Context(), id); err == nil {
			out.Card = &card
		}
		if badges, err := a.deps.Verify.BadgesOf(req.Context(), entityref.Ref{Type: typ, ID: id}); err == nil {
			out.Badges = badges
		}
		if m, active, err := a.deps.Membership.Me(req.Context(), id); err == nil && active {
			out.Member = &m
		}
		if uid > 0 {
			if following, err := a.deps.Social.IsFollowing(req.Context(), uid, ref); err == nil {
				out.Following = following
			}
		}
	case strings.HasPrefix(typ, "content."):
		if item, err := a.deps.Content.Get(req.Context(), uid, id); err == nil {
			out.Content = &item
			if card, err := a.deps.Profile.CardOf(req.Context(), item.OwnerRef.ID); err == nil {
				out.Owner = &card
			}
		}
	default:
		writeErr(w, 1, "不支持的实体类型")
		return
	}
	writeData(w, out)
}

// ContactData 联系方式（会员可见）。
type ContactData struct {
	Ref   string `json:"ref"`
	Phone string `json:"phone"`
}

// handleContact 揭示联系方式（手机号）：登录 + 会员权益校验后经 profile 解密下发。
func (a *Aggregator) handleContact(w http.ResponseWriter, req *http.Request) {
	ref := req.URL.Query().Get("ref")
	typ, id, ok := splitRef(ref)
	if !ok || (typ != "user" && typ != "org") {
		writeErr(w, 1, "参数错误")
		return
	}
	uid := userIDFrom(req.Context())
	_, active, err := a.deps.Membership.Me(req.Context(), uid)
	if err != nil {
		writeErrCode(w, http.StatusInternalServerError, 9, "服务错误")
		return
	}
	if !active {
		writeErrCode(w, http.StatusForbidden, 120007, "查看联系方式需要会员权益")
		return
	}
	phone, err := a.deps.Profile.ContactPhone(req.Context(), id)
	if err != nil {
		writeErrCode(w, http.StatusInternalServerError, 9, "服务错误")
		return
	}
	writeData(w, ContactData{Ref: ref, Phone: phone})
}

func (a *Aggregator) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			writeErr(w, 2, "未登录")
			return
		}
		uid, ver, err := a.deps.Identity.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, 2, "登录已失效")
			return
		}
		brief, err := a.deps.Identity.UserBrief(req.Context(), uid)
		if err != nil || brief.AuthzVer != ver || (brief.Status != 0 && brief.Status != 1) {
			writeErr(w, 2, "登录已失效")
			return
		}
		next(w, req.WithContext(context.WithValue(req.Context(), ctxUserID{}, uid)))
	}
}

// optionalAuth 可选登录：无令牌时以游客（uid=0）继续，令牌非法则拒绝。
func (a *Aggregator) optionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := bearer(req)
		if token == "" {
			next(w, req.WithContext(context.WithValue(req.Context(), ctxUserID{}, int64(0))))
			return
		}
		uid, ver, err := a.deps.Identity.Authenticate(req.Context(), token)
		if err != nil {
			writeErr(w, 2, "登录已失效")
			return
		}
		brief, err := a.deps.Identity.UserBrief(req.Context(), uid)
		if err != nil || brief.AuthzVer != ver || (brief.Status != 0 && brief.Status != 1) {
			writeErr(w, 2, "登录已失效")
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

func splitRef(ref string) (string, int64, bool) {
	i := strings.LastIndex(ref, ":")
	if i <= 0 || i == len(ref)-1 {
		return "", 0, false
	}
	id, err := strconv.ParseInt(ref[i+1:], 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	return ref[:i], id, true
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data})
}

func writeErr(w http.ResponseWriter, code int, message string) {
	status := http.StatusInternalServerError
	switch code {
	case 1:
		status = http.StatusBadRequest
	case 2:
		status = http.StatusUnauthorized
	}
	writeErrCode(w, status, code, message)
}

func writeErrCode(w http.ResponseWriter, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}
