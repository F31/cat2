// Package contract content 对外契约：四类内容发布、查询、下架。
package contract

import (
	"context"
	"time"

	dictcontract "catcube/internal/shared/dict/contract"
)

// 内容类型（ContentKind）。
const (
	KindProject = "project"
	KindProduct = "product"
	KindMeeting = "meeting"
	KindEvent   = "event"
)

// 内容状态（状态机 #2：审核中→展示中→已下架）。
const (
	StatusReviewing int16 = 0 // 审核中
	StatusPublished int16 = 1 // 展示中
	StatusTakenDown int16 = 2 // 已下架
)

// OwnerRef 发布人引用。
type OwnerRef struct {
	Type string `json:"type"` // user/org
	ID   int64  `json:"id,string"`
}

// Item 内容信封 + 类型字段。
type Item struct {
	ID           int64               `json:"id,string"`
	Kind         string              `json:"kind"`
	OwnerRef     OwnerRef            `json:"owner_ref"`
	Title        string              `json:"title"`
	CoverFileID  int64               `json:"cover_file_id,string,omitempty"`
	RegionCode   string              `json:"region_code,omitempty"`
	TagSet       dictcontract.TagSet `json:"tagset"`
	Status       int16               `json:"status"`
	RejectReason string              `json:"reject_reason,omitempty"`
	PublishedAt  *time.Time          `json:"published_at,omitempty"`
	Fields       map[string]any      `json:"fields"`
	MemberGate   []string            `json:"member_gate,omitempty"` // 因非会员被隐藏的字段
	UnlockReason string              `json:"unlock_reason,omitempty"`
}

// PublishInput 发布入参（创建草稿；Fields 按 kind 字段 schema 校验）。
type PublishInput struct {
	Kind        string              `json:"kind"`
	Title       string              `json:"title"`
	CoverFileID int64               `json:"cover_file_id"`
	RegionCode  string              `json:"region_code"`
	TagSet      dictcontract.TagSet `json:"tagset"`
	OrgRef      int64               `json:"org_ref"`
	Fields      map[string]any      `json:"fields"`
}

// Facade content 对外接口。
type Facade interface {
	Publish(ctx context.Context, userID int64, in PublishInput) (Item, error)
	Submit(ctx context.Context, userID, itemID int64) (Item, error)
	Get(ctx context.Context, viewerID, itemID int64) (Item, error)
	ListByOwner(ctx context.Context, ownerID int64, kind string) ([]Item, error)
	Close(ctx context.Context, userID, itemID int64) (Item, error)
}
