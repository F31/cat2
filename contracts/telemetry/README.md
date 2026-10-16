# 小程序埋点契约（p4 §7.2）

一期只采集漏斗必需项（PRD 1.2.3：避免过度采集）。事件名与属性在此登记；`tlm_event` 独立前缀存储，不污染业务表。

| 事件 | 触发点 | 属性（props） | 用途 |
| --- | --- | --- | --- |
| `register` | 登录/建号成功 | `{channel}` | 注册漏斗 |
| `profile_complete` | 名片保存（第二层画像填全） | `{tier}` | 画像完成度 |
| `feed_click` | 首页/广场点击卡片 | `{ref,type,position}` | 匹配点击 |
| `search` | 搜索提交 | `{keyword,type,result_count}` | 搜索行为 |
| `detail_view` | 详情页打开 | `{ref,type}` | 详情浏览 |
| `contact_initiate` | 点击「联系Ta」 | `{ref,result}` | 联系发起（付费前置） |
| `member_quote` | 会员报价页打开 | `{level,years,amount_fen}` | 会员转化 |
| `order_paid` | 支付成功页 | `{order_id,biz_type,amount_fen}` | 支付转化 |

约束：
- 仅登记事件可入库（未登记直接丢弃并计数 `catcube_telemetry_rejected_total`）。
- props 仅白名单键；不含手机号/证件号/联系方式等敏感信息（服务端二次过滤）。
- 批量上报：`POST /api/mp/v1/telemetry`（≤50 条/次），失败不阻断业务。
