# channel 模块预研（p4 开工前置设计；**不触发 #13 门禁**）

> 状态：**预研**。`openspec/changes/p4-channel-admin` 的开工前置为 p0 待确认 **#13（后台五角色划分）**；
> 本文只固化「与角色无关」的数据模型、契约边界与实现路径，确认后按 `p4-channel-admin/tasks.md` 顺序落地。
> 依赖结论若与产品/财务不符，先改本文件再改代码（契约先行）。

## 1. 边界与位置

| 项 | 取值 | 依据 |
| --- | --- | --- |
| 模块名 | `channel` | 命名约定（技术方案 6.5） |
| 依赖阶梯 | **R4**（可依赖 R0–R3：identity/trade/membership/cube…） | `tools/archcheck` |
| 表前缀 | `chn_` | 表前缀即归属（archcheck 强制） |
| 错误码段 | **15**（如 `150001`） | `contracts/errors/README.md` |
| 事件前缀 | `channel.*` | 事件命名 `module.PastTense.vN` |
| 移动端 | 小程序「运营商」身份卡（个人中心内） | PRD 运营商段 |

## 2. 领域模型（草案）

### 2.1 运营商与开户

```
chn_operator
  id, user_id UNIQUE, region_code, name, status(0申请中/1已开通/2已停用),
  referrer_operator_id  -- 单列、无上级链（红线：不得多级）
  agency_fee_paid_fen, agency_fee_year, guarantee_fen, opened_at, suspended_at, reason
  created_at, updated_at, version
```

- **红线（PRD/详细设计）**：`referrer_operator_id` 为**单层展示字段**，无「上级/下级」集合；contract 与 SQL 均不提供多级查询；不自建下级（提交时校验 referrer 必须已开通且区域不同）。
- 区域查重：`region_code` 唯一约束（一地一家）；`status=1` 时才占用。

```
chn_guarantee        -- 保证金台账（预收款，不进收入）
  id, operator_id, amount_fen, paid_at, refunded_at, refund_reason, voucher_ref
```

```
chn_annual_target    -- 保底任务（12 个月 ≥50 付费会员 或 ≥3 落地项目）
  operator_id, year, paid_member_count, landing_project_count, warn_flags jsonb, updated_at
```

### 2.2 归属关系

```
chn_attribution
  id, user_id UNIQUE(有效期内), operator_id, region_code,
  source(1链接/2二维码/3邀请码), source_ref, bound_at, unbind_at,
  correct_approval jsonb  -- 纠错审批留痕（申请人/审批人/前后值/时间）
```

- 归属在**注册来源绑定**时建立（identity `UserRegistered` 消费）；同一用户同时仅一条有效归属。
- 纠错必须走审批（`governance` 代操作清单）且留痕；审批生效写 `unbind_at` + 新归属。

### 2.3 佣金与结算

```
chn_commission_rule  -- 或直接走 SettingsSchema（推荐）
  key: channel.commission.first_fen_ratio / renew_ratio / scope
chn_commission
  id, operator_id, user_id, order_id, biz_type, base_fen, ratio_bp,
  ratio_snapshot jsonb,  -- 计佣时点的规则快照（配置变更不影响历史）
  amount_fen, month char(7), status(0待入账/1已入账/2已冲正), created_at
chn_settlement
  id, operator_id, month char(7), commission_fen, adjust_fen, payable_fen,
  status(0待结算/1已复核/2已打款/3已驳回), reviewed_by, paid_by, paid_at, voucher_ref
  UNIQUE(operator_id, month)
```

- 计佣触发：订阅 `trade.OrderPaid.v1`（首单/续费按 #16 结论），金额=快照价 × 比例；比例来自 `SettingsSchema`（全后台可配），**写流水时快照**。
- **状态机 #17 月结**：待结算 → 已复核 → 已打款（不可撤销）；驳回回到待结算并记原因。
- **「复核≠执行」**：`reviewed_by != paid_by`（内核 policy 互斥规则；违反 `denied` 记审计）。

### 2.4 推广物料

```
chn_link
  id, operator_id, channel(link/qrcode/invite_code), code UNIQUE, region_code,
  status(0有效/1停用), created_at, disabled_at
```

## 3. 契约草案（contract）

```go
type Operator struct { ID, UserID int64; RegionCode, Name string; Status int16; ... }

type Facade interface {
    OperatorOf(ctx context.Context, userID int64) (Operator, bool, error) // 移动端身份卡/后台
    AttributionOf(ctx context.Context, userID int64) (Attribution, bool, error)
    BindAttribution(ctx context.Context, userID int64, code string) error // 注册来源绑定（幂等）
    CommissionSum(ctx context.Context, operatorID int64, month string) (int64, error) // 业绩概览
    Attribute(ctx context.Context, order trade.Order) error // 内部：计佣
}
```

事件（`contracts/events/`，确认后新增 Schema）：

| 事件 | 触发 | 关键字段 |
| --- | --- | --- |
| `channel.OperatorStatusChanged.v1` | 开户/停用/恢复 | operator_id, from, to |
| `channel.CommissionAccrued.v1` | 计佣入账 | operator_id, user_id, order_id, amount_fen, month |
| `channel.SettlementPaid.v1` | 打款完成 | settlement_id, operator_id, month, amount_fen |

## 4. 与既有实现的复用点

| 需求 | 复用 |
| --- | --- |
| 幂等迁移 | `RunMigrations`（SQL 内容 crc32 版本；`IF NOT EXISTS`；ALTER 置后） |
| 金额/比例配置 | `settings.Registry` + `pg.Settings` 存储 + 后台 /admin/v1/settings 热生效 |
| 灰度/开关 | `featureflag`（如 `channel.commission` 灰度） |
| 审计 | `audit.Recorder`（资金类必记；`ratio_snapshot`/结算前后值入 detail） |
| 状态机 | `kernel/statemachine`（#16 运营商、#17 月结）+ `statemachine.Exhaustive` 全量校验 |
| 事件 | `eventbus` Publish/订阅（Outbox 至少一次 + 幂等 processed + trace 贯通） |
| 导出 | p4 §7.4 导出中心（对账/结算明细模板） |
| 后台菜单 | `module.AdminSection`（`channel.operators`/`channel.settlements`/`channel.attributions`） |
| 权限 | **依赖 #13**：五角色（超管/运营/审核员/财务/渠道经理）+ 权限点 `<module>:<resource>:<action>` |

## 5. 待决策（阻塞/影响实现，需在开工前书面确认）

| # | 事项 | 影响 | 默认建议（若无人拍板） |
| --- | --- | --- | --- |
| #13 | **后台五角色划分** | 全部后台权限点与菜单；`sys_admin_user/role` 表结构 | 超管/运营/审核员/财务/渠道经理五角色 + 权限点矩阵（p4 §3.6） |
| #16 | 佣金计佣范围 | 首单/续费是否均计、是否含立方订阅 | 首单 30%、续费 10%；含会员订阅、**不含**立方分摊与保证金 |
| — | 代理费与保证金入账口径 | 是否入收入、发票 | 代理费入收入（首年 3 万/次年 2 万）；保证金作预收台账可退（`chn_guarantee`），**不与收入混记** |
| — | 结算打款方式 | 打款凭证留存方式 | 对公打款 + 凭证上传（`voucher_ref` 指向 media 文件） |
| — | 渠道经理数据范围 | 仅本人区域 | 门店/区域维度过滤（`space_id` + `region_code`） |

## 6. 建议实施顺序（确认后）

1. 迁移 + 状态机 #16/#17 + 契约/事件 Schema + 表驱动测试（半天）
2. 归属绑定（identity `UserRegistered` 消费）+ 移动端身份卡/业绩概览（1 天）
3. 计佣（OrderPaid 订阅 + 快照 + 冲正）+ 月结（复核≠执行）+ 对账导出（1.5 天）
4. RBAC（#13 落地）：五角色 + 权限点矩阵 + 后台登录二次验证 + 审计埋点（1.5 天）
5. governance（用户 360/代操作 10 项）+ 看板/导出中心（2 天）
6. verify 认证全类型与组织管理员（1.5 天）
7. admin-web 页面与 E2E 回归（2 天）

> 预研结论：**channel 的数据模型与既有内核（settings/审计/状态机/事件/导出）完全对齐，无新增内核能力需求**；
> 唯一硬前置是 #13（权限点与后台账号模型），其余可在确认当日起并行开发。
