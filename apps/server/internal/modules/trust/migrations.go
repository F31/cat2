package trust

// migrations trs_ 前缀表（详细设计 4.2.3/4.2.4）。
const migrations = `
CREATE TABLE IF NOT EXISTS trs_review_task (
	id bigint PRIMARY KEY,
	source smallint NOT NULL, -- 1机器/2举报/3认证材料/4产品入库/5高风险抽检
	subject_type varchar(32) NOT NULL,
	subject_id bigint NOT NULL,
	submitter_id bigint NOT NULL DEFAULT 0,
	payload_ref jsonb,
	risk_level smallint NOT NULL DEFAULT 1,
	status smallint NOT NULL DEFAULT 0, -- 0待审/1通过/2驳回/3已关闭
	reviewer_id bigint,
	reason varchar(500),
	sla_due_at timestamptz NOT NULL,
	reviewed_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_trs_task_queue ON trs_review_task(status, sla_due_at);

CREATE TABLE IF NOT EXISTS trs_report (
	id bigint PRIMARY KEY,
	reporter_id bigint NOT NULL,
	target_ref jsonb NOT NULL, -- EntityRef{type,id}
	reason_code varchar(32) NOT NULL,
	detail varchar(500),
	status smallint NOT NULL DEFAULT 0, -- 0待处理/1成立/2驳回
	handler_id bigint,
	handled_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS trs_violation_score (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	user_id bigint NOT NULL,
	source_task_id bigint,
	points int NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS trs_restriction (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL,
	type smallint NOT NULL, -- 1限制发布/2封禁
	days int NOT NULL,
	reason varchar(500),
	expires_at timestamptz,
	lifted_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_trs_restriction_user ON trs_restriction(user_id, expires_at);

CREATE TABLE IF NOT EXISTS trs_sensitive_word (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	word varchar(64) NOT NULL UNIQUE,
	category varchar(32) NOT NULL DEFAULT 'sensitive', -- sensitive/引流词
	status smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS trs_moderation_record (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	subject_type varchar(32) NOT NULL,
	subject_id bigint NOT NULL,
	channel varchar(16) NOT NULL, -- text/image
	result jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS trs_media_check (
	id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	trace_id varchar(128) NOT NULL UNIQUE,
	subject_type varchar(32) NOT NULL,
	subject_id bigint NOT NULL,
	media_id varchar(64) NOT NULL DEFAULT '',
	status smallint NOT NULL DEFAULT 0, -- 0待回调/1通过/2拒绝/3其它媒体已拒绝取消
	result jsonb,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now(),
	checked_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_trs_media_subject ON trs_media_check(subject_type, subject_id, status);

`
