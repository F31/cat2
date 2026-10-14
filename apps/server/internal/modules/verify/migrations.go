package verify

// migrations vfy_ 前缀表（详细设计 4.2.11 及 Badge 表）。
// 一期 verify 仅落地实名；个人/企业/机构认证与身份核验（affiliation）表在后续增量补。
const migrations = `
CREATE TABLE IF NOT EXISTS vfy_certification (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL,
	cert_type smallint NOT NULL,            -- 1实名/2个人/3个人企业身份/4企业/5机构
	target_org_id bigint,
	material_file_ids jsonb,               -- 材料多图（实名无材料）
	real_name_enc bytea,                   -- 实名姓名密文（仅 cert_type=1）
	id_no_enc bytea,                       -- 身份证号密文（仅 cert_type=1）
	extra jsonb,
	status smallint NOT NULL DEFAULT 0,    -- 0待审核/1已通过/2已驳回
	reject_reason varchar(500),
	reviewer_type smallint NOT NULL DEFAULT 1, -- 1平台/2企业/3组织
	reviewer_id bigint,
	submitted_at timestamptz NOT NULL DEFAULT now(),
	reviewed_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_vfy_cert_user ON vfy_certification(user_id, cert_type, status);

CREATE TABLE IF NOT EXISTS vfy_badge_def (
	code varchar(64) PRIMARY KEY,
	label varchar(64) NOT NULL,
	level smallint NOT NULL DEFAULT 1,
	style jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vfy_badge_grant (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	subject_type varchar(16) NOT NULL,     -- user/org
	subject_id bigint NOT NULL,
	badge_code varchar(64) NOT NULL,
	granted_at timestamptz NOT NULL DEFAULT now(),
	revoked_at timestamptz,
	UNIQUE (subject_type, subject_id, badge_code)
);

INSERT INTO vfy_badge_def (code, label, level) VALUES
	('realname', '实名', 1),
	('verified', '已认证', 2),
	('official', '官方合作认证', 3)
ON CONFLICT (code) DO NOTHING;
`
