package dict

// migrations dct_ 前缀表（详细设计 4.2.5/4.2.6）。
const migrations = `
CREATE TABLE IF NOT EXISTS dct_taxonomy_option (
	id bigint PRIMARY KEY,
	dimension varchar(32) NOT NULL,
	code varchar(64) NOT NULL UNIQUE,
	label varchar(128) NOT NULL,
	parent_code varchar(64),
	sort int NOT NULL DEFAULT 0,
	status smallint NOT NULL DEFAULT 1, -- 1启用/0停用（只停用不删除）
	owner_space_id smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	version int NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_dct_option_dim ON dct_taxonomy_option(dimension, sort);

CREATE TABLE IF NOT EXISTS dct_subject_type (
	id bigint PRIMARY KEY,
	code varchar(64) NOT NULL UNIQUE, -- 创建后不可改
	name varchar(64) NOT NULL,
	icon_file_id bigint,
	can_publish boolean NOT NULL DEFAULT true,
	home_visible boolean NOT NULL DEFAULT true,
	field_template_id bigint,
	sort int NOT NULL DEFAULT 0,
	status smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dct_field_template (
	id bigint PRIMARY KEY,
	name varchar(64) NOT NULL,
	fields jsonb NOT NULL, -- [{field, required, sort}]
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);

-- 历史主体类型停用（只停用不删除；六类主题改用 service_org）
UPDATE dct_subject_type SET status=0 WHERE code='gov_service';

CREATE TABLE IF NOT EXISTS dct_field_def (
	field varchar(64) PRIMARY KEY, -- 代码侧登记的字段池（随版本发布）
	label varchar(64) NOT NULL,
	value_type varchar(16) NOT NULL
);
`
