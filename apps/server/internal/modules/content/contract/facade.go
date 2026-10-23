// Package contract content 对外契约：四类内容发布、查询、下架。
package contract

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

// UnmarshalJSON 兼容小程序把 file_id 序列化为字符串、后台/测试传数字两种形式。
func (in *PublishInput) UnmarshalJSON(raw []byte) error {
	var body struct {
		Kind        string              `json:"kind"`
		Title       string              `json:"title"`
		CoverFileID json.RawMessage     `json:"cover_file_id"`
		RegionCode  string              `json:"region_code"`
		TagSet      dictcontract.TagSet `json:"tagset"`
		OrgRef      int64               `json:"org_ref"`
		Fields      map[string]any      `json:"fields"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	in.Kind, in.Title, in.RegionCode, in.TagSet, in.OrgRef, in.Fields = body.Kind, body.Title, body.RegionCode, body.TagSet, body.OrgRef, body.Fields
	in.CoverFileID = 0
	if v := strings.TrimSpace(string(body.CoverFileID)); v != "" && v != "null" {
		v = strings.Trim(v, `"`)
		if v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return err
			}
			in.CoverFileID = id
		}
	}
	return nil
}

// SignupState 当前用户报名状态（活动/会议）。
type SignupState struct {
	SignedUp bool `json:"signed_up"`
	Count    int  `json:"count"`
	Limit    int  `json:"limit"`
}

// SignupItem 我的活动：报名记录 + 内容摘要。
type SignupItem struct {
	Item       Item      `json:"item"`
	SignedUpAt time.Time `json:"signed_up_at"`
}

// SignupUser 报名名单行（主办方查看）。
type SignupUser struct {
	UserID       int64     `json:"user_id,string"`
	Nickname     string    `json:"nickname,omitempty"`
	AvatarFileID int64     `json:"avatar_file_id,string,omitempty"`
	SignedUpAt   time.Time `json:"signed_up_at"`
}

// Facade content 对外接口。
type Facade interface {
	Publish(ctx context.Context, userID int64, in PublishInput) (Item, error)
	Submit(ctx context.Context, userID, itemID int64) (Item, error)
	Get(ctx context.Context, viewerID, itemID int64) (Item, error)
	ListByOwner(ctx context.Context, ownerID int64, kind string) ([]Item, error)
	Close(ctx context.Context, userID, itemID int64) (Item, error)
	// CreateFromCube 由猫立方引入的项目创建平台草稿（只带标题+正文；字段可在平台补全）。
	CreateFromCube(ctx context.Context, ownerID int64, title, body string) (int64, error)
	// Signup 报名活动/会议；重复报名幂等。
	Signup(ctx context.Context, userID, itemID int64) (SignupState, error)
	// CancelSignup 取消报名；未报名幂等。
	CancelSignup(ctx context.Context, userID, itemID int64) (SignupState, error)
	// SignupState 查询当前用户报名状态。
	SignupState(ctx context.Context, userID, itemID int64) (SignupState, error)
	// ListSignups 我的活动（当前用户已报名的活动/会议）。
	ListSignups(ctx context.Context, userID int64) ([]SignupItem, error)
	// ListSignupUsers 主办方查看报名名单。
	ListSignupUsers(ctx context.Context, ownerID, itemID int64) ([]SignupUser, error)
}
