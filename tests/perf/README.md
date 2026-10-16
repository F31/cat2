# 压测与造数（p6 §2）

## 造数（§2.1）

```bash
CATCUBE_PERF_DATABASE_URL=postgres://catcube:catcube@127.0.0.1:15432/catcube_perf ./tests/perf/seed.sh
```

- 规模：用户/名片 1 万、内容（项目）1 万 + 搜索投影 1 万、圈子 1 千（成员 5/圈）、猫立方 100（成员 3/立方）。
- 幂等：固定 ID 段（`9e17+`）按主键冲突忽略，可重复执行。
- **须使用独立压测库**（规则：压测不污染业务库），压测后 `DROP DATABASE` 销毁。

## 场景（§2.2，k6 脚本接入）

| 场景 | 目标 | 说明 |
| --- | --- | --- |
| feed 聚合 | 首屏 ≤3s（P95） | `GET /api/mp/v1/feed`、`/aggregate/*` |
| 搜索 | 常规查询 ≤1s | `GET /api/mp/v1/search` |
| 详情聚合 | ≤1s | `GET /api/mp/v1/aggregate/detail` |
| 消息同步 | ≤1s | 会话列表 + 消息增量 |
| 下单支付（mock 渠道） | 成功率 ≥99.9% | `POST /orders` → `POST /pay` → 回调 |

## 指标验证（§2.3）

- 峰值并发百级、5~10 倍余量；首屏 P95 ≤3s；常规查询 P99 ≤1s。
- 事件积压压测（§2.4）：Outbox 高峰积压后投递延迟与恢复时间达标（指标见 `docs/ops/dashboards.md`）。
- 压测报告产出后归档 `tests/perf/reports/`。

## 执行（内置工具，无外部依赖）

```bash
# 造数（独立压测库）
CATCUBE_PERF_DATABASE_URL=postgres://catcube:catcube@127.0.0.1:15432/catcube_perf make perf-seed
# 压测（对运行中的实例；门槛不达标退出码非 0）
make loadtest BASE=http://127.0.0.1:8080 TOKEN=$TOKEN REF=user:123 C=100 D=20s SCENARIOS=feed,search,detail
```

本地实测（并发 100）：feed P95 71ms、search P99 30ms、detail P99 777ms，错误率 0%，全部达标 →
报告见 `tests/perf/reports/2026-10-16-local.md`。

> 说明：正式验收需在 **S1 形态**（api×2 + PG 主从 + Redis + 容器化构建）按同样场景复跑并归档；
> 本仓库提供造数与压测工具、场景/指标口径与本地基线。
