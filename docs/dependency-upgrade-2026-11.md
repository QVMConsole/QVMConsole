# 2026 年 11 月依赖升级

前端依赖和 npm 锁文件已更新，包括 Semi Design、React、React Router、Vite 和相关开发工具。React Router 保持精确版本约束；其余版本约束见 `web/package.json`。使用 `cd web && npm ci && npm run build` 安装锁定版本并构建。

后端直接及间接 Go 模块已更新，包括 libvirt 客户端、加密和系统接口、SQLite 驱动等。`gin-contrib/sse` 的当前版本要求 Go 1.26.10，因此 `server/go.mod` 的最低 Go 版本同步调整为 1.26.10。使用 `cd server && go mod download` 下载依赖。构建启用 SQLite 的 Linux 二进制仍需要 CGO 和 C 编译器。

GitHub 发布工作流使用 Node.js 24；本地构建需满足前端依赖的 Node.js 版本要求。升级不会自动更新已部署的面板，发布前应在目标环境检查虚拟机管理、登录、数据库、控制台和主要页面。
