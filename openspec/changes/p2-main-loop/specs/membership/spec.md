# membership 增量规格（p2-main-loop）

## ADDED Requirements

### Requirement: 会员定价单一锚点
系统 SHALL 只维护一个年费价格锚点（settings 可配）；多年价=年费×年限×折扣系数（3 年 0.9、5 年 0.8，后台可配）；报价 SHALL 同时展示原价与优惠价；订单 SHALL 保存 price_snapshot（单价/年限/系数）。

#### Scenario: 三年报价
- **WHEN** 请求 VIP 三年报价（年费 P）
- **THEN** 返回原价 3P 与优惠价 3P×0.9，均来源于配置

### Requirement: 开通与续期履约
系统 SHALL 订阅 `OrderPaid` 经 `OrderFulfiller(biz_type=member)` 开通/续期会员（幂等）；`MemberActivated` SHALL 触发 identity `authz_ver` 递增与 discovery 权重刷新；支付前页面 SHALL 明示「开通后不支持退款」。

#### Scenario: 开通即刻生效
- **WHEN** 支付回调成功
- **THEN** 会员生效，旧令牌因 authz_ver 递增而失效，联系方式/核心优势/单聊能力解锁

### Requirement: 退款金额计算
退款金额 SHALL = 剩余完整年数 × 年费 × 实付折扣系数（取订单价格快照，不读现配置）；系统重复支付 SHALL 保留一单、多扣全额原路退回；支付成功即生效不支持用户主动退款。

#### Scenario: 按比例退款
- **WHEN** 三年单（0.9 系数）使用满一年后因平台原因退款
- **THEN** 退还 2 × 年费 × 0.9

### Requirement: 企业版与子账号
企业版 SHALL 仅已认证企业可开通；主账号=企业账号本身；3 个子账号按手机号邀请（须已注册已实名），共享权益不单独计费；到期未续费主+子 SHALL 同步回落普通，子账号归属保留 30 天。

#### Scenario: 子账号资格
- **WHEN** 邀请一个未注册手机号作为子账号
- **THEN** 邀请被拒绝并提示对方先注册并实名
