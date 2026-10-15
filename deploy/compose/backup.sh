#!/usr/bin/env bash
# PostgreSQL 每日全备（PITR 基线）+ 保留清理。与 db 的 WAL 归档（archive_command）配合：
#   全备 = 基线，WAL 归档 = 增量；恢复 = 还原最近全备 + 回放 WAL 至目标时间点。
set -euo pipefail

PGHOST="${PGHOST:-db}"
PGUSER="${PGUSER:-catcube}"
PGPORT="${PGPORT:-5432}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
WAL_DIR="${WAL_DIR:-/wal-archive}"
RETAIN_DAYS="${RETAIN_DAYS:-14}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-86400}" # 每日

export PGPASSWORD="${PGPASSWORD:-catcube}"

run_backup() {
  local stamp base
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  base="${BACKUP_DIR}/base-${stamp}"
  echo "[backup] 开始全备 → ${base}"
  pg_basebackup -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -D "${base}" -Ft -z -X none -P
  # 记录当前 WAL 位置（恢复起点），便于 PITR 定位
  psql -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" -tAc \
    "SELECT pg_walfile_name(pg_current_wal_lsn())" > "${base}.wal-start" || true
  echo "[backup] 完成：${base}"
  echo "[backup] 清理 ${RETAIN_DAYS} 天前的全备与 WAL"
  find "${BACKUP_DIR}" -maxdepth 1 -name 'base-*' -mtime "+${RETAIN_DAYS}" -exec rm -rf {} + || true
  find "${WAL_DIR}" -maxdepth 1 -type f -mtime "+${RETAIN_DAYS}" -delete || true
}

mkdir -p "${BACKUP_DIR}" "${WAL_DIR}"
while true; do
  if ! run_backup; then
    echo "[backup] 本次失败，${INTERVAL_SECONDS}s 后重试" >&2
  fi
  sleep "${INTERVAL_SECONDS}"
done
