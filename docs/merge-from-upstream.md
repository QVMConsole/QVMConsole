# 合并上游仓库后端修改指南

## 背景

- **标准仓库（上游）**: <https://github.com/QVMConsole/QVMConsole>（已配为 `upstream`）
- **本仓库（origin）**: <https://github.com/QVMConsole/new-web.git>
- **合并原则**: 只合并 `server/`（后端）修改，**前端（`web/`）完全不合并**

> 当前 `web/` 目录已清空进入重构阶段，原前端备份在 `web-backup/`。合并时必须确保上游的前端代码不会被带入本仓库。

---

## 一、远程仓库配置（首次一次性）

```bash
# 添加上游仓库（如未添加）
git remote add upstream https://github.com/QVMConsole/QVMConsole.git

# 验证
git remote -v
# 应该看到：
#   origin    https://github.com/QVMConsole/new-web.git (fetch)
#   origin    https://github.com/QVMConsole/new-web.git (push)
#   upstream  https://github.com/QVMConsole/QVMConsole.git (fetch)
#   upstream  https://github.com/QVMConsole/QVMConsole.git (push)
```

---

## 二、合并流程

### 策略 A（推荐）：Merge + 选择性保留

整体合并，然后恢复本仓库独有的文件。

```bash
# 1. 拉取上游最新代码
git fetch upstream

# 2. 创建备份分支（可选，安全操作）
git branch backup-before-merge

# 3. 开始合并，但不自动提交
git merge upstream/main --no-commit --no-ff

# 4. 恢复本仓库独有的文件（拒绝上游的前端代码）
git checkout HEAD -- web/ web-backup/ .gitignore docs/merge-from-upstream.md

# 5. 如果有冲突未自动解决，手动处理
git status

# 6. 提交合并
git commit -m "chore: merge upstream backend changes <日期>"
```

**关键点**: `git checkout HEAD -- web/` 会强制将 `web/` 恢复成本仓库状态（空目录），彻底规避上游前端代码被带入。

### 策略 B：Selective Cherry-Pick

只挑选包含后端修改的提交，逐个合并。

```bash
# 1. 查看上游日志，找到只包含 server/ 修改的提交
git log upstream/main --oneline -- server/

# 2. 逐个 cherry-pick（按提交先后顺序）
git cherry-pick <commit-hash>

# 3. 如果 cherry-pick 后意外带入了其他文件，恢复本仓库独有文件
git checkout HEAD -- web/ web-backup/ .gitignore
```

**适用场景**: 上游后端修改提交粒度高、与前端完全分离时使用。

### 策略 C：Subtree 模式（长期维护）

如果只需要追踪上游 `server/` 目录，可用 subtree：

```bash
# 首次添加
git subtree add --prefix=server upstream/main --squash

# 后续更新
git subtree pull --prefix=server upstream/main --squash
```

> 注意：subtree 会改写提交历史，与其他协作者同步时需谨慎使用。

---

## 三、冲突处理原则

合并过程中可能出现的冲突类型及处理方式：

| 冲突来源 | 处理方式 |
|---------|---------|
| `server/` 目录内冲突 | 根据业务需求判断取舍，解决后继续合并 |
| `web/` 目录冲突 | 一律采用本仓库版本（`git checkout HEAD -- web/`） |
| `web-backup/` 冲突 | 一律采用本仓库版本 |
| `docs/` 文档冲突 | 手动合并，保留双方必要内容 |

---

## 四、合并后验证

```bash
# 1. 检查是否有上游前端文件污染
git diff --name-status HEAD -- web/

# 2. 确认后端可编译
cd server
go build ./...

# 3. 确认 gitignore 生效
git status
# 不应出现 web-backup/ 相关文件（已在 .gitignore 中排除）

# 4. 提交并推送
git push origin main
```

---

## 五、注意事项

1. **前端零合并**: `web/` 目录是本仓库独立重构的前端，绝对不接受上游的任何前端修改。
2. **`web-backup/`**: 已加入 `.gitignore`，不会被提交到仓库。此目录仅作为本地参考备份。
3. **大版本合并**: 如果上游后端有重大架构变更，建议先开分支测试，验证通过后再合入 `main`。
4. **合并不频繁时建议用策略 A**：操作简单、确定性高。
5. **上游 `.gitignore` 变更**: 合并后检查是否有冲突，确保 `web-backup/` 仍被忽略。
