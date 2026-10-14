# p1-foundation · 设计要点

本阶段不重复设计文档内容，只做范围映射与执行决策。

## 范围映射

| 本阶段内容 | 权威来源 |
| --- | --- |
| 模块边界、依赖阶梯、internal 机制 | 技术方案 v2.5 第 4、6 章；详细设计 2.3 |
| 共享体系 L0~L3、决策树 | 技术方案 v2.5 第 5 章；详细设计 2.5 |
| kernel 机制清单 | 技术方案 v2.5 5.3 |
| 工程目录 | 技术方案 v2.5 第 6 章；详细设计 2.4 |
| 数据约定（主键/金额/时间/通用列/迁移） | 详细设计 4.1 |
| 审计模型（哈希链、字段） | 详细设计 11.1、11.2 |
| S0 部署 | 技术方案 v2.5 11.2；详细设计 2.7 |

## 本阶段执行决策

1. **sqlc 先于业务表接入**：`tools/archcheck` 的表前缀校验在只有 `sys_/dct_/med_/idn_/trs_` 表时就必须在 CI 生效，后续模块的迁移自动被约束。
2. **事件契约先行**：`contracts/events/` 从 identity 的 `UserRegistered/UserDeactivated/AuthzChanged` 三个事件开始建立 JSON Schema 与 CI 校验，后续模块照此办理。
3. **entityref 轻量包随 dict/media 一并交付**：无独立表，只含 `EntityRef{type,id}` 类型与 `EntityResolver` 注册表；本阶段无注册者，接口与单测先行。
4. **identity 本阶段不含实名核验**：`realname_status` 字段落表，但二要素核验属 verify（p2）；`CapabilityProvider` 先返回「未实名」。
5. **trust 本阶段只到「同步文本机检 + 队列骨架」**：敏感词库种子、p2 再接举报与记分闭环。
6. **种子数据**：`contracts/seeds/` 的行业（两级）、区域、合作需求/供给、主体类型六类来自 p0 任务 4.1 的初稿，seed 命令幂等可重复执行。
