# GitHub 构建变体

GitHub Actions 的“构建 Linux 发行包”工作流新增了可选的 `variant` 输入参数。

- 留空（默认）：执行 `build.sh` 默认行为，同时构建兼容版和原生版。
- 填写 `native`：执行 `build.sh --variant native`，仅构建宿主机原生版。

amd64 和 arm64 构建任务都会使用该参数。构建产物名称和上传位置保持原有规则不变。
