# 发布包二进制兼容性

`build.sh` 默认构建两个后端二进制：

- `kvm-console`：Zig 兼容版，作为低版本 GLIBC 系统的默认程序。
- `kvm-console-native`：使用构建宿主机系统编译器的原生版，适合较新的 GLIBC 环境。

## GLIBC 目标

兼容版始终通过 Zig 的 `-target` 参数指定 GLIBC 上限，避免继承构建机的 GLIBC 符号版本：

| 目标架构 | 默认 Zig 目标 | 最高 GLIBC 依赖 |
| --- | --- | --- |
| amd64 | `x86_64-linux-gnu.2.2.5` | 2.2.5 |
| arm64 | `aarch64-linux-gnu.2.17` | 2.17 |

可在构建时修改兼容版上限：

```bash
bash build.sh -v 0.3.0.2 --compat-glibc 2.17
```

兼容版构建需要安装 Zig。构建完成后，脚本会使用 `readelf` 检查实际最高 GLIBC 依赖；若超过指定上限，构建会失败，避免错误产物进入发布包。

## 手动检查

```bash
readelf --version-info -W release/kvm-console-linux-amd64/kvm-console \
  | grep -oE 'GLIBC_[0-9.]+' | sort -Vu
```

原生版的 GLIBC 依赖由构建宿主机的工具链决定，应使用相同命令检查 `kvm-console-native`。
