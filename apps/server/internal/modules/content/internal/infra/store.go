// Package infra content 仓储（PG，cnt_ 前缀）：信封表 cnt_item + 四张类型子表。
package infra

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/content/contract"
	dictcontract "catcube/internal/shared/dict/contract"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 内容不存在")

// Item 内容信封记录。
type Item struct {
	ID               int64
	Kind             string
	OwnerRef         contract.OwnerRef
	Title            string
	CoverFileID      int64
	RegionCode       string
	TagSet           dictcontract.TagSet
	Status           int16
	RejectReason     string
	PublishedAt      *time.Time
	Visibility       int16
	SearchDocVersion int
}

// Store content 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

// InsertItem 写入信封表。
func (s *Store) InsertItem(ctx context.Context, tx db.Tx, it Item) error {
	ts, _ := json.Marshal(it.TagSet)
	_, err := tx.Exec(ctx, `
		INSERT INTO cnt_item (id, kind, owner_type, owner_id, title, cover_file_id, region_code, tagset,
			status, reject_reason, published_at, visibility, owner_space_id, search_doc_version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,1,1,now(),now())`,
		it.ID, it.Kind, it.OwnerRef.Type, it.OwnerRef.ID, it.Title, nullID(it.CoverFileID),
		it.RegionCode, ts, it.Status, nullStr(it.RejectReason), it.PublishedAt)
	return err
}

const itemCols = `id, kind, owner_type, owner_id, title, COALESCE(cover_file_id,0), COALESCE(region_code,''),
	tagset, status, COALESCE(reject_reason,''), published_at, visibility, search_doc_version`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	var ts []byte
	err := row.Scan(&it.ID, &it.Kind, &it.OwnerRef.Type, &it.OwnerRef.ID, &it.Title, &it.CoverFileID,
		&it.RegionCode, &ts, &it.Status, &it.RejectReason, &it.PublishedAt, &it.Visibility, &it.SearchDocVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	if err != nil {
		return it, err
	}
	if len(ts) > 0 {
		_ = json.Unmarshal(ts, &it.TagSet)
	}
	return it, nil
}

// GetItem 取信封。
func (s *Store) GetItem(ctx context.Context, id int64) (Item, error) {
	return scanItem(s.pool.QueryRow(ctx, `SELECT `+itemCols+` FROM cnt_item WHERE id=$1`, id))
}

// ListByOwner 某账号发布列表（可按 kind 过滤，新→旧）。
func (s *Store) ListByOwner(ctx context.Context, ownerID int64, kind string) ([]Item, error) {
	q := `SELECT ` + itemCols + ` FROM cnt_item WHERE owner_id=$1`
	args := []any{ownerID}
	if kind != "" {
		q += ` AND kind=$2`
		args = append(args, kind)
	}
	q += ` ORDER BY created_at DESC, id DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// AdminList 后台内容列表（kind 空=全部；status<0=全部）。
func (s *Store) AdminList(ctx context.Context, kind string, status int16, limit int) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+itemCols+` FROM cnt_item
		 WHERE ($1='' OR kind=$1) AND ($2 < 0 OR status=$2)
		 ORDER BY created_at DESC, id DESC LIMIT $3`, kind, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateStatus 状态流转（含驳回原因/发布时间）。
func (s *Store) UpdateStatus(ctx context.Context, tx db.Tx, id int64, status int16, reason string, publishedAt *time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE cnt_item SET status=$2, reject_reason=$3, published_at=$4,
			search_doc_version=search_doc_version+1, version=version+1, updated_at=now()
		WHERE id=$1`, id, status, nullStr(reason), publishedAt)
	return err
}

// SaveSub 写类型子表（按 kind 落类型化列；联系方式密文由服务层预置 contact_phone_enc）。
func (s *Store) SaveSub(ctx context.Context, tx db.Tx, kind string, itemID int64, f map[string]any) error {
	switch kind {
	case contract.KindProject:
		_, err := tx.Exec(ctx, `
			INSERT INTO cnt_project (item_id, amount_wan, stage, core_strength, key_points, intro, target_customers, target_channels, contact_phone_enc)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			itemID, numF(f, "amount_wan"), text(f, "stage"), text(f, "core_strength"), text(f, "key_points"),
			text(f, "intro"), text(f, "target_customers"), text(f, "target_channels"), bytes(f, "contact_phone_enc"))
		return err
	case contract.KindProduct:
		carousel, _ := json.Marshal(strs(f, "carousel_file_ids"))
		_, err := tx.Exec(ctx, `
			INSERT INTO cnt_product (item_id, carousel_file_ids, target_customers, target_channels, core_strength, key_points, intro, contact_phone_enc, warehouse_status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			itemID, carousel, text(f, "target_customers"), text(f, "target_channels"), text(f, "core_strength"),
			text(f, "key_points"), text(f, "intro"), bytes(f, "contact_phone_enc"), numI(f, "warehouse_status"))
		return err
	case contract.KindMeeting:
		_, err := tx.Exec(ctx, `
			INSERT INTO cnt_meeting (item_id, start_at, end_at, location, venue_point, participant_limit, signup_method, intro)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			itemID, tm(f, "start_at"), tm(f, "end_at"), text(f, "location"), text(f, "venue_point"),
			numI(f, "participant_limit"), text(f, "signup_method"), text(f, "intro"))
		return err
	case contract.KindEvent:
		_, err := tx.Exec(ctx, `
			INSERT INTO cnt_event (item_id, poster_file_id, start_at, end_at, location, participant_limit, participant_count, signup_method, intro)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			itemID, numI(f, "poster_file_id"), tm(f, "start_at"), tm(f, "end_at"), text(f, "location"),
			numI(f, "participant_limit"), numI(f, "participant_count"), text(f, "signup_method"), text(f, "intro"))
		return err
	}
	return errors.New("content: 未知 kind")
}

// GetSub 读类型子表（time 以 RFC3339 字符串返回）。
func (s *Store) GetSub(ctx context.Context, kind string, itemID int64) (map[string]any, error) {
	out := map[string]any{}
	switch kind {
	case contract.KindProject:
		rows, err := s.pool.Query(ctx, `SELECT item_id, amount_wan, stage, core_strength, key_points, intro, target_customers, target_channels, contact_phone_enc FROM cnt_project WHERE item_id=$1`, itemID)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		if !rows.Next() {
			return out, rows.Err()
		}
		var itemID2 int64
		var amount *float64
		var stage, core, kp, intro, tc, tch *string
		var phone []byte
		if err := rows.Scan(&itemID2, &amount, &stage, &core, &kp, &intro, &tc, &tch, &phone); err != nil {
			return out, err
		}
		putF(out, "amount_wan", amount)
		putS(out, "stage", stage)
		putS(out, "core_strength", core)
		putS(out, "key_points", kp)
		putS(out, "intro", intro)
		putS(out, "target_customers", tc)
		putS(out, "target_channels", tch)
		if len(phone) > 0 {
			out["contact_phone_enc"] = phone
		}
		return out, nil
	case contract.KindProduct:
		rows, err := s.pool.Query(ctx, `SELECT item_id, carousel_file_ids, target_customers, target_channels, core_strength, key_points, intro, contact_phone_enc, warehouse_status FROM cnt_product WHERE item_id=$1`, itemID)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		if !rows.Next() {
			return out, rows.Err()
		}
		var itemID2, warehouse int64
		var carousel []byte
		var tc, tch, core, kp, intro *string
		var phone []byte
		if err := rows.Scan(&itemID2, &carousel, &tc, &tch, &core, &kp, &intro, &phone, &warehouse); err != nil {
			return out, err
		}
		var codes []string
		_ = json.Unmarshal(carousel, &codes)
		out["carousel_file_ids"] = codes
		putS(out, "target_customers", tc)
		putS(out, "target_channels", tch)
		putS(out, "core_strength", core)
		putS(out, "key_points", kp)
		putS(out, "intro", intro)
		out["warehouse_status"] = warehouse
		if len(phone) > 0 {
			out["contact_phone_enc"] = phone
		}
		return out, nil
	case contract.KindMeeting, contract.KindEvent:
		return s.getTimed(ctx, kind, itemID)
	}
	return out, errors.New("content: 未知 kind")
}

func (s *Store) getTimed(ctx context.Context, kind string, itemID int64) (map[string]any, error) {
	out := map[string]any{}
	var (
		itemID2                                    int64
		start, end                                 *time.Time
		location, venue, signup, intro             *string
		participantLimit, participantCount, poster *int64
	)
	var err error
	if kind == contract.KindMeeting {
		err = s.pool.QueryRow(ctx, `SELECT item_id, start_at, end_at, location, venue_point, participant_limit, signup_method, intro FROM cnt_meeting WHERE item_id=$1`, itemID).
			Scan(&itemID2, &start, &end, &location, &venue, &participantLimit, &signup, &intro)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return out, nil
			}
			return out, err
		}
		putT(out, "start_at", start)
		putT(out, "end_at", end)
		putS(out, "location", location)
		putS(out, "venue_point", venue)
		putI(out, "participant_limit", participantLimit)
		putS(out, "signup_method", signup)
		putS(out, "intro", intro)
		return out, nil
	}
	err = s.pool.QueryRow(ctx, `SELECT item_id, poster_file_id, start_at, end_at, location, participant_limit, participant_count, signup_method, intro FROM cnt_event WHERE item_id=$1`, itemID).
		Scan(&itemID2, &poster, &start, &end, &location, &participantLimit, &participantCount, &signup, &intro)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		return out, err
	}
	putI(out, "poster_file_id", poster)
	putT(out, "start_at", start)
	putT(out, "end_at", end)
	putS(out, "location", location)
	putI(out, "participant_limit", participantLimit)
	putI(out, "participant_count", participantCount)
	putS(out, "signup_method", signup)
	putS(out, "intro", intro)
	return out, nil
}

// ---- 值转换辅助 ----

func text(f map[string]any, key string) any {
	if s, ok := f[key].(string); ok && s != "" {
		return s
	}
	return nil
}
func bytes(f map[string]any, key string) any {
	if b, ok := f[key].([]byte); ok && len(b) > 0 {
		return b
	}
	return nil
}
func numI(f map[string]any, key string) any {
	switch t := f[key].(type) {
	case int:
		return t
	case int64:
		return t
	case float64:
		return int64(t)
	}
	return nil
}
func numF(f map[string]any, key string) any {
	switch t := f[key].(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case float64:
		return t
	}
	return nil
}
func tm(f map[string]any, key string) any {
	if s, ok := f[key].(string); ok && s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return nil
}
func strs(f map[string]any, key string) []string {
	switch t := f[key].(type) {
	case []string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return []string{}
}

func putS(m map[string]any, key string, v *string) {
	if v != nil && *v != "" {
		m[key] = *v
	}
}
func putI(m map[string]any, key string, v *int64) {
	if v != nil {
		m[key] = *v
	}
}
func putF(m map[string]any, key string, v *float64) {
	if v != nil {
		m[key] = *v
	}
}
func putT(m map[string]any, key string, v *time.Time) {
	if v != nil {
		m[key] = v.Format(time.RFC3339)
	}
}

func nullID(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
