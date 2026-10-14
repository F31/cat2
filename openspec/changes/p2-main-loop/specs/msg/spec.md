# msg 增量规格（p2-main-loop，单聊）

## ADDED Requirements

### Requirement: 单聊会话与增量同步
系统 SHALL 以 `(min_uid, max_uid)` 唯一约束单聊会话；详情页「联系Ta」发起的会话与消息列表 SHALL 是同一套会话；发起单聊 SHALL 是会员权益；消息同步 SHALL 走 cursor 增量拉取，前台 3 秒轮询、无新消息指数退避。

#### Scenario: 非会员联系Ta
- **WHEN** 非会员点击「联系Ta」
- **THEN** 不建会话，弹出引导开通会员弹窗（不展示联系方式）

### Requirement: 未读数口径
未读数 SHALL = 会话最大序号 − 已读游标（SQL 计算、幂等）；每会话显示未读条数，Tab 显示总数红点。

#### Scenario: 已读清零
- **WHEN** 用户提交 read cursor 到最新序号
- **THEN** 该会话未读数为 0，Tab 红点相应减少

### Requirement: 消息留存
聊天记录 SHALL 留存 ≥6 个月；`msg_message` 按月分区，超期分区压缩归档到对象存储并保留可检索元数据；非会话成员 SHALL NOT 能拉取消息（denied 记审计）。

#### Scenario: 越权拉取
- **WHEN** 非成员请求某会话消息
- **THEN** 拒绝并记审计
