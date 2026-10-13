# Git 规则（本仓库）

> 这份文件只记录**本仓库的 Git 操作规则**（分支、提交、同步、提 PR）。
> 上游的代码与文档约定见 [AGENTS.md](AGENTS.md)，对外贡献流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 1. 远端与网络

| 远端 | 地址 | 权限 |
| --- | --- | --- |
| `origin` | `https://github.com/xupengchong-del/open-ai-canvas.git` | 自己的 fork，**可推** |
| `upstream` | `https://github.com/ddcat-ai/open-ai-canvas.git` | **只读**（无 push 权限） |

- **永远不要尝试推 `upstream`**：没有权限，改动只能走「fork 分支 + PR」。
- 本机 Git 走代理，已写进 `~/.gitconfig` 的 `http.proxy = socks5://127.0.0.1:7890`。
- `gh` **不读** Git 的代理配置，用之前必须先导出，否则会 `dial tcp ... i/o timeout`：

```bash
export HTTPS_PROXY=socks5://127.0.0.1:7890
export HTTP_PROXY=socks5://127.0.0.1:7890
export ALL_PROXY=socks5://127.0.0.1:7890
```

## 2. 分支模型

| 分支 | 用途 | 能否推送 |
| --- | --- | --- |
| `local/daily` | **日常自用分支**：自己的全部改动先汇总到这里，本地 dev server 一直跑这个分支 | ❌ **绝不推送**（推上去会和 PR 重复、互相污染） |
| `feat/*`、`fix/*` | 单个功能或修复，**基于最新的 `upstream/main`**，只含这件事的提交 | ✅ 推到 `origin` 后开 PR |
| `main` | 跟随上游，只用来快进 | ❌ 不直接在上面提交 |

规则：

1. **一个 PR 只做一件事**。功能拆到各自的特性分支，PR 里不夹带无关改动。
2. 特性分支必须基于**最新的 `upstream/main`**；提交 PR 前先同步再 rebase：

   ```bash
   git fetch upstream
   git checkout feat/<模块>-<事情>
   git rebase upstream/main
   ```

3. 合并方向是单向的：**特性分支 → PR → 上游合并 → `main` 快进 → 再把 `main` 汇回 `local/daily`**。
4. 特性分支之间不要互相 merge；需要上游新代码就用上面的 rebase。

## 3. 提交信息格式（与上游 AGENTS.md 第 9 节一致）

```
<type>(<scope>): <业务模块> - <变更摘要>
```

- `type` 取：`feat` | `fix` | `refactor` | `perf` | `docs` | `test` | `build` | `ci` | `chore` | `revert`
- `scope` 用小写模块名，如 `create`、`auth`、`admin`
- 摘要写「**业务模块 - 做了什么**」，用中文；不要写 “update files” 这类没有信息量的话
- 正文说清：**现象/原因 → 改了什么 → 验证了什么、没验证什么**
- 示例：

  ```
  feat(create): 创作页 - 生成记录支持逐条删除
  fix(create): 创作页 - 上一条还在生成时无法提交下一条
  test(admin): 系统模型 - 同步「拉取模型」弹窗的源码回归断言
  docs(progress): 待测试 - 记录提交闸门只覆盖提交阶段的修复与验收点
  ```

## 4. 提交前必做

1. `git status --short`、`git diff` 先看一遍，**只提交与本次改动相关的文件**。
2. **不要提交**：`.local/`（本地数据目录、SQLite 库、缓存、日志、临时脚本）、`.env`、任何密钥或 token、构建产物、编辑器配置。
3. 改完**先验证再提交**（按改动范围选最小充分命令）：

   | 范围 | 命令 |
   | --- | --- |
   | 前端类型/构建 | `cd web && bunx tsc --noEmit`、`bun run build` |
   | 前端测试 | `cd web && bun test <相关文件>` |
   | 后端 | `cd backend && go build ./... && go test ./<相关包>` |
   | 文档站 | `cd docs && bun run build` |

4. 提交信息里**如实写验证结论**，包括已知失败项和它是否与本次改动有关（例如“唯一失败项在纯净上游基线上同样失败”）。

## 5. 禁止的操作

- `git push --force` / `--force-with-lease`（尤其对 `main`、`local/daily`）
- `git reset --hard`
- `git checkout -- <路径>`、`git clean -fd` 这类会批量丢弃工作区改动的命令
- 在 `main` 上直接 commit
- 覆盖、回滚、清理**不是本次产生的**改动
- 用 `git add -A` 一把梭（容易把临时文件、密钥、数据一起提交）

确实需要丢弃改动时：**先确认绝对路径与目标**、用最小范围精确操作，并且**征得用户同意**。

## 6. 跟随上游（手动拉取与自动同步）

### A. 一键安全手动拉取（推荐）

在项目根目录下直接运行：

```bash
./update-upstream.sh
```

它的安全保障（**100% 保护本地修改**）：
1. **自动先建备份**：每次拉取前自动在本地创建时间戳备份分支（如 `backup/local_daily_20261013_132544`），随时可一键撤销；
2. **防覆盖保护**：工作区有未提交代码时自动拦截，不会丢失任何未存内容；
3. **安全合并**：将官方最新更新合并入 `local/daily`，你的所有定制功能完整保留。

### B. 后台自动同步

本机装了 LaunchAgent `com.xupengchong.yingce-sync`：登录时运行一次，之后**每 30 分钟**（`StartInterval=1800`）运行一次。

- 脚本：`~/.local/share/yingce-sync/yingce-sync.sh`
- 日志：`~/.local/share/yingce-sync/sync.log`
- 可用环境变量：`YINGCE_SYNC_REPO`（仓库路径）、`YINGCE_SYNC_BASE`（日志/锁目录）

它的安全规则（**保证不会覆盖本地改动**）：

1. 工作区有未提交改动 → **本次直接跳过**，不 stash、不 reset、不 checkout；
2. 只更新 `main`，且只允许**快进**（`git merge --ff-only upstream/main`）；与上游分叉就放弃；
3. **绝不改动任何特性分支**（`local/daily` 与 `feat/*` 都不会被动），绝不 force push，绝不 `reset --hard`。

查看同步情况：

```bash
tail -30 ~/.local/share/yingce-sync/sync.log
```

## 7. 提 PR 流程

```bash
# 1. 基于最新上游开分支
git fetch upstream
git checkout -b feat/<模块>-<事情> upstream/main

# 2. 改代码 → 验证 → 提交（见第 3、4 节）

# 3. 推到自己的 fork
git push -u origin feat/<模块>-<事情>

# 4. 开 PR 到上游（description 建议写成文件，避免转义问题）
gh pr create --repo ddcat-ai/open-ai-canvas \
  --base main --head xupengchong-del:feat/<模块>-<事情> \
  --title "feat(create): 创作页 - xxx" --body-file .local/pr/xxx.md
```

PR 描述必须包含：**改了什么、为什么、怎么验证的、哪些没验证**。

注意：

- 多个 PR 若都改了同一个文件（例如 `docs/content/docs/progress/pending-test.mdx`），先合并的那个会让其余的冲突 → 在每个 PR 里注明。
- PR 提交后**不要**去推 `upstream`，等上游合并。
- 合并后把 `main` 快进（自动同步会做），再把 `main` 汇回 `local/daily`。

## 8. 常用命令

```bash
git status --short                      # 先看工作区
git log --oneline -5                    # 最近提交
git branch -vv                          # 分支与跟踪关系
git fetch upstream && git log --oneline upstream/main -3   # 上游有没有新提交

# 我的 PR 状态（记得先导出第 1 节的代理变量）
gh pr status --repo ddcat-ai/open-ai-canvas
gh pr view <编号> --repo ddcat-ai/open-ai-canvas --json state,title
```
