# discovery 增量规格（p2-main-loop）

## ADDED Requirements

### Requirement: 事件投影读模型
discovery SHALL 只订阅事件（ProfileUpdated/ContentPublished/ContentTakenDown/CertificationApproved/MemberActivated/Expired/Followed 等）自建 `dsc_search_doc` 与 `dsc_match_candidates` 读模型，SHALL NOT 读取其他模块的表；投影 SHALL 可由事件回放重建；SearchDoc SHALL NOT 携带手机号等敏感字段。

#### Scenario: 下架同步
- **WHEN** 收到 `ContentTakenDown` 事件
- **THEN** 对应搜索文档被删除或置灰，搜索结果不再出现

### Requirement: 匹配评分与排序
系统 SHALL 按「需求供给互补 40% + 行业 25% + 区域 20% + 身份互补 15%」计算匹配分，权重后台可配且带版本；同分 SHALL 按会员等级排序（企业版 > SVIP > VIP > 普通），不设推荐位或插队；再同分按近期活跃优先；推荐理由 SHALL 由规则模板生成，不依赖 LLM。

#### Scenario: 同分排序
- **WHEN** 两个候选匹配分相同
- **THEN** 会员等级高者在前；等级也相同则近期活跃者在前

### Requirement: 先给要后与候选预计算
系统 SHALL 在用户注册后按粗标签立即给出 3 条匹配结果；画像更新事件 SHALL 触发候选重算，Top-K 预计算写入 `dsc_match_candidates`，首页只读预计算结果以保证首屏 ≤3 秒。

#### Scenario: 注册即得匹配
- **WHEN** 新用户完成注册进入广场
- **THEN** 无需完善画像即可看到 3 条粗匹配卡片

### Requirement: 搜索独立可用
系统 SHALL 提供按名称检索六类内容（人/企业/项目/产品/活动/会议）的搜索（一期 PG 实现，经 Search 端口抽象）；标签筛选与关键词检索 SHALL 独立于匹配算法可用。

#### Scenario: 关键词搜索
- **WHEN** 输入产品名称关键词
- **THEN** 按内容类型分类返回结果；无结果显示「没有找到相关内容」
