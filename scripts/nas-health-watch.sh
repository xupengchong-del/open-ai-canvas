#!/bin/sh
# NAS 健康监控：定期采样资源指标，在负载异常时抓快照，用于定位「有时管理页很慢」的原因。
#
# 设计意图：慢是间歇性的，事后查不到现场。本脚本常驻采样，只在超过阈值时记录详情
# （Top 内存/CPU 进程、D 状态进程、各进程实际磁盘 I/O），避免日志膨胀。
#
# 部署与查看见 scripts/nas-health-watch-deploy.sh。

LOG_DIR=${LOG_DIR:-/volume1/docker/open-ai-canvas-data/ops}
LOG="$LOG_DIR/nas-health.log"
SNAP="$LOG_DIR/nas-health-snapshots.log"
INTERVAL=${INTERVAL:-60}
LOAD_THRESHOLD=${LOAD_THRESHOLD:-3.0}
SWAP_THRESHOLD_MB=${SWAP_THRESHOLD_MB:-500}
MAX_LOG_LINES=${MAX_LOG_LINES:-20000}
MAX_SNAP_LINES=${MAX_SNAP_LINES:-50000}

mkdir -p "$LOG_DIR" 2>/dev/null

trim() {
  file=$1 limit=$2
  [ -f "$file" ] || return 0
  lines=$(wc -l < "$file" 2>/dev/null || echo 0)
  if [ "$lines" -gt "$limit" ]; then
    tail -n "$((limit / 2))" "$file" > "$file.tmp" 2>/dev/null && mv "$file.tmp" "$file"
  fi
}

# 采样一次各进程的实际磁盘读写增量（MB/s），用于区分「谁在读写盘」。
io_snapshot() {
  for d in /proc/[0-9]*; do
    pid=${d#/proc/}
    [ -r "$d/io" ] || continue
    rb=$(awk '/^read_bytes/{print $2}' "$d/io" 2>/dev/null)
    wb=$(awk '/^write_bytes/{print $2}' "$d/io" 2>/dev/null)
    [ -n "$rb" ] || continue
    echo "$pid $rb $wb"
  done
}

echo "[$(date '+%F %T')] 监控启动 interval=${INTERVAL}s load>${LOAD_THRESHOLD} swap>${SWAP_THRESHOLD_MB}MB" >> "$LOG"

while true; do
  now=$(date '+%F %T')
  read -r l1 l5 l15 _ < /proc/loadavg
  swap_used=$(free -m | awk '/^Swap:/{print $3}')
  mem_avail=$(free -m | awk '/^Mem:/{print $7}')
  dcount=$(ps aux 2>/dev/null | awk '$8 ~ /^D/ {c++} END {print c+0}')

  echo "$now load=$l1/$l5/$l15 swap=${swap_used}MB avail=${mem_avail}MB D=$dcount" >> "$LOG"

  over=$(awk -v l="$l1" -v s="$swap_used" -v lt="$LOAD_THRESHOLD" -v st="$SWAP_THRESHOLD_MB" \
    'BEGIN { print (l+0 > lt+0 || s+0 > st+0) ? 1 : 0 }')
  if [ "$over" = "1" ]; then
    {
      echo "===== $now 异常快照（load=$l1 swap=${swap_used}MB D=$dcount）====="
      echo "--- 内存 Top 6 ---"
      ps aux 2>/dev/null | sort -k4 -rn | head -6 | awk '{printf "  %-12s RSS=%6.0fMB %s\n", $1, $6/1024, substr($0, index($0,$11), 50)}'
      echo "--- CPU Top 6 ---"
      ps aux 2>/dev/null | sort -k3 -rn | head -6 | awk '{printf "  %-12s CPU=%s%% %s\n", $1, $3, substr($0, index($0,$11), 50)}'
      echo "--- D 状态进程 ---"
      ps aux 2>/dev/null | awk '$8 ~ /^D/ {printf "  pid=%s stat=%s %s\n", $2, $8, substr($0, index($0,$11), 50)}' | head -8
      echo "--- 磁盘 I/O 前 5（5 秒采样）---"
      io_snapshot > /tmp/.io1.$$
      sleep 5
      io_snapshot > /tmp/.io2.$$
      awk 'NR==FNR {a[$1]=$2" "$3; next}
           { if ($1 in a) { split(a[$1], b, " "); r=($2-b[1])/5/1048576; w=($3-b[2])/5/1048576
               if (r>0.1 || w>0.1) printf "%s %.2f %.2f\n", $1, r, w } }' /tmp/.io1.$$ /tmp/.io2.$$ \
        | sort -k3 -rn | head -5 | while read -r pid r w; do
            cmd=$(tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | cut -c1-50)
            [ -z "$cmd" ] && cmd="[$(cat /proc/$pid/comm 2>/dev/null)]"
            printf "  pid=%-7s 读%5.2f 写%5.2f MB/s %s\n" "$pid" "$r" "$w" "$cmd"
          done
      rm -f /tmp/.io1.$$ /tmp/.io2.$$
      echo
    } >> "$SNAP"
  fi

  trim "$LOG" "$MAX_LOG_LINES"
  trim "$SNAP" "$MAX_SNAP_LINES"
  sleep "$INTERVAL"
done
