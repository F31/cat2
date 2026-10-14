# miniapp 增量规格（p2-main-loop）

## ADDED Requirements

### Requirement: 五 Tab 与分包
小程序 SHALL 以 5 个 Tab 组织（广场/圈子/猫立方/消息/我的）；主包 SHALL 只含 Tab 壳 + 广场 + 消息，其余功能分包按需加载，首屏 ≤3 秒。

#### Scenario: 首屏预算
- **WHEN** 冷启动进入广场
- **THEN** 主包加载 + 单次聚合接口返回（卡片流+画像进度+未读数），首屏 ≤3 秒

### Requirement: 完善资料提示条
提示条 SHALL 在第二层画像五项任一为空时显示（未登录不显示）；点击或向上滑动 SHALL 进入与身份类型对应的名片编辑页；随页面滚动不悬浮；不引导实名认证；统一称「完善资料」；不提供「X 关闭」。

#### Scenario: 负向声明
- **WHEN** 渲染提示条
- **THEN** 文案与跳转均不涉及实名认证，且无可关闭按钮

### Requirement: PolicyGate 受限能力
受限能力（联系方式、核心优势、联系Ta 等）SHALL 一律经 PolicyGate 组件：自动灰显并展示后端 `Decision.Reason` 与解锁动作；页面内 SHALL NOT 重复写「是否会员」判断。

#### Scenario: 非会员点击联系Ta
- **WHEN** 非会员在详情页点击「联系Ta」
- **THEN** 弹出引导开通会员弹窗，不展示联系方式

### Requirement: 空态与提交校验
各空态 SHALL 按规范文案与去向按钮（详细设计 5.14）；加载用骨架屏不用全屏遮罩；提交级校验 SHALL 定位第一个未填必填项并提示字段名；重复提交按钮置灰 3 秒；敏感词前端不拦截。

#### Scenario: 必填缺失
- **WHEN** 发布表单缺少必填项提交
- **THEN** 页面定位到第一个未填项并提示字段名
