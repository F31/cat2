package notice

// migrations ntc_ 前缀表（详细设计 4.2.20）。
// 业务模块从不直接发通知：全部由模板数据驱动（事件→接收人→文案→渠道→合并）。
const migrations = `
CREATE TABLE IF NOT EXISTS ntc_template (
	id bigint PRIMARY KEY,
	module varchar(32) NOT NULL,
	event_code varchar(64) NOT NULL UNIQUE,
	title_tpl varchar(128) NOT NULL,
	content_tpl varchar(512) NOT NULL,
	channels jsonb NOT NULL DEFAULT '["inapp"]'::jsonb, -- inapp/subscribe/sms
	receiver_rule varchar(64) NOT NULL DEFAULT 'payload.user_id',
	merge_rule varchar(32),                            -- 空=不合并；same_circle_day=同圈同日合并
	closeable boolean NOT NULL DEFAULT true,           -- 资金与审核类不可关闭
	status smallint NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ntc_notice (
	id bigint PRIMARY KEY,
	user_id bigint NOT NULL,
	template_code varchar(64) NOT NULL,
	title varchar(128) NOT NULL,
	content varchar(512) NOT NULL,
	ref varchar(64),
	channel varchar(16) NOT NULL DEFAULT 'inapp',
	is_read boolean NOT NULL DEFAULT false,
	read_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ntc_notice_user ON ntc_notice(user_id, is_read, created_at);

CREATE TABLE IF NOT EXISTS ntc_subscribe_quota (
	user_id bigint NOT NULL,
	template_code varchar(64) NOT NULL,
	remaining int NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, template_code)
);

CREATE TABLE IF NOT EXISTS ntc_digest (
	circle_id bigint NOT NULL,
	date date NOT NULL,
	count int NOT NULL DEFAULT 0,
	notice_id bigint,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (circle_id, date)
);

CREATE TABLE IF NOT EXISTS ntc_preference (
	user_id bigint NOT NULL,
	template_code varchar(64) NOT NULL,
	enabled boolean NOT NULL DEFAULT true,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, template_code)
);
`
