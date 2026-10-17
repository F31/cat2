# ADR-004：PRD 11.4 圈子批量处置与分享物料口径

- 状态：采纳（2026-10-17）
- 依据：`docs/猫平方-产品需求说明书-v2.5（对外交付版）.docx` §5.12.2/§5.12.3/§5.12.6/§6.5/§11.4
- 关联：`p4-channel-admin`、`p6-hardening-release` §8.1

## 决策

### 1. 批量处置（仅圈子）
- PRD §6.5 仅「圈子管理」列 `批量封禁 / 批量归档`；猫立方保持单条封禁/解散。
- 圈子状态扩展：`0 进行中 / 1 已结束 / 2 已解散 / 3 已封禁 / 4 已归档`；新增 `prev_status` 记录处置前状态。
- 批量封禁：前台隐藏（广场/搜索/详情）、禁止加入/发布，可解封；批量归档：后台默认隐藏、只读，可恢复；解散仍为永久删除。
- 接口：`POST /api/admin/v1/circles/batch`，`{action: ban|archive|unban|restore, ids[], reason}`；上限 100、必填原因、逐圈审计、部分失败逐条返回。
- 解封/恢复优先回到 `prev_status`（已结束→已结束），避免丢失时效状态。

### 2. 圈名片（Canvas 渲染）
- 小程序 Canvas 负责排版与保存；服务端 `GET /api/mp/v1/circles/{id}/card` 提供圈名/简介/成员数/圈主/邀请码与小程序码 PNG（base64）。
- 小程序码：`wxacode.getUnlimited`，`page=pages/circle/index`，`scene=c=<邀请码>`；开发态返回占位图，生产需 appid/secret 与域名白名单。
- 图片固定 750×1000；必显五项；保存相册需用户授权。
- 邀请重置：`POST /api/mp/v1/circles/{id}/invite/reset`；旧码立即失效。

### 3. 分享落地页
- 圈链接/二维码/名片 → 圈子详情落地页：非成员（含未登录）仅「关于本圈」+ 申请加入。
- 圈内挂载内容分享 → 内容详情带 `from_circle`；服务端校验内容确属该圈（`cir_mount`，防伪造），返回 `from_circle{name,status,joined,can_join}`。
- 非成员看摘要（`preview_only=true`，不返回字段）；加入后看全文；已结束 `can_join=false`；成员显示「进入圈子」。

## 后果
- 圈子新增状态需在所有前台查询排除 3/4；到期任务只处理 status=0。
- 服务端不引入图像/字体依赖；名片版式变更需小程序发版。
- 真机保存/扫码/分享入口依赖小程序发版与微信 appid 配置。
