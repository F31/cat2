package cube

// migrations cub_ 前缀表（详细设计 4.2.15/4.2.16）。
// 猫立方是独立实体：可关联一个组织（owner_org_id）复用字段，但保持独立。
const migrations = `
CREATE TABLE IF NOT EXISTS cub_cube (
	id bigint PRIMARY KEY,
	name varchar(64) NOT NULL,
	owner_org_id bigint,
	leader_id bigint NOT NULL,              -- 猫王
	honey_id bigint NOT NULL,               -- 猫蜜（必配）
	region_code varchar(32),
	industry_codes jsonb NOT NULL DEFAULT '[]'::jsonb,
	chain_stage varchar(32),
	direction varchar(64),
	resources text,
	strength text,
	cooperation text,
	plan_level smallint NOT NULL DEFAULT 1,-- 1基础/2进阶/3高阶
	plan_expire_at timestamptz,
	status smallint NOT NULL DEFAULT 0,    -- 0正常/1封禁/2已解散
	transfer_log jsonb NOT NULL DEFAULT '[]'::jsonb,
	owner_space_id smallint NOT NULL DEFAULT 1,
	version int NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cub_cube_leader ON cub_cube(leader_id);
CREATE INDEX IF NOT EXISTS idx_cub_cube_plan_expire ON cub_cube(plan_level, plan_expire_at);

CREATE TABLE IF NOT EXISTS cub_member (
	id bigint PRIMARY KEY,
	cube_id bigint NOT NULL,
	user_ref bigint NOT NULL,
	role smallint NOT NULL DEFAULT 3,       -- 1猫王/2猫蜜/3成员/4共建成员
	invited_by bigint,
	joined_at timestamptz NOT NULL DEFAULT now(),
	left_at timestamptz,
	remove_reason varchar(128),
	status smallint NOT NULL DEFAULT 0,     -- 0正常/1已退出/2被移除
	UNIQUE (cube_id, user_ref)
);
CREATE INDEX IF NOT EXISTS idx_cub_member_user ON cub_member(user_ref, status);

CREATE TABLE IF NOT EXISTS cub_plan (
	id bigint PRIMARY KEY,
	cube_id bigint NOT NULL,
	level smallint NOT NULL,
	period varchar(8) NOT NULL DEFAULT 'year',
	amount_fen bigint NOT NULL DEFAULT 0,
	price_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	start_at timestamptz NOT NULL DEFAULT now(),
	end_at timestamptz NOT NULL,
	status smallint NOT NULL DEFAULT 0,     -- 0生效/1已到期
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cub_plan_cube ON cub_plan(cube_id, status);
CREATE INDEX IF NOT EXISTS idx_cub_plan_expire ON cub_plan(status, end_at);

CREATE TABLE IF NOT EXISTS cub_invite (
	id bigint PRIMARY KEY,
	cube_id bigint NOT NULL,
	code varchar(32) NOT NULL UNIQUE,
	created_by bigint NOT NULL,
	expire_at timestamptz NOT NULL,
	max_uses int,
	used_count int NOT NULL DEFAULT 0,
	need_confirm boolean NOT NULL DEFAULT true,
	status smallint NOT NULL DEFAULT 0     -- 0有效/1失效
);
CREATE INDEX IF NOT EXISTS idx_cub_invite_cube ON cub_invite(cube_id, status);
`
