#!/usr/bin/env bash
# ==============================================================================
# update-upstream.sh
# 影策安全同步脚本：拉取官方最新代码，100% 保护本地修改与定制功能
# ==============================================================================

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$REPO_DIR"

# 1. 设置网络代理（支持 GitHub 访问）
export HTTPS_PROXY="${HTTPS_PROXY:-socks5://127.0.0.1:7890}"
export HTTP_PROXY="${HTTP_PROXY:-socks5://127.0.0.1:7890}"
export ALL_PROXY="${ALL_PROXY:-socks5://127.0.0.1:7890}"

echo "=================================================="
echo "🚀 开始同步官方上游（upstream）最新更新..."
echo "=================================================="

# 2. 检查工作区是否干净
if [ -n "$(git status --porcelain)" ]; then
    echo "⚠️ 检测到当前工作区有未提交的临时修改："
    git status --short
    echo ""
    echo "❌ 为了防止修改被冲掉，请先提交或保存你的改动后再执行同步！"
    exit 1
fi

CURRENT_BRANCH="$(git branch --show-current)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
BACKUP_BRANCH="backup/${CURRENT_BRANCH//\//_}_${TIMESTAMP}"

# 3. 自动创建安全备份分支（绝对保护本地代码）
git branch "$BACKUP_BRANCH" HEAD
echo "🛡️  已自动创建安全备份分支：$BACKUP_BRANCH"
echo "   （若有任何需要，可随时执行 'git reset --hard $BACKUP_BRANCH' 彻底恢复）"

# 4. 获取官方最新代码
echo "📥 正在从官方上游（upstream）抓取更新..."
git fetch upstream

UPSTREAM_LATEST="$(git log --oneline -1 upstream/main)"
echo "📌 官方最新提交：$UPSTREAM_LATEST"

# 5. 更新本地 main 分支（纯净快进）
echo "🔄 正在更新本地 main 分支..."
git fetch . upstream/main:main 2>/dev/null || true

# 6. 如果当前就在 local/daily，则平滑合并上游更新
if [ "$CURRENT_BRANCH" = "local/daily" ]; then
    echo "🔀 正在将官方最新改动合并到你的工作分支 local/daily..."
    if git merge --no-edit upstream/main; then
        echo "✅ 恭喜！官方最新代码已成功合并进 local/daily，本地定制功能全部保留！"
    else
        echo "⚠️ 合并时检测到代码冲突，请查看冲突文件："
        git status --short
        echo ""
        echo "💡 提示："
        echo "1. 解决冲突后执行：git commit"
        echo "2. 如需放弃合并并瞬间还原到更新前，请执行："
        echo "   git merge --abort && git reset --hard $BACKUP_BRANCH"
        exit 1
    fi
else
    echo "ℹ️ 当前不在 local/daily 分支（当前处于 $CURRENT_BRANCH）。"
    echo "   已更新本地 main 分支。如需同步到 local/daily，请切回后再次运行本脚本。"
fi

echo "=================================================="
echo "🎉 同步完成！当前处于最新官方基线。"
echo "=================================================="
