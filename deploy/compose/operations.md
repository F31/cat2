# S1 运维要点（编排 docker-compose.s1.yml）

## 三层配置
| 层 | 载体 | 变更方式 | 示例 |
| --- | --- | --- | --- |
| 启动配置 | 环境变量（`CATCUBE_*`） | 重启生效 | `CATCUBE_ROLES`、`CATCUBE_REDIS_ADDR` |
| 业务配置 | `sys_settings` 表 | 后台改，热生效 | 限额、开关阈值 |
| 特性开关 | `sys_feature_flag` 表 | 后台改，热生效（灰度/回滚） | 新功能按用户/比例放量 |

## 健康检查
- `GET /healthz`：进程存活（负载存活探针）。
- `GET /readyz`：DB 可连且启动就绪（负载就绪探针；未就绪返回 503，不接流量）。

## 停机（先摘流量，再排空）
> 应用侧已实现优雅停机（`CATCUBE_DRAIN_SECONDS`，默认 5s）：收到 SIGTERM → `/readyz` 立即置 503（负载摘流量）→ 等待排空窗口 → `Shutdown` 等在途请求结束 → 退出。演练脚本：`deploy/scripts/drain-drill.sh`（实测：129 请求 0 个 5xx、观察到 80 次 503、5s 内优雅退出）。

1. 负载摘除 api 实例（`readyz` 由负载停止引流；`docker compose stop api` 前先下线）。
2. 等 api 在途请求结束（graceful，`CATCUBE_HTTP_ADDR` 服务端超时内）。
3. `docker compose stop worker`：worker/scheduler 处理完当前批次；多副本 Job 由 PG 咨询锁保证单活，重启后自动接管。
4. 最后停 db/redis（`stop` 而非 `down -v`，保留卷）。

## 多副本单活
`scheduler` 角色在 api/worker 任一进程启动；多副本下通过 PG 咨询锁（`scheduler.NewRunner`）保证同一 Job 仅一个实例执行，无重复投递。

## 缓存
`CATCUBE_REDIS_ADDR` 配置后为 Redis 主缓存；连接失败自动回退进程内缓存（`adapters/redis.Fallback`，日志告警），业务不中断，Redis 恢复后自动切回。

## 云 S1 启动配置与生产前置

### 必需环境变量（api / worker 共用）

| 变量 | 说明 |
| --- | --- |
| `CATCUBE_DATABASE_URL` | 托管 PostgreSQL 16 连接串（含 `sslmode=require`）；迁移账号需 DDL 权限 |
| `CATCUBE_HTTP_ADDR` | api 监听地址（容器内 `:8080`） |
| `CATCUBE_ROLES` | api 实例 `api,admin`；worker 实例 `worker,scheduler` |
| `CATCUBE_REDIS_ADDR` / `CATCUBE_REDIS_PASSWORD` | Redis 7（S1 推荐主从/哨兵）；未配置回退进程内缓存 |
| `CATCUBE_WECHAT_APPID` / `CATCUBE_WECHAT_SECRET` | 小程序登录、手机号、小程序码；需配置服务器域名白名单 |
| `CATCUBE_IDENTITY_ENC_KEY` / `CATCUBE_BLIND_KEY` | 32 字节 hex；必须由 KMS/密钥管理注入，丢失不可逆 |
| `CATCUBE_OSS_ENDPOINT` / `_BUCKET` / `CATCUBE_CDN_HOST` / `_AK` / `_SK` | 对象存储与 CDN；未配置时本地 FS 回退（仅开发） |
| `CATCUBE_MODERATION` | 留空=不机检；生产需接微信内容安全适配器（当前仅 dev/骨架） |
| `CATCUBE_OTEL_*` | Collector 地址/采样；默认关闭 |
| `CATCUBE_DRAIN_SECONDS` | 优雅停机排空窗口（默认 5s） |
| `CATCUBE_ALERT_*` / `CATCUBE_CONTACT_DAILY_LIMIT` | 告警阈值与联系方式日限频 |

> 生产必须留空 `CATCUBE_SMS_DEV_CODE`；当前短信仅开发固定码，生产短信网关适配器待接入。

### 生产前置（代码侧未闭环）

| 能力 | 现状 | 云 S1 前需要 |
| --- | --- | --- |
| 微信支付 | `internal/adapters/pay` 仅 DevClient | 接入微信支付 V3 预下单/回调验签/退款，配置商户号与证书 |
| 实名核验 | `internal/adapters/realname` 仅 DevClient | 第三方二要素供应商适配器与密钥 |
| 短信 | 仅 `CATCUBE_SMS_DEV_CODE` | 短信服务商适配器 + 模板报备 |
| 内容安全 | `moderation.Client` 文本已实现、媒体异步 TODO，且未装配 | 接入微信内容安全（文本/图片异步回调）并装配 |
| 对象存储 | `oss.Client` 预签名/签名 TODO，未接供应商 | 实现 OSS/S3 兼容签名（预签名 PUT、访问 URL、CDN） |
| 小程序码 | `wechat.MiniProgramCode` 已实现 | 生产 appid/secret、页面路径白名单（圈名片依赖） |
