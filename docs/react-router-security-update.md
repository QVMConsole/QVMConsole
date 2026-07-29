# React Router 安全更新

## 变更内容

- 前端路由依赖由 `react-router-dom` 7.18.1 升级为 `react-router` 8.3.0。
- 所有浏览器路由导入已切换至 `react-router`；现有 `createBrowserRouter`、`RouterProvider` 与导航 Hooks 的调用方式保持不变。

## 运行环境

React Router 8.3.0 要求 Node.js `22.22+`，请在安装或构建前确认运行环境满足该版本要求。

## 验证

在 `web` 目录执行：

```bash
npm audit
npm run build
```

预期 `npm audit` 不报告漏洞，构建命令成功完成。
