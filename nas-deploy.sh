#!/usr/bin/env bash
# 影策 NAS 部署 / 更新命令（群晖 DS725+ / DSM 7.4 / Container Manager）
#
# 用法：
#   ./nas-deploy.sh                 更新 NAS 上的程序：拉代码 → 重建镜像 → 重启 → 健康检查
#   ./nas-deploy.sh deploy          同上；首次运行自动初始化（建目录、生成 .env、克隆仓库、签自签证书）
#   ./nas-deploy.sh sync-api        把本机开发库的模型渠道 / 密钥 / 管理员账号同步到 NAS
#   ./nas-deploy.sh pull-models     反向操作：把 NAS 上的模型配置拉回本机开发库（--dry-run 预览）
#   ./nas-deploy.sh status          容器状态与版本
#   ./nas-deploy.sh logs [服务名]   查看日志，默认 backend
#   ./nas-deploy.sh shell           登录 NAS shell
#
# 环境变量：FORCE_BUILD=1 强制重建镜像；BRANCH=… 换分支；PUBLIC_PORT=… 换对外端口。
# pull-models 的后台账号密码写在 .local/nas-admin.env（本机、已被 git 忽略），无需每次在命令行传。
# 连接信息来自 NAS_INFO_FILE（默认 ~/myProg/Nas726.xpc），支持两种写法：
#   NAS_HOST=… / NAS_USER=… / NAS_PASSWORD=… / NAS_SSH_PORT=…
#   IP=… / UserName=… / PWD=…
# 凭据只在 SSH 通道内使用，不回显、不落盘。
#
# 布局：
#   /volume1/docker/open-ai-canvas        git 工作副本（拉取用）
#   /volume1/docker/open-ai-canvas-data   数据卷 + TLS 编排（不受 git 覆盖影响）
#
# 构建发生在 NAS 本地（本机不需要 Docker）。NAS 无法直连 Docker Hub，
# 依赖 Container Manager 里配置的镜像加速。

set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAS_INFO_FILE="${NAS_INFO_FILE:-$HOME/myProg/Nas726.xpc}"

APP_DIR="${APP_DIR:-/volume1/docker/open-ai-canvas}"
DATA_DIR="${DATA_DIR:-/volume1/docker/open-ai-canvas-data}"
REPO_URL="${REPO_URL:-https://github.com/xupengchong-del/open-ai-canvas.git}"
BRANCH="${BRANCH:-local/daily}"
PUBLIC_PORT="${PUBLIC_PORT:-3050}"
INTERNAL_PORT="${INTERNAL_PORT:-3051}"
FORCE_BUILD="${FORCE_BUILD:-0}"
# NAS 只有 4 GB 内存：并行构建四个镜像会把整机压到无响应。设置 1 让 compose 逐个构建。
COMPOSE_PARALLEL_LIMIT="${COMPOSE_PARALLEL_LIMIT:-}"
# pull-models 的后台账号密码：优先环境变量，否则读 .local/nas-admin.env（本机、已被 git 忽略）。
NAS_ADMIN_USER="${NAS_ADMIN_USER:-}"
NAS_ADMIN_PASSWORD="${NAS_ADMIN_PASSWORD:-}"

bold() { printf '\n\033[1m==> %s\033[0m\n' "$1"; }
info() { printf '    %s\n' "$1"; }
warn() { printf '\033[33m    ! %s\033[0m\n' "$1"; }
die() { printf '\033[31m错误：%s\033[0m\n' "$1" >&2; exit 1; }

b64_encode() { base64 | tr -d '\n'; }

cred_value() {
  local key val
  for key in "$@"; do
    val="$(grep -iE "^[[:space:]]*${key}[[:space:]]*=" "$NAS_INFO_FILE" 2>/dev/null | head -1 | cut -d= -f2- | sed 's/[[:space:]]*$//')"
    [ -n "$val" ] && { printf '%s' "$val"; return 0; }
  done
  return 0
}

load_connection() {
  NAS_HOST="${NAS_HOST:-$(cred_value NAS_HOST IP Host)}"
  NAS_USER="${NAS_USER:-$(cred_value NAS_USER UserName User Username)}"
  NAS_PASSWORD="${NAS_PASSWORD:-$(cred_value NAS_PASSWORD PWD Password Pass)}"
  NAS_SSH_PORT="${NAS_SSH_PORT:-$(cred_value NAS_SSH_PORT SSH_PORT Port)}"
  NAS_SSH_PORT="${NAS_SSH_PORT:-22}"
  [ -n "$NAS_HOST" ] || die "没有拿到 NAS 地址：在 $NAS_INFO_FILE 里写 NAS_HOST=… 或 IP=…"
  [ -n "$NAS_USER" ] || die "没有拿到 NAS 用户名：写 NAS_USER=… 或 UserName=…"
  [ -n "$NAS_PASSWORD" ] || die "没有拿到 NAS 密码：写 NAS_PASSWORD=… 或 PWD=…"
  command -v sshpass >/dev/null 2>&1 || die "缺少 sshpass：brew install hudochenkov/sshpass/sshpass"
}

# 远程执行：第一行凭据，其余为脚本正文
remote() {
  # 两段式：先把脚本上传到 NAS，再用 sudo 以 root 执行整段。
  # 这样 sudo 只认证一次、不依赖时间戳，脚本内的管道与重定向 stdin 也不受影响。
  local script attempt rc
  script="$(cat)"
  if [ "${NAS_DEBUG:-0}" = "1" ]; then
    printf '%s
' "$script" | sed -E "s/'[A-Za-z0-9+/=]{40,}'/'<base64 数据已省略>'/g" >&2
  fi
  for attempt in 1 2 3 4; do
    rc=0
    { printf '%s
' "$NAS_PASSWORD"; printf '%s
' "$script"; } | sshpass -e ssh -p "$NAS_SSH_PORT" \
        -o PreferredAuthentications=password -o PubkeyAuthentication=no \
        -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 -o NumberOfPasswordPrompts=1 \
        "$NAS_USER@$NAS_HOST" 'read -r SUDO_PASSWORD; umask 077; cat > "$HOME/.canvas-job.sh"' >/dev/null 2>&1 || rc=$?
    if [ "$rc" -eq 0 ]; then
      printf '%s
' "$NAS_PASSWORD" | sshpass -e ssh -p "$NAS_SSH_PORT" \
          -o PreferredAuthentications=password -o PubkeyAuthentication=no \
          -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 -o NumberOfPasswordPrompts=1 \
          "$NAS_USER@$NAS_HOST" 'read -r SUDO_PASSWORD; printf "%s
" "$SUDO_PASSWORD" | sudo -S -p "" sh "$HOME/.canvas-job.sh"; rc=$?; rm -f "$HOME/.canvas-job.sh"; exit $rc' 2>&1 \
        | grep -v -e 'post-quantum' -e 'store now, decrypt later' -e 'may need to be upgraded'
      rc=${PIPESTATUS[0]}
      [ "$rc" -eq 0 ] && return 0
    fi
    if [ "$attempt" -lt 4 ]; then
      printf '    第 %s 次执行失败（rc=%s），5 秒后重试…
' "$attempt" "$rc" >&2
      sleep 5
    fi
  done
  return 1
}

prelude() {
  # 脚本整体以 root 执行，因此内部不需要再 sudo。
  cat <<PRELUDE
export PATH=/var/packages/Git/target/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/usr/syno/bin
APP='$APP_DIR'
DATA='$DATA_DIR'
OPS="\$DATA/ops"
DOCKER=/usr/local/bin/docker
d() { "\$@"; }
# 只在显式设置时生效，避免影响默认并行度。
COMPOSE_PARALLEL_LIMIT='$COMPOSE_PARALLEL_LIMIT'
[ -n "\$COMPOSE_PARALLEL_LIMIT" ] && export COMPOSE_PARALLEL_LIMIT
compose() { cd "\$APP" && "\$DOCKER" compose --env-file "\$APP/.env" -f "\$APP/docker-compose.deploy.yml" -f "\$APP/docker-compose.build.yml" -f "\$OPS/compose.tls.yml" "\$@"; }
PRELUDE
}

# ---------- 由本机生成、随部署上传的文件 ----------

env_content() {
  cat <<ENV
POSTGRES_DB=open_ai_canvas
POSTGRES_USER=open_ai_canvas
POSTGRES_PASSWORD=$1
DATABASE_URL=postgresql://open_ai_canvas:$1@postgres:5432/open_ai_canvas?sslmode=disable
CANVAS_IMAGE_TAG=source
CANVAS_BACKEND_IMAGE=open-ai-canvas-backend:server
CANVAS_WEB_IMAGE=open-ai-canvas-web:server
CANVAS_YINGCE_AGENT_IMAGE=open-ai-canvas-yingce-agent:server
YINGCE_AGENT_TOKEN=$2
CANVAS_UPDATER_TOKEN=$2
CANVAS_HTTP_PORT=127.0.0.1:$INTERNAL_PORT
CANVAS_CORS_ORIGINS=https://$NAS_HOST:$PUBLIC_PORT
CANVAS_PUBLIC_BASE_URL=https://$NAS_HOST:$PUBLIC_PORT
CANVAS_REGISTRATION_ENABLED=false
CANVAS_ALLOW_PRIVATE_UPSTREAMS=false
CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=
CANVAS_UPDATER_SOCKET_DIR=$DATA_DIR/updater
CANVAS_WORKER_CONCURRENCY=2
CANVAS_CHANNEL_CONCURRENCY=2
GOPROXY=https://goproxy.cn,direct
NPM_REGISTRY=https://registry.npmmirror.com
ENV
}

tls_compose() {
  cat <<YAML
# NAS 专用 TLS 前置层：自签证书终止 HTTPS，再转给 web 容器。
# 只对 SSE/长任务关闭缓冲，保持流式接口实时。
services:
  backend:
    environment:
      # 源码构建不走 Host Updater，清空 token 让后端不注册更新客户端。
      CANVAS_UPDATER_TOKEN: ""

  tls:
    image: nginx:1.27-alpine
    ports:
      - "$PUBLIC_PORT:3050"
    volumes:
      - $DATA_DIR/ops/nginx.conf:/etc/nginx/conf.d/default.conf:ro
      - $DATA_DIR/ops/certs:/etc/nginx/certs:ro
    depends_on:
      web:
        condition: service_started
    restart: unless-stopped
YAML
}

tls_nginx_conf() {
  cat <<NGINX
map \$http_upgrade \$connection_upgrade {
    default upgrade;
    ''      close;
}

# 容器重建会换 IP；固定走 Docker 内置 DNS 并让 nginx 自己按 TTL 重解析，
# 否则 nginx 只在启动时解析一次 web，web 换 IP 后会一直 502 到下次 reload。
resolver 127.0.0.11 valid=10s ipv6=off;

server {
    listen 3050 ssl;
    http2 on;
    server_name _;

    ssl_certificate     /etc/nginx/certs/server.crt;
    ssl_certificate_key /etc/nginx/certs/server.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_session_cache   shared:SSL:10m;

    # 用户在浏览器误敲 http:// 时，自动 301 重定向到 https://
    error_page 497 =301 https://\$host:3050\$request_uri;

    # 业务配置与素材上传上限与仓库内 nginx.conf 保持一致。
    client_max_body_size 1024m;

    add_header Referrer-Policy "no-referrer" always;

    location / {
        # 用变量形式触发运行时解析（写死域名时 nginx 只在启动时解析一次）
        set \$web_upstream http://web:3000;
        proxy_pass \$web_upstream;
        proxy_http_version 1.1;
        proxy_set_header Host              \$host;
        proxy_set_header X-Real-IP         \$remote_addr;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade           \$http_upgrade;
        proxy_set_header Connection        \$connection_upgrade;
        # 文本任务事件流依赖逐条下发，关闭缓冲与压缩。
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
NGINX
}

# ---------- 部署 / 更新 ----------

cmd_deploy() {
  bold "部署目标"
  info "NAS:  $NAS_USER@$NAS_HOST:$NAS_SSH_PORT"
  info "代码: ${APP_DIR}（分支 ${BRANCH}）"
  info "数据: $DATA_DIR"
  info "访问: https://$NAS_HOST:$PUBLIC_PORT"

  local env_b64 tls_compose_b64 tls_conf_b64
  env_b64="$(env_content "$(openssl rand -hex 24)" "$(openssl rand -hex 32)" | b64_encode)"
  tls_compose_b64="$(tls_compose | b64_encode)"
  tls_conf_b64="$(tls_nginx_conf | b64_encode)"

  remote <<SCRIPT || die "远程部署失败（远端日志见上）"
$(prelude)
set -e

echo '--- 目录 ---'
d mkdir -p "\$APP" "\$DATA/ops/certs" "\$DATA/updater"
# 群晖共享文件夹默认带 Windows ACL：容器内会把文件看成 000 权限。
# 对代码目录与 ops 目录清掉 ACL，子文件才按普通 Linux 权限继承。
synoacltool -del "\$APP" >/dev/null 2>&1 || true
synoacltool -del "\$DATA/ops" >/dev/null 2>&1 || true
synoacltool -del "\$DATA/ops/certs" >/dev/null 2>&1 || true
synoacltool -del "\$DATA/updater" >/dev/null 2>&1 || true
# 去掉 ACL 后真实模式是 000，必须显式恢复，否则连 cd 都进不去。
d chmod 755 "\$APP" "\$DATA" "\$DATA/ops" "\$DATA/ops/certs" "\$DATA/updater" 2>/dev/null || true
d chown -R "\$(id -un)":users "\$APP" "\$DATA" 2>/dev/null || true

echo '--- 代码 ---'
if [ -d "\$APP/.git" ]; then
  cd "\$APP"
  if ! git fetch --depth 1 origin '$BRANCH' >/tmp/canvas-fetch.log 2>&1; then
    echo '    origin fetch 失败，回退直连 GitHub'
    git remote set-url origin '$REPO_URL'
    git fetch --depth 1 origin '$BRANCH' >/tmp/canvas-fetch.log 2>&1 || echo '    fetch 仍失败，使用本地已有版本'
  fi
else
  cd /tmp
  d rm -rf "\$APP"
  d mkdir -p "\$APP"
  synoacltool -del "\$APP" >/dev/null 2>&1 || true
  d chown "\$(id -un)":users "\$APP"
  for mirror in 'https://ghfast.top/https://github.com' 'https://ghproxy.net/https://github.com' 'https://github.com'; do
    rm -rf "\$APP"; mkdir -p "\$APP"
    synoacltool -del "\$APP" >/dev/null 2>&1 || true
    d chmod 755 "\$APP"; d chown "\$(id -un)":users "\$APP"
    echo "    克隆自 \$mirror"
    if GIT_TERMINAL_PROMPT=0 git clone --depth 1 --single-branch --branch '$BRANCH' "\$mirror/xupengchong-del/open-ai-canvas.git" "\$APP" >/dev/null 2>&1; then
      echo '    克隆完成'; break
    fi
  done
fi
cd "\$APP"
if git rev-parse --verify FETCH_HEAD >/dev/null 2>&1; then git reset --hard FETCH_HEAD >/dev/null; fi
HEAD_SHA=\$(git rev-parse --short HEAD)
echo "    HEAD \$HEAD_SHA  \$(cat VERSION 2>/dev/null)"

echo '--- 配置 ---'
if [ ! -f .env ]; then
  printf '%s' '$env_b64' | base64 -d > .env
  chmod 600 .env
  echo '    已生成 .env（随机数据库密码 + Agent Token）'
else
  echo '    复用已有 .env'
fi
# 补齐后加的受管键（不覆盖已有值，例如数据库密码）
printf '%s' '$env_b64' | base64 -d | while IFS= read -r line; do
  [ -n "\$line" ] || continue
  key="\${line%%=*}"
  grep -q "^\${key}=" .env 2>/dev/null || echo "\$line" >> .env
done
printf '%s' '$tls_compose_b64' | base64 -d > "\$DATA/ops/compose.tls.yml"
printf '%s' '$tls_conf_b64' | base64 -d > "\$DATA/ops/nginx.conf"

if [ ! -f "\$DATA/ops/certs/server.crt" ]; then
  echo '    生成自签证书（10 年，SAN 含 IP）'
  openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
    -keyout "\$DATA/ops/certs/server.key" -out "\$DATA/ops/certs/server.crt" \
    -subj "/CN=$NAS_HOST" \
    -addext "subjectAltName=IP:$NAS_HOST" >/dev/null 2>&1 \
    || openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
         -keyout "\$DATA/ops/certs/server.key" -out "\$DATA/ops/certs/server.crt" \
         -subj "/CN=$NAS_HOST" >/dev/null 2>&1
  chmod 600 "\$DATA/ops/certs/server.key"
fi

echo '--- 镜像 ---'
if [ '$FORCE_BUILD' = '1' ] || ! d docker image inspect open-ai-canvas-web:server >/dev/null 2>&1 || [ ! -f "\$DATA/.deployed-sha" ] || [ "\$(cat "\$DATA/.deployed-sha")" != "\$HEAD_SHA" ]; then
  compose build backend web yingce-agent migrate
  echo "\$HEAD_SHA" > "\$DATA/.deployed-sha"
else
  echo '    镜像已是当前提交，跳过构建（FORCE_BUILD=1 可强制）'
fi

echo '--- 启动 ---'
compose up -d --remove-orphans --wait --wait-timeout 900 || {
  echo '    --wait 未通过，改为后台启动并轮询'
  compose up -d --remove-orphans
}

echo '--- 状态 ---'
compose ps
SCRIPT

  bold "健康检查"
  local attempt code
  for attempt in $(seq 1 15); do
    code="$(curl -sk -o /dev/null -m 8 -w '%{http_code}' "https://$NAS_HOST:$PUBLIC_PORT/api/health" || true)"
    if [ "$code" = "502" ] || [ "$code" = "504" ]; then
      # 前端容器换过 IP 时 tls 里的 nginx 会一直打到旧地址，reload 一次即可恢复。
      warn "tls 反代返回 $code，重载 nginx 后再检查"
      remote <<'RELOAD' >/dev/null 2>&1 || true
$(prelude)
compose exec -T tls nginx -s reload 2>/dev/null || compose restart tls >/dev/null 2>&1 || true
RELOAD
      sleep 5
      continue
    fi
    [ "$code" = "200" ] && { info "https://$NAS_HOST:$PUBLIC_PORT/api/health → 200 ✓"; break; }
    [ "$attempt" = "15" ] && { warn "健康检查未通过（状态码 ${code:-无响应}）"; warn "排查：./nas-deploy.sh logs tls  或  ./nas-deploy.sh logs web"; }
    sleep 3
  done

  printf '\n完成：https://%s:%s\n' "$NAS_HOST" "$PUBLIC_PORT"
  warn "自签证书首次访问会有浏览器告警，属正常；如需可信证书，替换 $DATA_DIR/ops/certs 下的 crt/key 后重跑本命令。"
}

# ---------- 同步 API 配置 ----------

cmd_sync_api() {
  local db key_file
  db="$REPO_ROOT/.local/project-workbench-debug/open_ai_canvas.db"
  key_file="$(dirname "$db")/.settings-key"
  [ -f "$db" ] || die "找不到本机开发数据库：$db"
  [ -f "$key_file" ] || die "找不到本机加密密钥：$key_file"

  bold "同步 API 配置到 NAS"
  info "来源: $(basename "$db") + .settings-key"

  local script
  script="$(prelude)"
  script="$script
set -e
if ! compose ps postgres 2>/dev/null | grep -q running; then
  echo '数据库未运行，先启动整套服务'; compose up -d postgres; sleep 5
fi
echo '--- 目标表结构 ---'
compose exec -T postgres psql -U open_ai_canvas -d open_ai_canvas -At -F '|' -c \\
  \"select table_name, column_name, data_type from information_schema.columns where table_schema='public' and table_name in ('users','model_channels','channel_models','system_settings','storage_locations','user_oss_settings') order by table_name, ordinal_position\"
"
  local schema_txt
  schema_txt="$(remote <<< "$script")" || die "读取 NAS 数据库结构失败"
  printf '%s\n' "$schema_txt" | grep -q 'model_channels' || die "NAS 数据库还没建表，请先跑 ./nas-deploy.sh deploy"

  local schema_file sql_b64 key_b64
  schema_file="$(mktemp)"
  printf '%s\n' "$schema_txt" > "$schema_file"
  sql_b64="$(python3 "$REPO_ROOT/scripts/nas/build-api-sync-sql.py" "$db" "$schema_file" | b64_encode)" || { rm -f "$schema_file"; die "生成本地数据 SQL 失败"; }
  rm -f "$schema_file"
  [ -n "$sql_b64" ] || die "生成的 SQL 为空"
  key_b64="$(b64_encode < "$key_file")"

  remote <<SCRIPT || die "远程同步失败（远端日志见上）"
$(prelude)
set -e

echo '--- 同步加密密钥 ---'
compose up -d backend >/dev/null 2>&1 || true
sleep 3
printf '%s' '$key_b64' | base64 -d | compose exec -T backend sh -c 'cat > /data/.settings-key && chmod 600 /data/.settings-key'
compose exec -T backend sh -c 'echo "    .settings-key \$(wc -c < /data/.settings-key) 字节"' 

echo '--- 导入渠道 / 模型 / 账号 ---'
printf '%s' '$sql_b64' | base64 -d > /tmp/canvas-api-sync.sql
compose exec -T postgres psql -U open_ai_canvas -d open_ai_canvas -v ON_ERROR_STOP=1 -q < /tmp/canvas-api-sync.sql
rm -f /tmp/canvas-api-sync.sql

echo '--- 重启后端 ---'
compose restart backend >/dev/null 2>&1 || compose up -d backend
sleep 6

echo '--- 核对 ---'
compose exec -T postgres psql -U open_ai_canvas -d open_ai_canvas -At -c \\
  "select 'users=' || count(*) from users union all select 'channels=' || count(*) from model_channels union all select 'models=' || count(*) from channel_models" | sed 's/^/    /'
SCRIPT

  bold "完成"
  info "渠道、模型、加密密钥与管理员账号已同步"
  info "用开发机上原来的账号密码登录 https://$NAS_HOST:$PUBLIC_PORT"
}

# pull-models 需要登录 NAS 后台。密码以 bcrypt 存库不可反解，所以从本机文件读一次；
# 优先级：环境变量 > $NAS_ADMIN_ENV_FILE（默认 .local/nas-admin.env，已被 .gitignore 忽略）。
load_admin_credentials() {
  local env_file="${NAS_ADMIN_ENV_FILE:-$REPO_ROOT/.local/nas-admin.env}"
  if [ -f "$env_file" ]; then
    # 只取 NAS_ADMIN_USER / NAS_ADMIN_PASSWORD 两个键，不让文件里其它内容进入环境。
    NAS_ADMIN_USER="${NAS_ADMIN_USER:-$(grep -E '^[[:space:]]*NAS_ADMIN_USER[[:space:]]*=' "$env_file" | head -1 | cut -d= -f2- | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')}"
    NAS_ADMIN_PASSWORD="${NAS_ADMIN_PASSWORD:-$(grep -E '^[[:space:]]*NAS_ADMIN_PASSWORD[[:space:]]*=' "$env_file" | head -1 | cut -d= -f2- | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')}"
  fi
  [ -n "$NAS_ADMIN_PASSWORD" ] || die "缺少 NAS 后台密码：在 $env_file 里写 NAS_ADMIN_PASSWORD=…，或运行时带 NAS_ADMIN_PASSWORD=…"
  [ -n "$NAS_ADMIN_USER" ] || die "缺少 NAS 后台账号：在 $env_file 里写 NAS_ADMIN_USER=…"
}

# ---------- 从 NAS 拉取模型配置 ----------

cmd_pull_models() {
  local db input_file dry_run=0 scope="system" channel=""
  for arg in "$@"; do
    case "$arg" in
      --dry-run) dry_run=1 ;;
      --scope=*) scope="${arg#*=}" ;;
      --channel=*) channel="${arg#*=}" ;;
      *) die "未知参数：$arg（支持 --dry-run / --scope=system / --channel=CHANNEL_000001）" ;;
    esac
  done
  db="$REPO_ROOT/.local/project-workbench-debug/open_ai_canvas.db"
  [ -f "$db" ] || die "找不到本机开发数据库：$db"

  load_admin_credentials

  bold "从 NAS 拉取模型配置"
  info "来源: https://$NAS_HOST:$PUBLIC_PORT"
  info "作用域: $scope  渠道: ${channel:-全部}"
  info "目标: ${db#$REPO_ROOT/}"

  input_file="$(mktemp)"
  local fetch_err
  if ! fetch_err="$(python3 "$REPO_ROOT/scripts/nas/fetch-nas-models.py" \
      --host "$NAS_HOST" --port "$PUBLIC_PORT" \
      --username "$NAS_ADMIN_USER" --password "$NAS_ADMIN_PASSWORD" \
      --scope "$scope" ${channel:+--channel "$channel"} > "$input_file" 2>&1)"; then
    rm -f "$input_file"
    printf '%s\n' "$fetch_err" | sed 's/^/    /'
    if printf '%s' "$fetch_err" | grep -q 'rate_limited'; then
      die "NAS 登录被限流（同一账号短时间登录太多次），等一两分钟再跑一次即可"
    fi
    die "抓取 NAS 模型失败：检查 .local/nas-admin.env 里的账号密码，或 NAS_ADMIN_USER / NAS_ADMIN_PASSWORD 覆盖"
  fi

  local count
  count="$(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))["models"]))' "$input_file")"
  info "抓到 $count 个模型"

  local apply_args=("$REPO_ROOT/scripts/nas/apply-nas-models.py" --db "$db" --input "$input_file")
  [ "$dry_run" = "1" ] && apply_args+=(--dry-run)
  python3 "${apply_args[@]}" || { rm -f "$input_file"; die "回灌本机数据库失败"; }
  rm -f "$input_file"

  [ "$dry_run" = "1" ] && { bold "预览结束"; info "去掉 --dry-run 才会真正写库"; return 0; }
  bold "完成"
  info "NAS 的模型配置已同步到本机（渠道与密钥未改动）"
}

# ---------- 其它 ----------

cmd_status() {
  remote <<SCRIPT
$(prelude)
compose ps 2>/dev/null || true
echo '--- 版本 ---'
cd "\$APP" 2>/dev/null && echo "\$(git rev-parse --short HEAD)  \$(cat VERSION 2>/dev/null)"
echo '--- 数据占用 ---'
du -sh "\$DATA"/data "\$DATA"/postgres-data 2>/dev/null || true
SCRIPT
}

cmd_logs() {
  local service="${1:-backend}"
  remote <<SCRIPT
$(prelude)
compose logs --tail=150 '$service'
SCRIPT
}

cmd_shell() {
  exec sshpass -e ssh -p "$NAS_SSH_PORT" \
    -o PreferredAuthentications=password -o PubkeyAuthentication=no \
    -o StrictHostKeyChecking=accept-new "$NAS_USER@$NAS_HOST"
}

usage() {
  awk 'NR>1 && /^#/ {sub(/^# ?/,""); print} NR>1 && !/^#/ {exit}' "$0"
}

main() {
  load_connection
  export SSHPASS="$NAS_PASSWORD"
  case "${1:-deploy}" in
    deploy|update) cmd_deploy ;;
    sync-api) cmd_sync_api ;;
    pull-models) shift; cmd_pull_models "$@" ;;
    status) cmd_status ;;
    logs) shift; cmd_logs "${1:-backend}" ;;
    shell) cmd_shell ;;
    -h|--help|help) usage ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"
