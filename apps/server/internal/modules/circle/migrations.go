package circle

// migrations cir_ 前缀表（详细设计 4.2.17/4.2.18）。
const migrations = `
CREATE TABLE IF NOT EXISTS cir_circle (
	id bigint PRIMARY KEY,
	name varchar(64) NOT NULL,
	intro varchar(500),
	cover_file_id bigint,
	region_code varchar(32),
	industry_codes jsonb NOT NULL DEFAULT '[]'::jsonb,
	type smallint NOT NULL DEFAULT 2,        -- 1常设/2临时
	duration_days smallint NOT NULL DEFAULT 30,
	end_at timestamptz,
	owner_user_id bigint NOT NULL,
	org_id bigint,
	member_count int NOT NULL DEFAULT 0,
	feed_count int NOT NULL DEFAULT 0,
	recommendable_at timestamptz NOT NULL DEFAULT now(),
	status smallint NOT NULL DEFAULT 0,      -- 0进行中/1已结束/2已解散/3已封禁/4已归档
	prev_status smallint,                   -- 封禁/归档前状态（解封/恢复用）
	archived_at timestamptz,
	dissolved_at timestamptz,
	owner_space_id smallint NOT NULL DEFAULT 1,
	version int NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cir_circle_owner ON cir_circle(owner_user_id, status);
CREATE INDEX IF NOT EXISTS idx_cir_circle_end ON cir_circle(status, end_at);
ALTER TABLE cir_circle ADD COLUMN IF NOT EXISTS prev_status smallint;
ALTER TABLE cir_circle ADD COLUMN IF NOT EXISTS archived_at timestamptz;
ALTER TABLE cir_circle ADD COLUMN IF NOT EXISTS source_ref varchar(64);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cir_circle_source ON cir_circle(source_ref) WHERE source_ref IS NOT NULL;

CREATE TABLE IF NOT EXISTS cir_member (
	id bigint PRIMARY KEY,
	circle_id bigint NOT NULL,
	user_ref bigint NOT NULL,
	joined_via smallint NOT NULL DEFAULT 1,  -- 1申请/2邀请/3组织/4活动报名
	joined_at timestamptz NOT NULL DEFAULT now(),
	status smallint NOT NULL DEFAULT 0,      -- 0正常/1已退出
	UNIQUE (circle_id, user_ref)
);
CREATE INDEX IF NOT EXISTS idx_cir_member_user ON cir_member(user_ref, status);

CREATE TABLE IF NOT EXISTS cir_invite (
	id bigint PRIMARY KEY,
	circle_id bigint NOT NULL,
	code varchar(32) NOT NULL UNIQUE,
	created_by bigint NOT NULL,
	expire_at timestamptz NOT NULL,          -- =圈剩余存续期
	reset_seq int NOT NULL DEFAULT 1,
	status smallint NOT NULL DEFAULT 0,      -- 0有效/1失效
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cir_invite_circle ON cir_invite(circle_id, status);

CREATE TABLE IF NOT EXISTS cir_demand (
	id bigint PRIMARY KEY,
	circle_id bigint NOT NULL,
	author_id bigint NOT NULL,
	title varchar(30) NOT NULL,
	content varchar(500) NOT NULL,
	demand_type varchar(32),
	region_code varchar(32),
	industry_codes jsonb NOT NULL DEFAULT '[]'::jsonb,
	expire_at timestamptz NOT NULL,
	scope smallint NOT NULL DEFAULT 1,       -- 1仅本圈/2全平台
	contact_phone_enc bytea,
	status smallint NOT NULL DEFAULT 0,      -- 0有效/1已过期/2已删除
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cir_demand_circle ON cir_demand(circle_id, status, created_at);

CREATE TABLE IF NOT EXISTS cir_feed (
	id bigint PRIMARY KEY,
	circle_id bigint NOT NULL,
	author_id bigint NOT NULL,
	text varchar(1000),
	image_file_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
	is_announcement boolean NOT NULL DEFAULT false,
	status smallint NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cir_feed_circle ON cir_feed(circle_id, created_at);

CREATE TABLE IF NOT EXISTS cir_mount (
	id bigint PRIMARY KEY,
	circle_id bigint NOT NULL,
	item_ref varchar(64) NOT NULL,           -- EntityRef 指向 cnt_item
	mounted_by bigint NOT NULL,
	mount_type smallint NOT NULL DEFAULT 1,  -- 1本人/2申请挂载
	approval_status smallint NOT NULL DEFAULT 1, -- 0待确认/1已同意/2已拒绝
	status smallint NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cir_mount_circle ON cir_mount(circle_id, status);

ALTER TABLE cir_demand ADD COLUMN IF NOT EXISTS transfer_ref varchar(64);

CREATE TABLE IF NOT EXISTS cir_apply (
	id bigint PRIMARY KEY,
	circle_id bigint NOT NULL,
	applicant_id bigint NOT NULL,
	status smallint NOT NULL DEFAULT 0,      -- 0待审/1已通过/2已拒绝
	reason varchar(200),
	decided_by bigint,
	decided_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (circle_id, applicant_id)
);
CREATE INDEX IF NOT EXISTS idx_cir_apply_circle ON cir_apply(circle_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_cir_apply_user ON cir_apply(applicant_id, status);
`
