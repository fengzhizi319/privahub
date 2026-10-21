# SecretPad 前端架构设计文档（最终版）

> 本目录包含 SecretPad 前端现代化重构的最终设计规范、架构设计、目录结构及 UI 设计图。
> 架构目标：构建基于「Vite 5 + FSD 6层架构 + TanStack 生态 + Zustand + 设计系统」的现代化、模块化工业级 B 端前端。

---

## 文件索引

| 文件 | 说明 | 阅读顺序 |
|------|------|----------|
| [01-工业化前端设计要求.md](./01-工业化前端设计要求.md) | 工业级前端应满足的 13 个维度设计要求与评估标准 | 1 |
| [02-架构设计方案.md](./02-架构设计方案.md) | 最终架构设计方案、技术选型、分层规范与迁移路线 | 2 |
| [03-工程目录结构.md](./03-工程目录结构.md) | 完整目录结构（apps/packages/tooling）与依赖规则 | 3 |
| [04-UI设计图.html](./04-UI设计图.html) | 交互式 HTML 设计图：Dashboard + DAG 画布 + 架构图 | 4 |
| [迁移一致性保障指南](../docs/frontend-migration-consistency.md) | 旧前端功能/接口迁移到新前端的验证策略与 checklist | 5 |

---

## 如何查看 UI 设计图

```bash
# 方式 1：直接用浏览器打开
open secretpad/web/design/04-UI设计图.html

# 方式 2：启动本地静态服务器（推荐）
cd secretpad/web/design
python3 -m http.server 8080
# 访问 http://localhost:8080/04-UI设计图.html
```

设计图支持：
- 顶部 Tab 切换页面（Dashboard / DAG 画布 / 架构图）
- 右上角主题切换（亮色 / 暗色）
- 响应式布局

---

## 核心技术决策

| 决策点 | 选择 | 架构设计依据 |
|--------|------|--------------|
| 构建工具 | Vite 5 | 原生 ESM 秒级冷启动与极速 HMR |
| 路由机制 | TanStack Router | 端到端类型安全、loader 数据预加载与 searchParams 强类型化 |
| 服务端状态 | TanStack Query v5 | 统一管理 API 缓存、去重、失效、自动重试与乐观更新 |
| 客户端状态 | Zustand + Immer | 轻量可销毁 Store，避免单例污染，支持隔离与微前端扩展 |
| API 客户端 | openapi-fetch + Zod | 零运行时类型推导 + 运行时 Response Schema 严格校验 |
| 格式化工具 | Biome + ESLint 9 | 高性能代码格式化与 Flat Config 规范校验 |
| 架构分层 | FSD 6 层 (app/pages/widgets/features/entities/shared) | 业务领域切片、严格单向依赖与 Public API 屏障 |
| DAG 画布 | 独立包 `packages/dag-next` | 图引擎物理隔离，ESM 模块化，与主应用高内聚低耦合 |
| 迁移策略 | 灰度双轨 + Nginx 路径切流 | 旧新应用并行构建运行，低风险平滑渐进迁移 |
| 权限校验 | 路由/按钮统一双守卫 (`RequiredAuth` / `RequiredPermission`) | 收敛分散的权限判断，统一 RBAC 模型 |
| 系统可扩展性 | Monorepo + 微前端就绪 + 插件化 | 支持后续独立模块发布与功能拓展 |
| 国际化方案 | ICU MessageFormat | 标准化多语言文案管理与数字/日期本地化 |

---

## 实施步骤与建议

1. 初始化 `frontend-next` Monorepo 脚手架（Vite 5 + pnpm + Turborepo）。
2. 优先下沉落地 `packages/api-client` 与 `packages/design-system` 基础包。
3. 按照实施路线逐步迁移：P0 基础设施 → P1 核心主流程 → P2 复杂画布与功能 → P3 全链路加固。
4. 补齐 Playwright E2E 自动化测试与 Sentry / Web Vitals 可观测性监控。
