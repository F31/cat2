package media

// migrations med_ 前缀表（详细设计 4.2.7）。
const migrations = `
CREATE TABLE IF NOT EXISTS med_file (
	id bigint PRIMARY KEY,
	bucket varchar(64) NOT NULL,
	object_key varchar(256) NOT NULL,
	mime varchar(64) NOT NULL,
	size_bytes bigint NOT NULL,
	width int,
	height int,
	thumb_key varchar(256),
	ref_count int NOT NULL DEFAULT 0,
	owner_ref jsonb,
	status smallint NOT NULL DEFAULT 1, -- 1有效/0孤儿待清理
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_med_file_orphan ON med_file(status) WHERE status=0;
`
