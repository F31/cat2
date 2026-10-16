#!/usr/bin/env bash
# PITR 备份恢复演练（p6 §3.4）：全备(WAL 归档) → 模拟损坏 → 时间点恢复 → 校验 RPO/RTO。
# 用法：./deploy/scripts/pitr-drill.sh
# 依赖：docker；使用临时容器与目录，演练结束自动清理。
set -euo pipefail

WORK_DIR="${WORK_DIR:-/tmp/catcube-pitr-drill}"
PG_IMAGE="${PG_IMAGE:-postgres:16}"
NET="catcube-pitr-net"
PRIMARY="catcube-pitr-primary"
RESTORE="catcube-pitr-restore"
PGUSER=catcube
PGPASSWORD=catcube
PGDB=demo

cleanup() {
  docker rm -f "$PRIMARY" "$RESTORE" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "== 0. 准备环境 =="
cleanup
rm -rf "$WORK_DIR" && mkdir -p "$WORK_DIR"/wal "$WORK_DIR"/base
chmod -R 777 "$WORK_DIR"
docker network create "$NET" >/dev/null

psql_primary() { docker exec -e PGPASSWORD="$PGPASSWORD" -i "$PRIMARY" psql -U "$PGUSER" -d "$PGDB" -tAc "$1"; }

echo "== 1. 启动主库（开启 WAL 归档）=="
docker run -d --name "$PRIMARY" --network "$NET" \
  -e POSTGRES_USER=$PGUSER -e POSTGRES_PASSWORD=$PGPASSWORD -e POSTGRES_DB=$PGDB \
  -v "$WORK_DIR/wal":/wal \
  "$PG_IMAGE" \
  -c wal_level=replica -c archive_mode=on -c archive_timeout=5 \
  -c "archive_command=test ! -f /wal/%f && cp %p /wal/%f" >/dev/null

for _ in $(seq 1 60); do
  if docker exec "$PRIMARY" pg_isready -U $PGUSER >/dev/null 2>&1; then break; fi
  sleep 1
done
echo "   主库就绪"

echo "== 2. 写入基线数据（batch-1：100 行）并落盘 =="
psql_primary "CREATE TABLE t (id int primary key, note text);" >/dev/null
psql_primary "INSERT INTO t SELECT g, 'batch-1' FROM generate_series(1,100) g;" >/dev/null
psql_primary "SELECT pg_switch_wal();" >/dev/null
BATCH1_COUNT=$(psql_primary "SELECT count(*) FROM t;")

echo "== 3. 全备（pg_basebackup）=="
docker exec -e PGPASSWORD="$PGPASSWORD" "$PRIMARY" \
  pg_basebackup -h 127.0.0.1 -U $PGUSER -D /tmp/basebackup -Ft -z -X none -P >/dev/null
docker cp "$PRIMARY":/tmp/basebackup/base.tar.gz "$WORK_DIR/base/base.tar.gz" >/dev/null
echo "   全备完成：$(du -h "$WORK_DIR/base/base.tar.gz" | cut -f1)"

# 恢复目标时间点：batch-1 之后、batch-2 之前
sleep 2
TARGET_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')
sleep 2

echo "== 4. 写入 batch-2（100 行）后模拟故障（删主库）=="
psql_primary "INSERT INTO t SELECT g + 1000, 'batch-2' FROM generate_series(1,100) g;" >/dev/null
psql_primary "SELECT pg_switch_wal();" >/dev/null
sleep 1
docker rm -f "$PRIMARY" >/dev/null

echo "== 5. 时间点恢复（PITR 至 ${TARGET_TIME} UTC）=="
R0=$(date +%s)
docker run -d --name "$RESTORE" --network "$NET" \
  -e POSTGRES_USER=$PGUSER -e POSTGRES_PASSWORD=$PGPASSWORD -e POSTGRES_DB=$PGDB \
  -e TARGET_TIME="$TARGET_TIME" \
  -v "$WORK_DIR/wal":/wal -v "$WORK_DIR/base":/backup \
  --entrypoint bash "$PG_IMAGE" -c '
    set -e
    PGD=/var/lib/postgresql/data
    mkdir -p "$PGD" && chown -R postgres:postgres "$PGD"
    cd "$PGD" && tar -xzf /backup/base.tar.gz
    {
      echo "restore_command = '"'"'cp /wal/%f %p'"'"'"
      echo "recovery_target_time = '"'"'$TARGET_TIME'"'"'"
    } >> "$PGD/postgresql.auto.conf"
    touch "$PGD/recovery.signal"
    chown -R postgres:postgres "$PGD"
    chmod 700 "$PGD"
    exec gosu postgres postgres
  ' >/dev/null

OK=0
for _ in $(seq 1 90); do
  if docker exec "$RESTORE" pg_isready -U $PGUSER >/dev/null 2>&1; then OK=1; break; fi
  sleep 1
done
if [ "$OK" != 1 ]; then
  echo "   恢复容器启动失败，输出如下：" >&2
  docker logs "$RESTORE" 2>&1 | tail -20 >&2
  exit 1
fi
for _ in $(seq 1 90); do
  if [ "$(docker exec "$RESTORE" psql -U $PGUSER -d $PGDB -tAc 'SELECT pg_is_in_recovery()' 2>/dev/null || echo t)" = "f" ]; then break; fi
  sleep 1
done
R1=$(date +%s)

echo "== 6. 校验 =="
RESTORED=$(docker exec "$RESTORE" psql -U $PGUSER -d $PGDB -tAc "SELECT count(*) FROM t;")
echo "   恢复后行数: ${RESTORED}（基线 ${BATCH1_COUNT}；batch-2 应丢失 → 预期 ${BATCH1_COUNT}）"
echo "   RTO: $((R1-R0)) 秒"

if [ "$RESTORED" != "$BATCH1_COUNT" ]; then
  echo "   演练结论: FAIL（恢复行数与目标时间点不一致，PITR 未精确恢复）"
  exit 1
fi
echo "   演练结论: PASS（目标时间点前数据完整；RTO=$((R1-R0))s）"
