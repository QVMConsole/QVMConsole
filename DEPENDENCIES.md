# 开发环境依赖安装指南

本文档说明如何安装 QVMConsole 开发所需的所有依赖。

## 环境要求

| 依赖 | 最低版本 | 用途 |
|------|---------|------|
| Go | 1.22+ | 后端开发语言 |
| Node.js | 18+ | 前端构建工具链 |
| npm | 9+ | 前端包管理 |
| air | 1.61.7 | Go 热重载开发工具 |

## Windows 安装步骤

### 1. 安装 Go

从 [go.dev](https://go.dev/dl/) 下载 Windows 安装包并安装。

验证安装：

```powershell
go version
```

### 2. 安装 Node.js

从 [nodejs.org](https://nodejs.org/) 下载 LTS 版本并安装（npm 随 Node.js 一起安装）。

验证安装：

```powershell
node --version
npm --version
```

### 3. 安装 air（Go 热重载）

```powershell
go install github.com/air-verse/air@v1.61.7
```

验证安装：

```powershell
where air
```

> 确保 `%USERPROFILE%\go\bin` 在系统 PATH 环境变量中。

### 4. 安装前端依赖

```powershell
cd web
npm install
```

### 5. 下载 Go 模块依赖

```powershell
cd server
go mod download
```

## Linux 安装步骤

### 1. 安装 Go

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# 或手动安装（推荐，获取最新版本）
wget https://go.dev/dl/go1.23.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### 2. 安装 Node.js

```bash
# 使用 NodeSource
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt install -y nodejs
```

### 3. 安装 air

```bash
go install github.com/air-verse/air@v1.61.7
```

确保 `$(go env GOPATH)/bin` 在 PATH 中：

```bash
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### 4. 安装前端依赖

```bash
cd web
npm install
```

### 5. 下载 Go 模块依赖

```bash
cd server
go mod download
```

## macOS 安装步骤

### 1. 安装 Homebrew（如未安装）

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

### 2. 安装 Go 和 Node.js

```bash
brew install go node
```

### 3. 安装 air

```bash
go install github.com/air-verse/air@v1.61.7
```

### 4. 安装前端依赖

```bash
cd web
npm install
```

### 5. 下载 Go 模块依赖

```bash
cd server
go mod download
```

## 一键安装脚本（Linux/macOS）

```bash
#!/bin/bash
set -e

echo "=== QVMConsole 开发环境依赖安装 ==="

# 安装 air
if ! command -v air &>/dev/null; then
    echo "安装 air..."
    go install github.com/air-verse/air@v1.61.7
else
    echo "air 已安装"
fi

# 前端依赖
echo "安装前端依赖..."
cd web && npm install
cd ..

# Go 模块
echo "下载 Go 模块..."
cd server && go mod download
cd ..

echo "=== 依赖安装完成 ==="
echo "启动开发服务器: ./start-dev.sh"
```

## 验证安装

全部安装完成后，执行以下命令确认环境：

```bash
go version          # 应显示 go1.22+
node --version      # 应显示 v18+
npm --version       # 应显示 9+
air -v              # 应显示 air v1.61.7
ls web/node_modules # 前端依赖目录应存在
```

## 注意事项

- Windows 下运行 `air` 不支持的参数 `--send_interrupt` 会被自动忽略
- 首次 `npm install` 可能需要较长时间，后续有缓存会快很多
- Go 模块会缓存到 `$GOPATH/pkg/mod`，后续项目共享
- 测试机（Linux）系统级 QEMU/KVM 依赖由 `install.sh` 管理，详见该脚本

## Windows 虚拟机初始化依赖

### genisoimage

**用途：** 为 Windows 虚拟机创建符合 OpenStack ConfigDrive 规范的 ISO 镜像（CloudbaseInit 初始化方案）

**安装方式（Debian/Ubuntu）：**

```bash
sudo apt install -y genisoimage
```

**说明：** 此工具在 Windows 克隆/重装时被调用，用于生成包含 `meta_data.json`（主机名、管理员密码、instance-id）的 config-2 标签 ISO，挂载到虚拟机 CD-ROM 后由 CloudbaseInit 读取完成初始化。该工具已由 `install.sh` 自动安装。
