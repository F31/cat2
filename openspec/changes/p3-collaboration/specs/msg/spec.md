# msg 增量规格（p3-collaboration，群聊）

## ADDED Requirements

### Requirement: 群聊成员解析注册表
msg SHALL 通过 `SessionMemberResolver` 注册表确定群聊会话成员（由 cube 注册，成员=猫立方成员表）；msg SHALL NOT 认识猫立方的领域概念；成员变更事件 SHALL 触发群成员同步。

#### Scenario: 成员移出同步
- **WHEN** 某用户被移出猫立方
- **THEN** 其自动退出对应群聊会话

### Requirement: 群聊会话形态
猫立方群聊 SHALL 按「1 项目 1 群聊」建立；群聊名称 SHALL 为「立方名·模块名（或项目名）」；消息列表最后一条 SHALL 显示「发言人：内容摘要」；消息异步走 trust 内容检测。

#### Scenario: 群聊命名
- **WHEN** 在立方「华东合作组」的项目群发言
- **THEN** 消息列表显示「华东合作组·<项目名>」与发言人摘要
