# social 增量规格（p2-main-loop）

## ADDED Requirements

### Requirement: 关注与收藏
系统 SHALL 提供关注/取关与收藏/取消，目标统一为 `EntityRef{type,id}`；关注关系以 `(follower_id, followee_ref)` 唯一；收藏 SHALL 按六类内容分 Tab 查询；不做排行榜与粉丝增长提醒。

#### Scenario: 重复关注幂等
- **WHEN** 对同一目标重复关注
- **THEN** 不产生第二条关系，返回既有状态

### Requirement: 关注即订阅
被关注者发布新内容时，系统 SHALL 向关注者推送一条系统通知并累计未读数（供 notice 消费 `Followed`/内容事件）。

#### Scenario: 关注后收更新
- **WHEN** 被关注者发布新内容
- **THEN** 关注者消息列表出现一条系统通知且未读数 +1
