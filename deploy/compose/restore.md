# PITR 恢复步骤（S0）

前提：`db` 开启了 WAL 归档（`archive_command` → `/wal-archive`），`backup` 边车每日全备到 `/backups`。

1. 停写：`docker compose -f deploy/compose/docker-compose.s0.yml stop app`
2. 取最近全备：`ls -t $(VOLUME)/pgbackups | head`（volume 实际路径见 `docker volume inspect`）
3. 还原基线：`tar -xzf base-<stamp>.tar.gz -C $PGDATA`（或新建数据目录）
4. 配置恢复目标（`postgresql.conf` / `recovery.signal`）：
   ```
   restore_command = 'cp /wal-archive/%f %p'
   recovery_target_time = '2026-10-16 12:00:00+08'
   ```
   创建 `recovery.signal` 后启动 `db`。
5. 回放完成后 `SELECT pg_is_in_recovery();` 应为 false；校验后取消恢复配置，恢复 `app`。

RPO ≤ 5 分钟（archive_timeout=60s + 每日全备）；RTO ≤ 1 小时（全档次序恢复演练目标，p6 演练确认）。
