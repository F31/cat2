package social

// migrations soc_ 前缀表（详细设计 4.2.x social 表）。
// 目标统一 EntityRef{type,id}：关注/收藏不复制实体，只存引用。
const migrations = `
CREATE TABLE IF NOT EXISTS soc_follow (
	id bigint PRIMARY KEY,
	follower_id bigint NOT NULL,
	followee_ref jsonb NOT NULL,      -- EntityRef{type,id}
	followee_type varchar(32) NOT NULL,
	followee_id bigint NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (follower_id, followee_type, followee_id)
);
CREATE INDEX IF NOT EXISTS idx_soc_follow_target ON soc_follow(followee_type, followee_id);

CREATE TABLE IF NOT EXISTS soc_favorite (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL,
	target_ref jsonb NOT NULL,        -- EntityRef{type,id}
	target_type varchar(32) NOT NULL,
	target_id bigint NOT NULL,
	kind varchar(16) NOT NULL,        -- person/company/project/product/meeting/event
	created_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (user_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_soc_favorite_user ON soc_favorite(user_id, kind, created_at DESC);

CREATE TABLE IF NOT EXISTS soc_follow_unread (
	user_id bigint PRIMARY KEY,
	unread_count int NOT NULL DEFAULT 0,
	last_follower_brief jsonb,
	updated_at timestamptz NOT NULL DEFAULT now()
);
`
