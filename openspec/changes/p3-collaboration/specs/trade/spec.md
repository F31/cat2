# trade 增量规格（p3-collaboration，分摊）

## ADDED Requirements

### Requirement: 分摊成团强一致
认筹支付成功后 SHALL 在同一 trade 事务内锁定 `trd_apportion_group` 行并计数，达到 N 即置「已成团」并将订单置「已支付」、关闭认筹入口；该过程 SHALL NOT 跨模块，保证强一致；超额认筹 SHALL 从源头杜绝（达 N 关闭入口）。

#### Scenario: 并发认筹
- **WHEN** 两笔认筹并发支付使计数越过 N
- **THEN** 行锁保证只有 N 笔成功计入，超出笔走退款

### Requirement: 未成团终结
分摊截止未达 N 时，系统 SHALL 对全部已付认筹逐笔幂等全额原路退回，全部完成才终结（`ApportionFailed`）；截止前成员可退出并即时退款。

#### Scenario: 退款幂等
- **WHEN** 未成团退款任务重复执行
- **THEN** 每笔认筹只退一次（幂等键=支付单号+原因）
