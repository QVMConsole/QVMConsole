# 开发启动脚本端口占用自动处理（start-dev.sh）

## 功能说明

`start-dev.sh` 在启动前后端之前，会先检测后端端口（默认 8080）和前端 dev 端口（默认 5173）。
**端口被占用时不做询问、直接结束占用进程**，然后继续启动开发环境。

这样做的原因：

- 上一次开发环境异常退出、或 `air` 被结束后残留的 `kvm-console` 仍占着 8080，会导致新进程启动失败；
- `air`、`vite` 等工具检测到子进程退出后会自动重新拉起子进程，只杀掉监听进程会出现"端口刚释放又被抢回"的情况，
  因此脚本会连带结束这类开发工具父进程链。

## 行为流程

1. 用 `ss`（缺失时回退 `lsof`、`fuser`）找出监听该端口的进程；
2. 从监听进程向上追溯父进程，**仅当父进程也是开发工具**（`air` / `vite` / `npm` / `npx` / `pnpm` / `yarn` / `node` / `go` / `kvm-console` / `esbuild` / `rollup` / `start-dev.sh`）时才继续向上，最多 4 层，
   避免误杀终端会话或其他无关服务；
3. 先发 `SIGTERM`，等待（默认 10 秒）后端口仍未释放则发 `SIGKILL`；
4. 端口确认释放后继续启动后端与前端；若最终仍被占用则报错退出。

脚本永远不会结束 `PID 1` 以及脚本自身/其父进程。

## 可配置项

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `KVM_PORT` | `8080` | 后端端口，同时导出给后端进程，与后端配置项一致 |
| `KVM_WEB_PORT` | `5173` | 前端 dev 端口（以 `vite --port` 传入） |
| `KVM_PORT_KILL` | `1` | 置为 `0` 时只检测不结束占用进程，被占用则报错退出 |
| `KVM_PORT_TERM_WAIT` | `10` | 等待占用进程优雅退出的秒数，超时后 `SIGKILL` |

示例：

```bash
# 只检查不结束
KVM_PORT_KILL=0 ./start-dev.sh

# 换端口启动（注意同步修改 web/vite.config.ts 里的代理地址）
KVM_PORT=18080 KVM_WEB_PORT=15173 ./start-dev.sh
```

## 输出示例

```text
[INFO] 检查端口占用...
[WARN] 端口 8080（后端）已被占用，占用进程如下，即将直接结束：
    PID 18382  /root/QVMConsole/server/tmp/kvm-console
    PID 18379  /bin/sh -c /root/QVMConsole/server/tmp/kvm-console
    PID 18293  air
[INFO] 正在结束占用进程...
[INFO] 端口 8080 已释放
[INFO] 端口 5173（前端）空闲
[INFO] 启动后端 (air 热重载, 0.0.0.0:8080)...
[INFO] 启动前端 (vite dev, 0.0.0.0:5173)...
```

## 手工排查命令

```bash
# 查看端口占用
ss -lptn 'sport = :8080'

# 查看某进程的父进程链
ps -o pid,ppid,args -p <PID>
```

## 常见问题

### `npx vite` 报 `sh: 1: vite: Permission denied`

与端口检测无关，是文件同步造成的：从 Windows 同步到 Linux 时 `node_modules/**/bin/*.js` 会丢失执行位
（例如 `node_modules/vite/bin/vite.js` 权限变成 `666`），`node_modules/.bin/` 下的软链因此无法执行。

修复（在 `web` 目录执行）：

```bash
for f in node_modules/.bin/*; do [ -L "$f" ] && chmod +x "$f"; done
npx vite --version
```

前端进程启动即退出时，`start-dev.sh` 会因为 `wait -n` 返回而进入退出清理，表现为"脚本自动停止"，
此时应优先检查上面这个权限问题。
