#!/usr/bin/env bash
# 停机排空演练（p6 §3.2）：先摘流量（/readyz 503）→ 等在途请求完成 → 进程优雅退出。
# 校验：业务请求全程无 5xx；停机窗口内观察到 readyz 503；退出码 0 且耗时在窗口内。
# 用法：CATCUBE_DRILL_DATABASE_URL=postgres://... ./deploy/scripts/drain-drill.sh
set -euo pipefail

: "${CATCUBE_DRILL_DATABASE_URL:?需设置 CATCUBE_DRILL_DATABASE_URL（演练库）}"
PORT="${PORT:-18088}"
DRAIN_SECONDS="${DRAIN_SECONDS:-5}"
BIN="${BIN:-/tmp/catcube-drain-drill}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"; kill "${SRV_PID:-0}" 2>/dev/null || true' EXIT

echo "== 0. 构建二进制 =="
( cd "$(dirname "$0")/../../apps/server" && go build -o "$BIN" ./cmd/catcube )

echo "== 1. 启动服务（排空窗口 ${DRAIN_SECONDS}s）=="
CATCUBE_DATABASE_URL="$CATCUBE_DRILL_DATABASE_URL" \
CATCUBE_HTTP_ADDR=":$PORT" \
CATCUBE_ROLES="api,worker,scheduler,admin" \
CATCUBE_MODERATION=dev \
CATCUBE_DRAIN_SECONDS="$DRAIN_SECONDS" \
CATCUBE_SMS_DEV_CODE=123456 \
"$BIN" serve >"$WORK/server.log" 2>&1 &
SRV_PID=$!

for _ in $(seq 1 60); do
  if curl -sf --noproxy '*' -m 2 "http://127.0.0.1:$PORT/readyz" >/dev/null 2>&1; then break; fi
  sleep 1
done
echo "   就绪（pid=$SRV_PID）"

echo "== 2. 持续请求（业务只读接口），随后 SIGTERM =="
(
  fail5xx=0
  total=0
  while true; do
    code=$(curl -s -o /dev/null -w '%{http_code}' --noproxy '*' -m 5 "http://127.0.0.1:$PORT/api/mp/v1/search?type=content.project&q=项目" || echo 000)
    total=$((total+1))
    case "$code" in
      5*) fail5xx=$((fail5xx+1)); echo "5xx:$code" >> "$WORK/status.log" ;;
      *)  echo "$code" >> "$WORK/status.log" ;;
    esac
    # 记录可用性探针状态变化
    r=$(curl -s -o /dev/null -w '%{http_code}' --noproxy '*' -m 2 "http://127.0.0.1:$PORT/readyz" || echo 000)
    echo "$r" >> "$WORK/ready.log"
    if ! kill -0 "$SRV_PID" 2>/dev/null; then break; fi
  done
  echo "$total $fail5xx" > "$WORK/count.log"
) &
CLIENT_PID=$!

sleep 3
T0=$(date +%s)
kill -TERM "$SRV_PID"

set +e
wait "$SRV_PID"; EXIT_CODE=$?
set -e
T1=$(date +%s)
wait "$CLIENT_PID" 2>/dev/null || true

echo "== 3. 校验 =="
read -r TOTAL FAIL5XX < "$WORK/count.log"
READYZ_503=$(grep -c '^503$' "$WORK/ready.log" || true)
ELAPSED=$((T1-T0))
echo "   业务请求 $TOTAL 次；其中 5xx $FAIL5XX 次"
echo "   停机窗口中 /readyz 返回 503 次数：$READYZ_503"
echo "   进程退出码：$EXIT_CODE；退出耗时：${ELAPSED}s（排空窗口 ${DRAIN_SECONDS}s）"

STATUS=PASS
if [ "$FAIL5XX" != 0 ]; then echo "   ✗ 停机期间出现 5xx（在途请求未优雅完成）"; STATUS=FAIL; fi
if [ "$READYZ_503" -lt 1 ]; then echo "   ✗ 未观察到摘流量（/readyz 未置 503）"; STATUS=FAIL; fi
if [ "$EXIT_CODE" != 0 ]; then echo "   ✗ 非正常退出：$EXIT_CODE"; STATUS=FAIL; fi
if [ "$ELAPSED" -gt $((DRAIN_SECONDS+30)) ]; then echo "   ✗ 退出耗时超出窗口"; STATUS=FAIL; fi

if [ "$STATUS" = PASS ]; then
  echo "   演练结论: PASS（先摘流量 → 排空在途请求 → 优雅退出，无 5xx）"
else
  echo "   演练结论: FAIL"; exit 1
fi
