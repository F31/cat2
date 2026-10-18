# 广场后台管理（圈子 / 猫立方）

一级菜单「广场」下分两个管理域：圈子（线上开放沟通/合作讨论）与猫立方（线下组织协作/项目承接）。

## 圈子管理

| 功能 | 接口 | 说明 |
| --- | --- | --- |
| 列表 | `GET /admin/v1/circles` | 圈名/圈主/类型/成员数/区域/状态/创建时间 |
| 数据概览 | `GET /admin/v1/circles/stats?days=30` | 新增圈数（常设/临时）、活跃圈、成员总数/新增趋势、需求发布量与有效率 |
| 详情 | `GET /admin/v1/circles/{id}` | 关于本圈、成员、动态、需求、挂载 |
| 需求处置 | `POST /admin/v1/circles/{id}/demands/{did}/remove` | 软删（status=2），原因必填，写审计 |
| 动态处置 | `POST /admin/v1/circles/{id}/feeds/{fid}/remove` | 软删并同步 `feed_count` |
| 挂载处置 | `POST /admin/v1/circles/{id}/mounts/{mid}/remove` | 软删挂载 |
| 批量/封禁 | `POST /admin/v1/circles/batch`、`/{id}/dissolve` | 批量封禁/归档/解封/恢复；解散 |

复用：`cir_circle / cir_member / cir_demand / cir_feed / cir_mount`，不新增重复实体；所有处置写 `sys_audit`。

## 猫立方管理

| 功能 | 接口 | 说明 |
| --- | --- | --- |
| 列表 | `GET /admin/v1/cubes` | 名称/猫王/猫蜜/成员数/项目数/创建时间/状态 |
| 详情 | `GET /admin/v1/cubes/{id}` | 基本信息、画像字段、成员、项目 |
| 项目 | `GET /admin/v1/cubes/{id}/projects` | 研究/承接项目、进度、状态 |
| 调整猫王 | `POST /admin/v1/cubes/{id}/leader` | 复用小程序的转让服务 |
| 调整猫蜜 | `POST /admin/v1/cubes/{id}/honey` | 需为在职成员；原猫蜜转普通成员 |
| 移除成员 | `POST /admin/v1/cubes/{id}/members/{uid}/remove` | 原因必填；猫王/猫蜜需先调整 |
| 群聊合规 | `GET /admin/v1/cubes/{id}/chats` | 立方会话 + 各项目会话；经 msg Facade 读取，写审计 |
| 封禁/解散 | `POST /admin/v1/cubes/{id}/ban|dissolve` | 原因必填，写审计 |

复用：`cub_cube / cub_member / cub_project` 与 cube 服务；群聊通过 `msgcontract.Facade.AdminHistory`，不跨模块查表。

## 闭环

- 圈子：发现/加入 → 动态/需求/挂载 → 分享拉新 → 到猫立方办 → 项目承接；后台负责概览、详情、处置、封禁归档。
- 猫立方：创建/邀请 → 猫王猫蜜分工 → 项目/任务/报备/群聊 → 订阅分摊 → 发布回平台；后台负责成员调整、项目与群聊合规、封禁解散。
