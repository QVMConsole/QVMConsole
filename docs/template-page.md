# 模板管理页（新前端）

> 对应路由：`/template`（仅管理员）
> 源码目录：`web/src/views/template/`
> 旧版对照：`web-backup/src/views/template/index.vue`

## 功能总览

| 功能 | 说明 |
|------|------|
| 模板族卡片 | 按 `template_uid` 聚合为族卡片，族头显示类型图标、族名、节点数 / 关联 VM 数 / 磁盘总量 |
| 树形节点列表 | 层级色条（按深度 5 色循环）+ 树状引导线（垂直线/中间分支/末尾分支）+ 展开箭头；点击行或箭头展开/收起 |
| 派生链摘要 | 含子节点的节点收起时显示「最新派生链 A → B → C（深 N 层 / 共 M 节点）」 |
| 节点状态标签 | OS 类型、发行版/版本分类、可见性（已禁用/用户可见/仅管理员）、VM 数、虚拟/实际容量、导出状态、哈希校验状态（已记录/缺失/大小变化）、Linux 离线预处理状态（仅 Linux） |
| 全部展开/收起 | 页头按钮批量控制；手动刷新后默认全部收起 |
| 导出 | 「导出节点」导出单节点包，「导出整树」（仅根节点）导出整棵派生树；均为异步任务 |
| 导出包管理 | 已导出节点可「下载导出包」（附带 token 新窗口打开）与「删除导出包」（二次确认） |
| 离线预处理 | 仅 Linux 节点显示，提交补齐 cloud-init 与磁盘扩容依赖的预处理任务（二次确认） |
| 发布设置 | 管理员名称、用户侧显示、Linux/Windows 分类、启用克隆、禁用模板、默认创建配置（CPU/内存/磁盘/磁盘驱动/网卡/显示设备/CPU 拓扑/首次重启）、Linux 启动后命令（含阻塞开关） |
| 其它模板 | 制作时选择「其它」会固定关闭系统初始化；克隆时支持链式克隆和完整克隆，但不会修改模板内的主机名、用户名、密码或网络配置 |
| 导入模板包 | 支持「上传文件」（分片上传：MD5 秒传 + 断点续传 + 缺片自愈补传，含哈希计算/上传双阶段进度）与「主机绝对路径」两种来源；解析预览展示节点链路（冲突/已存在/将导入）后确认导入（异步任务）；预览后未导入而关闭弹窗会自动清理临时包 |
| 删除模板链路 | 三种模式：级联删除 / 仅删除当前节点并提升子节点 / 热删除（在线切换 backing）；展示将删除节点、提升子模板、重定向 VM、关联 VM 及处理方式；提升模式存在阻塞项时禁止提交；删除为高风险操作，428 二次验证由请求层自动处理 |

## 目录结构

```
web/src/views/template/
├── index.tsx                        # 主入口：数据加载/展开状态/行内操作/弹窗分发
├── template.css                     # 页面样式（深空极光，浅色优先 + 深色适配）
├── types.ts                         # 视图类型（TemplateNodeView/TemplateFamily/GuideKind）
├── utils.ts                         # 族树构建/可见节点/引导线计算/状态标签映射
├── components/
│   ├── TemplateFamilyCard.tsx       # 模板族卡片（族头 + 节点列表）
│   └── TemplateNodeRow.tsx          # 单个模板节点行（引导线/箭头/色条/标签/操作）
└── dialogs/
    ├── ImportTemplateDialog.tsx     # 导入模板包（分片上传/预览/确认）
    ├── PublishSettingsDialog.tsx    # 发布设置
    └── DeleteTemplateChainDialog.tsx# 删除模板链路（三种模式）
```

相关共享模块：

- `web/src/api/template.ts`：模板全部接口（列表/制作/预处理/分片上传/导入/导出/删除/发布设置）
- `web/src/utils/chunkUploader.ts`：通用分片上传器（迁移自旧前端，API 注入式，可复用于用户存储）
- `web/src/utils/templateCategory.ts`：模板分类常量与归一化（原 `views/vm/templateCategory.ts`，已上移共享，虚拟机表单/制作模板弹窗同步更新引用）

## 涉及接口

- `GET /template/list`：模板列表（树节点扁平结构，前端构建族树）
- `POST /template/:name/prepare-linux`：Linux 模板离线预处理（管理员）
- `POST /template/upload/init|chunk|complete`、`DELETE /template/upload`：模板包分片上传与临时包清理
- `POST /template/import/preview`、`POST /template/import/confirm`：导入解析预览与确认
- `POST /template/:name/export?scope=node|root`、`DELETE /template/:name/export`、`GET /template/download/:filename`：导出与下载
- `GET /template/:name/delete-preview`、`DELETE /template/:name`：删除预览与删除（428 高风险验证）
- `PUT /template/:name/publish`：发布设置

## 与旧版差异

1. 树状引导线在旧版中仅有 DOM 无样式（实际无缩进效果），新版补齐了真实的引导线渲染。
2. 导入上传新增「计算文件哈希」阶段进度提示（旧版仅显示上传进度）。
3. 节点行操作改为「发布设置图标 + ⋯ 下拉菜单」（防误触，与虚拟机列表 VmActionsCell 交互一致）；小屏下操作区自动换行。深色模式下族卡片高亮文字降对比为柔和灰（#b8c1cf），避免刺眼。
4. 非管理员访问时展示权限提示页（旧版依赖路由/菜单隐藏）。
