# trust 增量规格（p1-foundation，骨架）

## ADDED Requirements

### Requirement: Moderation 端口统一入口
任何业务代码 SHALL NOT 直接调用微信内容安全接口；所有内容检测 SHALL 经 `Moderation` 端口；文本 SHALL 同步机检（命中拦截并记录），图片/音视频 SHALL 异步机检（回调前内容保持「审核中」占位）。内容安全接口超时 SHALL 进「审核中」队列重试，不放行。

#### Scenario: 机检超时不放行
- **WHEN** 内容安全接口超时
- **THEN** 内容进入审核中队列等待重试，用户端不展示

### Requirement: 统一审核任务队列
系统 SHALL 以 `trs_review_task` 承载统一审核队列，来源包括机器命中、用户举报、认证材料、产品入库、高风险类目抽检；任务 SHALL 带风险等级与 SLA 截止时间，按 SLA 排序。

#### Scenario: 队列入队
- **WHEN** 机器检测命中一条内容
- **THEN** 生成 source=机器的审核任务并带 sla_due_at

### Requirement: ReviewSubjectProvider 开口
trust SHALL 提供 `ReviewSubjectProvider` 注册表：由内容持有方注册「如何取对象文字/图片、审核结论如何落回」；新增一类需审核对象 SHALL NOT 要求修改 trust 代码。

#### Scenario: 新审核对象类型
- **WHEN** 某模块注册新的 ReviewSubjectProvider
- **THEN** trust 无需改动即可对该类型建审核任务并回写结论

### Requirement: 敏感词前置过滤
系统 SHALL 在提交时做本地敏感词/引流词前置过滤（含「加我微信/私下联系」类引流词）；词库 SHALL 后台可维护；命中先提示再记分。

#### Scenario: 引流词命中
- **WHEN** 发布内容含引流词
- **THEN** 用户收到提示，命中记录入库
