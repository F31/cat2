package profile

// migrations prf_ 前缀表（详细设计 4.2.8）。
// 一个账号一张名片；个人/组织字段差异由 identity_type + 字段模板渲染；认证主体信息锁定。
const migrations = `
CREATE TABLE IF NOT EXISTS prf_card (
	user_id bigint PRIMARY KEY,
	subject_type_code varchar(64) NOT NULL DEFAULT '',
	identity_type smallint NOT NULL DEFAULT 1, -- 1个人/2组织（注册时决定，本期不可自改）
	name varchar(128) NOT NULL DEFAULT '',
	avatar_file_id bigint,
	region_code varchar(12),
	industry_codes varchar(256),  -- 子行业 code，逗号分隔
	need_codes varchar(256),      -- 合作需求
	offer_codes varchar(256),     -- 合作提供/主要交付
	advantage_field varchar(128),
	background varchar(64),
	target_customers varchar(256),
	target_channels varchar(256),
	qualification_file_ids jsonb, -- 资质证明图片（组织）
	core_strength text,
	intro text,
	contact_phone_enc bytea,      -- 联系方式密文（会员可见；明文永不下发）
	visibility_settings jsonb NOT NULL DEFAULT '{}'::jsonb, -- 字段级对外展示开关
	profile_tier smallint NOT NULL DEFAULT 1,  -- 画像完成层级 1/2/3
	search_doc_version int NOT NULL DEFAULT 1, -- 投影版本（防旧事件覆盖）
	locked_fields jsonb NOT NULL DEFAULT '[]'::jsonb, -- 认证通过后锁定的字段名
	owner_space_id smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	version int NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_prf_card_tier ON prf_card(owner_space_id, profile_tier);
`
