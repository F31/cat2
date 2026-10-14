# trade 增量规格（p2-main-loop）

## ADDED Requirements

### Requirement: 订单与支付幂等
系统 SHALL 以 `trd_order(biz_type,biz_ref,amount_fen,price_snapshot,status)` + `trd_payment`（一单多笔，out_trade_no 唯一）建模；创建/支付类接口 SHALL 强制 `Idempotency-Key`；微信支付回调 SHALL 验签并幂等处理重复/乱序通知；验签失败 SHALL 记审计（防重放）。

#### Scenario: 重复回调
- **WHEN** 同一 out_trade_no 的支付成功回调到达两次
- **THEN** 订单只置一次「已支付」，`OrderPaid` 事件只发布一次

### Requirement: 订单履约最终一致
订单支付成功后 SHALL 发布 `OrderPaid`，由 `OrderFulfiller` 注册方按 biz_type 履约（幂等，以订单 ID 去重）；履约失败 SHALL 自动重试，超阈值告警并支持后台手动补偿——「付了钱没开通」不得成为沉默失败。

#### Scenario: 履约重试
- **WHEN** 履约方处理失败
- **THEN** 系统按退避重试，超阈值产生告警并出现在后台补偿入口

### Requirement: 退款与对账
退款 SHALL 只由后台财务发起（用户端无入口），原路退回，幂等键=支付单号+原因；系统 SHALL 每日对账（`trd_recon`），差异记录并告警。

#### Scenario: 用户端无退款入口
- **WHEN** 用户在小程序查找退款功能
- **THEN** 不存在该入口；退款仅出现在后台财务角色界面
