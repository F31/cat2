// Package infra profile 仓储（PG，prf_ 前缀）。
package infra

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"catcube/internal/kernel/db"
	"catcube/internal/modules/profile/internal/domain"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("infra: 名片不存在")

// Card 名片记录（本模块 infra 可见）。
type Card struct {
	UserID           int64
	SubjectTypeCode  string
	IdentityType     int16
	Name             string
	AvatarFileID     int64
	RegionCode       string
	IndustryCodes    []string
	NeedCodes        []string
	OfferCodes       []string
	AdvantageField   string
	Background       string
	TargetCustomers  string
	TargetChannels   string
	QualificationIDs []int64
	CoreStrength     string
	Intro            string
	ContactPhoneEnc  []byte
	Visibility       domain.Visibility
	ProfileTier      int16
	SearchDocVersion int
	LockedFields     []string
	Version          int
}

// Store profile 仓储。
type Store struct{ pool *db.Pool }

func NewStore(pool *db.Pool) *Store { return &Store{pool: pool} }

const cols = `user_id, subject_type_code, identity_type, COALESCE(name,''),
	COALESCE(avatar_file_id,0), COALESCE(region_code,''), COALESCE(industry_codes,''),
	COALESCE(need_codes,''), COALESCE(offer_codes,''), COALESCE(advantage_field,''),
	COALESCE(background,''), COALESCE(target_customers,''), COALESCE(target_channels,''),
	COALESCE(qualification_file_ids,'[]'::jsonb), COALESCE(core_strength,''), COALESCE(intro,''),
	contact_phone_enc, visibility_settings, profile_tier, search_doc_version, locked_fields, version`

func scan(row pgx.Row) (Card, error) {
	var c Card
	var industry, need, offer string
	var qualRaw, visRaw, lockRaw []byte
	err := row.Scan(&c.UserID, &c.SubjectTypeCode, &c.IdentityType, &c.Name,
		&c.AvatarFileID, &c.RegionCode, &industry, &need, &offer, &c.AdvantageField,
		&c.Background, &c.TargetCustomers, &c.TargetChannels, &qualRaw, &c.CoreStrength, &c.Intro,
		&c.ContactPhoneEnc, &visRaw, &c.ProfileTier, &c.SearchDocVersion, &lockRaw, &c.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.IndustryCodes = domain.SplitCodes(industry)
	c.NeedCodes = domain.SplitCodes(need)
	c.OfferCodes = domain.SplitCodes(offer)
	c.Visibility = domain.Visibility{}
	if len(visRaw) > 0 {
		_ = json.Unmarshal(visRaw, &c.Visibility)
	}
	if len(qualRaw) > 0 {
		_ = json.Unmarshal(qualRaw, &c.QualificationIDs)
	}
	if len(lockRaw) > 0 {
		_ = json.Unmarshal(lockRaw, &c.LockedFields)
	}
	return c, nil
}

// GetByUserID 按账号取名片。
func (s *Store) GetByUserID(ctx context.Context, userID int64) (Card, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM prf_card WHERE user_id=$1`, userID))
}

// CreateDefault 注册时建默认名片（第一层：账号ID + 身份类型 + 默认名称）。
func (s *Store) CreateDefault(ctx context.Context, tx db.Tx, userID int64, identityType int16, name string) error {
	if name == "" {
		name = "猫平方用户"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO prf_card (user_id, identity_type, name, visibility_settings, profile_tier, created_at, updated_at)
		VALUES ($1,$2,$3,'{}'::jsonb,1,now(),now())
		ON CONFLICT (user_id) DO NOTHING`, userID, identityType, name)
	return err
}

// Save 保存名片（存在则更新并递增 version/search_doc_version）。
func (s *Store) Save(ctx context.Context, tx db.Tx, c Card) error {
	qual, _ := json.Marshal(c.QualificationIDs)
	vis, _ := json.Marshal(c.Visibility)
	lock, _ := json.Marshal(c.LockedFields)
	_, err := tx.Exec(ctx, `
		INSERT INTO prf_card (user_id, subject_type_code, identity_type, name, avatar_file_id,
			region_code, industry_codes, need_codes, offer_codes, advantage_field, background,
			target_customers, target_channels, qualification_file_ids, core_strength, intro,
			contact_phone_enc, visibility_settings, profile_tier, search_doc_version, locked_fields,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,now(),now())
		ON CONFLICT (user_id) DO UPDATE SET
			subject_type_code=EXCLUDED.subject_type_code, identity_type=EXCLUDED.identity_type,
			name=EXCLUDED.name, avatar_file_id=EXCLUDED.avatar_file_id, region_code=EXCLUDED.region_code,
			industry_codes=EXCLUDED.industry_codes, need_codes=EXCLUDED.need_codes,
			offer_codes=EXCLUDED.offer_codes, advantage_field=EXCLUDED.advantage_field,
			background=EXCLUDED.background, target_customers=EXCLUDED.target_customers,
			target_channels=EXCLUDED.target_channels, qualification_file_ids=EXCLUDED.qualification_file_ids,
			core_strength=EXCLUDED.core_strength, intro=EXCLUDED.intro,
			contact_phone_enc=EXCLUDED.contact_phone_enc, visibility_settings=EXCLUDED.visibility_settings,
			profile_tier=EXCLUDED.profile_tier, search_doc_version=EXCLUDED.search_doc_version,
			locked_fields=EXCLUDED.locked_fields, version=prf_card.version+1, updated_at=now()`,
		c.UserID, c.SubjectTypeCode, c.IdentityType, c.Name, c.AvatarFileID,
		c.RegionCode, domain.JoinCodes(c.IndustryCodes), domain.JoinCodes(c.NeedCodes),
		domain.JoinCodes(c.OfferCodes), c.AdvantageField, c.Background,
		c.TargetCustomers, c.TargetChannels, qual, c.CoreStrength, c.Intro,
		c.ContactPhoneEnc, vis, c.ProfileTier, c.SearchDocVersion, lock)
	return err
}
