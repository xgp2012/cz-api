# cz-api 专属开发文档

> 面向本仓库（`github.com/xgp2012/cz-api`，代码实体为 **New API** / `github.com/QuantumNous/new-api`）的开发、调试与二次开发指南。
>
> - 分析基线：commit `e03439033238722c0b8d4437a052445f3aeeed83`（main 分支），全量分析于 2026-11-06。
> - 代码规模：约 2673 个受版本管理文件（不含 `web/node_modules`）；后端 Go ≈ 25 万行，前端 TS/TSX ≈ 27 万行，`relaykit` + `tokenkit` 两个独立子模块 ≈ 3.7 万行。
> - 本文档**以仓库实际代码为准**。官方在线文档 <https://docs.newapi.ai/zh> 可用作概念与运维参考，但其「本地开发」页仍在描述半年前的架构（`App.js`、`vite.config.js`、`semi design`、`bun.lockb`、`one-api.service`），**与实际代码不符，勿照做**。
> - 仓库内的权威规范文件（改动前必读）：根目录 `AGENTS.md`、`web/AGENTS.md`、`.agents/rules/billing.md`、`docs/plugin-api/v1.md`。
> - **必备工具**：`git`、**`gh`（GitHub CLI，强制使用）**、Go ≥ 1.26.10、Bun 1.4.4、Docker + BuildKit。`gh` 是本仓库远端状态的唯一事实来源，详见 [18.6](#186-必备工具与强制使用-gh)。

---

## 目录

1. [项目定位与整体架构](#1-项目定位与整体架构)
2. [技术栈与模块拓扑](#2-技术栈与模块拓扑)
3. [目录地图](#3-目录地图)
4. [请求全链路：一次 relay 请求发生了什么](#4-请求全链路一次-relay-请求发生了什么)
5. [路由体系](#5-路由体系)
6. [中间件](#6-中间件)
7. [认证与授权](#7-认证与授权)
8. [渠道适配层与新增渠道](#8-渠道适配层与新增渠道)
9. [协议转换（relaykit）](#9-协议转换relaykit)
10. [计费体系](#10-计费体系)
11. [数据层与缓存](#11-数据层与缓存)
12. [配置体系](#12-配置体系)
13. [多节点与主从](#13-多节点与主从)
14. [任务插件系统](#14-任务插件系统) — 含 [14.2.1 示例插件](#1421-可复制的示例插件examplesplugins)、[14.3 三种 kind 契约](#143-插件契约docsplugin-apiv1md权威定义-v1schemajson--v1dts)、[14.3.2 job 桥接](#1432-kind-job-的系统任务桥接)
15. [前端开发](#15-前端开发)
16. [Electron 桌面端](#16-electron-桌面端)
17. [构建、测试与部署](#17-构建测试与部署)
18. [开发工作流与硬性规范](#18-开发工作流与硬性规范) — 含 [18.5 构建与流水线效率（强制）](#185-构建与流水线效率强制)、[18.6 必备工具与强制使用-gh](#186-必备工具与强制使用-gh)
19. [常见任务操作手册](#19-常见任务操作手册)
20. [坑点与雷区总表](#20-坑点与雷区总表)
21. [附录](#21-附录)
22. [变更记录](#22-变更记录文档本身)

---

## 1. 项目定位与整体架构

**New API 是一个自托管的 AI API 网关（AI Gateway）**：把 40+ 家上游模型服务（OpenAI、Anthropic Claude、Google Gemini/Vertex、Azure、AWS Bedrock、DeepSeek、通义千问、豆包、Kling、Sora 等）统一到一套 API 后面，并提供用户管理、令牌、计费、限流、渠道调度、异步任务与管理控制台。

它同时是**多协议转换器**：客户端可以用 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages、Gemini `generateContent` 中的任意一种格式进来，网关按需转换成上游需要的格式，再把响应转回来。

```
                    ┌───────────────────────────────────────────────┐
   客户端 / SDK       │              New API (Go / Gin)               │
   ─────────────►   │                                               │
   /v1/chat/...      │  路由 → 中间件 → 认证 → 渠道选择 → 适配器      │   ──────►  上游 40+ 厂商
   /v1/messages      │                        ↓                      │
   /v1beta/models    │              协议转换 (relaykit)              │   ◄──────  响应 / SSE 流
   /v1/responses     │                        ↓                      │
   /v1/tasks         │        计费预扣 → 转发 → 结算/退款            │
                    │                        ↓                      │
                    │       日志落库 / 缓存 / 系统任务              │
                    └───────────────────────────────────────────────┘
                                    ▲
                        React 控制台（web/，被 go:embed 内嵌）
```

核心设计取向：

- **单体优先**：一个 Go 二进制 + 内嵌前端，无外部服务强依赖（SQLite 即可起）。
- **全局状态换性能**：渠道、令牌、用户、选项、定价全部缓存在进程内存中，靠周期全量重载 + TTL 在集群间最终一致。
- **计费安全优先**：所有额度换算集中在 `common/quota_math.go`，禁止裸类型转换；饱和会被审计。
- **插件化扩展**：新增视频/图片类厂商不必改 Go 代码，写一个 JS 任务插件即可。

---

## 2. 技术栈与模块拓扑

### 2.1 后端

| 项 | 版本 / 说明 |
|---|---|
| Go（根模块） | `go 1.26.10` |
| Go（`relaykit/`） | `go 1.25.1` |
| Go（`tokenkit/`） | `go 1.26` |
| Web 框架 | Gin `v1.9.1` |
| ORM | GORM v2 `gorm.io/gorm v1.25.12` |
| 数据库 | SQLite（`glebarez/sqlite`，纯 Go）、MySQL `>= 5.7.8`、PostgreSQL `>= 9.6`；**日志库额外支持 ClickHouse** |
| 缓存 | Redis（`go-redis/redis/v8`）+ 进程内存（`samber/hot`） |
| 计费表达式 | `expr-lang/expr v1.17.8` |
| JS 插件运行时 | `github.com/Calcium-Ion/moejs v0.1.0-alpha.7` |
| 认证相关 | `golang-jwt/jwt/v5`、`go-webauthn/webauthn`、`pquerna/otp`（TOTP）、`casbin/v2` |
| 支付 | `stripe/stripe-go/v81`、`Calcium-Ion/go-epay`、`waffo-com/waffo-go` |
| 其它 | `tidwall/gjson`、`shopspring/decimal`、`grafana/pyroscope-go`、`shirou/gopsutil` |

模块拓扑（**三个独立 Go module，`go.work` 未提交，必须 `GOWORK=off`**）：

```
new-api (根模块 github.com/QuantumNous/new-api)
├── replace github.com/QuantumNous/new-api/relaykit => ./relaykit
└── replace github.com/QuantumNous/new-api/tokenkit => ./tokenkit
```

- **`relaykit/`** — 协议 DTO 与协议间转换，**不得** import 根模块任何包，也不得依赖根模块配置/生成物。
- **`tokenkit/`** — 文本/图片 token 计数与估算，**不得**依赖根模块，也**不得**依赖 `relaykit`。
- 三者都必须能独立构建：改动后必须在该模块内跑 `GOWORK=off go build ./...`。**根模块编译通过不能替代这个验证。**

### 2.2 前端

| 项 | 版本 |
|---|---|
| React / React DOM | `^19.2.7` |
| 构建 | Rsbuild `^2.1.4`（`@rsbuild/core`）+ `@rsbuild/plugin-react` |
| 路由 | `@tanstack/react-router ^1.170.17`（文件式路由） |
| 数据 | `@tanstack/react-query ^5.101.2` + `axios ^1.20.0` |
| 表格/虚拟滚动 | `@tanstack/react-table ^8.21.3`、`@tanstack/react-virtual ^3.14.5` |
| 状态 | `zustand ^5.0.14` |
| UI | `@base-ui/react ^1.6.0`（shadcn `style: base-nova`）、`Hugeicons` |
| 样式 | `tailwindcss ^4.3.2`、`clsx`、`class-variance-authority`、`tailwind-merge` |
| i18n | `i18next ^26.5.3`、`react-i18next ^17.1.0` |
| 表单 | `react-hook-form ^7.80.0` + `zod ^4.4.3` |
| 图表 | `@visactor/vchart`、`@visactor/react-vchart` |
| 工具链 | `oxlint ^1.72.0`、`oxfmt ^0.72.0`、`vitest ^5.0.4`、`knip` |
| 包管理器 | **Bun**（`packageManager: bun@1.4.4`；Dockerfile 用 `oven/bun:1.4.4`） |

> **没有 ESLint，也没有 `typescript` 直接依赖。** `makefile` 里的 `DISABLE_ESLINT_PLUGIN` 是历史遗留物。类型检查由 `bun check -b` 完成（走 `tsconfig.json` 的 project references）。

### 2.3 桌面端

Electron `42.11.16` + `electron-builder ^26.15.3`，单文件主进程 `electron/main.js`（CJS，无 TS、无构建步骤），生产模式把 Go 二进制作为子进程拉起。

---

## 3. 目录地图

```
cz-api/
├── main.go                     # 入口：初始化资源、启动 HTTP、注册后台任务、优雅退出
├── go.mod / go.sum             # 根模块
├── makefile                    # 本地开发/构建/测试入口
├── VERSION                     # 版本号文件（本检出为空，靠构建期注入）
├── Dockerfile / Dockerfile.dev
├── docker-compose.yml / docker-compose.dev.yml
├── new-api.service             # systemd 单元
├── .env.example                # 环境变量样例（57 个键，全为注释形式；代码实际读取 108 个变量名）
├── AGENTS.md / CLAUDE.md       # 项目规范（必读）
├── .agents/
│   ├── rules/billing.md        # 计费强制规范（billing 相关改动前必读）
│   └── skills/                 # shadcn-ui、i18n-translate、vercel-react-best-practices
│
├── router/                     # 路由注册（19 文件）
├── middleware/                 # 中间件（43 文件）
├── controller/                 # HTTP 处理器（136 文件）
├── service/                    # 业务服务（116 文件，含 authz/、passkey/）
├── model/                      # GORM 模型与数据访问（117 文件）
├── relay/                      # 上游中转核心（256 文件）
│   ├── channel/                #   40 个渠道适配器目录（35 个有 adaptor.go）+ task/
│   ├── common/                 #   RelayInfo、出站体、覆盖、流状态
│   ├── helper/                 #   请求校验、价格、模型映射、流扫描
│   ├── constant/
│   └── common_handler/
├── relaykit/                   # 【独立 module】协议 DTO 与转换
├── tokenkit/                   # 【独立 module】token 计数
├── common/                     # 全局工具与环境变量（69 文件）
├── setting/                    # 配置中心（11 个子包）
├── constant/                   # 常量：渠道类型 + channel_registry.go 渠道目录、API 类型、任务类型
├── dto/                        # 请求/响应 DTO
├── types/                      # 价格等共享类型
├── oauth/                      # OAuth/OIDC provider 注册表
├── i18n/                       # 后端 i18n（en / zh-CN / zh-TW）
├── logger/                     # 日志
├── pkg/                        # billingexpr、cachex、ionet、jsplugin、perf_metrics、wsmanager
├── plugins/                    # 内置 JS 任务插件（tasks/ 下 11 个）
├── examples/plugins/           # 6 个可加载的插件示例（不是内置插件，见 §14.2.1）
├── web/                        # React 前端（构建产物 web/dist 被 go:embed 内嵌）
├── electron/                   # 桌面壳
├── docs/                       # 鉴权说明、渠道配置、翻译术语表、OpenAPI、插件 API 契约
├── integration/                # 跨层集成测试（doc_parse_test.go）
├── assets/                     # README 截图
└── .github/workflows/          # CI（6 个 workflow）
```

> 已移除的目录：`e2e/` 已并入 `integration/`（其内容只是一次文档解析集成校验，不是端到端套件）；`bin/` 下的历史迁移 SQL 与脚本已删除。新增目录时请同步更新本图。

---

## 4. 请求全链路：一次 relay 请求发生了什么

以 `POST /v1/chat/completions` 为例（这是理解整个后端的最高价值路径）：

**阶段 0 — 引擎级中间件**（`main.go`）

```
ConfigureTrustedProxies → PanicRecover → RequestId → Version → I18n → SetUpLogger
```

注意：**引擎级 gzip 被显式注释禁用**，源码注释写着 `This will cause SSE not to work!!!`。

**阶段 1 — 路由与组级中间件**（`router/relay-router.go`）

```
CORS → DecompressRequestMiddleware → BodyStorageCleanup → StatsMiddleware
RouteTag("relay") → SystemPerformanceCheck() → TokenAuth() → ModelRequestRateLimit() → Distribute()
```

**阶段 2 — 请求校验与计费预扣**（`controller/relay.go` → `controller.Relay(c, types.RelayFormat)`）

1. Realtime 走 WebSocket 升级分支；
2. 注册错误渲染 defer（`ToOpenAIError` / `ToClaudeError` / `writeRelayStreamError`，按格式选）；
3. `helper.GetAndValidateRequest` 解析并校验请求体；
4. `relaycommon.GenRelayInfo` 构建 `RelayInfo`；
5. `relay.PrepareRequestBilling` —— 敏感词检查 → `service.EstimateRequestToken` → `info.SetEstimatePromptTokens` → `helper.ModelPriceHelper` 算价 → `service.PreConsumeBilling` 预扣；
6. 同时 defer `relay.RefundFailedRequestBilling`，失败时退款 + 违规扣费。

**阶段 3 — 渠道选择与重试循环**

```go
for retryParam.GetRetry() <= common.RetryTimes {
    getChannel(...)                                  // 选择渠道，绑定上下文
    service.AppendUsedChannel(...)
    service.PrepareTieredBillingForSelectedGroup(...) // 换组时刷新阶梯计费 GroupRatio
    common.GetBodyStorage(c).Seek(0, io.SeekStart)   // 重试前重置 body
    // 按 info.RelayMode 分派
    relay.WssHelper / relay.ClaudeHelper / geminiRelayHandler / relayHandler
}
```

失败走 `service.DecideRelayRetry` + `service.ProcessChannelError`（后者决定是否自动禁用渠道）。

**阶段 4 — 单次转发**（`relay.TextHelper`，定义在 `relay/compatible_handler.go`）

```
info.InitChannelMeta(c)              // 重置 per-attempt 状态
common.DeepCopy(...)                 // 避免污染原始请求
helper.ModelMappedHelper(...)        // 模型名映射
helper.ApplyReasoningModelSuffix(...) // 推理后缀处理
StreamOptions 裁剪                    // 依据 streamSupportedChannels
GetAdaptor(info.ApiType) → adaptor.Init(...)
adaptor.ConvertOpenAIRequest(...)    // 协议转换
relaycommon.RemoveDisabledFields / ApplyParamOverrideWithRelayInfo
relaycommon.NewOutboundJSONBody(...)
adaptor.DoRequest(...)               // 真正发往上游
service.RelayErrorHandler(...)
adaptor.DoResponse(...)              // 返回 *dto.Usage
service.PostTextConsumeQuota(...)    // 结算
```

**阶段 5 — 结算与日志**

```
service.PostTextConsumeQuota
  → service/billing_usage.go  effectiveBillingUsage()      // 统一 usage 语义
  → service/text_quota.go     calculateTextQuotaSummary()  // decimal 计价
  → service/tiered_settle.go  BuildTieredTokenParams + TryTieredSettle
  → service.SettleBilling()                                // 差额结算
  → model.RecordConsumeLog()                               // 落日志库
```

### `RelayInfo`：单请求全生命周期上下文

定义在 `relay/common/relay_info.go`，是整个 relay 层的核心数据结构。关键字段：

| 字段 | 作用 |
|---|---|
| `Request dto.Request` | 客户端原始请求 |
| `RelayFormat` / `RelayMode` / `IsStream` | 决定走哪个 Helper |
| `OriginModelName` / `BillingModelName` | **路由用名与计价身份分离** |
| 内嵌 `*ChannelMeta` | `ChannelType`/`ChannelId`/`ApiType`/`ApiKey`/`ChannelBaseUrl`/`ParamOverride`/`ChannelSetting`/`SupportStreamOptions` |
| `PriceData hosttypes.PriceData` | 价格与倍率 |
| `TieredBillingSnapshot *billingexpr.BillingSnapshot` | 冻结的表达式快照 |
| `BillingRequestInput *billingexpr.RequestInput` | 表达式请求输入 |
| `BillingImageCount *int` | 可被上游响应修正的图片数量 |
| `Billing BillingSettler` | 预扣/结算/退款会话 |
| `ForcePreConsume` / `FinalPreConsumedQuota` | 强制预扣（异步任务）与最终预扣额 |
| `QuotaClamp *common.QuotaClamp` | 饱和审计标记 |
| `RequestConversionChain` / `FinalRequestRelayFormat` | 转换链（多跳） |
| `StreamStatus` | 流式终止状态 |
| `RetryIndex` / `LastError` | 重试信息 |

> ⚠️ `InitChannelMeta` **只应重置 per-attempt 状态**，不得清空 `StreamStatus`、`PriceData` 或计费累计量。

---

## 5. 路由体系

`main.go` → `router.SetRouter(server, router.WebAssets{...})`，挂载顺序见 `router/main.go`：

```
SetApiRouter → SetDashboardRouter → SetRelayRouter → SetTaskPluginProtocolRouter
→ SetVideoRouter → SetTaskRouter → SetPluginRouter → SetWebRouter
```

若设置了 `FRONTEND_BASE_URL`（且非 master 节点），最后的 `SetWebRouter` 换成 301 重定向的 `NoRoute`。

> ⚠️ **在 `SetRouter` 之前，`main.go` 已挂了引擎级中间件**，它们对**每个**路由组都最先执行：
> `PanicRecover` → `RequestId` → `Version` → `I18n` → `SetUpLogger`（内含请求体敏感参数脱敏）。
> 因此下表「中间件链（执行序）」描述的是**组内**顺序，前面还叠着这 5 层。排查顺序问题时别漏掉它们。

> Gin 要求同名通配段一致，因此 `/v1` 在多个文件中被重复分组注册，这是**有意的**，不要"重构合并"。

| 前缀 | 文件 | 中间件链（执行序） | 作用 |
|---|---|---|---|
| `/api` | `api-router.go` | `RouteTag("api")` → `gzip` → `AccessTokenAudit()` → `BodyStorageCleanup()` → `GlobalAPIRateLimit()` | 控制台 API |
| `/`（旧） | `dashboard.go` | `RouteTag("old_api")` → `gzip` → `GlobalAPIRateLimit` → `CORS` → `TokenAuth` | `/dashboard/billing/*`、`/v1/dashboard/billing/*` |
| `/v1/models`、`/v1beta/models`、`/v1beta/openai/models` | `relay-router.go` | `RouteTag("relay")` → `TokenAuth` | 模型列表（按 header 选协议形态） |
| `/v1` | `relay-router.go` | `RouteTag` → `SystemPerformanceCheck` → `TokenAuth` → `ModelRequestRateLimit`（`/realtime` 与 HTTP 子组再各自 `Distribute()`） | 主转发入口 |
| `/pg` | 同上 | ... → `UserAuth` → `Distribute` | Playground |
| `/mj`、`/:mode/mj` | `registerMjRouterGroup` | `RouteTag` → `SystemPerformanceCheck`；`GET /image/:id` 优先于 `TokenAuth`+`Distribute` | Midjourney |
| `/v1beta` | 同上 | 内联短路（`:countTokens`→`RelayNotFound`，`:predictLongRunning`→`defaultPluginDispatcher.dispatch`）→ 常规链 | Gemini 原生 |
| `/v1/tasks` | `task-router.go` | 提交 `PrepareTaskPluginSubmit`+`Distribute`；读取 `TokenAuth`；内容 `TokenOrTaskArtifactAccessAuth("key","artifact_key")` | 任务插件通用接口 |
| `/v1/video(s)` | `video-router.go` | `PinTaskPluginEndpoint` → `TaskPluginEndpointOnly(ModelRequestRateLimit)` → `PrepareTaskPluginEndpoint` → `Distribute` | 视频任务 |
| 宿主协议 | `task-plugin-protocol-router.go` | 按 `HostProtocols()` 展开（`/v1/responses`、`/v1/images/*`、`/v1/videos*`） | 插件协议接管 |
| 插件动态 | `plugin-router.go` | 外层 `pinRoute`→`NativeRouteCredential`→`TokenAuth`→`SystemPerformanceCheck`→`ModelRequestRateLimit`→`PrepareTaskPluginRoute`→`Distribute`→`RelayTask` | 由 `jsplugin.RoutingGeneration` 生成 |
| 兜底 | `web-router.go` | `pluginDispatcher` → `RouteTag("web")` → 条件 gzip → `AccessTokenAudit` → `GlobalWebRateLimit` → `Cache` → `ServeFrontendFiles` | SPA；`/v1`、`/api`、`/assets` 回落 `RelayNotFound` |

`/v1` 下由 **`relay-router.go`** 注册的端点：`/messages`(Claude)、`/chat/completions`、`/completions`、`/responses/compact`、`/alpha/search`、`/tools/{search,fetch}`、`/embeddings`、`/engines/:model/embeddings`、`/edits`、`/audio/*`、`/rerank`、`/models/*path`、`/moderations`、`/realtime`、`GET /responses`（WebSocket）。

> ⚠️ **`/v1/images/*`、`/v1/videos*`、`POST /v1/responses` 等不在 `relay-router.go`**：它们由 `task-plugin-protocol-router.go` 依据 `jsplugin.HostProtocols()` 用 `router.Handle` **动态注册**（`openai_image` / `openai_video` / `openai_responses` 三个宿主协议）。要改这些路径的行为，改的是协议表或 `task-plugin-protocol-router.go`，不是 `relay-router.go`。`relay-router.go:126-130` 的注释也说明了这点：未被插件认领的模型才回落到 `Relay`。
>
> 另有 `POST /v1/edits`（回落，OpenAI Images 旧形态）与 `POST /v1/images/edits`（宿主协议）**两条并存**，不要混淆。

> **曾经「声明但不实现」的路由已删除**：`/images/variations`、`/files*`、`/fine-tunes*`、`/models/:model` 过去被注册为返回 501 的 `controller.RelayNotImplemented`，现已从 `relay-router.go` 移除，该函数也随之删除。现在访问这些路径会落到兜底的 `RelayNotFound`（404），而不是 501——如果客户端曾依赖 501 判断能力，需改为 404。**不要再新增这种空路由**：要么真实实现，要么不注册。

`/api/channel/*` 的 45 条权限路由集中在 `channel-router.go:channelPermissionRoutes`，由 `handlePermissionRoute` 同时声明 PAT scope 与 Casbin 权限。

> ⚠️ `POST /api/channel/:id/key` **不在** `channelPermissionRoutes` 里：它单独注册、走 `RootAuth` + `CriticalRateLimit` + `DisableCache` + `SecureVerificationRequired`，其 PAT scope 是在 `middleware/access_token_routes.go` 的静态表里**手工声明**的。新增渠道路由时若放在 loop 之外却忘了补静态表，会得到**静默的 403**。

插件元数据查询（只读，全部 `RootAuth`，PAT 需要 `plugin:read`）：`/api/plugin`、`/api/plugin/kinds`、`/api/plugin/protocols`、`/api/plugin/:key`。

> ⚠️ `GET /api/plugin/:key` 与既有的 `GET /api/plugin/task/:key` 同时存在，新增子路径时注意 gin 的通配段冲突规则。

---

## 6. 中间件

按重要性排序说明，全部位于 `middleware/`。

### `auth.go` — 认证与令牌
- 角色体系：`UserAuth`(>=1) / `AdminAuth`(>=10) / `RootAuth`(==100)，常量 `common.RoleCommonUser=1`、`RoleAdminUser=10`、`RoleRootUser=100`、`RoleGuestUser=0`。
- **控制台凭证判定顺序** `classifyDashboardCredential`：
  1. `service.ParseDashboardAccessToken` —— 只看 `issuer`/`audience`/`token_use`，**不验签**（目的是抢先识别"内部 JWT"，避免误落到 PAT 路径，真正验签在后面）；
  2. `nap_` 前缀 PAT → `model.FindUserAccessTokenByHash`（SHA-256 指纹）；
  3. legacy 不透明 token → 可能返回 `ErrLegacyAccessTokenRetired`。
- **`enforceAccessTokenRoute` 对 PAT 是 fail-closed**：用 `METHOD + c.FullPath()` 查规则，**未声明即 403**（`ACCESS_TOKEN_ROUTE_UNDECLARED`，同时写一条服务端日志）。
  - ⚠️ **legacy 不透明 token 在 `switch` 里先于 `!declared` 命中并 `return true`**，因此未声明路由对它放行。fail-closed 只覆盖 `nap_` 前缀的 PAT；legacy 路径本身正在退役（`ErrLegacyAccessTokenRetired`）。
  - 规则来源有**两张表**：静态的 `accessTokenRouteRules`，以及路由注册期由 `DeclareAccessTokenPermissionRoute` 填充的 `permissionRouteRules`；查询时合并。**重复或冲突声明会 panic**——这是必须遵守的不变量，改路由注册代码时最容易踩。
  - 规则有三种：需要 scope、`any`（任意有效 scoped token，由 handler 再判）、`session`（**拒绝所有** token，含 legacy，用于 `/api/user/access_tokens*` 与 `/pg/chat/completions`）。
- `TokenAuth`（API Token，供 `/v1` 使用）取值顺序：
  1. WebSocket 子协议 `openai-insecure-api-key.`；
  2. `x-api-key`（`/v1/messages`、`/v1/models`）；
  3. `?key=` / `x-goog-api-key`；
  4. `mj-api-secret`；
  5. 去 `sk-` 前缀，**按 `-` 取首段**；
  6. `model.ValidateUserToken` → IP CIDR 校验 → 分组校验 → `SetupContextForToken`（管理员用 key 第二段 pin 指定渠道，`dto.PinSourceToken`）。
- `TokenAuthReadOnly`：放行"过期/耗尽"的 token，仅拒绝禁用、IP 不符与封禁（用于只读端点）。

### `distributor.go` — 渠道分发
`Distribute()` 先追加 `FilterRequestPath` 与 `AppendTaskPluginIdentityFilter`，再 `getModelRequest` 按路径解析模型名（Gemini 的 `:action`、`/v1/realtime?model=`、audio 缺省 `whisper-1`/`tts-1`、`moderations` 默认值等）。非 pin 时校验 token 模型白名单 `TokenModelLimitAllows`（三态匹配：原名 / `FormatMatchingModelName` / `RoutingMatchModelName`），然后 `service.SelectChannelForRequest`，最后 `SetupContextForSelectedChannel` 写入 `ContextKeyChannelId/ChannelKey/ChannelBaseUrl/ChannelModelMapping/ChannelParamOverride`。`channel.GetNextEnabledKey()` 支持多 key 轮转。`<400` 时 `service.RecordChannelAffinity`。

### `rate-limit.go` / `model-rate-limit.go` — 两套限流
- `rate-limit.go`：Redis **固定窗口** Lua 脚本（`rateLimit:v2:ip|user:<mark>:...`）。IP 维度标记 `GA`/`GW`/`GS`/`CT`/`DW`/`UP`；用户维度 `UC:<scope>`、`SR`。内存版为 `common.InMemoryRateLimiter`。超限输出 429 + `Retry-After`。
- `model-rate-limit.go`：模型请求数限流，**双计数且两者算法不同**：
  - `MRRL`（总请求）用 **GCRA 令牌桶**（`common/limiter`，键 `rateLimit:<subject>`）；`totalMaxCount == 0` 时整体跳过；
  - `MRRLS`（成功请求）用 **Redis List 存毫秒时间戳**（`LLen`/`LIndex`/`LPush`/`LTrim`）。
  - 内存版同样不一致：总请求走 `Request`，成功走 `Reserve`/`Complete`。
  - 改成"都用 List"或"都用令牌桶"会改变限流语义，**调参要分别改**。

> `TokenAuthReadOnly` 只从 `Authorization` 取凭证（剥 `Bearer `/`sk-` 后按 `-` 截断），**不读** `x-api-key`、`?key=`、`x-goog-api-key`、`mj-api-secret` 或 WebSocket 子协议，也不设置分组/模型上限上下文。不要把它当成 `TokenAuth` 的"宽松版"用在需要完整凭证来源的路由上。

### 其它
| 中间件 | 职责 |
|---|---|
| `logger.go` | 仅 gin 访问日志格式化（`SetUpLogger`）；**不落库**。落库在 `service/log_info_generate.go` + `model.RecordConsumeLog` |
| `cors.go` | `AllowAllOrigins=true` 与 `AllowCredentials=true` 同时开启（已知取舍）；`Version()` 写 `X-New-Api-Version` |
| `gzip.go` | `DecompressRequestMiddleware` 解 gzip/br/zstd；解压后 `http.MaxBytesReader(constant.MaxRequestBodyMB)`；zstd 限窗 8MiB、`WithDecoderConcurrency(1)` |
| `request_body_limit.go` | 匿名请求全量读入，超限 413 |
| `secure_verification.go` | `SecureVerificationRequired()` 消费一次性 `X-Security-Proof`（带 scope 与 context hash）；错误码 `SECURITY_PROOF_*` |
| `turnstile-check.go` | Cloudflare `siteverify`，失败返回 200 + `success:false` |
| `audit.go` | `auditResponseWriter`（64KiB 截断）；`TokenOperationAudit`（6 类 token 动作）；`AccessTokenAudit`（认证前捕获拒绝原因） |
| `performance.go` | `SystemPerformanceCheck`，对 `/v1/messages` 输出 Claude 结构错误 |
| `recover.go` | `PanicRecover` / `RelayPanicRecover` |
| `request-id.go` | `X-Oneapi-Request-Id` 同时写入 header 与 ctx |
| `frontend_static.go` | 静态文件按文件 gzip 一次后缓存进 `sync.Map` |
| `email-verification-rate-limit.go` | 标记 `EV`，30s / 2 次 |
| `auth_origin.go` | `SessionCookieOriginGuard`，**仅在 `SESSION_COOKIE_SECURE=true` 时生效**；它比对的来源白名单是 `SESSION_COOKIE_TRUSTED_URL` |
| `task_plugin.go` | 约 1800 行。`PinTaskPluginEndpoint`/`PrepareTaskPluginEndpoint`/`PrepareTaskPluginRoute`/`TaskPluginEndpointOnly`/`NativeRouteCredential`；按请求体 `stream`/`background` 过滤候选插件，并把请求体 `model` 重写为 `pinnedEndpoint.Model` 以对齐计费名 |

> **`SecureVerificationRequired` 不是通用二次验证闸门**：它写死了 `VerificationScopeChannelKeyRead` 并从 `c.Param("id")` 取渠道 ID，目前只用于 `POST /api/channel/:id/key`。其它敏感操作在自己的 controller 里校验各自的更窄 scope。想复用到没有 `:id` 的路由必须先改这个中间件。
>
> PAT 能通过二次验证，是因为 scoped PAT 会被赋予一个合成会话 `pat:<id>`（`SessionVersion=1`），`GetStepUpIdentity` 据此接受它；没有这层，PAT 永远过不了 step-up。


---

## 7. 认证与授权

| 机制 | 位置 | 要点 |
|---|---|---|
| 浏览器会话 | `service/auth_session.go` | Cookie `new_api_refresh` + 提示 Cookie `new_api_has_session`；**全部写为 `HttpOnly` + `SameSite=Strict`**；受 `USER_SESSION_ACTIVE_LIMIT`(50) 限制；`ValidateLoginSession` 校验 `sid` + 用户版本 + 会话版本；刷新重放窗口 `RefreshReplayWindow=30s` |
| 短期 JWT | `service/auth_token.go` | HS256，密钥由 `SESSION_SECRET` HMAC 派生；`AccessTokenTTL=15m`；`issuer="new-api"`、`audience="new-api-dashboard"` |
| API Token | `middleware/auth.go:TokenAuth` | `sk-` 前缀（非强制）+ 多来源 header；key 按 `-` 截断 |
| PAT | `model/user_access_token.go` + `middleware/access_token_routes.go` | 前缀 `nap_`；会话前缀 `pat:`；scope 表按路由声明，fail-closed |
| Passkey / TOTP | `controller/passkey.go`、`controller/twofa.go` | `/api/user/passkey/*`、`POST /api/user/login/2fa`、`/2fa/{setup,enable,disable,backup_codes}` |
| OAuth / OIDC | `oauth/` | `Provider` 接口 + 注册表；`github`/`discord`/`linuxdo`/`oidc`/`telegram` 在各自 `init()` 里 `Register(...)`；DB 里的自定义 provider 经 `oauth.LoadCustomProviders()`；`RegisterCustom` 拒绝与内置 slug 冲突 |
| 密码存储 | `common/crypto.go` 等 | **默认 argon2id**（`m=19456,t=2,p=1`），`ACCOUNT_PASSWORD_HASH_ALGORITHM` 可切到 bcrypt 做滚动升级；bcrypt 拒绝 >72 字节口令（`ErrPasswordLegacyLimit`）；可选登录密码 RSA-OAEP 信封（`PASSWORD_LOGIN_ENCRYPTION_ENABLED`） |

### Casbin（`service/authz/`）

- 模型 `modelText` 为 `r = sub, obj, act` / `p = sub, obj, act, eft`，matcher 是**精确相等**，**没有 RBAC 继承**。
- `Init(db)`：master 节点先 `seedBuiltInRoles` + `resetBuiltInRolePolicies`，再 `NewSyncedEnforcer(m, newGormAdapter(db))`、`EnableAutoSave(true)`，最后 `seedDefaultPolicies()`。
- `StartPolicySync(frequency)` 由 `main.go` 以 `common.SyncFrequency` 起协程周期 `ReloadPolicy()` —— 这是多节点权限变更传播的机制。
  - ⚠️ `frequency <= 0` 时该协程**直接 return 且不打日志**：`SYNC_FREQUENCY=0` 会静默关闭多节点权限传播。
  - `ReloadPolicy` 取写锁并整表重载（不是增量），期间 `Can` 被阻塞。
- `resolver.Can` 判定顺序：`resolveSubjectRoles`（`root` 为 Superuser）→ superuser 短路 → `isKnownPermission` → 用户级 override（`user:<id>`，deny 优先）→ 角色基线并集（`role:<key>`）。
  - 未知权限**静默返回 false**（无日志），但 root 已先行短路，所以只有管理员会被拒。
  - **deny 优先是 Go 侧实现的**：`Can` 不用 `Enforce`，而是 `GetFilteredPolicy` 后按 deny 优先合并。因此**新增授权调用点若直接用 `e.Enforce(...)`，会绕过 deny-override**。matcher 里的 `p.eft == "allow"` 也让 deny 行根本匹配不上。
- 主体→角色**只看 `User.Role` 数值**（`>=100`→root、`>=10`→admin、其余无角色），`userID` 参数未使用，没有按用户分配角色；普通用户走 `UserAuth` + PAT scope，不经过 Casbin。
- 资源在 `init()` 注册：`resources_channel.go`（`read`/`operate`/`write`/`sensitive_write`/`secret_view`）、`resources_audit.go`、`resources_task_plugin.go`。
  - `RegisterResource` 加了读写锁并**去重**：同名资源重复注册**静默忽略后者**（含其 `DefaultRoles`）。由于注册都在 `init()`，顺序决定谁生效。

---

## 8. 渠道适配层与新增渠道

### 8.1 `Adaptor` 接口（`relay/channel/adapter.go`）

```go
Init, GetRequestURL, SetupRequestHeader,
ConvertOpenAIRequest, ConvertRerankRequest, ConvertEmbeddingRequest,
ConvertAudioRequest, ConvertImageRequest, ConvertOpenAIResponsesRequest,
ConvertClaudeRequest, ConvertGeminiRequest,
DoRequest, DoResponse, GetModelList, GetChannelName
```

任务类另有 `TaskAdaptor`：`ValidateRequestAndSetAction`、`EstimateBilling`、`AdjustBillingOnSubmit`、`AdjustBillingOnComplete`、`BuildRequestURL/Header/Body`、`ParseResponse`、`FetchTask`、`ParseTaskResult`；可选能力接口 `TaskUsageFactsProvider`、`TaskValidatedBillingProvider`、`TaskValidatedUsageFactsProvider`、`OpenAIVideoConverter`、`TaskArtifactProvider`。

### 8.2 渠道清单（`constant/channel.go`，53 个 `ChannelType*` 常量）

`relay/channel/` 下 40 个目录，其中 **35 个有 `adaptor.go`**：

- **有 `adaptor.go`**：`openai`(1)、`ollama`(4)、`palm`(11)、`claude`(14)、`baidu`(15)、`zhipu`(16)、`ali`(17)、`xunfei`(18)、`tencent`(23)、`gemini`(24)、`moonshot`(25，复用 Claude API)、`zhipu_4v`(26)、`perplexity`(27)、`aws`(33)、`cohere`(34)、`minimax`(35)、`dify`(37)、`jina`(38)、`cloudflare`(39)、`siliconflow`(40)、`vertex`(41)、`mistral`(42)、`deepseek`(43)、`mokaai`(44)、`volcengine`(45)、`baidu_v2`(46)、`xai`(48)、`coze`(49)、`jimeng`(51)、`submodel`(53)、`replicate`(56)、`codex`(57)、`advancedcustom`(58)、`sub2api`(59)、`newapi`(60)
- **无 `adaptor.go`，复用 `openai.Adaptor`**：`ai360`(19)、`lingyiwanwu`(31)、`openrouter`(20)、`xinference`(47)
- **仅任务侧常量**：`Midjourney`(2)、`Azure`(3)、`SunoAPI`(36)、`Kling`(50)、`Vidu`(52)、`DoubaoVideo`(54)、`Sora`(55)、`TaskPlugin`(61)、`VLLM`(62)、`SGLang`(63)

> **空槽位**：`0、5、6、7、9、12、13、21、22、28–30、32` 中有多个是历史遗留的已删渠道。已删的是 `OpenAIMax`(6)、`OhMyGPT`(7)、`AILS`(9)、`API2GPT`(12)、`AIGC2D`(13)、`FastGPT`(22)。**删除时保留空槽、绝不重排既有数值**——`channels.type` 是持久化列，重排会让存量数据指向错误厂商。`APITypeAIProxyLibrary` 同理在 `constant/api_type.go` 以 `_` 占位。
>
> **`ChannelCloudflare`(39) 是命名例外**：它没有 `Type` 前缀，写新代码时容易拼错。

### 8.2.1 渠道目录注册表（`constant/channel_registry.go`）

渠道的「机器可读目录」与运行时常量分离。新增渠道时除了改 `channel.go`，还应让目录能描述它：

| API | 用途 |
|---|---|
| `ChannelCatalog() []ChannelMetadata` | 按数值升序的完整目录，返回副本，调用方无法改动注册表 |
| `GetChannelMetadata(t int)` | 二分查找，只覆盖内置区间；未分配值与插件区间返回 `false` |
| `IsPluginChannelType(t int) bool` | `t >= ChannelPluginTypeStart`（=1000），预留给插件声明的渠道 |
| `IsChannelTypeNameTaken(name) bool` | 大小写+空白不敏感的重名检查，供插件绑定渠道时拒绝与内置冲突 |

`init()` 守卫会在 `ChannelTypeNames` 与目录不一致时 panic，防止新增渠道时漏登记。`GetChannelTypeName` 保持逐字节不变的输出契约（有测试锁定）。

### 8.3 新增一个渠道的完整清单

1. `constant/channel.go`：加 `ChannelType*` 常量，并同步 `ChannelBaseURLs`、`ChannelTypeNames`；
2. 新建 `relay/channel/<name>/`，实现 `channel.Adaptor`（通常内嵌 `openai.Adaptor` 复用大部分逻辑）；
3. 注册三处映射：
   - `relay/relay_adaptor.go` 的 `GetAdaptor(apiType)`，
   - `constant/api_type.go` 的 `APIType*` 常量，
   - `common/api_type.go` 的 `ChannelType2APIType(channelType)`；
4. 若上游支持 `StreamOptions`，把渠道类型加入 `relay/common/relay_info.go:437` 的 `streamSupportedChannels`；
5. 若是任务平台，登记到 `relay/relay_adaptor.go` 的 `taskPluginKeys`；
6. 前端渠道表单：`web/src/features/channels/`。

> **`streamSupportedChannels` 很关键**：`InitChannelMeta` 据此设置 `ChannelMeta.SupportStreamOptions`；`TextHelper` 只在"支持 + 请求 stream"时保留 `StreamOptions`（或按 `constant.ForceStreamOption` 强制 `IncludeUsage:true`），否则置 `nil`。它直接决定流式末帧是否带 usage，**进而影响结算精度**。

---

## 9. 协议转换（relaykit）

`relaykit/` 是独立 Go module（`go 1.25.1`），**只含 DTO 与转换逻辑**；传输、认证、数据库、计费全部留在宿主。

```
relaykit/
├── dto/           # openai_request.go, openai_response.go, claude.go, gemini.go,
│                  # billing_usage.go, usage_merge.go, channel_settings.go
├── types/         # RelayFormat, NewAPIError/ErrorCode, FileSource, ConversionDiagnostic
├── relayconvert/  # request_registry.go, response_registry.go, text_converter_registry.go,
│                  # request_compat.go, response_compat.go, reasoning/, convmeta/, kitutil/,
│                  # internal/{oai_chat,oai_responses,claude_messages,gemini_chat,
│                  #           toolconv,shared,media,convdiag,jsonutil}
└── reasonmap/
```

支持的 `RelayFormat`（`relaykit/types/relay_format.go`）：`RelayFormatOpenAI`、`RelayFormatClaude`、`RelayFormatGemini`、`RelayFormatOpenAIResponses`、`RelayFormatOpenAIAudio`、`RelayFormatOpenAIImage`、`RelayFormatOpenAIRealtime`、`RelayFormatTask`、`RelayFormatMjProxy` 等。

主要转换器（请求侧）：`ConverterClaudeMessagesToOpenAIChat`、`ConverterOpenAIChatToClaudeMessages`、`ConverterOpenAIChatToOpenAIResponses`、`ConverterOpenAIResponsesToOpenAIChat`、`ConverterOpenAIResponsesToGemini`、`ConverterGeminiContentToOpenAIChat`、`ConverterOpenAIChatToGeminiContent`、`ConverterClaudeMessagesToOpenAIResponses`、`ConverterGeminiContentToOpenAIResponses`、`ConverterOpenAIResponsesToClaudeMessages`。
响应侧：`ResponseConverterOAIChatToOAIResponses`、`ResponseConverterClaudeMessagesToOAIChat`、`ResponseConverterOAIChatToGeminiChat`、`ResponseConverterOAIResponsesToOAIChat`、`ResponseConverterOAIChatToClaudeMessages`、`ResponseConverterGeminiChatToOAIChat`（均有 `*_stream_resp` 变体）。

入口：`ConvertRequest` / `ConvertRequestVia`（多跳） / `ConvertRequestByID`、`ConvertResponse`、`ConvertStreamResponseChunk` + `FinalizeStreamResponse`。宿主封装在 `service/request_converter.go`（`init()` 注入 `MediaResolver`）与 `service/response_converter.go`。

**关键解耦点**：转换器只能通过 `convmeta.Meta` 接口读取会话状态，宿主的实现是 `RelayInfo.ConvOptions()`。这是 relaykit 能独立构建的原因，**不要在 relaykit 里 import 宿主类型**。

---

## 10. 计费体系

> 改动任何计费代码前，**必须完整阅读** `.agents/rules/billing.md` 与 `pkg/billingexpr/expr.md`（用 read 工具读全文，不是 grep 或摘要）。

### 10.1 配额单位

```go
// common/constants.go
var QuotaPerUnit = 500 * 1000.0   // 500000 quota = 1 USD
```

### 10.2 三条计价路径

| 路径 | 触发条件 | 公式 |
|---|---|---|
| **按量（倍率）** | 默认 | 按 `ModelRatio` / `CompletionRatio` / `CacheRatio` / `ImageRatio` / `AudioRatio` / `CacheCreation*Ratio` 逐项乘 |
| **按次** | `ratio_setting.GetModelPrice` 命中 `UsePrice` | `quota = ModelPrice * QuotaPerUnit * GroupRatio * ∏OtherRatios` |
| **表达式/阶梯** | `billing_setting.BillingMode = BillingModeTieredExpr` | `quota = exprOutput / 1e6 * QuotaPerUnit * groupRatio`（任务表达式不除 `1e6`） |

**两个路径的浮点处理不同，不得混用**：传统倍率路径用 `shopspring/decimal`，表达式路径用 `float64` + `QuotaRound`。

### 10.3 表达式计费（`pkg/billingexpr/`）

- 引擎：`expr-lang/expr v1.17.8`。
- **变量**：`p`（prompt tokens）、`c`（completion）、`len`、`cr`（cache read）、`cc` / `cc1h` / `cc12h`（cache creation）、`img` / `img_cr` / `img_o`（图片 token）、`ai` / `ao`（audio）、`image_count`。
- **函数**：`tier`、`fixed`、`param`、`header`、`u`、`has`、`hour`/`minute`/`weekday`/`month`/`day`、`max`/`min`/`abs`/`ceil`/`floor`。
- `p`/`c` 会**按 AST 真实引用自动扣减**子类（所以 `len` 不会重复计入）；请求规则以 `|||` 追加，编译期 patch 成 `<probe条件>?乘数:1` 并记录 `RequestRuleTrace`。
- 缓存：`compile.go` 的 `CompileFromCache` / `compileFromCacheByHash`，`maxCacheSize=256`。
- `fixed()` **只能**作为 `tier(name, fixed(x))` 的完整叶子，`validateFixedPricingTree` 禁止多叶相加或再乘 token；**任务与 Realtime 拒绝 `fixed()`**（内部乘 1,000,000 复用 token 归一）。
- 内置价目在 `setting/billing_setting/builtin_billing.go`，用**真实 USD / 1M tokens**；含 DeepSeek 峰谷时段表达式与 `tier("request", fixed(0.00075))`。**新增内置价格必须写成自包含表达式，不要加到旧的 model/completion/cache ratio 表。**

### 10.4 存储与配置

`setting/billing_setting/tiered_billing.go`：`BillingSetting{BillingMode, BillingExpr, PluginBillingExpr}`、`BillingModeRatio`、`BillingModeTieredExpr`、`GetBillingExpr`、`ResolveTaskBillingExpr`、`SmokeTestExpr`、`SmokeTestTaskExpr`。
任务倍率在 `setting/task_pricing_setting/config.go`：`SoraSizeRatio`、`VertexResolution4K`。

### 10.5 预扣 → 结算 → 退款

```
service/billing.go          PreConsumeBilling / SettleBilling / BillingSourceWallet|Subscription
service/billing_session.go  BillingSession: preConsume / Reserve / ReserveBeforeSubmit /
                            Settle / Refund / shouldTrust（依 quota_setting.TrustQuotaUSD）
service/quota.go            PreConsumeTokenQuota → model.TryReserveTokenQuota（原子）
                            PostConsumeQuota / PreWssConsumeQuota / PostWssConsumeQuota
service/text_quota.go       PostTextConsumeQuota → calculateTextQuotaSummary → composeTieredTextQuota
service/tiered_settle.go    BuildTieredTokenParams / PrepareTieredBillingForSelectedGroup /
                            TryTieredSettle / isFixedPriceSettlement
service/image_billing.go    PrepareImageBillingForRequest（每次尝试按最终 n 重算并 Reserve）
service/task_billing.go     LogTaskConsumption / RefundTaskQuota / RecalculateTaskQuota /
                            EvaluateTaskCompletionUsage
service/violation_fee.go    ChargeViolationFeeIfNeeded
```

- `BillingSession` 用 `sync.Mutex` 串行化 `Settle`/`Refund`/`Reserve`，`settled`/`refunded` 幂等。
- `ForcePreConsume` 对异步任务**关闭旁路**，必须预扣。旁路还要求 token 余额与钱包余额都高于 `quota_setting.TrustQuotaUSD * QuotaPerUnit`；**订阅扣费来源永远不能走旁路**（`shouldTrust` 对 `BillingSourceSubscription` 返回 false）。
- **预扣饱和即失败**：`PreConsumeBilling` 遇到非 nil 的 `relayInfo.QuotaClamp` 直接以 `ErrorCodeModelPriceError`/400 + `SkipRetry` 拒绝，负值预扣也单独拒绝——这是"估算溢出不得静默扣款"的落地机制。

#### 任务结算优先级（`service/task_polling.go`）

判定顺序固定，**改顺序等于改钱**：

1. **阶梯额度快照 / 用量表达式** → `RecalculateTaskQuota`；
2. `bc.PerCallBilling`（按次计费）为真 → **跳过差额结算**，保留预扣；
3. `adaptor.AdjustBillingOnComplete` 返回 > 0 → 按它调整（返回 0 表示保持预扣）；
4. 有 token → `RecalculateTaskQuotaByTokens`；
5. 都不成立 → 保持预扣。

关键约定：**表达式求值失败时返回"已处理"，保留预扣而不全额退款**——否则上游已经出片、账单却归零。失败任务才由 `RefundTaskQuota` 全额退。

> `fixed()` 在**结算期**会被再次拒绝（`ComputeTieredQuotaWithRequest` 对 `TaskUsageBilling` 返回错误），不只是编译期。给任务用量表达式用 `fixed()` 的结果是"一路走到结算才失败并保留预扣"，别指望编译期就拦住。

### 10.6 `PriceData` 与 `OtherRatios`

`types/price_data.go` 中的 `PriceData`：`FreeModel`、`ModelPrice`、`ModelRatio`、`CompletionRatio`、`CacheRatio`、`CacheCreation*Ratio`、`ImageRatio`、`AudioRatio`、`AudioCompletionRatio`、私有 `otherRatios`、`UsePrice`、`Quota`、`QuotaToPreConsume`、`GroupRatioInfo`。

**请求级乘数**（`n`、`seconds`、`size`、分辨率）**只能**经 `AddOtherRatio` 写入。守卫是 `isValidOtherRatio = ratio > 0 && !math.IsInf(ratio, 1)`，所以 `≤0`、`NaN`、`+Inf` 都写不进去。

> ⚠️ **但它是静默丢弃，不是报错**：`AddOtherRatio` 无返回值，非法乘数被直接忽略，调用方无从察觉。后果是本该按 `n=4` / `seconds=10` 计费的单子按**基准价**结算，而客户端照样拿到 4 张图 / 10 秒视频——**少收钱且无日志**。
> - 需要知道是否写成功时用 `ReplaceOtherRatios(map) bool`（返回是否至少写入一项）。
> - 乘数来自客户端可控字段（`n`、时长、分辨率）时，**先自行校验再写入**，不要把守卫当兜底。
> - 任务重新结算与日志回填用的是 `ReplaceOtherRatios` / `OtherRatioMultiplier` / `ApplyOtherRatiosToDecimal` / `RemoveOtherRatiosFromFloat`；只认识 `AddOtherRatio` 会把小数路径的乘积算错。
> **不要直接写 `OtherRatios` map，也不要削弱这些守卫。**

长效缓存写倍数：`ClaudeCacheCreation1hMultiplier = 6 / 3.75`、`OpenAICacheCreation12hMultiplier = 2`。

### 10.7 计费安全不变量（违反即事故）

1. **配额饱和集中化**：`common/quota_math.go` 的 `MaxQuota = math.MaxInt32`、`MaxWalletQuota = 1<<53-1`；使用 `QuotaFromFloat`（截断）/ `QuotaRound`（half-away-from-zero）/ `QuotaFromDecimal`，以及 `*Checked`（返回 `*QuotaClamp`）与 `*Strict`（返回 error）变体。
   **禁止裸转换**：`int(float64(quota) * ratio)`、`int(math.Round(...))`、`int(decimal.IntPart())` 一律不允许。
2. **饱和必须可审计**：计费路径用 `*Checked` 变体，把 clamp 存到 `relayInfo.QuotaClamp`，写日志前调 `service/log_info_generate.go` 的 `attachQuotaSaturation` → 落到 `other.admin_info.quota_saturation`（挂在 `admin_info` 下天然只有管理员可见）并发 `logger.LogWarn`。
3. **用户可控乘数必须先行钳制**：图片 `n` 用 `dto.MaxImageN`（128）；视频时长用 `relaycommon.MaxTaskDurationSeconds`（3600，`relay/common/relay_utils.go:146`）；max-token 家族用 `relay/helper/valid_request.go:125` 的 `maxTokensLimit = math.MaxInt32 / 2`。**复用这些常量，不要新造同类限制。**
4. **注意绕过路径**：`Extra["parameters"]`、任务的 `metadata` map、multipart 表单字段都能绕过标准 DTO 校验携带同样的量。任何从这些路径读取乘数的适配器都必须在本地施加同样的界。
5. **无符号类型陷阱**：`*uint` 字段可以接受 `18446744073686646784` 这类巨大正数（一个回绕的负数），**`>= 0` 检查不够，必须有上界**。
6. **不得产生负费用**：`baseTokens`（= prompt − cached − cacheCreation − image − audio）要**截零**；`ratio != 0 && quota <= 0` 时抬到 `1`。
7. **介质时长也是用户可控的**：音频文件头（转写 token 计数、TTS 响应时长）与上游扣减数（如 Kling 的 `FinalUnitDeduction`）可能声称荒谬值，转成 token 前必须饱和处理。
8. **上游图片数量结算的唯一入口**：HTTP 图片 relay 只能通过 `RelayInfo.UpdateImageCount` 修正，任务插件只能通过返回 `image_count` usage fact；为 0 或越界时保留请求时的预扣数量。
   - **禁止**用 gjson `data.#` / `len(data)` / 元素个数来数图片。
   - 对象形态的 `data` 有非空 `url` 或 `b64_json` 才算 1 张，否则 0 张；数组形态取 `max(有 url 的条目数, 有 b64_json 的条目数)`。
   - 参考实现：`relay/channel/openai/relay_image.go` 的 `openaiImageResponseCount`（xAI 适配器已通过 `OpenaiImageHandler` 继承）。
   - OpenAI 格式 JSON body 重新以 SSE 发出时，每张图片要发一个 `image_generation.completed` 事件，**不能让客户端只收到 `[DONE]` 却被扣了图片钱**。
   - 真实 SSE 流数 `image_generation.completed` / `image_edit.completed` 事件，且**只在上游正常结束**（`done`/`eof`）时才采信更低的计数；客户端中断不得把计费降到请求数量以下。
9. **回归测试位置**：这些不变量的测试放在它们保护的边界旁边 —— `relay/helper/openai_image_request_test.go`、`relay/common/relay_utils_test.go`、`common/quota_math_test.go`、`relay/channel/openai/image_stream_test.go`。**扩展已有的表，不要新建文件。**
10. **预扣倍率只影响预扣**：`quota_setting.pre_consume_multiplier`（默认 1）冻结进 `BillingSnapshot.PreConsumeMultiplier`，**只乘 token 计价的预扣额**；不改阶梯选择、不改最终结算、也不碰按次计价的分支。旧快照里该字段为 0 时按 1 处理。
11. **任务 fact 饱和是双向不对称的**：宿主会**钳制**插件返回的 `second` / `count`，但对 `token` / `credit` 只做 **int32 边界饱和**。所以转发时长 `-1`（让厂商自选）的插件必须按该模型**最长时长**预留，再按厂商回的 usage 结算——这类判断宿主不会替你做。
12. **钱包与配额是两条边界**：`MaxQuota = math.MaxInt32` 约束单次配额，`MaxWalletQuota = 1<<53-1` 约束钱包总额，充值/退款路径用 `WalletQuotaFromDecimalStrict`。两者用途不同，**不要互相替换**。
13. **结算期严格转换**：阶梯结算用 `QuotaRoundChecked`，图片预扣用 `QuotaFromFloatStrict` / `QuotaRoundStrict`——溢出时**中止本次尝试**而不是扣一个饱和值。非严格变体只可用于"溢出可接受、仅需夹具"的场景。

### 10.8 `tokenkit`

独立 module（`go 1.26`），依赖 `tiktoken-go/tokenizer`。

- `tokenkit.go`：`Family`（`FamilyOpenAI` / `FamilyClaude` / `FamilyClaude47` / `FamilyGemini` / `FamilyUnknown`）、`ModelFamily`、`Estimate` / `EstimateFamily`（`familyWeights` 可加性权重，按 word/number/CJK/emoji/math/URL/换行分类后 `math.Ceil`）。
- `tokenizer.go`：`Count` 对 OpenAI 家族用**精确 tiktoken**（`codec.NewO200kBase` / `codec.NewCl100kBase`，`usesCl100k` 判 GPT-4/GPT-3.5），其余回退 `Estimate`；`maxRunBytes=1024` 切块，避免 BPE 平方级耗时。
- `image.go`：`Image{Width,Height,Detail}`、`ImageTokens`、`ImageNeedsSize`、`openAIImageRule`（tile 与 patch 两套）、`claudeResizedSize`、`geminiTileImageTokens`；常量 `unknownImageSide=1024`、`maxImageSide=65535`、`maxImagePatches=30000`、`unknownFamilyImageTokens=520`。
- 宿主调用点：`service/token_counter.go`（`EstimateRequestToken`、`CountRequestToken`、`getImageToken`、`CountTokenRealtime`）。

---

## 11. 数据层与缓存

### 11.1 核心实体

| 实体（结构体） | 表 | 关键字段与约束 |
|---|---|---|
| `User`（`model/user.go`） | `users` | `username` unique+index；`access_token char(32)` uniqueIndex（弃用过渡列）；`aff_code` uniqueIndex；`quota`/`used_quota`/`aff_quota`/`aff_history`；`group varchar(64) default 'default'`；`auth_version`；`setting text`；软删 |
| `Token`（`token.go`） | `tokens` | `key varchar(128)` uniqueIndex；`remain_quota`；`unlimited_quota`；`model_limits text`；`allow_ips`；`auto_groups text`；`expired_time default -1` |
| `Channel`（`channel.go`） | `channels` | `key not null`（多 key 状态记在 `ChannelInfo.MultiKeyStatusList`）；`models`；`priority`；`weight`；`model_mapping text`；`status`（1 启用 / 2 手动禁用 / 3 自动禁用） |
| `Ability`（`ability.go`） | `abilities` | **复合主键 `(group, model, channel_id)`**；路由内存索引由此派生 |
| `Log`（`log.go`） | `logs`（写 `LOG_DB`） | 复合索引 `idx_created_at_id`、`idx_user_id_id`、`idx_created_at_type`、`index_username_model_name`、`idx_logs_request_id`、`idx_logs_upstream_request_id` |
| `Redemption`（`redemption.go`） | `redemptions` | `key char(32)` uniqueIndex；`quota`；`expired_time` |
| `TopUp`（`topup.go`） | `top_ups` | `trade_no varchar(255) unique`；`payment_provider`；`status` |
| `SubscriptionPlan/Order/UserSubscription`（`subscription.go`） | `subscription_plans` / `subscription_orders` / `user_subscriptions` | `price_amount decimal(10,6)`；`duration_unit/value`；`quota_reset_period` |
| `Option`（`option.go`） | `options` | 仅 `(Key primaryKey, Value)` |
| `Task`（`task.go`） | `tasks` | `task_id varchar(191)`；`platform varchar(30)`；`Properties`（JSON Scanner/Valuer）；`TaskBillingContext` 快照 |
| `TaskPlugin`（`task_plugin.go`） | `task_plugins` | **唯一索引 `uk_task_plugin_key_version(key,version)`**；`source_hash`；`icon LongText` |
| `Midjourney` | `midjourneys` | `user_id`/`action`/`mj_id`/`submit_time` index |
| `Model`/`Vendor`（`model_meta.go`） | `models` / `vendors` | `uk_model_name_delete_at(model_name, deleted_at)` 支持软删后同名重建 |
| `QuotaData`（`usedata.go`） | `quota_data` | `idx_qdt_model_user_name`、`idx_qdt_created_at`、`node_name` |
| `PerfMetric`（`perf_metric.go`） | `perf_metrics` | 唯一 `idx_perf_model_group_bucket(model_name,"group",bucket_ts)` |
| `SystemInstance`/`SystemTask`/`SystemTaskLock` | `system_instances`/`system_tasks`/`system_task_locks` | 见第 13 节 |

`model/pricing.go` 的 `Pricing` **不是表**，而是运行时定价视图（`Model` + `ratio_setting` + `billing_setting` 合成）。

### 11.2 初始化与迁移（`model/main.go`）

- `InitDB()`：`chooseDB("SQL_DSN", false)` **按 DSN 前缀选驱动**：
  - `clickhouse://` / `tcp://` / `http(s)://` → ClickHouse（**仅日志库允许**，主库直接报错）；
  - `postgres://` / `postgresql://` → PostgreSQL（`PreferSimpleProtocol: true`，规避 PgBouncer 的 `08P01`/`42P05`）；
  - `local*` → SQLite；
  - 其余 → MySQL（自动补 `parseTime=true`）。
- MySQL 前置校验 `checkMySQLChineseSupport()`：库默认字符集/排序规则须为 `utf8mb4`/`utf8`/`gbk`/`big5`/`gb18030`，否则 **panic**。
- `ensureUserQuotaColumns()`：非 SQLite 且未设 `SKIP_64BIT_QUOTA_SCHEMA_CHECK=true` 时校验四个额度列为 64 位整型，**旧的 32 位钱包直接拒绝启动**（不自动升级）。
- **迁移只由 master 执行**：非 master 分支**不是单纯 `return`**——它会先跑 `EnsureLegacyAccessTokenRetireAt`（失败只 `SysError` 不中断）再返回 `nil`。改这段时别把那个调用一起删掉。
- `migrateDB()` 顺序：`migrateTokenKeyUniqueness` → `migratePrefillGroupUniqueness` → `migrateSubscriptionPlanPriceAmount` → `migrateTokenModelLimitsToText` → `migrateOptionPrimaryKey` → `DB.AutoMigrate(...)`（**33 个模型**）→ `InitializeUserAuthVersions` → `EnsureLegacyAccessTokenRetireAt` → `InitializeExternalIdentityClaims` → `SubscriptionPlan`（SQLite 走 `ensureSubscriptionPlanTableSQLite()` 的 `PRAGMA table_info` + `ALTER TABLE ADD COLUMN`）。
  - ⚠️ `migrateOptionPrimaryKey` 的错误被 `common.SysError` **吞掉**、不 return，所以它失败不会中止启动。这个箭头链不是"任一步失败即退出"。
- `migrateLOGDB()`：`MigrateAuditLogs` → ClickHouse 走 `migrateClickHouseLogDB()`（`ENGINE = MergeTree()`、`PARTITION BY toYYYYMM(toDateTime(created_at))`、`ORDER BY (created_at, request_id)`、TTL 由 `LOG_SQL_CLICKHOUSE_TTL_DAYS` 控制），否则 `LOG_DB.AutoMigrate(&Log{})`。

### 11.3 三库兼容工具

| 工具 | 位置 | 说明 |
|---|---|---|
| `commonGroupCol` / `commonKeyCol` / `commonTrueVal` / `commonFalseVal` | `model/main.go:22-25` | PG 用 `"group"`/`"key"`/`true`/`false`；MySQL/SQLite 用反引号与 `1`/`0` |
| `logGroupCol` / `logKeyCol` | `model/main.go:27-28` | **日志库专用**的保留字列名（与主库那对分开）。写日志库裸 SQL 却用 `commonGroupCol` 会拿到错误方言的引号 |

> ⚠️ `commonGroupCol` 等变量**在 `initCol()` 执行前是空字符串**。`init()` 里直接拼裸 SQL 会拼出非法语句；需要用到它们时确认 `initCol()` 已跑过。

| `lockForUpdate(tx)` | `model/locking.go` | MySQL/PG 发 `FOR UPDATE`，**SQLite 跳过**（语法不支持） |
| `mysqlMigrationDialector` / `postgresMigrationDialector` | `model/migration_dialector.go` | 重写 `MigrateColumn`：MySQL decimal 默认值等价比较、PG `bpchar`→`char` 归一化；`MigrateColumnUnique` 从 `pg_catalog` 解析真实约束名再 DROP |
| `jsonScanBytes()` | `model/main.go:32` | 归一化 JSON 列的 `[]byte`/`string` 差异 |
| `common.UsingMainDatabase(...)` / `common.UsingLogDatabase(...)` | `common/database.go` | 分支主库/日志库 SQL |

> ⚠️ **禁止**使用 `tx.Set("gorm:query_option", "FOR UPDATE")` —— GORM v2 会静默忽略，拿不到锁。

### 11.4 主库 / 日志库分离

`InitLogDB()`：`LOG_SQL_DSN` 为空则 `LOG_DB = DB`；否则独立连接（额外支持 ClickHouse）与独立连接池参数。`CloseDB()` 判断 `LOG_DB != DB` 防止双重关闭。`createLog` 写 `LOG_DB`。ClickHouse 没有自增主键，`assignDisplayLogIds` 负责展示 ID。

### 11.5 缓存体系

| 层 | 位置 | 内容 |
|---|---|---|
| 进程内路由索引 | `model/channel_cache.go` | `group2model2channels map[string]map[string][]int`、`channelsIDM map[int]*Channel`、`channel2advancedCustomConfig`，受 `channelSyncLock sync.RWMutex` 保护。`InitChannelCache()` 全量重建，`SyncChannelCache(frequency)` 周期同步 |
| Redis | `common/redis.go` | `common.RDB`、`RedisEnabled`；`REDIS_CONN_STRING` 未设即关闭 |
| 混合缓存 | `pkg/cachex` | `Namespace`、`HybridCache[V]`（Redis 优先，否则 `samber/hot` 内存 LRU）、`ValueCodec`（`IntCodec`/`StringCodec`/`JSONCodec`）。使用者：`model/subscription.go`、`service/channel_affinity.go` |
| 鉴权缓存 | `model/token_cache.go`、`user_cache.go`、`user_auth_cache.go`、`auth_cache_integrity.go` | 键 `token:<HMAC>` + fence 键保证 Redis Cluster 同槽；`UserBase`，`userCacheSchemaVersion=2`；`authCacheMAC` 防篡改。**TTL = `common.RedisKeyCacheSeconds()` = `SYNC_FREQUENCY`** |
| 批量更新 | `model/utils.go` | `BatchUpdateEnabled` + `InitBatchUpdater()`，`batchUpdateStores[type][id]` 累加后落库（含溢出保护）。`BatchUpdateEnabled` 在 `main.go` 里直接读环境变量 |

**多节点一致性策略 = 周期全量重载 + TTL**。`main.go` 同时起：

```go
go model.SyncChannelCache(common.SyncFrequency)
go model.SyncOptions(common.SyncFrequency)
go authz.StartPolicySync(common.SyncFrequency)
go controller.SyncTaskPlugins()          // 首次同步后 30s ticker
```

写路径对 `billing_setting.*` 变更会主动 `InvalidatePricingCache()`。

---

## 12. 配置体系

### 12.1 `setting/` 子包

| 包 | 注册键 | 职责 |
|---|---|---|
| `config` | — | `ConfigManager`：`Register`/`Get`/`LoadFromDB`/`SaveToDB`/`ExportAllConfigs`；DB 键格式 `<name>.<field>` |
| `billing_setting` | `billing_setting` | 计费模式与表达式、内置价目 |
| `console_setting` | `console_setting` | `ApiInfo` / `Announcements` / `FAQ` / `UptimeKumaGroups` |
| `model_setting` | `claude`/`gemini`/`global`/`grok` | 厂商请求改写策略 |
| `operation_setting` | `channel_affinity_setting`/`checkin_setting`/`general_setting`/`log_setting`/`monitor_setting`/`payment_setting`/`quota_setting`/`token_setting`/`tool_price_setting` | 自动禁用状态码、支付、签到、额度上限 |
| `performance_setting` | `performance_setting` | 磁盘缓存（`ThresholdMB`、`MaxSizeMB`、`Path`）与 CPU/内存/磁盘阈值；变更后 `UpdateAndSync()` |
| `perf_metrics_setting` | `perf_metrics_setting` | `Enabled`、`FlushInterval=5`、`BucketTime="hour"`、`RetentionDays=0` |
| `ratio_setting` | `group_ratio_setting` | `USD=500`、`USD2RMB=7.3`、`defaultModelRatio`、`cache_ratio`、`expose_ratio` |
| `reasoning` | — | 转发 `relaykit/relayconvert/reasoning` 的 `EffortSuffixes` |
| `system_setting` | `discord`/`fetch_setting`/`legal`/`oidc`/`passkey`/`telegram` | OAuth/OIDC/法务；`ServerAddress`、`TaskPublicAddress`、`WorkerUrl` |
| `task_pricing_setting` | `task_pricing_setting` | `SoraSizeRatio`、`VertexResolution4K` |

### 12.2 选项热更新

```
model.InitOptionMap()        # 写 common.* 默认值 → maps.Copy(OptionMap, config.GlobalConfig.ExportAllConfigs()) → loadOptionsFromDatabase()
model.updateOptionMap(k, v)  # 权限/开关分支 → handleConfigUpdate()
model.SyncOptions(freq)      # 按 SYNC_FREQUENCY 周期全量重载
```

`handleConfigUpdate()` 按 `.` 切分键名 → `GlobalConfig.Get(configName)` → `UpdateConfigFromMap`，并对 `performance_setting` / `billing_setting` 执行副作用。

### 12.3 关键环境变量

`.env.example` 里有 57 个键（**全部以注释形式给出**，直接 `cp .env.example .env` 得到的是空生效值），而 Go 代码实际读取 108 个不同的变量名——大部分只有默认值、不需要配置。最关键的 29 个：

| 变量 | 默认 | 作用 |
|---|---|---|
| `PORT` | `3000` | 监听端口 |
| `SQL_DSN` | 空 | **空 = 用 SQLite**（`one-api.db`）；决定驱动类型 |
| `LOG_SQL_DSN` | 空 | 空 = 复用主库；可指向 ClickHouse |
| `SQLITE_PATH` | `one-api.db` | SQLite 路径 |
| `SQL_MAX_IDLE_CONNS` / `SQL_MAX_OPEN_CONNS` / `SQL_MAX_LIFETIME` | `100` / `1000` / `60` | 连接池 |
| `SQL_SLOW_THRESHOLD_MS` | `200` | 慢查询阈值，0 关闭 |
| `SKIP_64BIT_QUOTA_SCHEMA_CHECK` | 未设 | 跳过额度列 64 位校验（**仅在确认可容纳时使用**） |
| `REDIS_CONN_STRING` | 空 | 空 = 无 Redis；**设了会强制 `MemoryCacheEnabled=true`** |
| `REDIS_POOL_SIZE` | `10` | Redis 连接池 |
| `MEMORY_CACHE_ENABLED` | `false` | 内存缓存 |
| `SYNC_FREQUENCY` | `60` | **同时是缓存 TTL 和跨节点不一致窗口** |
| `BATCH_UPDATE_ENABLED` / `BATCH_UPDATE_INTERVAL` | `false` / `5` | 批量落库 |
| `CHANNEL_UPDATE_FREQUENCY` | — | 上游模型自动更新频率 |
| `NODE_TYPE` | 未设 | **只有 `slave` 是特例，其他任何值都是 master** |
| `NODE_NAME` | hostname | 节点标识 |
| `SESSION_SECRET` | — | **设为 `random_string` 会直接 Fatal**；多节点必须一致 |
| `CRYPTO_SECRET` | 回退 `SESSION_SECRET` | 加密密钥 |
| `SESSION_COOKIE_SECURE` / `SESSION_COOKIE_TRUSTED_URL` | `false` / — | 安全 Cookie 与严格 OriginGuard |
| `PASSWORD_LOGIN_ENCRYPTION_ENABLED` | 未设 | 登录密码 RSA-OAEP 信封 |
| `ACCOUNT_PASSWORD_HASH_ALGORITHM` | — | `argon2id` / bcrypt（滚动升级需谨慎） |
| `TRUSTED_PROXIES` | 内网默认 | `none` = 严格模式；显式列表完全替代默认 |
| `RELAY_TIMEOUT` | `0` | 全请求超时，0 = 不限 |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | `1800` | **仅约束"等待响应头"**；非流式请求要留足余量 |
| `STREAMING_TIMEOUT` | `300` | 流式无响应超时 |
| `MAX_REQUEST_BODY_MB` | `128` | 请求体上限 |
| `SHUTDOWN_TIMEOUT_SECONDS` | `120` | 优雅退出（SSE 可能跑很久） |
| `DEBUG` | `false` | 调试模式 |
| `DEFAULT_LANGUAGE` | 无代码默认值 | 无人声明语言时的返回语言 + 服务器日志语言（仅 `zh*` 生效）。代码里 `os.Getenv` 直接取值、**没有 `en` 兜底**，空值交给 i18n 自己的默认；`.env.example` 的样例值与 `AGENTS.md` 都写 `zh-CN` |
| `ENABLE_PPROF` | `false` | 开 8005 端口 pprof |
| `TASK_PLUGIN_ENABLED` | `true` | JS 任务插件总开关 |
| `TASK_TIMEOUT_MINUTES` | `4320` | 异步任务硬超时（0 = 禁用） |
| `TASK_POLL_MAX_FAILURES` | `20` | 连续轮询失败阈值 |
| `GLOBAL_API_RATE_LIMIT` / `CRITICAL_RATE_LIMIT` | `360/180s` / `20/1200s` | 限流 |
| `LOG_SQL_CLICKHOUSE_TTL_DAYS` | `0` | ClickHouse 日志 TTL |

其它分类：`PYROSCOPE_*`、`TLS_INSECURE_SKIP_VERIFY`、`TRUSTED_REDIRECT_DOMAINS`、`USER_SESSION_ACTIVE_LIMIT=50`、`USER_SESSION_ISSUANCE_LIMIT=100`、`USER_SESSION_ISSUANCE_WINDOW_SECONDS=86400`、`USER_SESSION_REVOKED_RETENTION_DAYS=7`、`USER_SESSION_HOURLY_ALERT_THRESHOLD=5000`、`RELAY_HTTP2_PING_IDLE_TIMEOUT`、`RELAY_HTTP2_PING_TIMEOUT`、`STREAM_SCANNER_MAX_BUFFER_MB`、`GEMINI_VISION_MAX_IMAGE_NUM=16`、`GET_MEDIA_TOKEN`、`GENERATE_DEFAULT_TOKEN`、`UPDATE_TASK`、`FRONTEND_BASE_URL`。

**主要环境变量入口是 `common.InitEnv()`**（`common/init.go`）：`flag.Parse` → `VERSION` → `DEFAULT_LANGUAGE` → `SESSION_SECRET` → `CRYPTO_SECRET` → 会话/限流/Relay 连接池 → `initConstantEnv()` 写 `constant.*`（如 `MaxRequestBodyMB=128`）。

> ⚠️ 它**不是唯一入口**：`main.go` 与 `model/`、`common/redis.go` 里还有直接 `os.Getenv` 的地方，例如 `PORT`、`GIN_MODE`、`BATCH_UPDATE_ENABLED`、`ENABLE_PPROF`、`CHANNEL_UPDATE_FREQUENCY`、`UMAMI_*`、`GOOGLE_ANALYTICS_ID`、`SQL_DSN`/`LOG_SQL_DSN`、`REDIS_CONN_STRING`。排查"环境变量为何没生效"时不要只查 `InitEnv`。

---

## 13. 多节点与主从

- `common.IsMasterNode = os.Getenv("NODE_TYPE") != "slave"`（`common/init.go:91`）。**只有 `slave` 是特例。**
- 节点身份：`common/NodeName`、`NodeNameSource`（`manual` | `hostname`）、`NodeNameManuallyConfigured`（`common/node_identity.go`）→ `common.GetNodeIdentity()`。
- **实例上报**（`service/system_instance.go`）：`StartSystemInstanceReporter()` 每 30s 调 `ReportCurrentSystemInstance()`，组装 `SystemInstanceInfo{SchemaVersion, Node, Role, Runtime, Host, Resources}`，`model.UpsertSystemInstance()` 用 `clause.OnConflict{Columns: node_name}` 落库；`SystemInstanceStaleAfterSeconds = 90` 标记 stale。控制台"系统信息"页据此展示所有存活节点。
- **系统任务的 DB 租约去重**（`service/system_task.go` + `model/system_task.go`）：

| 常量 | 值 |
|---|---|
| `systemTaskRunnerIdleInterval` | 15s |
| `systemTaskLockTTL` | 60s |
| `systemTaskSchedulerInterval` | 15s |
| `systemTaskStaleLockInterval` | 30s |

`StartSystemTaskRunner()` 先判 `!common.IsMasterNode` 直接返回；`runnerID = NodeName + "-" + random8`。核心原语：`ClaimSystemTask`、`acquireSystemTaskLock`、`RenewSystemTaskLock`（心跳续租）、`ExpireStaleSystemTaskLocks`、`MarkSystemTaskLeaseExpired`、`FinishSystemTask`、`ReleaseSystemTaskLock`。`SystemTask.ActiveKey *string uniqueIndex` 保证**同类型只有一个活动任务**。

任务类型：`SystemTaskTypeLogCleanup` / `ChannelTest` / `ModelUpdate` / `MidjourneyPoll` / `AsyncTaskPoll`；handlers 在 `controller/system_task_handlers.go` 注册（`RegisterScheduledSystemTasks`），接口为 `ScheduledSystemTaskHandler`（`Enabled` / `Interval` / `NewPayload`）。

---

## 14. 任务插件系统

### 14.1 架构

```
plugins/embed.go          //go:embed tasks  → init() 逐个 jsplugin.DefaultRegistry.RegisterFactory(source, Options{Key: key})
                                ↓
pkg/jsplugin/engine.go    基于 moejs：Compile / Runtime / Hook；Call、CallPath、CallPathInto、
                          CallPathWithAdmissionTimeout；HookError / ResultError；acquireCallSlot 并发准入
pkg/jsplugin/registry.go  NewRegistry / Register / RegisterFactory / CompilePlugin / ReplaceOverrides /
                          Generation() / RoutingStatus()；RoutingGeneration 原子发布路由；
                          Meta.Kind + APIVersion2 分流 task/job/identity；
                          TaskDriverHookRequirements 是 hook 要求的唯一来源
pkg/jsplugin/cli.go       new-api plugin lint | test --fixture
                                ↓
controller.SyncTaskPlugins()   首次同步 + 30s ticker
  → model.GetTaskPluginSyncSnapshot()  按 SourceHash 复用未变更插件
  → CompilePlugin → ReplaceOverrides

service/system_task_plugin.go   RegisterPluginJobHandler：kind=job → 系统任务 plugin:<key>
controller/task_plugin.go       ListPlugins / GetPlugin / GetPluginKinds / GetPluginProtocols
```

限制：插件 key ≤ 30 字符（`pluginKeyPattern`）；版本为 semver（`pluginVersionPattern`）；插件状态 ≤ 1 MiB；图标 ≤ 512 KiB。上传走 `model.SaveTaskPlugin()`：**同 key + version 但 `source_hash` 不同直接拒绝**。

### 14.2 内置插件（`plugins/tasks/`，11 个）

| key | 版本 | 关联渠道类型 |
|---|---|---|
| `alibaba` | 1.5.0 | 17 (Ali) |
| `doubao` | 1.4.0 | 54 / 45 (DoubaoVideo / VolcEngine) |
| `google` | 1.2.0 | 24 (Gemini) |
| `hailuo` | 1.4.0 | 35 (MiniMax) |
| `jimeng` | 1.2.0 | 51 |
| `kling` | 1.2.0 | 50 |
| `sora` | 1.1.0 | 55 / 1 |
| `sunoapi` | 1.1.0 | 36 |
| `vertex-ai` | 1.2.0 | 41 |
| `vidu` | 1.2.0 | 52 |
| `xai` | 1.2.0 | 48 |

平台 → 插件的映射表在 `relay/relay_adaptor.go` 的 `taskPluginKeys`。

### 14.2.1 可复制的示例插件（`examples/plugins/`）

内置插件体量大、不适合当入门模板。`examples/plugins/` 下 6 个示例各覆盖一种契约形状，**每个都能被宿主编译器加载**（由 `pkg/jsplugin/examples_test.go` 保证，它们不是「展示用假代码」）：

| 目录 | 演示什么 |
|---|---|
| `echo-minimal` | 最小 hello world：`apiVersion 1`、**不写 `kind`**、`per_task` 轮询 |
| `fanout-batch` | `fetchMode: "batch"`，以及提交后立即终态 |
| `stream-sse` | `submitResponseTypes: ["sse"]` + `parseSubmitEvent`（快照形式） |
| `artifact-content` | `listArtifacts` + `buildContentRequest` 成对、`allowedHosts` + `utils.fetch`、`openai_video` 协议 |
| `nightly-job` | `kind: "job"` + `apiVersion 2`，无 task hook、无 routes |
| `ordered-json-task` | `kind: "task"` 显式写在 `apiVersion 2` 上、`json-order@1`、`usageProfiles` |

新增插件建议从最接近的示例复制后改写。

### 14.3 插件契约（`docs/plugin-api/v1.md`，权威定义 `v1.schema.json` / `v1.d.ts`）

- 单文件**同步 ESM**。
- `meta.apiVersion` 可为 `1` 或 `2`。**v1 清单不写 `kind` 时按 `task` 处理**，因此所有存量插件无需改动即可加载。
- **`meta.kind`** 决定路径，缺省 `task`：

| kind | 必需 `apiVersion` | 必需 hook | 禁止声明 |
|---|---|---|---|
| `task`（缺省） | 1 或 2 | 见下 | — |
| `job` | **必须 2** | `run`（其余 `enabled` / `intervalSeconds` / `newPayload` / `taskType` 可选） | `routes`、`protocols`、`channelTypes` |
| `identity` | **必须 2** | 无编译期 hook 校验 | 同上 |

- **`kind: "task"` 的必需导出**（上面表格的 hook 要求只对 task 成立）：
  - 恒定必需：`meta`、`buildSubmitRequest`、`parseSubmitResponse`、`parseTaskResult`。
  - `fetchMode: "per_task"` 需要 `buildQueryRequest`；**`"batch"` 是替换而非追加**，需要 `buildBatchQueryRequest` / `parseBatchResult`，不再需要 `buildQueryRequest`。
  - `submitResponseTypes` 含 `sse` 时需要 `parseSubmitEvent` 或 `parseSubmitEventDelta`（两者都导出时以 `parseSubmitEvent` 为准，除非声明 `submit-sse-delta@1`）。
  - `listArtifacts` 与 `buildContentRequest` **必须成对出现**，缺一即编译失败。
  - 注意：即使驱动只会「立即完成」（SSE/终态），task 插件仍必须导出 `parseTaskResult` 与查询 hook。
- 两套入口：
  - `meta.routes` —— 厂商原生路径（`submit` / `query` / `dynamic`），网关同时挂 `/{key}`；
  - `meta.protocols` —— `openai_responses` / `openai_video` / `openai_image`。
- 数值计费字段（`usageSchema` / `usageProfiles[].schema`）用 `unit: second | count | token | credit`，`count` 可带 `unitLabel`。
- **`usageExamples` 只按插件级 `usageSchema` 校验**：即使所有模型都被 `usageProfiles` 覆盖，只要出现过 token 类字段（哪怕只在某个 profile 内），仍需要插件级示例向量。

**计费字段的 `description` 必须写"计费主体 + 单价"**（因为它渲染为 UI 里的价格输入标签），例如 `image_count` 用 `Image generation unit price` / `图片生成单价`，**不是** `Generated image count`；单位放 `unit`；描述要短、各语言等价、不含具体价格数字与结尾标点。这是**编写要求**，编译通过或 schema 校验通过都不能证明符合。

### 14.3.1 契约查询 API（新增）

插件元数据不再只能靠读 Go 源码得知。以下只读端点全部挂 `middleware.RootAuth()`，个人访问令牌需要 `plugin:read` 作用域：

| 端点 | 用途 |
|---|---|
| `GET /api/plugin` | 列出所有 kind 的已加载插件（**摘要投影，不含密钥/上游地址/鉴权字段**） |
| `GET /api/plugin/:key` | 单个插件元数据，未知返回 404 |
| `GET /api/plugin/kinds` | 各 kind 的必需/禁止字段与必需 hook |
| `GET /api/plugin/protocols` | 宿主协议目录（操作、模式、body 种类、driver hook） |

`/kinds` 与 `CompilePlugin` 调用**同一个** `TaskDriverHookRequirements`，所以契约只存在一份，不会漂移。修改 hook 要求时改这一处即可。

### 14.3.2 `kind: "job"` 的系统任务桥接

`service/system_task_plugin.go` 的 `RegisterPluginJobHandler` 把 job 插件接到调度器，任务类型固定为 `plugin:<key>`——这个类型同时是它的**锁与幂等去重边界**，因此：

- 若插件导出 `taskType`，其返回值必须等于自身命名空间，否则注册失败；
- 声明的类型不能别名到内置类型（`log_cleanup` 等）；
- 缺 `run` 导出时**注册期**就失败，而不是等首次调度才发现。

安全取向是**失败即关闭**：`enabled()` 抛错按未启用处理（不能因为读不到开关就默认开启）；`run` 内部 panic 只让该次任务失败并记为失败，不会拖垮 runner（runner 同时服务其它系统任务）；传给插件的参数只有 `task_id`、`type`、`payload`，**不含 lease / `locked_by` 等排他凭证**。

### 14.4 执行与结算

- 计费走 `billing_setting` 的 `<pluginKey>::<model>` 覆盖，缺失则继承模型级表达式。
- 选中通道决定执行哪个插件：`ChannelTypeTaskPlugin`(61) 看 `task_plugin_key`；type 60 看 `task_extend_plugin_keys`。
- 宿主**会**钳制插件返回的 `second` 与 `count` fact，但只把 `token` 与 `credit` fact 饱和到 int32 额度边界。
- 插件必须从**它自己发给上游的请求**推导 fact（含从 `metadata` 等透传 map 合并的值），并对任何会变成 token/credit 的请求量施加界。
- 转发时长 `-1`（让厂商选长度）时，必须按该模型**最长时长**预留，再按厂商返回的 usage 结算。

---

## 15. 前端开发

> 改动前端前必读 `web/AGENTS.md`（规范全文）与项目 `shadcn-ui` 技能（`.agents/skills/shadcn-ui/`）。

### 15.1 脚本（`web/package.json`）

| 命令 | 作用 |
|---|---|
| `bun run dev` | Rsbuild 开发服务器 |
| `bun run build` | 生产构建 → `web/dist` |
| `bun run build:check` | `bun check -b && rsbuild build` |
| `bun run typecheck`（=`bun check -b`） | 类型检查（project references） |
| `bun run lint` / `lint:fix` | `oxlint -c .oxlintrc.json .` |
| `bun run test` / `test:watch` | `vitest run` / watch |
| `bun run format` / `format:check` | `scripts/format-with-protected-headers.mjs`（**保护版权头**） |
| `bun run copyright` / `copyright:check` | 版权头检查 |
| `bun run i18n:sync` | `scripts/sync-i18n.mjs` |
| `bun run knip` | 死代码检测 |
| `bun run lint:plugins` / `format:plugins` | `plugins/` 目录 |

### 15.2 目录结构（`web/src/`）

- `routes/` —— TanStack Router 文件式路由；`routeTree.gen.ts` 是**生成产物，勿手改**。
- `features/<feature>/` —— 27 个业务域：`auth`、`channels`(116 文件)、`system-settings`(184)、`usage-logs`(82)、`pricing`、`models`、`playground`、`keys`、`wallet`、`users`、`dashboard`、`task-plugins`、`security`、`subscriptions`、`rankings`、`redemption-codes`、`model-pricing`、`performance-metrics`、`system-info`、`system-update`、`about`、`home`、`legal`、`errors`、`setup`、`chat`、`profile`。每个含 `components/`、`lib/`、`hooks/`、`api.ts`、`types.ts`、`constants.ts`。
- `components/` —— `ui/`（62 个 shadcn 基础件）、`data-table/`（含 `README.md`、`core/`、`hooks/`、`layout/`、`static/`、`toolbar/`）、`ai-elements/`、`layout/`、`model-group-selector/`、`multi-select/`、`floating-window/`、`json-code-editor/`，以及 `confirm-dialog.tsx`、`copy-button.tsx`、`empty-state.tsx`、`loading-state.tsx`、`error-state.tsx`、`dialog.tsx`。
- `hooks/` —— `use-system-config`、`use-table-url-state`、`use-copy-to-clipboard`、`use-mobile.tsx` 等。
- `lib/` —— `api.ts`、`http-client.ts`、`auth-session.ts`、`query-client.ts`、`format.ts`、`currency.ts`、`handle-server-error.ts`、`server-error-message.ts`、`legacy-route.ts`、`nav-modules.ts`、`roles.ts`、`theme-*.ts`。
- `stores/` —— 4 个 Zustand store（见下）。
- `i18n/` —— `config.ts`、`languages.ts`、`static-keys.ts`、`locales/`。
- `styles/` —— `index.css`、`theme.css`、`theme-presets.css`。

### 15.3 路由（TanStack Router）

`rsbuild.config.ts` 通过 `tools.rspack.plugins` 注入 `@tanstack/router-plugin/rspack`，dev 与 build 都会重写 `src/routeTree.gen.ts`。

- `__root.tsx`：`createRootRouteWithContext`（注入 `queryClient`），`notFoundComponent` / `errorComponent`；`beforeLoad` 做 legacy 重定向 + setup 状态检查 + `bootstrapAuthentication()`。
- `_authenticated/route.tsx`：再 `resolveAuthentication()`，未登录重定向 `/sign-in?redirect=`。
- `(auth)`、`(errors)` 为无路径分组；`$section` / `$chatId` / `$modelId` / `$provider` / `$error` 为动态段。
- 搜索参数用 Zod schema + `validateSearch`（示例：`_authenticated/usage-logs/$section.tsx`）。
- 开发不分包；生产 `autoCodeSplitting: isProd` 按路由分包。
- 开发代理：`/api`、`/v1`、`/mj`、`/pg` → `VITE_REACT_APP_SERVER_URL`（默认 `http://localhost:3000`）。

### 15.4 状态与数据层

**Zustand（4 个，均在 `web/src/stores/`）**：`auth-store.ts`（`useAuthStore`，含 `auth.user`/`accessToken`/`session.sid`）、`notification-store.ts`、`system-config-store.ts`、`pricing-preferences-store.ts`。规范要求用选择器订阅、持久化状态在 store 内读写 localStorage。

**TanStack Query**（`web/src/lib/query-client.ts` 的 `createAppQueryClient(onInternalServerError)`）：`staleTime: 10s`、`refetchOnWindowFocus: false`、生产环境 retry ≤ 3 且不重试 401/403；`QueryCache.onError` → `handleServerError`（`meta.errorToast === false` 可关）；Mutation **仅在未自定义 `onError` 时**才自动提示；500 跳 `/500`。会话 `sid` 变更会在 `__root.tsx` 中 `queryClient.clear()`。

**Axios**（`web/src/lib/http-client.ts`）：`api = axios.create({ baseURL: '', withCredentials: true })`。
- 重写 `api.get` 做 **in-flight 去重**（`disableDuplicate` 关闭）；
- 请求拦截器注入 `Authorization: Bearer <accessToken>`（`singleUseAuthorization` / `X-Security-Proof` 走 `getFreshAuthHeaders()` 并禁用重试）；
- 响应拦截器 401 → `refreshAuthentication()` 重试一次，失败则提示 + `window.location.replace('/sign-in')`；
- 自定义配置项：`skipBusinessError`、`skipErrorHandler`、`disableDuplicate`、`skipAuthRefresh`、`authRetry`、`acceptAuthRotation`、`singleUseAuthorization`。

### 15.5 i18n

- **7 种语言**：`zhCN`、`en`、`fr`、`ru`、`ja`、`vi`、`zhTW`；文件 `locales/{en,zh,fr,ru,ja,vi,zh-TW}.json`（**`zh.json` ↔ `zhCN`，`zh-TW.json` ↔ `zhTW`**）。
- **结构不是顶层扁平**，而是 `{ "translation": { "<英文源文本>": "<译文>" } }`（`en.json` 约 7405 行 / 562KB）。**键就是英文源句。**
- `config.ts`：`fallbackLng: 'en'`、`load: 'currentOnly'`、`nsSeparator: false`（允许 key 内有冒号）、检测顺序 `localStorage → navigator`，`convertDetectedLanguage` 把浏览器 `zh-CN`/`zh-TW`/`zh-hant` 映射到 `zhCN`/`zhTW`。
- **`bun run i18n:sync` 做什么**（`scripts/sync-i18n.mjs`）：选叶子键最多的 locale 作基准 → 按基准键序重排并重写全部 7 个文件 → 缺失键用 `en.json` 回填 → 多余键导出到 `locales/_extras/{locale}.extras.json` → 疑似未翻译导出到 `locales/_reports/{locale}.untranslated.json` → 汇总 `_reports/_sync-report.json`。
  **它不扫描代码，不会自动发现新 key。** 新增 key 必须手工加到 `en.json`；动态 key 还要登记 `src/i18n/static-keys.ts` 的 `STATIC_I18N_KEYS`。
- **`toIntlLocale()`**（`languages.ts`）：`zhCN → zh-CN`、`zhTW → zh-TW`，其余走 `Intl.getCanonicalLocales(value)[0]`，异常返回 `undefined`。**`zhCN` 直接传给 `Intl.*` 会抛 `RangeError`。**

### 15.6 与后端的消息契约

后端控制台 API 的响应体是 `{ success, message, message_key, message_params, data }`：
- `message` = 英文源文本（给非浏览器调用方）；
- `message_key` + `message_params` = 给前端做翻译；
- 前端 `web/src/lib/server-error-message.ts` 的 `getServerMessage()` **优先用 `message_key`** 调 i18next。

> ⚠️ **所以：新增一条面向控制台的后端文案 = 同时在 7 个 locale 文件里加英文 key。** 后端用 `common.NewMessage` / `common.ApiErrorT` / `common.ApiSuccessT` 构造。

### 15.7 前端组件强制规范（摘要）

1. **先检索，再复用，最后才新增**：改 UI 前必须读 `shadcn-ui` 技能，用 `rg --files` / `rg` 检索 `src/components/` 与相关 `src/features/`，**按行为而非组件名查**。
2. **复用顺序**：业务封装（`@/components/dialog`、`confirm-dialog`、`copy-button`、`empty-state`、`loading-state`、`error-state`、`data-table`）→ `src/components/ui/` 基础件组合 → 外部库/自写。
3. **复用到行为层**：只 import `Button` / `AlertDialog` **不算**复用；已有封装能覆盖时必须用它。新增替代实现要说明具体能力缺口。
4. **数字/金额格式化**：普通数字用 `@/lib/format` 的 `formatNumber` / `formatCompactNumber`；金额用 `@/lib/currency`（`formatCurrencyFromUSD`、`formatQuotaWithCurrency`）。不得为统一调用方式改变精度。
5. **Intl locale**：任何进入 `Intl.*` / `toLocaleString` 的语言码必须先过 `toIntlLocale()`；`oxlint` 的 **`project/intl-locale` 是 error，不得 disable**。组件内 `const { i18n } = useTranslation()` 后在渲染时计算 locale，**不得固定在模块常量**。
6. 其它：禁 2 层嵌套三元（`no-nested-ternary: error`）、非必要不解构 props、`import type`、显式返回类型、可访问性（语义 HTML、`aria-*`、`button type`、WCAG AA 对比度 ≥ 4.5:1）。
7. **测试**：放在模块专属 `__tests__/` 目录，按职责命名（`layout.test.ts`、`selection.test.ts`、`validation.test.ts`）；禁止实现细节断言与 `sleep`。

---

## 16. Electron 桌面端

`electron/` 结构极简：`main.js`（唯一逻辑文件，CJS）、`preload.js`、`package.json`（`new-api-electron`）、`build.sh`、`create-tray-icon.js`、`entitlements.mac.plist`、图标、`README.md`。

- **主进程**：`PORT = 3000`、`DEV_FRONTEND_PORT = 5173`。
  - 生产模式 `startServer()`：设置 `env.PORT`、`SQLITE_PATH = app.getPath('userData')/data/new-api.db`、`ELECTRON_DATA_DIR`，然后 `spawn(getBinaryPath(), [], { cwd: process.resourcesPath })` 拉起**内嵌 Go 二进制**（win: `resources/bin/new-api.exe`；mac/linux: `resources/bin/new-api`），再 `checkServerAvailability` 轮询 `127.0.0.1:3000`（30 次 × 1s，**显式 IPv4 规避 localhost 解析成 IPv6 的问题**）。
  - 开发模式：不启动后端，只等 5173 就绪。
  - `createWindow()`：1080×720，`contextIsolation: true`、`nodeIntegration: false`；`close` 事件 → 隐藏到托盘（macOS 同时 `app.dock.hide()`）；`before-quit` 先 `SIGTERM` 子进程，5s 后 `SIGKILL`。
  - 崩溃时 `analyzeError()` 识别端口占用/数据库锁定/权限/网络/配置/内存/文件缺失并弹 `dialog`，可把日志写到 `app.getPath('logs')` 并打开。
- **preload**：`contextBridge.exposeInMainWorld('electron', { isElectron, version, platform, versions, dataDir })`。
- **打包**：`electron-builder`，`appId: com.newapi.desktop`、`productName: New-API-App`、输出 `electron/dist`；`extraResources` 把 `../new-api(.exe)` 拷到 `bin/`；目标 mac `dmg+zip`、win `nsis+portable`、linux `AppImage+deb`。
- **`build.sh` 流程**：`cd web && bun install --frozen-lockfile && bun run build` → 根目录 `CGO_ENABLED=1 go build -pgo=<moejs>/default.pgo -ldflags="-s -w" -o new-api(.exe)` → `npm install && npm run build:<platform>`。

---

## 17. 构建、测试与部署

### 17.1 本地开发最小闭环

```bash
# 1. 前端必须先构建，否则 Go 编译失败（//go:embed web/dist）
cd web && bun install --frozen-lockfile && bun run build && cd ..

# 2. 配置
cp .env.example .env      # 至少确认 PORT / SQL_DSN / SESSION_SECRET

# 3. 起后端
go run main.go            # 默认 http://localhost:3000

# 4. 前端热更新开发（另开终端）
cd web && bun run dev     # Rsbuild，代理 /api /v1 /mj /pg → :3000
```

`makefile` 目标：`all`、`build-web`、`build-all-web`、`start-api`、`dev`、`dev-api`、`dev-api-rebuild`、`dev-web`（5173）、`test`、`reset-setup`（清 `setups`、`role=100` 的用户、`SelfUseModeEnabled`/`DemoSiteEnabled` 选项，用于重测安装向导）。

### 17.2 ⚠️ `web/dist` 的强制约束

根 `main.go` 有：

```go
//go:embed web/dist
var buildFS embed.FS
//go:embed web/dist/index.html
var indexPage []byte
```

因此：
- **仓库里没有 `web/dist` 时，`go build` 直接失败**；
- 改了前端但没重新 `bun run build`，跑的是旧产物；
- CI 用 `touch web/dist/index.html` 绕过，`Dockerfile.dev` 用占位 `index.html` 绕过。

### 17.3 测试

```bash
make test          # 根模块（排除含 embed 的主包）+ relaykit + tokenkit，均 GOWORK=off
```

> `make test` 通过 `grep -vxF "$root_module"` **排除了根模块自身包**，覆盖范围与 `go test ./...` 不同。

前端：`cd web && bun run test`（Vitest，jsdom + `src/test-setup.ts`，`testTimeout: 20000`）；`src` 下约 203 个测试文件、2490 个用例。

`integration/` 目录只有 `doc_parse_test.go`：用 `sqlite.Open(":memory:")` + `gorm.Open` 覆盖 `model.DB`，内联一个 `doc-parse` 插件源码，跑 `service`/`controller` 的批量 + artifact 链路。**不是真实网络 e2e**（原名 `e2e/`，名不副实已改正）。

#### 判断测试结果时的三个坑

1. **不要用包退出码判断成败。** Windows 上 SQLite 临时文件句柄会让 `TempDir RemoveAll cleanup` 报 `being used by another process`，包 exit=1 但所有子测试都 PASS。应统计 `--- FAIL` 行数与失败用例名。
2. **并发跑多包会互相干扰。** 同一台机器并行 `go test` 多个包时，Windows 的 Go 构建缓存争用会报出假的 `[build failed]`；逐包串行跑即可，不要据此认为代码坏了。
3. **`go build` 缓存会掩盖删除引发的编译错误。** 删除常量后如果引用未清理，增量构建可能仍显示成功，直到 `go clean -cache` 后才暴露 `undefined: xxx`。**删除公共符号后务必 `go clean -cache && go build ./...` 复核一次。**

前端另有一个坑：`web/src/lib/lobe-icon.tsx` 的图标缓存是模块级 `Map`，缓存了 `React.lazy` 组件。开启 vitest `isolate: false`（可提速约 77%）会让这批缓存跨测试文件泄漏，导致 provider 图标相关用例**只在全量跑时**随机失败、单独跑必过。当前配置保持 `isolate: true`；若要提速请用 `--shard` 并行而非共享模块图。

### 17.4 Docker

**设计目标：把构建时长压到最低。** 结构上让每个阶段只被自己的输入失效，前后的工具链缓存都跨次复用。

**`Dockerfile`（三阶段 + 缓存挂载，`# syntax=docker/dockerfile:1.7`）**：

1. `web_deps`（`oven/bun:1.4.4`，digest 固定）：只 `COPY web/package.json web/bun.lock` 与 `web/.npmrc`，然后 `bun install --frozen-lockfile`，依赖树落在 `--mount=type=cache,id=bun-install,target=/root/.bun/install/cache`。**前端源码改动不会重装依赖。**
2. `build_web`（`FROM web_deps`）：`COPY web/ ./` + `COPY ./VERSION`，`NODE_ENV=production` 下 `bun run build`。
3. `build_backend`（`golang:1.26.10-alpine`，digest 固定）：`CGO_ENABLED=0 GOWORK=off GOEXPERIMENT=greenteagc`；先 `COPY go.mod go.sum` + `relaykit/go.mod` + `tokenkit/go.mod`（**子模块 go.mod 必须存在才能解析依赖图**），`go mod download` 与 `/root/.cache/go-build` 都用 cache mount；再 `COPY . .` 与 `COPY --from=build_web /build/web/dist ./web/dist`，最后
   ```
   go build -pgo="$(go list -m -f '{{.Dir}}' github.com/Calcium-Ion/moejs)/default.pgo" \
            -trimpath -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=${VERSION}'" -o new-api
   ```
   **`build_web` 与 `build_backend` 互不依赖，BuildKit 会并行调度两阶段。**
4. 运行阶段（`debian:bookworm-slim`，digest 固定）：`ca-certificates tzdata libasan8 wget`、`EXPOSE 3000`、`WORKDIR /data`、`ENTRYPOINT ["/new-api"]`。

三个基础镜像都用 `ARG BUN_IMAGE` / `GO_IMAGE` / `RUNTIME_IMAGE` 参数化，改版本只动一行。

**`.dockerignore` 是构建时长的一部分，不是可有可无的文件。** 现有排他项覆盖 `.github`、`.git`、`docs`、`electron`、`bin`、`e2e`、`integration`、`web/node_modules`、`web/dist`、`web/.tanstack` 等。两条硬规则：

- **不要排除会被 `COPY` 读到的目录**（`relaykit/`、`tokenkit/`、`plugins/`、`web/`、`VERSION`、`LICENSE`/`NOTICE`/`THIRD-PARTY-LICENSES.md`）。
- **不要写负向规则（`!xxx`）**：Docker 只在父目录未被排除时才应用否定，父目录一旦命中就整片丢弃。历史上排除 `.git` 曾连带把 `web/tsconfig.json`、`web/knip.config.ts` 挡在前端构建上下文之外。

**`Dockerfile.dev`**：只建后端，`mkdir -p web/dist` + 占位 `index.html` 满足 embed，配合 `make dev-web` 做前端热更新；同样带 `go-mod` / `go-build` 缓存挂载。

**`docker-compose.yml`**：`new-api`（`calciumion/new-api:latest`、卷 `./data:/data` 与 `./logs:/app/logs`、健康检查 `wget http://localhost:3000/api/status`）+ `redis`（`--requirepass 123456`）+ `postgres:15`；注释里备有 `mysql:8.2`、`clickhouse/clickhouse-server:24.8`。
**`docker-compose.dev.yml`**：源码构建（`Dockerfile.dev`），`SQL_DSN=postgresql://root:123456@postgres:5432/new-api`、`SESSION_COOKIE_SECURE=false`。

**本 fork 的自有镜像流程（`docker-publish.yml`，见 §17.5）**：main 每次推送构建并推送到 GHCR。本地等价命令：

```bash
# 本地构建（需 BuildKit，cache mount 才生效）
DOCKER_BUILDKIT=1 docker build -t cz-api:local .

# 只跑某一段
DOCKER_BUILDKIT=1 docker build --target build_backend -t cz-api:backend .

# 看每层耗时与缓存命中
DOCKER_BUILDKIT=1 docker build --progress=plain -t cz-api:local .
```

### 17.5 CI（`.github/workflows/`）

| workflow | 触发 | 作用 |
|---|---|---|
| `ci.yml` | PR | backend：`go vet` / `go build` 三个模块 + `make test`（先 `touch web/dist/index.html`）；frontend：`bun check -b` + `bun run test` |
| `docker-publish.yml` | push main / `v*` tag / 手动 / 触及 Dockerfile 的 PR | 唯一镜像工作流：amd64 + arm64 **原生分机并行**构建，推送到 `ghcr.io/xgp2012/cz-api`，再拼多架构 manifest |
| `electron-build.yml` | tag（忽略预发布） | Windows Electron 打包 |
| `release.yml` | tag（忽略 alpha） | Linux/macOS/Windows 二进制 + `checksums-*.txt`；arm64 用 `CC=aarch64-linux-gnu-gcc CGO_ENABLED=1` |
| `sync-release-to-gitcode.yml` | 手动 | 同步 release 到 GitCode |

**`docker-publish.yml` 的关键约束**（改这个文件前必读）：

- `build-push-action` 在 `push: true` 时**必须**同时给 `tags`，否则 buildx 立刻报 `ERROR: failed to build: tag is needed when pushing to registry`——注意这个错误发生在构建开始前，与 Dockerfile 无关。
- tag 形如 `sha-<7位>`（per-arch 阶段是 `sha-<7位>-<arch>`）；main 额外推 `latest`，`v*` tag 额外推该 tag。`latest` **只在 main 上更新**。
- `provenance: false` + `sbom: false`：省掉额外的 attestation layer 与 registry 往返。
- `cache-from/to: type=gha,scope=czapi-<ref>-<arch>`：缓存按架构隔离，互不覆盖。
- 并发组 `docker-publish-<workflow>-<ref>` + `cancel-in-progress: true`：同一 ref 的旧构建会被取消，不浪费 runner。
- 只用内置 `GITHUB_TOKEN`（`permissions.packages: write`），**不依赖任何 repository secret**。

**已删除的上游工作流**：`docker-build.yml` 与 `docker-image-branch.yml` 推送 `calciumion/new-api` 到 Docker Hub，依赖本 fork 没有的 `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` 与 cosign keyless 身份，在本仓库必然失败，已移除。**不要恢复它们**；需要镜像就改 `docker-publish.yml`。

### 17.6 版本注入

`VERSION` 文件（**本检出为空，0 字节**）→ 构建期 `-ldflags -X .../common.Version=$(cat VERSION)`；`common.Version` 默认 `"v0.0.0"`；运行时 `common.InitEnv()` 允许 `VERSION` 环境变量覆盖。发布流水线里由 CI 写入 `VERSION`。

**`VERSION` 为空时的表现**：镜像版本串为空，`/api/status` 与前端关于页显示的版本即为空。`docker-publish.yml` 因此改用 `git rev-parse --short HEAD` 生成 `sha-<7位>` 作为标签，不依赖 `VERSION` 内容。`VERSION` 仍必须留在构建上下文里（Dockerfile 要 `cat` 它）。

---

## 18. 开发工作流与硬性规范

### 18.1 改动前的必读清单

| 改动范围 | 必读 |
|---|---|
| 任何后端/前端改动 | 根 `AGENTS.md` |
| 任何 `web/` 改动 | 根 `AGENTS.md` + `web/AGENTS.md` + `.agents/skills/shadcn-ui/SKILL.md` |
| 涉及计费（见下） | `.agents/rules/billing.md` **全文**（用 read 工具，不是 grep/摘要） |
| 涉及时长/阶梯表达式 | 再加 `pkg/billingexpr/expr.md` |
| 涉及 JS 任务插件 | `docs/plugin-api/v1.md` + `v1.schema.json` + `v1.d.ts` |
| 涉及认证流程 | OWASP ASVS 最新稳定版 + 相关 Cheat Sheet（`AGENTS.md` 强制） |

**"涉及计费"的判定**（任一命中即需读 `billing.md`）：编辑 `pkg/billingexpr/`、`setting/billing_setting/`、`common/quota_math.go`、`types/price_data.go`、`relay/request_billing.go`、`relay/image_handler.go`、`relay/relay_task.go`、`relay/helper/price.go`、`relay/helper/billing_expr_request.go`、`relay/helper/valid_request.go`、`service/quota.go`、`service/text_quota.go`、`service/image_billing.go`、`service/tiered_settle.go`、`service/task_billing.go`、`service/responses_usage.go`、`service/log_info_generate.go` 的计费部分、`model/pricing*.go`、或 `relay/common/relay_info.go` 的计费字段/方法；或读写 `PriceData`/`OtherRatios`/预扣/结算/退款/消费日志计费字段；或从上游响应推导计费量；或校验/转发会变成计费乘数的字段；或新增/修改模型价格。

### 18.2 后端硬性规则

- **JSON**：业务代码**禁止**直接 import `encoding/json` 做 marshal/unmarshal，必须走 `common.Marshal` / `common.Unmarshal` / `common.UnmarshalJsonStr` / `common.DecodeJson` / `common.GetJsonType`。`relaykit/` 内用 `kitutil.*`。类型定义（`json.RawMessage` 等）仍可引用。
  *原因*：`common/json.go` 的 `init()` 里 `kitutil.SetCodec(hostJSONCodec{})`，宿主与 relaykit 共用同一个 JSON 引擎，便于整体替换（如 sonic）与测试一致。
- **三库兼容**：所有 DB 代码必须同时支持 SQLite / MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6。用 `lockForUpdate`、`commonGroupCol`、`commonKeyCol`、`commonTrueVal`、`commonFalseVal`、`UsingMainDatabase` / `UsingLogDatabase`。禁用 `ALTER COLUMN`（SQLite 不支持），SQLite 迁移用 `ALTER TABLE ... ADD COLUMN`。
- **避免 `gorm:"default:true"`**：MySQL 与 PG 对布尔默认值的归一化不同，会导致 `AutoMigrate` 每次启动都重发 `ALTER TABLE`。把默认值放在请求/模型归一化、hook、构造函数或服务逻辑里。
- **现代 Go 约定**：`any` 代替 `interface{}`；`for i := range n`；`strings.SplitSeq`；`strings.Cut` / `CutPrefix` / `CutSuffix`；`slices.Contains` / `slices.Sort`；`maps.Copy`；内置 `min` / `max`；`strings.Builder`；`reflect.TypeFor[T]()`；`sync.WaitGroup.Go`。
- **可选标量必须用指针 + `omitempty`**：从客户端 JSON 解析后再转发给上游的请求结构体，可选标量字段（`*int`、`*uint`、`*float64`、`*bool`）必须能区分"缺省"与"显式 0/false"。**显式零值必须原样转发。**（`relaykit/dto/openai_request_zero_value_test.go` 保护这个契约。）
- **模块独立性**：`relaykit/` 与 `tokenkit/` 必须能独立构建；`tokenkit/` 还不得依赖 `relaykit/`。改完后在模块内跑 `GOWORK=off go build ./...`。
- **修改的 Go 文件要 `gofmt` 并清理未用 import。**
- **不要再往 `docs/` 加文件**（除非用户明确要求），也不要在 `plugins/` 下生成任何文档文件。

### 18.3 后端测试质量

- **不要为一个小改动散落测试**：优先扩展现有合适的测试文件；确需新文件时最多加一个，把关键回归用例集中在那里。**不得**因为调用链跨 `controller/`、`service/`、`setting/` 就为同一个功能在各层各建一个测试文件。
- 不要写只为提高覆盖率、只证明代码能跑、或锁定实现细节的测试。
- 不要写假的 fuzz / stress / smoke / 性能测试（随机输入、大循环、sleep、计时比较、只断言日志）。
- 优先**确定性表驱动测试**，输入与期望输出明确。
- 需要数据库/请求上下文/用户组/设置/缓存状态时，在测试 fixture 内显式初始化。
- 新的或大幅重写的 Go 测试必须用 `github.com/stretchr/testify/require` 做 setup 与致命断言，`assert` 做非致命值检查。

### 18.4 治理约束

- **受保护的项目信息**：任何与 **new-api**（项目名/身份）和 **QuantumNous**（组织/作者身份）相关的引用、品牌、元数据、归属**不得**修改、删除、替换或移除，包括 README、license 头、版权声明、包元数据、HTML 标题、meta 标签、页脚、关于页、Go module 路径、包名、import 路径、Docker 镜像名、CI 引用、部署配置、注释、文档与 changelog。被要求这么做时应拒绝并解释这是项目策略保护的信息。
- **提交 PR 前**：先对比当前 git 用户与仓库历史核心开发者（`git log` 里反复出现的作者）。若当前 git 用户不是其中之一，**必须在 PR 正文中明确声明代码是 AI 生成或 AI 辅助的**。
- **Issue / PR 模板**：项目所有者的 PR 用 `.github/PULL_REQUEST_TEMPLATE.md`（中文）或 `.github/PULL_REQUEST_TEMPLATE/en.md`（英文）；其他 agent 创建的 PR 用 `.agents/github/PR.md` 作为整个正文。Issue 先按 `.agents/github/ISSUE.md` 拒绝超范围请求（Coding Plan、逆向渠道、第三方包装、Codex 反代兼容、纯透传转发、第三方主机）。
- 验证必须是**实际运行并观察到的命令与结果**，不能只写"`go build` / 测试通过了"。

### 18.5 构建与流水线效率（强制）

这些规则的目的是**避免把时间浪费在重复劳动上**。违反其中任何一条，都必须在变更说明里解释原因。

**Docker 镜像构建**

1. **阶段必须按输入失效。** 新增 `COPY` 时先问：这一行会让哪个阶段失效？依赖安装阶段（`web_deps`）只允许出现 `package.json` / `bun.lock` / `.npmrc`；把任何源码 `COPY` 进依赖层，都会让每次改动重装 1.5 GB 依赖。
2. **保持 `build_web` 与 `build_backend` 互不依赖。** 除了后端 `COPY --from=build_web /build/web/dist` 之外，不要让两侧互相依赖——一旦串联，两个阶段就只能串行。
3. **`RUN` 必须带 cache mount。** 依赖下载与编译类命令（`bun install`、`go mod download`、`go build`）一律用 `--mount=type=cache`，并用固定 `id` 复用。
4. **不要用单次全量 `COPY . .` 喂依赖解析。** 依赖阶段用精确 `COPY`，源码阶段才 `COPY . .`。
5. **`.dockerignore` 必须覆盖无关目录**：新增顶层目录（文档、脚本、生成物、本地构建产物）时，同步评估是否加入排除列表；**但不得排除任何 `COPY` 需要读取的路径**。
6. **`.dockerignore` 不写负向规则。** 见 §20 第 16 条。
7. **镜像体积也算构建时长**：不要往运行阶段装构建工具；需要新系统包时优先复用已有包。

**GitHub Actions**

8. **禁止 QEMU。** 多架构一律用原生 runner（`ubuntu-latest` + `ubuntu-24.04-arm` 并行），不要用 `--platform linux/amd64,linux/arm64` 在单机上模拟。
9. **必须使用 `type=gha` 缓存**，并按架构分 scope，避免两个架构互相覆盖缓存条目。
10. **同一 ref 必须有并发取消**（`concurrency.cancel-in-progress: true`）；否则连续推送会同时跑多个注定被替代的构建。
11. **不要无理由打开 `provenance` / `sbom`**：它们产生额外 layer 与额外的 registry 往返。
12. **不新增需要 secret 的工作流。** 本 fork 只依赖内置 `GITHUB_TOKEN`；引入新的 repository secret 前必须先确认它确实存在，否则该工作流每次都会失败。
13. **`push: true` 必须给 `tags`**（§20 第 17 条）。
14. **一个目的只留一个工作流。** 已删除的上游镜像工作流不要恢复（§20 第 18 条）；要改发布行为就改 `docker-publish.yml`。

**本地迭代**

15. **改 Go 用 `Dockerfile.dev`**，不要为了验证后端去跑完整三阶段构建。
16. **判读测试结果不要看包退出码**，看 `--- FAIL` 行（§17.3）。Windows 上大量 `unlinkat` 清理失败是环境噪声，不是回归。
17. **改动前先在基线 commit 上跑一遍可疑用例**（`git worktree add ../cz-api-base <sha>`），确认它本来就失败再动手——本仓库大量 `*DatabaseMatrix`、passkey、security 用例在 Windows 下常驻失败。
18. **构建结论只看真实运行**：本地无 Docker 时用 `gh run watch` 等 CI 结果，不要以静态分析代替实际构建结论（§18.6）。

### 18.6 必备工具与强制使用 `gh`

**`gh`（GitHub CLI）是本仓库开发的必备工具，不是可选项。** 凡是涉及远端状态的事实，都必须用 `gh` 取得，不得凭推测下结论。

**必须使用 `gh` 的场景**：

| 场景 | 命令 | 为什么必须 |
|---|---|---|
| 判断 CI 是否通过 | `gh run list --limit 5` | 本地测试不代表流水线结果 |
| 定位失败原因 | `gh run view <id> --log-failed` | 只有日志能给出真实报错，不要靠猜 |
| 等构建结果 | `gh run watch <id> --interval 30 --exit-status` | 以完成态为依据，不做轮询猜测 |
| 查看/触发 workflow | `gh workflow list` / `gh workflow run <file>` | 手动验证用 |
| 查看 tag 与 release | `gh release list` / `gh release view` | 发布流程的事实来源 |
| PR 与 Issue | `gh pr create` / `gh issue view` | 正文按 §18.4 的模板要求填写 |
| 仓库设置与权限 | `gh api repos/<owner>/<repo>` | 诊断权限类失败（如 GHCR 写入） |

**硬性要求**：

1. **报告 CI 结果前必须先 `gh run list` 确认**，不允许出现"应该通过了"这类表述。失败时**必须贴出 `--log-failed` 中的真实报错行**。
2. **诊断构建/部署问题先取日志再改代码。** 先 `gh run view <id> --log-failed`，确认失败在哪个步骤、哪个工具，再决定改什么。本仓库曾把 `tag is needed when pushing to registry`（缺 `tags`）误判成代码或权限问题——看日志一次就能定位。
3. **多架构/多 job 的失败要分别看。** `gh run list` 只给 run 级 conclusion，`gh run view <id>` 才给出每个 job 的结论与耗时。
4. **证明耗时改善要用 run 元数据**：`gh run view <id> --json jobs` 里有每个 job 的 `startedAt` / `completedAt`，用它对比优化前后，而不是凭感觉。
5. **`gh` 已登录（`gh auth status` 确认）时不要让用户代查远端信息。** 只有确实缺少权限（例如 `read:packages`）时，才向用户说明并请求。

**其它必备工具**：

| 工具 | 用途 | 说明 |
|---|---|---|
| `git` | 版本控制 | 分支 `main`，远端 `origin` = `github.com/xgp2012/cz-api` |
| `gh` | 远端事实来源 | 见上，**必备** |
| Go ≥ 1.26.10 | 后端 | 三个模块均需 `GOWORK=off` 验证 |
| Bun 1.4.4 | 前端 | `bun install` / `bun run build` / `bun check -b` |
| Docker + BuildKit | 镜像 | 本地无 Docker 时改由 CI 验证，并用 `gh run watch` 取结果 |
| `python`（可选） | 一次性辅助校验 | 可用来校验 workflow YAML 语法等；**不得因此把脚本文件留在仓库里** |

---

## 19. 常见任务操作手册

### 19.1 新增一个渠道适配器
见 [8.3](#83-新增一个渠道的完整清单)。

### 19.2 新增一个任务插件（视频/图片厂商）
1. 读 `docs/plugin-api/v1.md`、`v1.schema.json`、`v1.d.ts`；
2. 从 `examples/plugins/` 里最接近的一个复制改写（见 [14.2.1](#1421-可复制的示例插件examplesplugins)），不要从零写；
3. 新建 `plugins/tasks/<key>/plugin.js`（单文件同步 ESM）。`kind: "task"` 需要 `meta` + `buildSubmitRequest` + `parseSubmitResponse` + `parseTaskResult`，再按 `fetchMode` / `submitResponseTypes` / artifact 需求补齐（见 [14.3](#143-插件契约docsplugin-apiv1md权威定义-v1schemajson--v1dts)）；
4. `//go:embed tasks` 会自动包含新目录，但需确认 `Options.Key` 与目录名一致；
5. 若要作为默认平台，登记到 `relay/relay_adaptor.go` 的 `taskPluginKeys`；
6. 计费字段的 `description` 必须写"计费主体 + 单价"，单位放 `unit`；
7. 本地校验：`new-api plugin lint`（`pkg/jsplugin/cli.go`）；
8. **不要在 `plugins/tasks/<plugin>/` 下生成 README 或任何文档文件。**

### 19.2.1 新增一个定时作业插件（`kind: "job"`）

区别于任务插件：job 不面向客户端请求，而是由宿主调度器按周期触发。

1. `plugin.js` 里写 `apiVersion: 2` 与 `kind: "job"`——**v1 上声明非 task 的 kind 会被拒绝**；
2. **不要**写 `models`、`fetchMode`、`routes`、`protocols`、`channelTypes`（前三项对 job 不适用，后三项会被拒绝）；
3. 必须导出 `run(request)`；可选 `enabled()`、`intervalSeconds()`、`newPayload()`、`taskType()`；
4. 若导出 `taskType()`，返回值必须正好是 `plugin:<你的 key>`；
5. 调用 `service.RegisterPluginJobHandler(plugin)` 完成绑定（内置插件走 `plugins/embed.go` 的注册路径）；
6. 作业拿到的 `request` 只有 `task_id` / `type` / `payload`——**拿不到 lease、渠道或用户凭据**，需要外部数据请在 `payload` 里带。

写好后的行为约定：`enabled()` 抛错视为未启用；`run()` 抛错或 panic 只把该次执行记为失败。

### 19.3 新增一个模型价格
- 写成**自包含计费表达式**放 `setting/billing_setting/builtin_billing.go`，用真实 USD / 1M tokens；
- **不要**加到旧的 model / completion / cache ratio 表；
- 保留管理员的显式价格覆盖；
- 核对公开价格，覆盖适用的上下文长度阈值与缓存类别。

### 19.4 新增一条控制台 API 错误文案
1. 后端用 `common.NewMessage` / `common.ApiErrorT` / `common.ApiSuccessT` 构造（英文源文本）；
2. 在 `web/src/i18n/locales/` 的**全部 7 个** JSON 的 `translation` 对象里加同字面量键；
3. 如果该错误是网关写入存储日志的失败原因，它必须是固定英文句子，且同样是前端 locale 文件的键；
4. 如果文案只面向"没有 Web 控制台在中间"的场景（返回给 AI 客户端的错误、邮件、通知），则用后端 `i18n.T` / `i18n.Translate`（`i18n/keys.go`），支持 en / zh-CN / zh-TW。

### 19.5 新增一个前端页面
1. 在 `web/src/routes/` 建文件（`createFileRoute`），让 dev/build 重新生成 `routeTree.gen.ts`；
2. 受保护页面放在 `_authenticated/` 下；
3. 搜索参数用 Zod + `validateSearch`；
4. 业务 UI 放 `web/src/features/<feature>/`，通用组件放 `web/src/components/`；
5. 所有用户可见文案走 `t('English key')` 并补 7 个 locale；
6. 数字/金额用 `@/lib/format` / `@/lib/currency` + `toIntlLocale()`；
7. 跑 `bun check -b`、`bun run lint`、受影响测试。

### 19.6 新增一个 API 路由（控制台）
1. 在 `router/api-router.go` 的对应子组注册；
2. **如果该路由要支持 PAT，必须加进 `middleware/access_token_routes.go` 的 `accessTokenRouteRules`**，否则 fail-closed 直接 403 并打 `access token route is not declared`；
3. 渠道类路由应走 `channel-router.go` 的 `handlePermissionRoute`（同时声明 PAT scope 与 Casbin 权限）；
4. 若引入新权限，**同时注册到 authz Registry 与角色基线**；
5. 涉及插件契约时同步 `docs/plugin-api/v1.md` / `v1.schema.json` / `v1.d.ts`（三者必须一致）；
6. 用 `gh` 验证：改动推上去后 `gh run list` 确认 CI 结论，失败时 `gh run view <id> --log-failed` 取真实报错（§18.6）。

### 19.7 修改 Docker/CI 流水线

1. 先读 §17.4–17.6（Docker 结构、CI 表、版本注入）与 §18.5（效率强制规则）；
2. 改 `Dockerfile` 后**必须**跑 `gh run list` + `gh run view <id> --log-failed` 确认：
   - 失败发生在哪个 stage（`web_deps` / `build_web` / `build_backend` / 运行阶段）；
   - 缓存是否命中（日志里找 `importing cache manifest`）；
3. 声称"变快"必须有依据：用 `gh run view <id> --json jobs` 对比各 job 的 `startedAt` / `completedAt`；
4. **不要恢复** `docker-build.yml` / `docker-image-branch.yml`（§20 第 18 条）；
5. 镜像发布只走 `docker-publish.yml`，且只依赖内置 `GITHUB_TOKEN`。

---

## 20. 坑点与雷区总表

### 数据层
1. **SQLite 不支持 `FOR UPDATE`** —— 必须走 `lockForUpdate()`，否则报语法错。
2. **布尔 `default:true` 导致每次启动重发 `ALTER TABLE`** —— MySQL/PG 归一化不同，`migration_dialector.go` 只对 decimal 做了兜底。
3. **`//go:embed web/dist` 要求目录存在** —— 缺目录 `go build` 直接失败。
4. **迁移必须幂等** —— 需在"全新库"和"上一发布版升级库"各验证一次，并**至少启动两次**。
5. **迁移只由 master 执行** —— 从节点提前升 schema 会错乱。
6. **32 位额度 schema 直接拒启** —— 老库需先手工改列或设 `SKIP_64BIT_QUOTA_SCHEMA_CHECK=true`。
7. **MySQL 字符集不符启动即 panic**（不是告警）。
8. **`SESSION_SECRET=random_string`**（compose 示例值）会 `log.Fatal`；多节点必须一致。
9. **`REDIS_CONN_STRING` 存在即强制 `MemoryCacheEnabled=true`**，且 TTL = `SYNC_FREQUENCY` —— 调大它同时放大缓存 TTL 与跨节点不一致窗口。
10. **`NODE_TYPE` 只认 `slave`** —— 写成 `Slave`/`worker` 会被当 master，可能双跑迁移与定时任务。
11. **`BATCH_UPDATE_ENABLED` 滞后落库** —— 崩溃可能丢失最多一个 `BATCH_UPDATE_INTERVAL` 的额度增量。
12. **ClickHouse 只能做日志库** —— 主库 DSN 为 `clickhouse://` 直接报错。
13. **`VERSION` 为空** —— `docker build` 会注入空版本串；发布靠 CI 写入。
14. **`make test` 排除了根模块自身包**。
15. **`VERSION` 为空 → 镜像版本串为空**；打标签靠 CI 写 `VERSION`，或像本 fork 一样用 commit SHA。
16. **`.dockerignore` 排除了父目录就别指望 `!` 能救回来** —— Docker 的否定规则只在父目录未被排除时生效。曾因排除 `.git` 连带丢弃 `web/tsconfig.json` 与 `web/knip.config.ts`。
17. **`push: true` 的 build 步骤必须显式给 `tags`** —— 否则 buildx 在开始构建前就报 `tag is needed when pushing to registry`，与本仓库代码无关。
18. **不要恢复 `docker-build.yml` / `docker-image-branch.yml`** —— 它们依赖本 fork 不持有的 Docker Hub 与 cosign 凭据，必然失败。

### 后端 / 转发
19. **SSE 与 gzip 冲突** —— 引擎级 gzip 被注释禁用；`/api` 组仍 gzip，`/v1` 不压缩。给转发路由加 gzip 会截断流。
20. **全局状态 + init 顺序** —— `common` 全是包级变量，`InitEnv` → `model.InitDB` → `model.InitOptionMap` 顺序不可换。
21. **注册几乎都在 `init()`** —— oauth provider、authz 资源、system task handler 的跨包顺序不可控。
22. **函数变量解耦循环依赖** —— `service.GetTaskAdaptorFunc` 必须在 `service.StartSystemTaskRunner()` **之前**赋值；同类还有 `i18n.SetUserLangLoader`、`common.TranslateMessage`。
23. **PAT 路由 fail-closed** —— 新 `/api` 路由未声明即 403。
24. **Casbin 无 RBAC 继承** —— 新权限必须同时进 Registry 与角色基线，否则 `isKnownPermission` 直接拒绝。
25. **混淆常量勿改** —— `Auth-Version`、`specific_channel_version`、`Cache-Version` 是前端用于探测版本差异的固定值。
26. **`TokenAuth` 按 `-` 截断 key** —— token 本体不能含 `-`，第二段被当渠道 ID。
27. **`Distribute` 与 `controller.Relay` 双重重试** —— `Relay` 用 `service.RetryParam` 循环 `getChannel`，每次重试都要重建渠道上下文。
28. **两套限流算法并存**（固定窗口 vs List 滑窗），调参要分别改。
29. **`RELAY_RESPONSE_HEADER_TIMEOUT` 只约束"等响应头"** —— 非流式请求要等上游生成完才会返回响应头，值需留足余量。

### 计费
30. **禁止裸配额转换** —— 必须用 `common/quota_math.go` 的 helper 与 `*Checked` 变体。
31. **`*uint` 字段可接受巨大正值** —— `>= 0` 不够，必须有上界。
32. **透传绕过路径** —— `Extra["parameters"]`、任务 `metadata`、multipart 字段；`relay/common/override.go` 的 ParamOverride / HeaderOverride 同样是可以绕过 DTO 校验的乘数通道，对 `n` / `max_tokens` 仍需二次钳制。
33. **不得用 `data.#` 数图片** —— 用 `openaiImageResponseCount` 的规则。
34. **客户端中断不得降低图片计费**；JSON-as-SSE 转发时不能让客户端只收 `[DONE]` 却被扣图片钱。
35. **表达式路径与传统倍率路径的浮点处理不同，不得混用**。
36. **`fixed()` 只能作完整 `tier` 叶子**；任务与 Realtime 拒绝 `fixed()`。
37. **任务差额结算**：`AdjustBillingOnComplete` 返回 0 = 保持预扣；落库前 `Reserve` 失败但已向客户端写过时仍继续结算，**可能透支**。
38. **`InitChannelMeta` 不得清空 `StreamStatus` / `PriceData` / 计费累计量**。

### 前端
39. **`web/dist` 必须存在才能编译 Go**；改了前端不重新 build 就跑的是旧产物。
40. **i18n 新 key 不会自动提取** —— `i18n:sync` 只做对齐/回填；动态 key 还要登记 `static-keys.ts`。
41. **`_extras` / `_reports` 是同步产物**，不参与运行时。
42. **`zhCN` 不能直接传给 `Intl.*`** —— 会抛 `RangeError`；`project/intl-locale` 是 error 且不得 disable。
43. **`src/routeTree.gen.ts` 是生成物** —— 手改会被覆盖；新增页面必须新建 `routes/` 文件。
44. **错误提示只报一次** —— `handleServerError` 用 WeakSet 沿 `cause` 去重；Mutation 自定义 `onError` 后自动 Toast 失效。
45. **`api.get` 默认 in-flight 去重** —— 需要重复调用要 `disableDuplicate`。
46. **Electron 端口写死 3000** —— 与本地已运行的 Go 实例冲突会触发内置"端口被占用"诊断弹窗；同名 SQLite 被两个进程占用会 `database is locked`。

---

## 21. 附录

### 21.1 官方资源

- 文档：<https://docs.newapi.ai/zh>
- LLM 索引：<https://docs.newapi.ai/llms.txt>、全文 <https://docs.newapi.ai/en/llms-full.txt>
- 上游仓库：<https://github.com/QuantumNous/new-api>（本仓库 fork 自/基于它）
- 插件市场与插件文档：<https://docs.newapi.ai/zh/plugins>

### 21.2 后端 i18n（`i18n/`）

- 库：`nicksnyder/go-i18n/v2`；语言：**en / zh-CN / zh-TW**；`DefaultLang = LangEn`；`//go:embed locales/*.yaml`。
- `normalizeLang` 把 `zh-HK` / `zh-MO` / `zh-Hant` 归为繁体。
- 声明语言四级：已加载的 `dto.UserSetting.Language` → `SetUserLangLoader(model.GetUserLanguage)` 惰性加载 → `middleware.I18n()` 写入的 `ContextKeyLanguage` → `Accept-Language`。
- **`DEFAULT_LANGUAGE` 的双重作用**：无人声明语言时的备用语言；以及服务器日志行语言（通过 `SetLogLanguage`，**仅 `zh*` 生效**，读 `common/log_text.zh-CN.json`）。
- 键集中在 `i18n/keys.go`（如 `distributor.no_available_channel`、`token.invalid`、`rate_limit.total_reached`）。
- 服务器日志行用 `common.LogText("english %s", ...)`。

### 21.3 日志与可观测性

- `logger.SetupLogger()` 初始化；`common.SysLog` / `SysError` / `FatalLog` / `LogText`。
- `ENABLE_PPROF=true` → 8005 端口 pprof + `common.Monitor()`。
- Pyroscope：`PYROSCOPE_URL` / `PYROSCOPE_APP_NAME` / `PYROSCOPE_BASIC_AUTH_*` / `PYROSCOPE_MUTEX_RATE` / `PYROSCOPE_BLOCK_RATE`。
- `pkg/perf_metrics`（`perfmetrics.Init()`）+ `setting/perf_metrics_setting`：按模型/分组/时间桶记录性能指标（`perf_metrics` 表）。
- 消费日志落库：`service/log_info_generate.go` + `model.RecordConsumeLog`；存储的日志内容是 `[]*common.Message`。
- 饱和审计：`other.admin_info.quota_saturation`（挂在 `admin_info` 下天然仅管理员可见，非管理员日志视图会剥离 `admin_info`）。
- 请求追踪：`X-Oneapi-Request-Id`；日志表有 `idx_logs_request_id` / `idx_logs_upstream_request_id`。

### 21.4 支付与订阅

- 支付渠道：Stripe（`setting/payment_stripe.go`、`controller/topup_stripe.go`）、易支付 epay（`service/epay.go`、`controller/topup.go`）、Creem（`setting/payment_creem.go`、`controller/topup_creem.go`）、Waffo / Waffo Pancake（`setting/payment_waffo*.go`、`controller/topup_waffo*.go`、`service/waffo_pancake.go`）。
- Webhook 回调路由：`/api/user/epay/notify`、Stripe / Creem webhook 等，参见 `docs/openapi/api.json`。
- **回调 URL 域名安全**由 `TRUSTED_REDIRECT_DOMAINS` 校验。
- 订阅：`model/subscription.go` 的 `SubscriptionPlan` / `SubscriptionOrder` / `UserSubscription` / `SubscriptionPreConsumeRecord`；重置任务 `service/subscription_reset_task.go`（日/周/月/自定义）；`BillingSourceSubscription` 是与钱包并列的扣费来源。
- 合规检查：`controller/payment_compliance.go`。

### 21.5 未验证项（分析时未能确认）

以下各项是**已知的空白**，不要把它们当作已确认的事实：

- ClickHouse 的实际运行行为（本地无实例）。
- `system_task_locks` 在 SQLite 下的并发竞争表现。
- `service/http_client.go` 的客户端缓存容量与淘汰策略。
- `middleware/task_plugin.go` 的全部分支语义。
- `/v1beta` 与插件路由的实际优先级冲突边界。
- `relay/common/override.go` 的 ParamOverride / HeaderOverride 对 `n` / `max_tokens` 的钳制是否逐条覆盖。
- `web/` 实际使用的 TypeScript 编译器版本（由 Bun 提供）。

已确认的部分（原列于此，现已核实）：`plugins/tasks/*/plugin.js` 的契约字段——`docs/plugin-api/v1.md` 为权威定义，`examples/plugins/` 的 6 个示例覆盖了各形状且由 `pkg/jsplugin/examples_test.go` 保证可加载。

---

## 22. 变更记录（文档本身）

| 日期 | 变更 |
|---|---|
| 初次生成 | 基于 commit `e03439033` 的全量静态分析 |
| 本轮 | 修正 §3 目录地图（`e2e/`→`integration/`、删除的 `bin/`、新增 `examples/`、各目录文件数）；修正 §8.2 渠道数量 59→53 与适配器目录数；新增 §8.2.1 渠道目录注册表；§14.1 补充新增文件；新增 §14.2.1 示例插件、§14.3 三种 `kind` 契约、§14.3.1 契约查询 API、§14.3.2 job 桥接；修正 §19.2「4 个必需函数」的错误表述并新增 §19.2.1 job 插件手册；§17.3 补充三个测试判断坑点与前端隔离坑；§21.5 更新已核实项 |
| 2026-11-08 | 重写 §17.4 Docker（三阶段 + BuildKit cache mount + `.dockerignore` 约束）、§17.5 CI（`docker-publish.yml` 为唯一镜像工作流、上游 Docker Hub 工作流已删除及原因）、§17.6 版本注入（`VERSION` 为空时的表现）；新增 §18.5 构建与流水线效率强制规则、§18.6 必备工具与强制使用 `gh`；§20 新增 Docker 相关坑点 4 条并整体重新编号（46 条）；§19.6 补充 `gh` 验证要求 |

*本文档由对 `cz-api` 仓库的全量静态分析生成。文档不修改仓库任何代码；文中数字随代码演进会再次过时，**修改相关代码时请同步更新对应小节**。*
