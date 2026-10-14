# p1-foundation · 地基

## Why

所有业务模块都建立在内核机制与共享业务域之上（《技术方案 v2.5》第 5、6 章）。本阶段先把「机制优于约定」的地基打牢：monorepo 工程结构、`internal` 编译期边界 + `archcheck` CI 校验、契约生成管线、pkg/kernel/ports/adapters 四层共享体系，以及 R0 骨架（identity、trust）与共享业务域（dict、media）。没有这些，后续 15 个业务模块无法按依赖阶梯装载，边界也无从强制。退出标准（第 13 章）：S0 一键起；空模块能按阶梯装载；边界检查在 CI 生效。

## What Changes

- monorepo 脚手架：`apps/server`（Go）、`apps/miniapp`（Taro+React+TS）、`apps/admin-web`（React+AntD）、`packages/*`（pnpm workspace）、`contracts/*`、`deploy/`、`tools/`、`tests/`、CODEOWNERS、Makefile（`make gen/lint/test/up`）
- CI 管线：`tools/archcheck`（依赖阶梯、sqlc 表前缀、infra 外禁数据库驱动、bff 只读）+ 契约一致性校验（OpenAPI、事件 JSON Schema）
- `pkg/` L0 纯工具库：errs、page（游标分页）、ids（int64 时间有序、JSON 字符串）、money（int64 分 + 舍入）、clock（UTC/Asia-Shanghai 自然日）、mask、retry、validate、phone
- `kernel/` L1 内核：模块装载（DependsOn 拓扑、缺依赖启动即失败）、config、settings（SettingsSchema、scope、版本与审计）、eventbus（Outbox 同事务、至少一次、订阅方幂等、死信重放）、scheduler（PG 咨询锁单活）、statemachine 引擎、policy 引擎（CapabilityProvider/PolicyRule/Actor 装配）、quota 引擎（QuotaDef、sys_quota_counter）、idempotency 中间件、scopedrepo/tx、sensitive（Visible[T]、AES-256-GCM、phone_hmac 盲索引）、audit（sys_audit 哈希链）、obs（OTel 带 module 标签）、migrate（拓扑执行、expand-migrate-contract）
- `ports/` 接口 + 一期适配器：pg（queue/search）、local（cache）、wechat（登录/pay 骨架）、oss（storage）、moderation（微信内容安全）；每端口带契约测试套件 `ports/*/suite`
- R0 模块骨架：`identity`（微信登录、令牌体系、authz_ver、注销发事件）与 `trust` 骨架（Moderation 端口接入、trs_review_task 统一队列、ReviewSubjectProvider 开口）
- 共享业务域：`dict`（taxonomy、主体类型、字段模板、TagSet 校验、种子导入）与 `media`（预签名上传、校验、缩略图、引用计数、孤儿清理）
- 权限矩阵与状态机表驱动测试框架（`tests/matrix`）；S0 部署（compose：app 全角色 + PostgreSQL + PITR 备份）

## Impact

- **capabilities**：platform-kernel、dict、media、identity、trust
- **代码**：整个仓库骨架；`apps/server/internal/{pkg→pkg 在根,kernel,ports,adapters,shared/{dict,media,entityref},modules/{identity,trust}}`；`tools/archcheck`；`deploy/compose`
- **数据**：`sys_` 内核表（settings/outbox/job/audit/quota_counter/feature_flag/migration）、`dct_`、`med_`、`idn_`、`trs_` 前缀首批迁移
- **契约**：`contracts/openapi/mp.yaml` 首个版本（auth/dict/media 路径）、`contracts/errors/` 错误码段分配、`contracts/seeds/` 字典种子
- **不做**：任何 R1+ 业务模块的业务行为；identity 只做登录/会话/注销骨架（实名核验在 p2 verify）；trust 只做机器检测链路与队列骨架（举报复核流在 p2）
