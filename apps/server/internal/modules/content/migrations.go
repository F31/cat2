package content

// migrations cnt_ 前缀表（详细设计 4.2.13/4.2.14）。
// 信封表 + 四张类型子表：类型化列用于筛选排序，仅展示用的少量扩展走 JSONB。
const migrations = `
CREATE TABLE IF NOT EXISTS cnt_item (
	id bigint PRIMARY KEY,
	kind varchar(32) NOT NULL,             -- project/product/meeting/event
	owner_type varchar(16) NOT NULL DEFAULT 'user',
	owner_id bigint NOT NULL,
	title varchar(128) NOT NULL,
	cover_file_id bigint,
	region_code varchar(12),
	tagset jsonb NOT NULL DEFAULT '{}'::jsonb,
	status smallint NOT NULL DEFAULT 0,    -- 0审核中/1展示中/2已下架
	reject_reason varchar(500),
	published_at timestamptz,
	visibility smallint NOT NULL DEFAULT 1, -- 1 platform/2 space（一期恒 platform）
	owner_space_id smallint NOT NULL DEFAULT 1,
	search_doc_version int NOT NULL DEFAULT 1,
	version int NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cnt_item_owner ON cnt_item(owner_type, owner_id, kind, status);
CREATE INDEX IF NOT EXISTS idx_cnt_item_status ON cnt_item(kind, status, published_at DESC);

CREATE TABLE IF NOT EXISTS cnt_project (
	item_id bigint PRIMARY KEY,
	amount_wan numeric(12,2),
	stage varchar(64),
	core_strength text,
	key_points text,
	intro text,
	target_customers varchar(256),
	target_channels varchar(256),
	contact_phone_enc bytea
);

CREATE TABLE IF NOT EXISTS cnt_product (
	item_id bigint PRIMARY KEY,
	carousel_file_ids jsonb,
	target_customers varchar(256),
	target_channels varchar(256),
	core_strength text,
	key_points text,
	intro text,
	contact_phone_enc bytea,
	warehouse_status smallint NOT NULL DEFAULT 0 -- 0未入库/1待审/2已入库
);

CREATE TABLE IF NOT EXISTS cnt_meeting (
	item_id bigint PRIMARY KEY,
	start_at timestamptz,
	end_at timestamptz,
	location varchar(256),
	venue_point varchar(128),
	participant_limit int,
	signup_method varchar(128),
	intro text
);

CREATE TABLE IF NOT EXISTS cnt_event (
	item_id bigint PRIMARY KEY,
	poster_file_id bigint,
	start_at timestamptz,
	end_at timestamptz,
	location varchar(256),
	participant_limit int,
	participant_count int,
	signup_method varchar(128),
	intro text
);
`
