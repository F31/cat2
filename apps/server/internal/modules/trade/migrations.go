package trade

// migrations trd_ 前缀表（详细设计 4.2.10 / TABLE 53）。
// trade 只管钱：不认识「会员/订阅」，履约由各业务模块经 OrderFulfiller 注册实现。
const migrations = `
CREATE TABLE IF NOT EXISTS trd_order (
	id bigint PRIMARY KEY,
	biz_type varchar(32) NOT NULL,           -- member/cube_plan...
	biz_ref varchar(64),                     -- 业务侧引用（档位/年限等快照键）
	buyer_id bigint NOT NULL,
	amount_fen bigint NOT NULL,
	price_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	status smallint NOT NULL DEFAULT 0,      -- 0待支付/1已支付/2已关闭
	pay_expire_at timestamptz NOT NULL,
	paid_at timestamptz,
	closed_at timestamptz,
	idempotency_key varchar(128),
	version int NOT NULL DEFAULT 1,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_trd_order_idem ON trd_order(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_trd_order_buyer ON trd_order(buyer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trd_order_expire ON trd_order(status, pay_expire_at);

CREATE TABLE IF NOT EXISTS trd_payment (
	id bigint PRIMARY KEY,
	order_id bigint NOT NULL,
	out_trade_no varchar(64) NOT NULL UNIQUE,
	channel varchar(16) NOT NULL DEFAULT 'wechat',
	amount_fen bigint NOT NULL,
	status smallint NOT NULL DEFAULT 0,      -- 0待支付/1已支付/2已关闭
	paid_at timestamptz,
	raw_notify jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_trd_payment_order ON trd_payment(order_id);

CREATE TABLE IF NOT EXISTS trd_refund (
	id bigint PRIMARY KEY,
	payment_id bigint NOT NULL,
	reason varchar(128) NOT NULL,
	amount_fen bigint NOT NULL,
	status smallint NOT NULL DEFAULT 0,      -- 0处理中/1成功/2失败
	refund_no varchar(64) NOT NULL,
	operator_id bigint NOT NULL,
	idem_key varchar(160) NOT NULL UNIQUE,   -- payment_id + reason
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS trd_recon (
	recon_date date NOT NULL,
	channel varchar(16) NOT NULL,
	order_count int NOT NULL DEFAULT 0,
	amount_fen bigint NOT NULL DEFAULT 0,
	diff_status smallint NOT NULL DEFAULT 0, -- 0一致/1有差异
	diff_detail jsonb,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (recon_date, channel)
);

CREATE TABLE IF NOT EXISTS trd_fulfillment (
	order_id bigint PRIMARY KEY,
	biz_type varchar(32) NOT NULL,
	status smallint NOT NULL DEFAULT 0,      -- 0待履约/1已履约/9死信
	retry_count int NOT NULL DEFAULT 0,
	last_error varchar(500),
	updated_at timestamptz NOT NULL DEFAULT now(),
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_trd_fulfillment_status ON trd_fulfillment(status, retry_count);

CREATE TABLE IF NOT EXISTS trd_apportion_group (
	id bigint PRIMARY KEY,
	cube_plan_ref varchar(64),
	initiator_id bigint NOT NULL,
	total_fen bigint NOT NULL,
	per_fen bigint NOT NULL,
	target_n int NOT NULL,
	pledged_count int NOT NULL DEFAULT 0,
	deadline timestamptz NOT NULL,
	status smallint NOT NULL DEFAULT 0,      -- 0进行中/1已成团/2未成团/3已终结
	price_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS trd_pledge (
	id bigint PRIMARY KEY,
	group_id bigint NOT NULL,
	user_id bigint NOT NULL,
	amount_fen bigint NOT NULL,
	payment_id bigint,
	status smallint NOT NULL DEFAULT 0,      -- 0已付/1已退
	refunded_at timestamptz,
	created_at timestamptz NOT NULL DEFAULT now(),
	UNIQUE (group_id, user_id)
);
`
