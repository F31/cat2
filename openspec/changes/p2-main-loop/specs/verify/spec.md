# verify 增量规格（p2-main-loop，实名部分）

## ADDED Requirements

### Requirement: 实名认证
系统 SHALL 提供实名认证：真实姓名+身份证号+协议勾选，经第三方二要素接口实时核验，无需上传证件照；核验通过 SHALL 回写 `realname_status` 并授予「实名」Badge；核验失败 SHALL 允许重新提交。

#### Scenario: 实时核验通过
- **WHEN** 用户提交真实姓名与身份证号且二要素一致
- **THEN** 实时返回通过，状态机 #1 转「已实名」

### Requirement: 实名门槛不影响浏览
实名 SHALL 仅作为发布、建圈、建猫立方、申请认证的门槛；未实名 SHALL NOT 影响浏览、搜索、详情、被检索与获得匹配。

#### Scenario: 未实名浏览
- **WHEN** 未实名用户访问广场与详情页
- **THEN** 功能正常；仅在其发起发布时被拦截并提示
