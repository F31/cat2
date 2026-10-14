# identity 增量规格（p1-foundation）

## ADDED Requirements

### Requirement: 微信登录注册一体化
系统 SHALL 以微信授权登录为主推方式（手机号经微信获取），注册与登录一体化：已注册直接登录，未注册进入绑定手机号与身份类型选择；手机号 SHALL 以 phone_enc + phone_hmac 存储并支撑「是否已注册」唯一校验；`session_key` SHALL 只在服务端短期使用，不下发、不入日志。

#### Scenario: 未注册动线
- **WHEN** 微信授权后手机号未注册
- **THEN** 进入注册流程（绑定手机号 → 选择身份类型：个人/组织/跳过按个人处理）

#### Scenario: 未填手机号阻断
- **WHEN** 用户未完成手机号绑定
- **THEN** 不能进入主流程（PRD 11.4）

### Requirement: 令牌与会话
系统 SHALL 签发 Access Token（2 小时，RS256，只含 `sub/sid/authz_ver`）与 Refresh Token（7 天滚动轮换、哈希存储、可吊销）；同一账号 SHALL 最多 3 个活跃会话；连续登录失败 SHALL 触发账号锁定。

#### Scenario: 会话上限
- **WHEN** 第 4 个设备登录同账号
- **THEN** 最早会话被顶替或登录被拒绝（按配置），并记录

### Requirement: authz_ver 无陈旧窗口
会员到期、被移出猫立方、封禁等权限变更事件 SHALL 递增用户 `authz_ver`；网关 SHALL 比对令牌中的 authz_ver，不一致即令牌失效，无陈旧窗口。

#### Scenario: 封禁即刻生效
- **WHEN** 用户被封禁
- **THEN** 其既有 Access Token 在下一次请求即失效

### Requirement: 注销与匿名化
系统 SHALL 提供账号注销入口；注销 SHALL 发布 `identity.UserDeactivated` 事件，各模块订阅后自行匿名化本模块数据（手机号/证件号/头像等）；群聊与项目记录因他人协作保留，发送人显示「已注销用户」。

#### Scenario: 注销后数据归属
- **WHEN** 用户注销
- **THEN** identity 匿名化自身数据，且 identity 不触碰其他模块的表
