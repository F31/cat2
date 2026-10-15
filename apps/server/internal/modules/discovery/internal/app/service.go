// Package app discovery 应用服务：事件投影、候选预计算、搜索与解释。
package app

import (
	"context"
	"encoding/json"
	"fmt"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/kernel/settings"
	"catcube/internal/modules/discovery/contract"
	"catcube/internal/modules/discovery/internal/domain"
	"catcube/internal/modules/discovery/internal/infra"
)

// 订阅的事件 Topic（R5 只订阅事件、不 import 任何模块 contract 的 Facade）。
const (
	TopicProfileUpdated   = "profile.ProfileUpdated.v1"
	TopicOrgCardUpdated   = "profile.OrgCardUpdated.v1"
	TopicContentPublished = "content.ContentPublished.v1"
	TopicContentTakenDown = "content.ContentTakenDown.v1"
	TopicContentClosed    = "content.ContentClosed.v1"
	TopicCertApproved     = "verify.CertificationApproved.v1"
	TopicMemberActivated  = "membership.MemberActivated.v1"
	TopicMemberExpired    = "membership.MemberExpired.v1"
	TopicFollowed         = "social.Followed.v1"
	TopicUserRegistered   = "identity.UserRegistered.v1"
	TopicUserDeactivated  = "identity.UserDeactivated.v1"
)

// 配置键。
const (
	KeyWeightNeed     = "dsc.weight.need"
	KeyWeightIndustry = "dsc.weight.industry"
	KeyWeightRegion   = "dsc.weight.region"
	KeyWeightIdentity = "dsc.weight.identity"
	KeyMatchTopK      = "dsc.match_topk"
)

// Schemas 配置声明。
var Schemas = []settings.Schema{
	{Key: KeyWeightNeed, Type: settings.TypeInt, Default: "40", Description: "匹配权重：需求供给互补"},
	{Key: KeyWeightIndustry, Type: settings.TypeInt, Default: "25", Description: "匹配权重：行业"},
	{Key: KeyWeightRegion, Type: settings.TypeInt, Default: "20", Description: "匹配权重：区域"},
	{Key: KeyWeightIdentity, Type: settings.TypeInt, Default: "15", Description: "匹配权重：身份互补"},
	{Key: KeyMatchTopK, Type: settings.TypeInt, Default: "20", Description: "每人候选预计算 Top-K"},
}

const minGive = 3 // 「先给后要」注册即给 3 条

// OutboxReader 事件回放读取端口（由装配根注入，避免 discovery 触碰 sys_ 表）。
type OutboxReader interface {
	List(ctx context.Context, limit int) ([]eventbus.Event, error)
}

// Repo 应用层仓储能力。
type Repo interface {
	UpsertDoc(ctx context.Context, d infra.Doc) (bool, error)
	SetActive(ctx context.Context, ref string, active bool) error
	SetVerified(ctx context.Context, userID int64) error
	SetMemberLevel(ctx context.Context, userID int64, level int16) error
	TouchActive(ctx context.Context, userID int64) error
	GetDoc(ctx context.Context, ref string) (infra.Doc, error)
	CandidatePool(ctx context.Context, my domain.Profile, excludeRef string, limit int) ([]infra.Doc, error)
	AllActiveDocs(ctx context.Context, limit int) ([]infra.Doc, error)
	AllProfiles(ctx context.Context, limit int) ([]infra.Doc, error)
	ReplaceCandidates(ctx context.Context, userID int64, list []domain.Ranked) error
	ListCandidates(ctx context.Context, userID int64, limit, offset int) ([]infra.Candidate, error)
	Search(ctx context.Context, f infra.Filter) ([]infra.Doc, error)
	Reset(ctx context.Context) error
	DeleteUser(ctx context.Context, userID int64) error
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Settings *settings.Registry
	Outbox   OutboxReader
}

// Service discovery 应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	settings *settings.Registry
	outbox   OutboxReader
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, settings: o.Settings, outbox: o.Outbox}
}

// Subscriptions 事件订阅（wire 注册进 bus）。
func (s *Service) Subscriptions() []eventbus.Subscription {
	return []eventbus.Subscription{
		{Topic: TopicProfileUpdated, Handler: s.HandleProfileUpdated},
		{Topic: TopicOrgCardUpdated, Handler: s.HandleOrgCardUpdated},
		{Topic: TopicContentPublished, Handler: s.HandleContentPublished},
		{Topic: TopicContentTakenDown, Handler: s.handleContentInactive},
		{Topic: TopicContentClosed, Handler: s.handleContentInactive},
		{Topic: TopicCertApproved, Handler: s.HandleCertificationApproved},
		{Topic: TopicMemberActivated, Handler: s.HandleMemberActivated},
		{Topic: TopicMemberExpired, Handler: s.HandleMemberExpired},
		{Topic: TopicFollowed, Handler: s.HandleFollowed},
		{Topic: TopicUserRegistered, Handler: s.HandleUserRegistered},
		{Topic: TopicUserDeactivated, Handler: s.HandleUserDeactivated},
	}
}

// ---- 投影处理器 ----

type profilePayload struct {
	UserID           int64  `json:"user_id,string"`
	Name             string `json:"name"`
	SubjectTypeCode  string `json:"subject_type_code"`
	SearchDocVersion int    `json:"search_doc_version"`
	Match            struct {
		Region   string   `json:"region"`
		Industry []string `json:"industry"`
		Need     []string `json:"need"`
		Offer    []string `json:"offer"`
	} `json:"match_profile"`
}

func (s *Service) HandleProfileUpdated(ctx context.Context, e eventbus.Event) error {
	return s.projectProfile(ctx, e, domain.TypeUser)
}
func (s *Service) HandleOrgCardUpdated(ctx context.Context, e eventbus.Event) error {
	return s.projectProfile(ctx, e, domain.TypeOrg)
}

func (s *Service) projectProfile(ctx context.Context, e eventbus.Event, docType string) error {
	var p profilePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	ref := docType + ":" + itoa(p.UserID)
	if _, err := s.repo.UpsertDoc(ctx, infra.Doc{
		Ref: ref, Type: docType, SubjectID: p.UserID, Title: p.Name,
		Region: p.Match.Region, Industry: p.Match.Industry, Need: p.Match.Need, Offer: p.Match.Offer,
		Active: true, Version: p.SearchDocVersion,
	}); err != nil {
		return err
	}
	return s.recompute(ctx, p.UserID, domain.Profile{
		DocType: docType, Region: p.Match.Region, Industry: p.Match.Industry, Need: p.Match.Need, Offer: p.Match.Offer,
	})
}

type contentPayload struct {
	ItemID           int64  `json:"item_id,string"`
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	RegionCode       string `json:"region_code"`
	SearchDocVersion int    `json:"search_doc_version"`
	TagSet           struct {
		Industry []string `json:"industry"`
		Region   string   `json:"region"`
		Need     []string `json:"need"`
		Offer    []string `json:"offer"`
	} `json:"tagset"`
}

func (s *Service) HandleContentPublished(ctx context.Context, e eventbus.Event) error {
	var p contentPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.ItemID == 0 {
		return nil
	}
	region := p.RegionCode
	if region == "" {
		region = p.TagSet.Region
	}
	_, err := s.repo.UpsertDoc(ctx, infra.Doc{
		Ref: "content." + p.Kind + ":" + itoa(p.ItemID), Type: "content." + p.Kind, SubjectID: p.ItemID,
		Title: p.Title, Region: region, Industry: p.TagSet.Industry, Need: p.TagSet.Need, Offer: p.TagSet.Offer,
		Active: true, Version: p.SearchDocVersion,
	})
	return err
}

func (s *Service) handleContentInactive(ctx context.Context, e eventbus.Event) error {
	var p contentPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.ItemID == 0 {
		return nil
	}
	return s.repo.SetActive(ctx, "content."+p.Kind+":"+itoa(p.ItemID), false)
}

func (s *Service) HandleCertificationApproved(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	return s.repo.SetVerified(ctx, p.UserID)
}

func (s *Service) HandleMemberActivated(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
		Level  int16 `json:"level"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	return s.repo.SetMemberLevel(ctx, p.UserID, p.Level)
}

func (s *Service) HandleMemberExpired(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	return s.repo.SetMemberLevel(ctx, p.UserID, 0)
}

func (s *Service) HandleFollowed(ctx context.Context, e eventbus.Event) error {
	var p struct {
		FollowerID int64 `json:"follower_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.FollowerID == 0 {
		return nil
	}
	return s.repo.TouchActive(ctx, p.FollowerID)
}

// HandleUserRegistered 注册即给：无画像也按粗标签给 3 条（先给后要）。
func (s *Service) HandleUserRegistered(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	return s.recompute(ctx, p.UserID, domain.Profile{DocType: domain.TypeUser})
}

func (s *Service) HandleUserDeactivated(ctx context.Context, e eventbus.Event) error {
	var p struct {
		UserID int64 `json:"user_id,string"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.UserID == 0 {
		return nil
	}
	return s.repo.DeleteUser(ctx, p.UserID)
}

// ---- 候选预计算 ----

func (s *Service) recompute(ctx context.Context, userID int64, my domain.Profile) error {
	selfRef := refOf(my.DocType, userID)
	pool, err := s.repo.CandidatePool(ctx, my, selfRef, 200)
	if err != nil {
		return err
	}
	if len(pool) < minGive {
		fallback, err := s.repo.AllActiveDocs(ctx, 50)
		if err != nil {
			return err
		}
		pool = mergeUnique(pool, fallback, selfRef)
	}
	w := s.weights(ctx)
	ranked := make([]domain.Ranked, 0, len(pool))
	for _, d := range pool {
		score, reason := domain.Evaluate(my, domain.Profile{
			DocType: d.Type, Region: d.Region, Industry: d.Industry, Need: d.Need, Offer: d.Offer,
		}, w)
		ranked = append(ranked, domain.Ranked{Ref: d.Ref, Score: score, Reason: reason,
			MemberLevel: d.MemberLevel, LastActive: d.LastActive.Unix()})
	}
	domain.SortCandidates(ranked)
	if k := s.topK(ctx); k > 0 && len(ranked) > k {
		ranked = ranked[:k]
	}
	return s.repo.ReplaceCandidates(ctx, userID, ranked)
}

// RefreshAll 周期性为全部人/组织画像重算候选（跨用户新加入场景的收敛）。
func (s *Service) RefreshAll(ctx context.Context) (int, error) {
	docs, err := s.repo.AllProfiles(ctx, 2000)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range docs {
		if err := s.recompute(ctx, d.SubjectID, domain.Profile{
			DocType: d.Type, Region: d.Region, Industry: d.Industry, Need: d.Need, Offer: d.Offer,
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ---- 只读接口 ----

// Feed 匹配卡片流（offset 用于换一批）。
func (s *Service) Feed(ctx context.Context, userID int64, limit, offset int) ([]contract.Card, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.repo.ListCandidates(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Card, 0, len(rows))
	for _, c := range rows {
		out = append(out, contract.Card{Ref: c.Ref, Type: c.Type, Title: c.Title, Cover: c.Cover,
			Region: c.Region, Score: c.Score, MemberLevel: c.MemberLevel, Verified: c.Verified, Reason: toReasons(c.Reason)})
	}
	return out, nil
}

// Search 六类内容检索。
func (s *Service) Search(ctx context.Context, q contract.SearchQuery) ([]contract.Card, error) {
	if q.Type != "" && !domain.ValidType(q.Type) {
		return nil, domain.ErrBadType
	}
	limit := q.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.repo.Search(ctx, infra.Filter{
		Keyword: q.Keyword, Type: q.Type, Region: q.Region,
		Industry: q.Industry, Need: q.Need, Offer: q.Offer, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]contract.Card, 0, len(rows))
	for _, d := range rows {
		out = append(out, contract.Card{Ref: d.Ref, Type: d.Type, Title: d.Title, Cover: d.Cover,
			Region: d.Region, MemberLevel: d.MemberLevel, Verified: d.Verified})
	}
	return out, nil
}

// Explain 候选匹配解释（先读预计算，缺失时按当前画像即时计算）。
func (s *Service) Explain(ctx context.Context, userID int64, ref string) (contract.Explain, error) {
	rows, err := s.repo.ListCandidates(ctx, userID, 200, 0)
	if err != nil {
		return contract.Explain{}, err
	}
	for _, c := range rows {
		if c.Ref == ref {
			return contract.Explain{Ref: ref, Score: c.Score, Reason: toReasons(c.Reason)}, nil
		}
	}
	my, err := s.repo.GetDoc(ctx, refOf(domain.TypeUser, userID))
	if err != nil {
		return contract.Explain{}, domain.ErrNoProfile
	}
	their, err := s.repo.GetDoc(ctx, ref)
	if err != nil {
		return contract.Explain{}, domain.ErrNoCandidate
	}
	score, reason := domain.Evaluate(
		domain.Profile{DocType: my.Type, Region: my.Region, Industry: my.Industry, Need: my.Need, Offer: my.Offer},
		domain.Profile{DocType: their.Type, Region: their.Region, Industry: their.Industry, Need: their.Need, Offer: their.Offer},
		s.weights(ctx))
	return contract.Explain{Ref: ref, Score: score, Reason: toReasons(reason)}, nil
}

// Rebuild 投影回放重建（清空读模型后按 Outbox 事件顺序重放）。
func (s *Service) Rebuild(ctx context.Context) (int, error) {
	if err := s.repo.Reset(ctx); err != nil {
		return 0, err
	}
	if s.outbox == nil {
		return 0, nil
	}
	events, err := s.outbox.List(ctx, 100000)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range events {
		if err := s.dispatch(ctx, e); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Service) dispatch(ctx context.Context, e eventbus.Event) error {
	switch e.Topic {
	case TopicProfileUpdated:
		return s.HandleProfileUpdated(ctx, e)
	case TopicOrgCardUpdated:
		return s.HandleOrgCardUpdated(ctx, e)
	case TopicContentPublished:
		return s.HandleContentPublished(ctx, e)
	case TopicContentTakenDown, TopicContentClosed:
		return s.handleContentInactive(ctx, e)
	case TopicCertApproved:
		return s.HandleCertificationApproved(ctx, e)
	case TopicMemberActivated:
		return s.HandleMemberActivated(ctx, e)
	case TopicMemberExpired:
		return s.HandleMemberExpired(ctx, e)
	case TopicFollowed:
		return s.HandleFollowed(ctx, e)
	case TopicUserRegistered:
		return s.HandleUserRegistered(ctx, e)
	case TopicUserDeactivated:
		return s.HandleUserDeactivated(ctx, e)
	default:
		return nil
	}
}

// ---- 内部 ----

func (s *Service) weights(ctx context.Context) domain.Weights {
	w := domain.DefaultWeights()
	if s.settings == nil {
		return w
	}
	if v, err := s.settings.Int(ctx, 1, KeyWeightNeed); err == nil && v > 0 {
		w.Need = int(v)
	}
	if v, err := s.settings.Int(ctx, 1, KeyWeightIndustry); err == nil && v > 0 {
		w.Industry = int(v)
	}
	if v, err := s.settings.Int(ctx, 1, KeyWeightRegion); err == nil && v > 0 {
		w.Region = int(v)
	}
	if v, err := s.settings.Int(ctx, 1, KeyWeightIdentity); err == nil && v > 0 {
		w.Identity = int(v)
	}
	return w
}

func (s *Service) topK(ctx context.Context) int {
	if s.settings == nil {
		return 20
	}
	if v, err := s.settings.Int(ctx, 1, KeyMatchTopK); err == nil && v > 0 {
		return int(v)
	}
	return 20
}

func toReasons(items []domain.ReasonItem) []contract.Reason {
	out := make([]contract.Reason, 0, len(items))
	for _, r := range items {
		out = append(out, contract.Reason{Dimension: r.Dimension, Text: r.Text})
	}
	return out
}

func refOf(docType string, id int64) string { return docType + ":" + itoa(id) }

func itoa(id int64) string { return fmt.Sprintf("%d", id) }

func mergeUnique(a, b []infra.Doc, excludeRef string) []infra.Doc {
	seen := map[string]bool{excludeRef: true}
	out := make([]infra.Doc, 0, len(a)+len(b))
	for _, d := range a {
		if !seen[d.Ref] {
			seen[d.Ref] = true
			out = append(out, d)
		}
	}
	for _, d := range b {
		if !seen[d.Ref] {
			seen[d.Ref] = true
			out = append(out, d)
		}
	}
	return out
}
