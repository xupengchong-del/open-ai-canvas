#!/bin/sh
# 容器化 NAS 健康监控：随 docker compose 启动，断电后由 restart:unless-stopped 自动拉起。
#
# 慢查询由 PostgreSQL 自身的 log_min_duration_statement 记录到 postgres 容器日志，
# 本脚本不重复抓取，只在负载异常时把 postgres 慢查询尾部并入快照，便于事后定位。
#
# 容器内 /proc/loadavg 与 /proc/meminfo 是内核全局值，直接反映宿主机（namespace 不隔离）；
# 进程列表用 docker top/stats 通过挂载的 docker.sock 获取（容器内 ps 看不到宿主进程）。

LOG_DIR=${MONITOR_LOG_DIR:-/var/log/monitor}
LOG="$LOG_DIR/nas-health.log"
SNAP="$LOG_DIR/nas-health-snapshots.log"
INTERVAL=${MONITOR_INTERVAL:-60}
LOAD_THRESHOLD=${MONITOR_LOAD_THRESHOLD:-3.0}
MAX_LOG_LINES=${MONITOR_MAX_LOG_LINES:-20000}
MAX_SNAP_LINES=${MONITOR_MAX_SNAP_LINES:-50000}
DOCKER=${DOCKER_BIN:-docker}

mkdir -p "$LOG_DIR" 2>/dev/null

trim() {
  file=$1 limit=$2
  [ -f "$file" ] || return 0
  lines=$(wc -l < "$file" 2>/dev/null || echo 0)
  if [ "$lines" -gt "$limit" ]; then
    tail -n "$((limit / 2))" "$file" > "$file.tmp" 2>/dev/null && mv "$file.tmp" "$file"
  fi
}

echo "[$(date '+%F %T')] 容器化监控启动 interval=${INTERVAL}s load>${LOAD_THRESHOLD}" >> "$LOG"

while true; do
  now=$(date '+%F %T')
  read -r l1 l5 l15 _ < /proc/loadavg
  mem_avail_kb=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo 2>/dev/null)
  swap_used_kb=$(awk '/^SwapTotal:/{t=$2} /^SwapFree:/{print t-$2}' /proc/meminfo 2>/dev/null)
  mem_avail=$(( ${mem_avail_kb:-0} / 1024 ))
  swap_used=$(( ${swap_used_kb:-0} / 1024 ))
  unhealthy=$($DOCKER ps --format '{{.Status}}' 2>/dev/null | grep -c unhealth)
  unhealthy=${unhealthy:-0}

  echo "$now load=$l1/$l5/$l15 avail=${mem_avail}MB swap=${swap_used}MB unhealthy=$unhealthy" >> "$LOG"

  over=$(awk -v l="$l1" -v lt="$LOAD_THRESHOLD" 'BEGIN { print (l+0 > lt+0) ? 1 : 0 }')
  if [ "$over" = "1" ] || [ "$unhealthy" -gt 0 ]; then
    {
      echo "===== $now 异常快照 (load=$l1 avail=${mem_avail}MB swap=${swap_used}MB unhealthy=$unhealthy) ====="
      echo "--- 容器状态 ---"
      $DOCKER ps --format 'table {{.Names}}\t{{.Status}}' 2>/dev/null | head -12
      echo "--- 容器内存 Top ---"
      $DOCKER stats --no-stream --format '{{.Name}} {{.MemUsage}} {{.CPUPerc}}' 2>/dev/null | head -10
      echo "--- postgres 慢查询末尾(>1s) ---"
      $DOCKER logs --tail 200 open-ai-canvas-postgres-1 2>&1 | grep 'duration:' | tail -10 | cut -c1-240
      echo
    } >> "$SNAP"
  fi

  trim "$LOG" "$MAX_LOG_LINES"
  trim "$SNAP" "$MAX_SNAP_LINES"
  sleep "$INTERVAL"
done
