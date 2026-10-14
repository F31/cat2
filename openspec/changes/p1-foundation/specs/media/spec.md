# media 增量规格（p1-foundation）

## ADDED Requirements

### Requirement: 统一上传与校验
系统 SHALL 提供小程序预签名直传对象存储的上传通道；仅允许 JPG/JPEG/PNG 且 ≤5MB；封面图 SHALL 为 16:9 且最小 750×422（PRD 8.9.2）；不合规文件 SHALL 在上传完成校验时被拒绝。

#### Scenario: 超大小拒绝
- **WHEN** 上传 6MB 的 PNG
- **THEN** 校验失败并提示大小限制

### Requirement: 引用计数与孤儿清理
系统 SHALL 以 `Bind(ownerRef)`/`Unbind` 维护文件引用计数；实体关闭/删除事件 SHALL 触发引用回收；引用计数为 0 的孤儿文件 SHALL 由定时任务清理。

#### Scenario: 孤儿回收
- **WHEN** 绑定某图片的实体被关闭
- **THEN** 引用计数递减，归零后文件进入孤儿清理队列
