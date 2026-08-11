# raypx2-center UI：shadcn 重构 + NodeDetail 对齐本地控制台

**日期：** 2026-08-11  
**状态：** 已批准  
**范围：** `apps/raypx2-center/ui`  
**参考：** `/home/jack/src/raypx2/src/admin_console/`（本地 Admin Console 实用交互）

## 1. 目标

- 将 center SPA 风格统一到 **shadcn/ui**（`new-york` + zinc/neutral）。
- NodeDetail 信息架构与操作流对齐本地控制台的**实用子集**。
- Fleet 壳层换皮并补齐轻量 UX（Toast、AlertDialog、Sheet/Dialog、空态/加载）。

## 2. 已确认决策

| 项 | 决定 |
|---|---|
| 产品范围 | 风格 + 功能体验；非业务 API 大改 |
| NodeDetail IA | Overview / Peers / Connections / Tunnels / ACL(server) / Config / Audit |
| 不做 | Relay、Diagnostics、react-router、i18n |
| 视觉 | shadcn 默认产品风；浅色默认，可选暗色；不用旧青绿主题 |
| Fleet | 换皮 + 轻量 UX；Templates/Apply 业务语义不变 |
| 数据 | 继续 `proxyNode` / config / audit 现有契约 |

## 3. 信息架构

### 3.1 Fleet

- Login → AppShell（Sidebar + Topbar）
- Overview / Nodes / Templates / Apply Jobs
- Nodes → NodeDetail

### 3.2 NodeDetail 角色可见性

| Tab | client | server | unknown |
|---|---|---|---|
| Overview | 是 | 是 | 摘要 only |
| Peers | 可写 | 只读汇总 | 隐藏/空态 |
| Connections | 列表 + 详情 Sheet + pacing | 同左 | 隐藏/空态 |
| Tunnels | 只读 | 只读 | 隐藏/空态 |
| ACL | 隐藏 | 独立编辑 allow/deny | 隐藏 |
| Config | Form/JSON + revisions | Form/JSON + revisions | 只读提示 |
| Audit | 保留 | 保留 | 保留 |

## 4. 技术路径

1. `npx shadcn@latest init -d --base radix`（Vite + Tailwind v4）
2. AppShell：侧栏导航、顶栏 status pills、refresh pause/now、theme、sign out
3. 拆分 `pages/node-detail/*`；废除 Ops 聚合页
4. ACL 从 Config 表单迁出到独立 tab
5. 更新测试与 `ui/dist`

## 5. 非目标

- 不 iframe 嵌入本地 `/console/`
- 不改 PocketBase 核心；后端仅在 proxy 路径缺失时做最小补丁

## 6. 验证

- `npm test` / `npm run build`
- 手工：登录 → fleet → client/server NodeDetail 各 tab → 主题切换 → 删除确认
