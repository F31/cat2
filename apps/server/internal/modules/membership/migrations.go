package membership

// migrations mbr_ 前缀表（详细设计 membership）。
// 年费价格锚点维护在 settings；订单 price_snapshot 存单价/年限/系数快照。
const migrations = `
CREATE TABLE IF NOT EXISTS mbr_member (
	user_id bigint PRIMARY KEY,
	level smallint NOT NULL,                 -- 1 VIP / 2 SVIP(预留不上架) / 3 企业版
	status smallint NOT NULL DEFAULT 1,      -- 1生效中/2已过期
	started_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz NOT NULL,
	last_order_id bigint,
	price_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	auto_renew boolean NOT NULL DEFAULT false,
	owner_space_id smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mbr_member_expire ON mbr_member(status, expires_at);

CREATE TABLE IF NOT EXISTS mbr_sub_account (
	id bigint PRIMARY KEY,
	master_id bigint NOT NULL,
	sub_user_id bigint NOT NULL,
	status smallint NOT NULL DEFAULT 1,      -- 1生效/2已移除
	invited_at timestamptz NOT NULL DEFAULT now(),
	expires_at timestamptz,
	removed_at timestamptz,
	UNIQUE (master_id, sub_user_id)
);
-- 同一时间唯一归属：一个子账号同时只能属于一个主账号
CREATE UNIQUE INDEX IF NOT EXISTS uq_mbr_sub_active ON mbr_sub_account(sub_user_id) WHERE status=1;
`
