package discovery

// migrations dsc_ 前缀表（详细设计 4.2.19 discovery 投影）。
// dsc_search_doc / dsc_match_candidates 是事件投影只读模型，可由事件回放重建（非事实来源）。
const migrations = `
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS dsc_search_doc (
	doc_ref text PRIMARY KEY,                 -- EntityRef type:id，如 user:123 / content.project:456
	doc_type varchar(32) NOT NULL,            -- user/org/content.project/content.product/content.meeting/content.event
	subject_id bigint NOT NULL,
	title text NOT NULL DEFAULT '',
	cover text,
	region_code text,
	industry text[] NOT NULL DEFAULT '{}',
	need text[] NOT NULL DEFAULT '{}',
	offer text[] NOT NULL DEFAULT '{}',
	member_level smallint NOT NULL DEFAULT 0, -- 0普通/1VIP/2SVIP/3企业
	verified boolean NOT NULL DEFAULT false,
	active boolean NOT NULL DEFAULT true,     -- 下架/关闭=false（置灰）
	search_doc_version int NOT NULL DEFAULT 0,
	last_active_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dsc_doc_type ON dsc_search_doc(doc_type, active);
CREATE INDEX IF NOT EXISTS idx_dsc_doc_region ON dsc_search_doc(region_code);
CREATE INDEX IF NOT EXISTS idx_dsc_doc_industry ON dsc_search_doc USING gin (industry);
CREATE INDEX IF NOT EXISTS idx_dsc_doc_need ON dsc_search_doc USING gin (need);
CREATE INDEX IF NOT EXISTS idx_dsc_doc_offer ON dsc_search_doc USING gin (offer);
CREATE INDEX IF NOT EXISTS idx_dsc_doc_title_trgm ON dsc_search_doc USING gin (title gin_trgm_ops);

CREATE TABLE IF NOT EXISTS dsc_match_candidates (
	user_id bigint NOT NULL,
	candidate_ref text NOT NULL,
	score numeric(8,4) NOT NULL DEFAULT 0,
	reason jsonb NOT NULL DEFAULT '[]'::jsonb,
	rank int NOT NULL DEFAULT 0,
	computed_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, candidate_ref)
);
CREATE INDEX IF NOT EXISTS idx_dsc_match_user ON dsc_match_candidates(user_id, rank, score DESC);

CREATE TABLE IF NOT EXISTS dsc_projection_meta (
	name text PRIMARY KEY,
	last_rebuilt_at timestamptz
);
`
