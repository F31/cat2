# p2-main-loop · 设计要点

## 范围映射

| 本阶段内容 | 权威来源 |
| --- | --- |
| 模块职责与契约 | 技术方案 v2.5 4.3 模块画布；详细设计第 3 章（3.7~3.13、3.17） |
| 数据模型 | 详细设计 4.2.8~4.2.14（prf/msg/trd/vfy/cnt/mbr/soc） |
| 主链路流程 | 详细设计 7.1（注册入驻）、7.2（发布审核）、7.3（匹配广场）、7.4（会员支付退款） |
| UI 与交互 | 详细设计 5.3~5.7、5.10、5.12、5.14 |
| API 清单 | 详细设计 6.2 |
| 权限矩阵关键格 | 详细设计 8.1 |

## 本阶段执行决策

1. **会员可见字段统一走 policy**：「联系方式可见、核心优势可见、发起单聊」三格权限在详情聚合（bff）与 content 的 `Get` 中共用同一 CapabilityProvider 输出，前端只用 `PolicyGate` 渲染，页面内不写「是否会员」判断。
2. **SearchDoc 版本防旧事件覆盖**：`prf_card.search_doc_version` 与 `cnt_item.search_doc_version` 随事件携带，discovery 投影时丢弃旧版本事件。
3. **实名核验失败不阻塞浏览**：实名只是发布/建圈/建立方/申请认证的门槛；未实名不影响浏览/搜索/详情/被检索/获得匹配（详细设计 5.4.3 三条负向声明），权限矩阵用例须覆盖。
4. **会员价格只有一个锚点**：年费价格唯一维护在 settings，多年价=年费×年限×折扣系数；订单 `price_snapshot` 存单价/年限/系数快照，退款计算只读快照不读现配置。
5. **msg 一期纯异步**：不引入 WebSocket；`Realtime` 适配器 Noop。未读数以 SQL（最大序号−游标）为准，`msg_session_member.unread_count` 冗余可重建。
6. **discovery 只依赖事件类型**：R5 规则——不 import 任何模块 contract 的 Facade，只订阅事件；投影表可随时由事件回放重建（提供回放工具命令）。
7. **审核结论驱动状态机**：trust 不直接改 `cnt_item.status`；`ModerationPassed/Rejected`、`ReportConfirmed` 事件由 content 订阅后走自己的状态机 #2。
