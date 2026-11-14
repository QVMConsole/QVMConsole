# GitHub 构建与发布

“构建 Linux 发行包”工作流定义在 [build.yml](../.github/workflows/build.yml)，支持发布 Release 自动构建和 Actions 手动构建。产物上传到文件中继，发布场景同时上传到 GitHub Release；已移除原来的 `artifact_storage` 选项和阿里云 OSS 上传步骤，不再使用 `ALIYUN_OSS_*` Secrets。

## 发布 Release 自动构建

1. 将工作流改动合并到默认分支，再基于包含该工作流的提交创建版本标签。
2. 在 GitHub Releases 页面填写标签、标题和发布说明，然后发布 Release。
3. `release.published` 事件自动触发工作流，构建该标签提交的 amd64 和 arm64 包，每个平台默认包含兼容版和原生版二进制。
4. 所选平台全部构建成功后，同一批产物分别上传到当前 Release 的附件和文件中继。上传任务独立执行，一侧失败不会阻止另一侧上传；任何一侧失败都会使本次工作流失败。

正式版、预发布版以及从草稿发布的 Release 都会触发。仅保存草稿、创建标签、编辑已有 Release 的说明不会触发。自动上传只补充或替换附件，保留用户填写的标题、说明和预发布状态。

标签支持 `v1.0.0`、`1.0.0`、`v1.0.0-rc.1` 等形式。传给构建脚本的版本号会去掉单个开头的 `v`，GitHub 附件仍上传到原始标签对应的 Release。版本号去掉 `v` 后必须以数字开头，只允许字母、数字、点、下划线、加号和连字符。

## 手动构建

在 Actions 页面选择“构建 Linux 发行包”并点击 Run workflow：

| 输入 | 说明 |
| --- | --- |
| `version` | 必填版本号，可填写 `1.0.0` 或 `v1.0.0`；手动发布的标签统一为 `v1.0.0` |
| `build_platforms` | 选择 `amd64,arm64`、`amd64` 或 `arm64`，默认构建两种架构 |
| `variant` | 留空时同时构建兼容版和原生版；填写 `native` 时仅构建原生版 |
| `create_release` | 默认关闭；开启后创建或更新 GitHub Release，并上传同一批附件 |

手动构建始终保存 Actions Artifacts 并上传文件中继。开启 `create_release` 时，如果标签尚不存在，Release 标签指向本次选择的分支或标签提交；如果标签已经存在，请在 Run workflow 中选择该标签，保证构建源码与 Release 标签一致。

由本工作流使用 `GITHUB_TOKEN` 创建的 Release 不会再次触发发布工作流；本次运行已经包含 GitHub 和中继上传，无需二次触发。需要重试时在 Actions 中重新运行原工作流。

## 文件中继配置

复用共享工作流 `yxsj245/Download_Station_CICD/.github/workflows/relay-upload.yml@v1`，不在本项目复制上传脚本。当前共享工作流允许 `QVMConsole` 组织调用，仓库 Actions 策略也需允许引用该共享工作流及其 Actions。

在 `QVMConsole/QVMConsole` 仓库的 Settings → Secrets and variables → Actions 配置下列项目，也可以使用已授权该仓库的组织级配置：

| 类型 | 名称 | 用途 |
| --- | --- | --- |
| Variable | `RELAY_BASE` | GitHub runner 可访问的中继地址，不带尾斜杠；有反代前缀时包含该前缀 |
| Secret | `RELAY_ADMIN_KEY` | 中继管理员 Key（`SR_ADMIN_API_KEY`），用于登记统一逻辑路径 |
| Secret | `RELAY_UPLOAD_KEY` | 中继上传 Key（`SR_UPLOAD_API_KEY`），用于上传和查询结果 |

两个 Key 必须非空且互不相同。工作流显式将这两个 Secret 传给共享工作流，密钥不要写入仓库、日志或构建产物。GitHub Release 上传使用内置 `GITHUB_TOKEN`，仅 Release 上传任务请求 `contents: write`，无需另外配置 PAT。

中继采用以下固定路径，不额外添加版本子目录：

```text
开源项目/QVMConsole/Open/kvm-console-linux-amd64.tar.gz
开源项目/QVMConsole/Open/kvm-console-linux-arm64.tar.gz
```

每次成功上传覆盖对应架构的同名文件，便于保持固定下载路径；GitHub Release 保留各标签的附件。手动只选一个架构时，仅更新该架构，另一架构的中继文件保持原状。预发布版、旧版本重跑和手动构建同样会覆盖固定路径，因此目录内容代表最后成功上传的构建。

工作流通过并发组避免多个运行同时写入固定路径，不主动取消正在执行的发布；GitHub 默认只保留一个待运行实例，频繁发布可能替换尚未开始的待运行实例。中继轮询设置为 `poll-timeout: '0'`，不额外限制等待终态的时间，但仍受 GitHub Actions 平台运行时限约束。必须等到中继任务终态为 `succeeded` 才算上传成功，各渠道结果可在 Actions Summary 中查看。

## 依赖与验证范围

构建工具沿用原来的 Node.js、Go 和 Zig 0.14.0；新增共享中继工作流及其依赖记录在 [dependencies.md](dependencies.md)。此次修改只涉及 CI 配置和文档，不改变程序构建逻辑、安装流程或虚拟机兼容性测试。工作流可用 actionlint 检查 YAML、表达式和任务依赖；真实发布与中继分发需要在 GitHub 配置凭据后运行验收。
