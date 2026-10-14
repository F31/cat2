# OpenSpec 工作约定

本仓库使用 OpenSpec 管理开发任务。规则如下。

## 目录结构

- `openspec/project.md`：项目上下文（技术栈、架构约定、红线），所有变更必须遵守
- `openspec/changes/<change-id>/`：进行中的变更提案，每个包含：
  - `proposal.md`：`## Why` / `## What Changes` / `## Impact`（影响的 capabilities 与代码）
  - `tasks.md`：分组编号清单，`## 1. 组名` + `- [ ] 1.1 任务`，任务须具体可验证（引用真实目录、表名、事件名、API 路径）
  - `design.md`（可选）：有非显而易见的技术决策时编写，否则引用 `docs/` 下的设计文档章节
  - `specs/<capability>/spec.md`：增量规格，`## ADDED Requirements` / `## MODIFIED Requirements`，每个 `### Requirement:` 含 SHALL 语句与至少一个 `#### Scenario:`（WHEN/THEN）
- `openspec/specs/`：已落地能力的当前规格（一期从空开始，变更归档后合并进来）

## 变更划分

一期按实施路线分为 7 个变更（与《技术方案 v2.5》第 13 章、《详细设计 v1.4》9.2 节一致）：

| change-id | 阶段 | 退出标准 |
| --- | --- | --- |
| `p0-initiation` | 立项准备 | 外部事项均有负责人与时间表 |
| `p1-foundation` | 地基 | S0 一键起；边界检查在 CI 生效 |
| `p2-main-loop` | 主链路 | 「找对人 + 联系上 + 付费开通」闭环跑通 |
| `p3-collaboration` | 协作 | 支付幂等与并发成团测试通过；状态机测试通过 |
| `p4-channel-admin` | 渠道与后台 | 对账结算闭环 |
| `p5-ai-scenarios` | AI 一期场景 | 关闭开关全链路无影响；采纳率与成本可观测 |
| `p6-hardening-release` | 联调上线 | 达到 PRD 11.4 验收标准 |

阶段间按依赖阶梯自底向上，阶梯内并行；分工按模块 owner 三条泳道（A 身份与内容 / B 交易与协作 / C 触达与发现）。

## 写作规范

- 正文用中文；OpenSpec 关键字（SHALL / WHEN / THEN / ADDED / MODIFIED）保持英文
- Requirement 与 Scenario 必须可测试，不写「提升性能」这类不可验证表述
- tasks.md 中完成一项勾一项；任务要落到具体产物（文件、表、接口、测试）
- 涉及边界、契约、数据模型的决策须在 `docs/adr/` 留 ADR

## 阻塞项

《详细设计 v1.4》9.3 节的待确认事项中，#13（后台五角色划分）为阶段 4 的立项阻塞项；其余待确认项在 `p0-initiation` 中闭环或显式记录决策。
