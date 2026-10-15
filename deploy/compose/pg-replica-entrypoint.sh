#!/usr/bin/env bash
# 从库初始化：空数据目录时从主库 pg_basebackup（-R 写入 primary_conninfo 与 standby.signal），
# 否则直接启动（重启用已有数据）。之后交给官方 entrypoint 起 postgres。
set -e

PRIMARY_HOST="${PRIMARY_HOST:-db}"
PRIMARY_PORT="${PRIMARY_PORT:-5432}"
REPL_USER="${REPL_USER:-replicator}"
REPL_PASSWORD="${REPL_PASSWORD:-replicator}"
PGDATA="${PGDATA:-/var/lib/postgresql/data}"

if [ ! -s "${PGDATA}/PG_VERSION" ]; then
  echo "[replica] 等待主库可用: ${PRIMARY_HOST}:${PRIMARY_PORT}"
  until gosu postgres pg_isready -h "${PRIMARY_HOST}" -p "${PRIMARY_PORT}" -U "${REPL_USER}" >/dev/null 2>&1; do
    sleep 2
  done
  echo "[replica] pg_basebackup 全量克隆"
  rm -rf "${PGDATA:?}"/*
  PGPASSWORD="${REPL_PASSWORD}" gosu postgres pg_basebackup \
    -h "${PRIMARY_HOST}" -p "${PRIMARY_PORT}" -U "${REPL_USER}" \
    -D "${PGDATA}" -Fp -Xs -R -P
  chmod 700 "${PGDATA}"
fi

exec docker-entrypoint.sh postgres \
  -c hot_standby=on \
  -c hot_standby_feedback=on
