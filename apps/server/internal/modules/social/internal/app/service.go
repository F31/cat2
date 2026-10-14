// Package app social 应用服务：关注/取关、收藏、未读；订阅内容发布做「关注=订阅」。
package app

import (
	"context"
	"encoding/json"
	"errors"

	"catcube/internal/kernel/db"
	"catcube/internal/kernel/eventbus"
	"catcube/internal/modules/social/contract"
	"catcube/internal/modules/social/internal/domain"
	"catcube/internal/modules/social/internal/infra"
	"catcube/internal/shared/entityref"
	"catcube/pkg/ids"
)

// ContentPublishedTopic 内容发布事件 topic（social R1 不 import content R2，按契约字符串订阅）。
const ContentPublishedTopic = "content.ContentPublished.v1"

// Repo 应用层仓储能力。
type Repo interface {
	InsertFollow(ctx context.Context, tx db.Tx, followerID int64, ref domain.Ref, id int64) error
	DeleteFollow(ctx context.Context, followerID int64, ref domain.Ref) (bool, error)
	IsFollowing(ctx context.Context, followerID int64, ref domain.Ref) (bool, error)
	Following(ctx context.Context, followerID int64) ([]infra.Follow, error)
	FollowersOf(ctx context.Context, ref domain.Ref) ([]int64, error)
	InsertFavorite(ctx context.Context, tx db.Tx, userID int64, ref domain.Ref, kind string, id int64) error
	DeleteFavorite(ctx context.Context, userID int64, ref domain.Ref) (bool, error)
	Favorites(ctx context.Context, userID int64, kind string) ([]infra.Favorite, error)
	BumpUnread(ctx context.Context, tx db.Tx, userID int64, brief []byte) error
	Unread(ctx context.Context, userID int64) (int, error)
}

// Options 构造参数。
type Options struct {
	Repo     Repo
	Pool     *db.Pool
	Bus      *eventbus.Bus
	Resolver *entityref.Registry
}

// Service social 应用服务。
type Service struct {
	repo     Repo
	pool     *db.Pool
	bus      *eventbus.Bus
	resolver *entityref.Registry
}

func New(o Options) *Service {
	return &Service{repo: o.Repo, pool: o.Pool, bus: o.Bus, resolver: o.Resolver}
}

// Follow 关注（幂等）。
func (s *Service) Follow(ctx context.Context, followerID int64, target string) error {
	ref, err := domain.ParseRef(target)
	if err != nil {
		return err
	}
	if ref.Type == "user" && ref.ID == followerID {
		return errors.New("social: 不能关注自己")
	}
	if err := s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertFollow(ctx, tx, followerID, ref, int64(ids.New()))
	}); err != nil {
		return err
	}
	return s.emit(ctx, contract.NewFollowed(ids.New().String(), contract.FollowPayload{
		FollowerID: followerID, TargetType: ref.Type, TargetID: ref.ID,
	}))
}

// Unfollow 取关（幂等）。
func (s *Service) Unfollow(ctx context.Context, followerID int64, target string) error {
	ref, err := domain.ParseRef(target)
	if err != nil {
		return err
	}
	removed, err := s.repo.DeleteFollow(ctx, followerID, ref)
	if err != nil {
		return err
	}
	if removed {
		return s.emit(ctx, contract.NewUnfollowed(ids.New().String(), contract.FollowPayload{
			FollowerID: followerID, TargetType: ref.Type, TargetID: ref.ID,
		}))
	}
	return nil
}

// IsFollowing 是否已关注。
func (s *Service) IsFollowing(ctx context.Context, followerID int64, target string) (bool, error) {
	ref, err := domain.ParseRef(target)
	if err != nil {
		return false, err
	}
	return s.repo.IsFollowing(ctx, followerID, ref)
}

// Following 我关注的。
func (s *Service) Following(ctx context.Context, userID int64) ([]contract.FollowItem, error) {
	rows, err := s.repo.Following(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.FollowItem, 0, len(rows))
	for _, f := range rows {
		item := contract.FollowItem{
			FollowerID: f.FollowerID,
			Target:     contract.Target{Type: f.Ref.Type, ID: f.Ref.ID},
			CreatedAt:  f.CreatedAt,
		}
		item.Title, item.Cover = s.backfill(ctx, f.Ref)
		out = append(out, item)
	}
	return out, nil
}

// Followers 关注我的人。
func (s *Service) Followers(ctx context.Context, userID int64) ([]contract.FollowItem, error) {
	ids_, err := s.repo.FollowersOf(ctx, domain.Ref{Type: "user", ID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]contract.FollowItem, 0, len(ids_))
	for _, fid := range ids_ {
		ref := domain.Ref{Type: "user", ID: fid}
		title, cover := s.backfill(ctx, ref)
		out = append(out, contract.FollowItem{FollowerID: fid, Target: contract.Target{Type: ref.Type, ID: ref.ID}, Title: title, Cover: cover})
	}
	return out, nil
}

// Favorite 收藏（kind 由目标类型推导）。
func (s *Service) Favorite(ctx context.Context, userID int64, target string) error {
	ref, err := domain.ParseRef(target)
	if err != nil {
		return err
	}
	kind, ok := domain.KindOf(ref.Type)
	if !ok {
		return domain.ErrKindRequired
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.repo.InsertFavorite(ctx, tx, userID, ref, kind, int64(ids.New()))
	})
}

// Unfavorite 取消收藏。
func (s *Service) Unfavorite(ctx context.Context, userID int64, target string) error {
	ref, err := domain.ParseRef(target)
	if err != nil {
		return err
	}
	_, err = s.repo.DeleteFavorite(ctx, userID, ref)
	return err
}

// Favorites 我的收藏（按 kind）。
func (s *Service) Favorites(ctx context.Context, userID int64, kind string) ([]contract.FavoriteItem, error) {
	rows, err := s.repo.Favorites(ctx, userID, kind)
	if err != nil {
		return nil, err
	}
	out := make([]contract.FavoriteItem, 0, len(rows))
	for _, f := range rows {
		item := contract.FavoriteItem{ID: f.ID, Kind: f.Kind, Target: contract.Target{Type: f.Ref.Type, ID: f.Ref.ID}, CreatedAt: f.CreatedAt}
		item.Title, item.Cover = s.backfill(ctx, f.Ref)
		out = append(out, item)
	}
	return out, nil
}

// Unread 未读关注动态数。
func (s *Service) Unread(ctx context.Context, userID int64) (int, error) {
	return s.repo.Unread(ctx, userID)
}

// HandleContentPublished 订阅内容发布：为每个关注者累加未读并产生通知事件（供 notice 消费）。
func (s *Service) HandleContentPublished(ctx context.Context, e eventbus.Event) error {
	var p struct {
		ItemID   int64  `json:"item_id,string"`
		Kind     string `json:"kind"`
		Title    string `json:"title"`
		OwnerRef struct {
			Type string `json:"type"`
			ID   int64  `json:"id,string"`
		} `json:"owner_ref"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return nil
	}
	if p.OwnerRef.Type != "user" || p.OwnerRef.ID == 0 {
		return nil
	}
	followers, err := s.repo.FollowersOf(ctx, domain.Ref{Type: "user", ID: p.OwnerRef.ID})
	if err != nil {
		return err
	}
	brief, _ := json.Marshal(map[string]any{"author_id": p.OwnerRef.ID, "item_id": p.ItemID, "title": p.Title})
	for _, fid := range followers {
		if fid == p.OwnerRef.ID {
			continue
		}
		if err := s.pool.InTx(ctx, func(tx db.Tx) error {
			return s.repo.BumpUnread(ctx, tx, fid, brief)
		}); err != nil {
			return err
		}
		_ = s.emit(ctx, contract.NewFolloweePublished(ids.New().String(), contract.FolloweePublishedPayload{
			UserID: fid, AuthorID: p.OwnerRef.ID, ItemID: p.ItemID, Kind: p.Kind, Title: p.Title,
		}))
	}
	return nil
}

// ---- 内部 ----

func (s *Service) backfill(ctx context.Context, ref domain.Ref) (title, cover string) {
	if s.resolver == nil {
		return "", ""
	}
	info, err := s.resolver.Resolve(ctx, entityref.Ref{Type: ref.Type, ID: ref.ID})
	if err != nil {
		return "", ""
	}
	return info.Title, info.Cover
}

func (s *Service) emit(ctx context.Context, e eventbus.Event) error {
	if s.bus == nil {
		return nil
	}
	return s.pool.InTx(ctx, func(tx db.Tx) error {
		return s.bus.Publish(ctx, tx, e)
	})
}
