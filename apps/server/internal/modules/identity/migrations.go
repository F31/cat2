package identity

// migrations idn_ 前缀表（详细设计 4.2.1/4.2.2）。
const migrations = `
-- 过渡期后台管理员标记（五角色 RBAC 在 p4 细化；此处仅区分平台管理员/普通用户）
ALTER TABLE idn_user ADD COLUMN IF NOT EXISTS admin_role smallint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS idn_user (
	id bigint PRIMARY KEY,
	wechat_openid varchar(64), -- 加密存储（一期先明文占位，接入加密后迁移）
	nickname varchar(64),
	avatar_file_id bigint,
	phone_enc bytea,
	phone_hmac bytea UNIQUE,
	id_card_name_enc bytea,
	id_card_no_enc bytea,
	realname_status smallint NOT NULL DEFAULT 0, -- 0未实名/1已实名/2核验失败
	authz_ver int NOT NULL DEFAULT 1,
	status smallint NOT NULL DEFAULT 0, -- 0正常/1限制/2封禁/3已注销
	identity_type smallint NOT NULL DEFAULT 1, -- 1个人/2组织
	region_code varchar(12),
	last_active_at timestamptz,
	owner_space_id smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	version int NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS idn_session (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL,
	refresh_token_hash bytea NOT NULL,
	device_info varchar(128),
	expires_at timestamptz NOT NULL,
	revoked_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_idn_session_user ON idn_session(user_id, revoked_at);

CREATE TABLE IF NOT EXISTS idn_login_attempt (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id bigint,
	phone_hmac bytea,
	success boolean NOT NULL,
	ip inet,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS idn_sms_code (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	phone_hmac bytea NOT NULL,
	code_hash bytea NOT NULL,
	expires_at timestamptz NOT NULL,
	used_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS idn_deactivation_log (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id bigint NOT NULL,
	reason varchar(256),
	created_at timestamptz NOT NULL DEFAULT now()
);
`
