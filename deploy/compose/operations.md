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
1. 负载摘除 api 实例（`readyz` 由负载停止引流；`docker compose stop api` 前先下线）。
2. 等 api 在途请求结束（graceful，`CATCUBE_HTTP_ADDR` 服务端超时内）。
3. `docker compose stop worker`：worker/scheduler 处理完当前批次；多副本 Job 由 PG 咨询锁保证单活，重启后自动接管。
4. 最后停 db/redis（`stop` 而非 `down -v`，保留卷）。

## 多副本单活
`scheduler` 角色在 api/worker 任一进程启动；多副本下通过 PG 咨询锁（`scheduler.NewRunner`）保证同一 Job 仅一个实例执行，无重复投递。

## 缓存
`CATCUBE_REDIS_ADDR` 配置后为 Redis 主缓存；连接失败自动回退进程内缓存（`adapters/redis.Fallback`，日志告警），业务不中断，Redis 恢复后自动切回。
