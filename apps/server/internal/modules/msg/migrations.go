package msg

// migrations msg_ 前缀表（详细设计 4.2.x msg 表）。
// 单聊会话 (min_uid,max_uid) 唯一；消息按月分区；未读数=最大序号−已读游标（SQL 幂等、免 Redis）。
const migrations = `
CREATE TABLE IF NOT EXISTS msg_session (
	id bigint PRIMARY KEY,
	kind smallint NOT NULL DEFAULT 1,        -- 1单聊/2群聊
	min_uid bigint,                          -- 单聊：双方较小/较大 uid（群聊为空）
	max_uid bigint,
	biz_ref text,                            -- 群聊：会话标识（成员由 SessionMemberResolver 决定）
	last_seq bigint NOT NULL DEFAULT 0,
	last_message_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
-- 单聊唯一：同一对用户只有一套会话（PRD A9：详情页发起与消息列表同会话）
CREATE UNIQUE INDEX IF NOT EXISTS uq_msg_session_dm ON msg_session(min_uid, max_uid) WHERE kind = 1;
CREATE UNIQUE INDEX IF NOT EXISTS uq_msg_session_biz ON msg_session(biz_ref) WHERE kind = 2;

CREATE TABLE IF NOT EXISTS msg_session_member (
	session_id bigint NOT NULL,
	user_id bigint NOT NULL,
	role smallint NOT NULL DEFAULT 1,        -- 1成员/2群主
	unread_count int NOT NULL DEFAULT 0,     -- 冗余，可由 last_seq−cursor 重建
	joined_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (session_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_msg_member_user ON msg_session_member(user_id, session_id);

CREATE TABLE IF NOT EXISTS msg_read_cursor (
	session_id bigint NOT NULL,
	user_id bigint NOT NULL,
	read_seq bigint NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (session_id, user_id)
);

-- 消息按月分区（留存 ≥6 个月；超期分区经归档元数据表标记）
CREATE TABLE IF NOT EXISTS msg_message (
	id bigint NOT NULL,
	session_id bigint NOT NULL,
	seq bigint NOT NULL,
	sender_id bigint NOT NULL,
	msg_type smallint NOT NULL DEFAULT 1,     -- 1文本/2图片
	content text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE TABLE IF NOT EXISTS msg_message_default PARTITION OF msg_message DEFAULT;
CREATE INDEX IF NOT EXISTS idx_msg_message_session_seq ON msg_message(session_id, seq);

-- 归档元数据：超期分区的压缩归档对象与可检索元数据
CREATE TABLE IF NOT EXISTS msg_archive_meta (
	partition_name varchar(64) PRIMARY KEY,
	range_from timestamptz NOT NULL,
	range_to timestamptz NOT NULL,
	row_count bigint NOT NULL DEFAULT 0,
	object_key text,
	status smallint NOT NULL DEFAULT 0,       -- 0待归档/1已归档
	created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE msg_session ADD COLUMN IF NOT EXISTS title varchar(64);
`
