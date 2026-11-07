# new-api 全面插件化架构方案 + 可剔除清单

> 分析基线：本仓库 `github.com/xgp2012/cz-api`（代码实体 **New API** / `github.com/QuantumNous/new-api`），commit `e03439033`，2026-11-06。
> 本文档为**提案**，不含任何代码改动。所有论断均带 `file:line` 证据；标注【推断】的是基于代码结构的判断，非直接可证。
> 相关强制规范：根 `AGENTS.md`、`web/AGENTS.md`、`.agents/rules/billing.md`、`docs/plugin-api/v1.md`。

---

## 目录

1. [结论摘要](#1-结论摘要)
2. [现状：已有的插件系统，以及它为什么不够](#2-现状已有的插件系统以及它为什么不够)
3. [设计原则：什么能插件化，什么不能](#3-设计原则什么能插件化什么不能)
4. [T0 冻结核心（不可插件化）](#4-t0-冻结核心不可插件化)
5. [插件化分区](#5-插件化分区)
6. [插件宿主 ABI v2](#6-插件宿主-abi-v2)
7. [信任模型与权限分级](#7-信任模型与权限分级)
8. [实施里程碑](#8-实施里程碑)
9. [可剔除清单（决策表）](#9-可剔除清单决策表)
10. [需要你决策的问题](#10-需要你决策的问题)

---

## 1. 结论摘要

**你已经有一套可运行的插件系统**，而且质量不低：`pkg/jsplugin` 约 5200 行非测试代码 + 3800 行测试，moEJs 进程内 JS 运行时，带注册表、原子路由代际发布、冲突准入、并发准入、看门狗超时、受限出网白名单、fixture 回放 CLI。`plugins/tasks/` 有 11 个生产插件，`relay/channel/task/jsplugin/adaptor.go` 约 2100 行把 JS driver 桥接成 `channel.TaskAdaptor`。

**任务渠道的插件化已经成功落地过一次**——这是本次方案最重要的先例：它证明了「第三方写 JS → 宿主统一计费/路由/存储」这条路在本代码库可行。

**但现有 ABI 是任务专用的**，把它变成「全面插件化」缺的不是运行时，而是三样东西：

| 缺口 | 证据 | 严重度 |
|---|---|---|
| **宿主协议表封闭**：只有 `openai_responses` / `openai_video` / `openai_image` | `pkg/jsplugin/routing.go:89-106` | 高——JS 插件**无法**表达一个 chat/embedding/rerank 渠道 |
| **渠道身份是编译期 iota 常量**，且前端渠道表是前端硬编码 | `constant/api_type.go:3-43`、`constant/channel.go:3-64`、`web/src/features/channels/constants.ts:41-102` | 高——新增渠道必须改宿主并重新发版 |
| **分派是平行 switch 链**，无注册点，且未知类型**静默回退 OpenAI** | `relay/relay_adaptor.go:50-128`（约 35 case）、`common/api_type.go:5-94`（fallback 在 `:85-92`） | 高——是隐性错误源 |

**推荐路线**：不新造体系，把现有 `pkg/jsplugin` 从「任务插件」泛化为「通用插件宿主」，分 4 个里程碑推进。核心改动是**把三张硬编码表（协议表、渠道类型表、适配器 switch）变成运行时注册表**，其余基础设施全部复用。

**同时必须明确拒绝插件化的边界**：计费换算、额度事务、鉴权判定、密钥与凭证、迁移、核心 relay 管道。见 §4。

---

## 2. 现状：已有的插件系统，以及它为什么不够

### 2.1 已经存在且可直接复用的部分

| 能力 | 位置 | 说明 |
|---|---|---|
| 双层覆盖注册表（factory / override）+ 哈希增量同步 | `pkg/jsplugin/registry.go:300-330,620-646,713-816` | 内置插件与管理员上传同构 |
| 原子路由代际发布 | `pkg/jsplugin/routing.go:417-436,1106-1197`；`registry.go:764,786` | 单调 generation + `atomic.Store`，请求侧钉住代际 |
| 冲突准入与 incumbent 回退 | `routing.go:1026-1097,1204` | 冲突插件被剔除，旧代际继续服务 |
| JS 引擎层 | `pkg/jsplugin/engine.go`（898 行） | 语法门禁、hook 句柄缓存、runtime 池、`acquireCallSlot`（`:548-571`）、watchdog（`:734-801`）、`HookError`/`ResultError` |
| 受限出网 | `pkg/jsplugin/fetch.go`、`request.go:14-36` | 唯一出网通道，host 白名单 + header 黑名单 + 额度限制 |
| 严格 meta 解码与本地化文案 | `registry.go:1016-1118` | 未知字段即拒绝 |
| CLI 与 fixture 回放 | `pkg/jsplugin/cli.go:13-60`、`fixture.go:39-116` | `new-api plugin lint` / `test --fixture` |
| 动态路由注册 + NoRoute 分发 + 响应闸门 | `router/plugin-router.go:99-158,438-473,509-604` | 在 Gin 外层重写路由分组 |
| 宿主协议表驱动 | `routing.go:73-110`、`LookupHostProtocolOperation`（`:190-203`） | 已是数据驱动，**这是最容易扩展的一处** |

### 2.2 硬编码绑死任务语义的部分

| 项 | 位置 |
|---|---|
| 协议表固定 3 项及固定 path | `routing.go:89-106` |
| `RouteType` 只有 submit / query / dynamic | `routing.go:19-25` |
| 必填 hook 集 = 提交/轮询生命周期 | `registry.go:351-372` |
| 平台身份 ≡ 插件 key | `relay/relay_adaptor.go:130-139` |
| 平台→插件映射硬表（12 项） | `relay/relay_adaptor.go:141-154` |
| 计费钩子 `extractUsage` / `OnSubmit` / `OnComplete` | `relay/channel/task/jsplugin/adaptor.go:240-318` |
| 表名 `task_plugins`、路由 `/api/plugin/task` | `model/task_plugin.go`、`router/api-router.go:263` |
| 观测限流按 pluginKey/userID/tokenID 硬编码 | `controller/plugin_protocol_limiter.go:65-78` |

### 2.3 安全现状（重要）

现有 JS 插件**不能**：
- 直接发任意 HTTP（唯一出网是 `utils.fetch`，且宿主执行侧二次校验：`relay/channel/task/jsplugin/adaptor.go:827,870`）
- 读写 DB / Redis / env / 文件系统（无任何对应 host function）
- 看计费、额度、用户对象（只能「报告用量事实」，价格与扣费由宿主表达式决定）
- 拿到上游凭证原值（`authHeader` 由宿主 `resolveAuth` 解析：`jsplugin/auth.go:22-45`；客户端凭证头被剥离：`adaptor.go:111`）

**这个安全姿态必须在泛化过程中保持，不能为了「更强的插件能力」而放开。** 详见 §7。

---

## 3. 设计原则：什么能插件化，什么不能

判定一个功能能否插件化，用这四条测试（**四条全过才能插件化**）：

1. **可失败而不损坏**：插件崩溃/超时/返回垃圾，宿主能降级到「无此插件」且**不丢钱、不越权、不丢数据**。
2. **可审计**：插件做的每件影响计费/权限/数据的事，宿主都能独立观测并记录。
3. **不持有不可逆状态**：插件不拥有唯一的真相来源（额度、订单、密钥、审计）。
4. **契约可版本化**：接口能向后兼容演进，旧插件不被破坏。

**反例（因此必须留核心）**：额度扣减——插件崩溃时钱已经动了，违反 (1)；上游密钥——插件持有即可外泄，违反 (2)；Casbin 判定——插件改判定即提权，违反 (3)。

---

## 4. T0 冻结核心（不可插件化）

以下模块**任何形式的插件化都必须拒绝**，包括「只读钩子」。每条给出理由与关键证据。

### 4.1 计费与额度（最硬的边界）

| 冻结项 | 位置 | 为什么 |
|---|---|---|
| 配额换算与饱和 | `common/quota_math.go`（`QuotaFromFloat` / `QuotaRound` / `QuotaFromDecimal` + `*Checked` / `*Strict`） | `.agents/rules/billing.md:14` 明令禁止裸转换；一个浮点错误就是资损 |
| 预扣/结算/退款会话 | `service/billing_session.go`（`BillingSession` 的 `Settle`/`Refund`/`Reserve` 幂等） | 幂等与串行化是资金正确性基础 |
| 钱包额度事务 | `service/quota.go`、`model.TryReserveTokenQuota` | 原子条件更新 + 行锁 |
| 计费表达式求值 | `pkg/billingexpr/` | 表达式是**管理员**授予的定价权威，不能让插件改写 |
| 结算编排 | `service/text_quota.go`、`service/tiered_settle.go`、`service/task_billing.go`、`service/image_billing.go` | 预扣→结算→退款全链不变量 |
| `PriceData` / `OtherRatios` 守卫 | `types/price_data.go` 的 `AddOtherRatio` | 拒绝非正/NaN/+Inf，规则明令不得削弱 |

**允许的插件面**：插件只能**上报用量事实**（`usageSchema` 声明的 `second`/`count`/`token`/`credit`），由宿主做边界钳制后代入表达式。这正是现有任务插件已在做的（`docs/plugin-api/v1.md:151`），**扩展时必须沿用，不得放宽为「插件返回 quota」**。

### 4.2 身份认证与凭证

| 冻结项 | 位置 | 为什么 |
|---|---|---|
| 密码哈希 | `common/account_password.go:44-63` | OWASP ASVS 强制项；`AGENTS.md:56-60` 明文要求 |
| TOTP / Passkey | `common/totp.go`、WebAuthn 路径 | 同上 |
| 会话与令牌签发 | `service/auth_session.go`、`service/auth_token.go` | 密钥派生、重放窗口是安全属性 |
| API Token 校验 | `middleware/auth.go` 的 `TokenAuth` | 鉴权入口 |
| 上游渠道密钥解密 | 渠道 key 解密路径 | 凭证只应在出站那一刻被解密 |
| 管理员密钥查看 | `POST /api/channel/:id/key` 的 `RootAuth` + `SecureVerificationRequired` | 已有二次验证 |

**允许的插件面**：OAuth/OIDC **provider 是身份来源，不是身份判定**——可以插件化「拿到用户标识」（现有 `oauth.LoadCustomProviders()` 已支持纯配置接入），但**颁发会话、校验会话、判定身份必须留核心**。

### 4.3 授权判定

| 冻结项 | 位置 | 为什么 |
|---|---|---|
| Casbin 模型与判定器 | `service/authz/enforcer.go:19-31` | 判定器即权限边界 |
| 策略持久化与多节点同步 | `service/authz/adapter.go:22-54,78-94` | 策略篡改 = 提权 |
| superuser / root 短路 | `service/authz/role.go:19-36,83-86` | root 全权规则不可被插件改写 |
| 角色基线与 seeding | `service/authz/seed.go:49-66` | 同上 |

**允许的插件面**：注册**资源定义**（现有 `RegisterResource` 已是 append 型注册表，`service/authz/registry.go:24`）——即「多了一种可授权的东西」，而不是「改判定逻辑」。

### 4.4 核心 relay 管道

| 冻结项 | 为什么 |
|---|---|
| `TokenAuth` → `Distribute` → 计费预扣 → **渠道选择** → 重试 → 结算 的骨架（`controller/relay.go`、`middleware/distributor.go`） | 这是「网关」本身；放开等于让插件接管网关 |
| 中间件链顺序（`router/main.go:15-40` 及各组 `Use(...)`） | `AccessTokenAudit → 限流 → UserAuth` 的顺序**本身就是安全属性**；开放动态插入点 = 开放绕过鉴权/限流 |
| 请求体校验与计费乘数上界（`relay/helper/valid_request.go` 的 `maxTokensLimit`、`dto.MaxImageN`、`relaycommon.MaxTaskDurationSeconds`） | 这些上界是计费安全第一道闸 |
| `RelayInfo` 的计费字段与方法（`relay/common/relay_info.go` 的 `PriceData`、`TieredBillingSnapshot`、`UpdateImageCount`） | `AGENTS.md:132` 明确列入计费门禁 |
| 出站请求的凭证注入、`ParamOverride`/`HeaderOverride` 的最终应用点（`relay/common/override.go`） | 是绕过 DTO 校验的乘数通道，必须由宿主统一钳制 |

**允许的插件面**：适配器（协议转换 + URL/头构造）、响应解析、用量事实提取。插件**不**决定选哪个渠道、**不**决定扣多少钱、**不**直接写日志库。

### 4.5 数据层与迁移

| 冻结项 | 位置 | 为什么 |
|---|---|---|
| Schema 与迁移 | `model/main.go` `migrateDB()` | 三库兼容 + 幂等；插件迁移会造成不可逆碎片 |
| 迁移只由 master 执行 | `model/main.go`（`if !common.IsMasterNode { return nil }`） | 从节点提前升 schema 会错乱 |
| 额度列 64 位校验 | `ensureUserQuotaColumns()` | 32 位钱包直接拒启 |
| 日志/审计写入 | `model/log.go:99-102`、`model/audit_log.go:74-137` | 计费凭证与审计的唯一事实来源 |
| `admin_info` 可见性裁剪 | `model/log.go:114-136` | 权限边界 |

**允许的插件面**：**只读的日志 sink**（把已落库的日志转发到外部系统）、**命名空间的插件私有存储**（插件自己的 KV，绝不共享宿主表）。宿主表一律不可写。

### 4.6 治理与品牌

`AGENTS.md:176-183` 保护 `new-api` / `QuantumNous` 的一切引用、品牌、module 路径、许可头。**插件系统不得提供任何改写这些信息的扩展点**（例如插件不能替换 `/api/status` 里的项目名）。

---

## 5. 插件化分区

按「收益 / 风险 / 可行性」排序，分三档。

### T1 — 优先插件化（基础设施已具备或接近具备）

| # | 领域 | 现状 | 要补什么 |
|---|---|---|---|
| 1 | **系统任务** | 已是接口 + 注册表 + 调度器 + DB 租约（`service/system_task.go:34-47,57-64,263-303`；`model/system_task.go:313-352`） | 几乎不用改结构，只需新增 job 类插件类型，把 `RegisterScheduledSystemTasks` 变成「内置 + 插件」双来源 |
| 2 | **OAuth / OIDC provider** | `oauth/registry.go:101-132` 已支持从 DB 全量装载 | 收敛 `controller/misc.go:50-63` 的内置字段为统一列表；给前端一个 provider 描述字段（图标/排序），消除 `oauth-providers.tsx:85-131` 的逐个硬编码 |
| 3 | **授权资源定义** | `service/authz/registry.go:24` append 型注册表，seeding 与 `/api/authz/catalog` 均由其推导 | 插件 meta 增加 `resources` 声明；判定器保持冻结 |
| 4 | **模型元数据来源** | `controller/model_sync.go:43-54` 已用 env 参数化 base URL | 提升为「来源注册表」（name → URL 模板 + 解析器），接入任意第三方模型库；**不动 `model/pricing.go` 的计算逻辑** |
| 5 | **任务渠道继续扩展** | 已有 11 个生产插件 | 把 `taskPluginKeys` 硬表（`relay/relay_adaptor.go:141-154`）降级为兼容映射，新的走 `channelTypes` 声明 |

### T2 — 中期插件化（需要新 ABI 能力）

| # | 领域 | 现状 | 要补什么 |
|---|---|---|---|
| 6 | **文本/多模态渠道适配器** | 协议表只有 3 个任务协议（`routing.go:89-106`） | 见 §6.4：新增 `openai_chat` / `anthropic_messages` / `gemini_generate` / `openai_embeddings` 宿主协议；**最大的一块工作** |
| 7 | **通知渠道（Webhook / Bark / Gotify / 邮件）** | 4 个 enum + `service/user_notify.go:84-121` 的 switch，case 内联 URL 拼接与 HTTP | 抽象 `Notifier` 接口；插件只提供「把消息发出去」，**接收方与模板留核心** |
| 8 | **前端控制台页面（受限）** | 无运行时动态路由；导航三处硬编码（`use-sidebar-data.ts:51-90`、`sidebar-view-registry.ts:31`） | 不做任意前端代码注入。改为**声明式 UI 描述**：插件声明「一个表单页 / 一个表格页」，由宿主用已有组件渲染。见 §6.5 |
| 9 | **日志/可观测 sink（只读）** | 写入是单函数 `model/log.go:99-102` 直连 DB，无 fan-out | 加 `afterLogPersisted` 事件钩子（**持久化之后**触发，插件失败不影响落库） |

### T3 — 完全插件化（需要进程外隔离）

| # | 领域 | 为什么要进程外 |
|---|---|---|
| 10 | **支付渠道（Stripe/Creem/Waffo/易支付之外的）** | 涉及资金核销事务，插件必须**不能**触碰额度；只能提供「创建支付会话 + 验证回调签名」，核销仍由宿主 `model/topup.go` 的原子事务完成。且第三方支付代码风险高，需要崩溃隔离与审计 |
| 11 | **审计导出 / 合规对接** | 会读取全量日志与用户数据，必须显式授权 + 可审计 |

> **不建议插件化**：中间件链、计费、鉴权、密码学、迁移、渠道选择算法、限流算法。

---

## 6. 插件宿主 ABI v2

### 6.1 总体形态

保留 v1 的全部安全姿态，把「任务插件」升格为「插件」的**一种类型**：

```
plugin/
├── manifest.js      // meta + 类型声明（v2 新增 kind）
├── entries/
│   ├── provider.js  // kind: "provider"   渠道适配器
│   ├── task.js      // kind: "task"       现有任务驱动（v1 兼容）
│   ├── job.js       // kind: "job"        系统任务
│   ├── identity.js  // kind: "identity"   OAuth provider
│   ├── notifier.js  // kind: "notifier"   通知渠道
│   └── console.js   // kind: "console"    声明式 UI 描述
└── icon.svg
```

**兼容策略**：`apiVersion: 1` 的现有插件**原样加载**，等价于 `kind: "task"`。新插件用 `apiVersion: 2`。宿主同时支持两代。这与现有 `docs/plugin-api/v1.md` 反复强调的「additive extension」哲学一致（例如 `unitLabel`、`usageProfiles` 都是这样加的）。

### 6.2 能力清单与权限

插件在 meta 中**声明**需要的能力，宿主在**安装时**向管理员展示并授权。声明之外的能力，调用即抛错。

```js
export const meta = {
  apiVersion: 2,
  key: "acme",
  kinds: ["provider"],
  capabilities: [
    "http:outbound",        // 出网（仍受 allowedHosts 白名单限制）
    "storage:plugin-kv",    // 插件私有 KV（命名空间隔离，非宿主表）
    "events:after-log",     // 只读订阅已落库日志
    "ui:console-page",      // 声明式控制台页面
  ],
};
```

**能力的默认值全部为「不授予」**。现有 v1 插件的隐式能力集保持不变（等于 `http:outbound` 受限版），不因升级而获得新权限。

### 6.3 宿主原语扩展（按 kind）

| kind | 宿主必须提供的原语 | 复用现有件 |
|---|---|---|
| `provider` | 协议转换入口、URL/头构造、SSE 逐帧回调、usage 提取、错误归一 | `service.ConvertRequest`、`channel.DoApiRequest`、`helper.StreamScannerHandler`、`service.RelayErrorHandler` |
| `task` | 现有全部 | 现有全部 |
| `job` | 调度注册、租约、运行历史 | `service/system_task.go`、`model/system_task.go` |
| `identity` | 授权码交换、用户标识解析、账号绑定 | `oauth/provider.go` |
| `notifier` | 消息投递 | 需新建 `Notifier` 接口 |
| `console` | 表单/表格 schema、数据源绑定 | 复用 `web/src/components/` 与 `data-table/` |

### 6.4 关键设计：渠道身份去硬编码

这是「全面插件化」最大的技术账。当前：

- 渠道类型是 iota 常量（`constant/api_type.go:3-43`、`constant/channel.go:3-64`）
- 名字表与 BaseURL 表**下标即渠道类型**（`constant/channel.go:68-205`）
- 前端渠道表是**前端硬编码**（`web/src/features/channels/constants.ts:41-102`）
- `GetAdaptor` 是约 35 分支 switch（`relay/relay_adaptor.go:50-128`）
- `ChannelType2APIType` 是平行 switch，未知类型**静默回退 OpenAI**（`common/api_type.go:85-92`）
- `streamSupportedChannels` 硬编码 24 项（`relay/common/relay_info.go:437-462`）

**改造方案（分三步，每步可独立发布、可回滚）**：

**Step 1 — 注册表化，保留常量。** 引入 `relay/channel/registry.go`：

```go
type Registration struct {
    ChannelType      int      // 保留现有 int 值，不破坏存量数据
    APIType          int
    Name             string
    DefaultBaseURL   string
    SupportsStream   bool
    NewAdaptor       func() channel.Adaptor
    PluginKey        string   // 空 = 内置
}
```

内置适配器在各自包的 `init()` 里 `Register(...)`（**现成的先例**：`oauth/*/init()` 注册 provider、`service/authz/resources_*.go` 的 `init()` 注册资源——这个模式在本仓库已被验证两次）。`GetAdaptor` 改为查表；`ChannelType2APIType`、`ChannelBaseURLs`、`ChannelTypeNames` 全部改为从注册表派生。**此步不改变任何外部行为**，前端仍是硬编码表。

**Step 2 — 后端下发渠道目录。** 新增 `GET /api/channel/registry`（管理员可见），返回所有已注册渠道（内置 + 插件）。前端 `CHANNEL_TYPES` 改为运行时拉取 + 内置兜底。**此步让插件渠道出现在下拉里，无需重建前端。**

**Step 3 — 开放插件渠道。** 插件可声明 `channelTypes` 并携带完整适配器实现（JS，走 §6.3 的 `provider` 原语）。分配渠道类型值的策略：给插件保留一个数值区间（例如 1000+），**绝不与内置常量冲突**，且不参与 iota。

> ⚠️ **不要重排 `iota`**：`constant/api_type.go:3-43` 的值可能已被持久化（渠道的 `api_type`、任务 `platform`）。只能追加或改为显式赋值。

**工作量参考**（基于本仓库既有密度推算）：

| 阶段 | 人日 | 推算依据 |
|---|---|---|
| Step 1 注册表化 | 6–10 | 40 个渠道目录加 `init()` + 4 张表派生 + 全量回归（35 case 的 switch 是全量回归面）；测试:实现 ≈ 0.57（`relay/channel` 11377/19890） |
| Step 2 后端下发 + 前端对接 | 4–6 | 1 个新 API + 前端改为异步拉取 + 7 语言文案 |
| Step 3 插件渠道（含 `openai_chat` 协议 + JS 桥） | 15–22 | 对照 `relay/channel/task/jsplugin/adaptor.go` 的 2184 行实现 + 2805 行测试；文本流式桥约为其 0.6–0.8 倍（少轮询/产物，多 SSE 逐帧） |
| 合计 | **25–38 人日** | 中位数约 30 |

**若只做「声明式透传渠道」**（URL 模板 + 固定头 + body 直通，零 JS）：可压缩到 **4–7 人日**，但只能覆盖 `siliconflow` / `openrouter` / `xinference` 这一类「body 完全透传」的渠道（约 4–5 个现存渠道可被这样表达）。

### 6.5 关键设计：前端扩展点（声明式，不是代码注入）

**明确拒绝**「插件注入任意前端 JS/React」。理由：一个能执行前端代码的插件可以窃取管理员的会话令牌——这违反 §3 测试 (2)(3)。

**改为声明式 UI 描述**，由宿主用**已有组件**渲染：

```js
export const console = {
  pages: [{
    id: "acme-settings",
    path: "/console/plugins/acme",
    title: {en: "Acme Provider", zh: "Acme 渠道"},
    permission: {resource: "channel", action: "read"},   // 复用现有 authz
    layout: [
      {component: "form", fields: [
        {name: "apiKey", type: "password", label: {en: "API key", zh: "API 密钥"}},
      ]},
      {component: "table", dataSource: "rpc:listJobs", columns: [...]},
    ],
  }],
};
```

宿主侧：`rpc:*` 数据源绑定到插件自己的原生路由（复用 `router/plugin-router.go` 的分发）。**组件白名单由宿主固定**，插件不能引入新组件类型。

---

## 7. 信任模型与权限分级

**核心原则：插件是「不可信扩展」，不是「核心的一部分」。**

| 级别 | 谁能安装 | 能获得的能力 | 隔离 |
|---|---|---|---|
| **L1 内置** | 随二进制发布 | 全部 | 进程内 |
| **L2 管理员上传** | `RootAuth`（现有 `router/api-router.go:263`） | 按声明授予；`http:outbound` 默认开、`storage:plugin-kv` 需确认 | 进程内 + 5s/8 并发 + 看门狗 |
| **L3 第三方高风险**（支付、审计导出） | `RootAuth` + 二次验证 | 显式逐项授权 | **进程外**（go-plugin/gRPC 或 WASM） |

**必须保持的既有约束（不得放松）**：

- 出网 host 白名单：`meta.allowedHosts` + 频道 baseURL（`pkg/jsplugin/request.go:14-36`）
- 出网方法限制：`utils.fetch` 仅 GET/HEAD（`fetch.go:252`）——**上游转发必须由宿主做**，这符合网关设计
- 请求/响应尺寸、并发、超时：`engine.go:25-26`、`fetch.go:36-42`、`middleware/task_plugin.go:52-58`
- 未知 meta 字段即拒绝（`registry.go:1016-1034`）
- 语法门禁：禁 `async`/`await`/`import`（`engine.go:92,148-161`）——**v2 如需放开必须配套重新设计调度**，不要顺手放开

**必须新增的**：

1. **能力声明 + 安装时授权**（§6.2），默认拒绝。
2. **插件私有 KV 的命名空间硬隔离**，且**绝不**暴露宿主表名。
3. **L3 的跨进程隔离与资源配额**（CPU/内存/文件描述符），避免单进程内嵌 JS 的「无法硬隔离」问题。
4. **审计**：插件的安装、启用、能力授予、每次出网目标，都写审计日志（复用 `model/audit_log.go`）。

---

## 8. 实施里程碑

| 里程碑 | 内容 | 产出 | 风险 | 状态 |
|---|---|---|---|---|
| **M0 决策** | 确认本文档的 T0 边界与剔除清单 | 决策记录 | — | 已确认（Tier 1 + Tier 2 无硬阻塞项） |
| **M1 泛化宿主** | 插件 `kind` 字段 + `apiVersion: 2`；v1 插件兼容；能力声明与授权 UI | 不改变任何现有行为，只是打开结构 | 低 | **已完成（宿主侧）** |
| **M2 低垂果实** | 系统任务插件化、OAuth 收敛、authz 资源声明、模型来源注册表 | 4 个领域零结构改动落地 | 低 | **M2a、M2c 已完成；M2b、M2d 暂缓（见下）** |
| **M3 渠道注册表** | §6.4 Step 1 + Step 2 | 渠道不再需要编译期枚举；前端下拉由后端驱动 | 中（全量回归面） | **Step 1+2 已完成**（后端目录 + 插件区间；前端驱动未接） |
| **M4 插件渠道** | §6.4 Step 3：`openai_chat` 协议 + provider 桥 + SSE 流式 | 第三方可新增 OpenAI 兼容渠道 | **高**（流式是唯一硬骨头） | 未开始 |
| **M5 声明式控制台** | §6.5 UI 描述 + 组件白名单渲染 | 插件自带管理页 | 中（需严守「不注入代码」） | 未开始 |
| **M6 高风险插件** | L3 进程外隔离；支付渠道、审计导出 | 完整插件生态 | 高（需要新的 IPC 层） | 未开始 |

### 8.3 本轮交付明细

| 交付物 | 位置 | 说明 |
|---|---|---|
| 插件契约查询 API | `GET /api/plugin`、`/api/plugin/:key`、`/api/plugin/kinds`、`/api/plugin/protocols` | 全部只读、`RootAuth`，摘要投影显式剔除密钥/上游地址/鉴权字段；个人访问令牌新增 `plugin:read` 作用域 |
| kind/hook 契约**单一来源** | `pkg/jsplugin/registry.go` 的 `TaskDriverHookRequirements` | `CompilePlugin` 与 `/api/plugin/kinds` 调用同一函数，不存在第二份手写 hook 清单可漂移 |
| 可加载插件示例 ×6 | `examples/plugins/` | minimal / batch / SSE / artifact / job / v2-task 各一，全部经真实宿主编译器校验（`pkg/jsplugin/examples_test.go`） |
| 渠道目录注册表 | `constant/channel_registry.go` | `ChannelCatalog()` 按数值升序、返回副本；插件保留区间 `ChannelPluginTypeStart = 1000`；`init()` 守卫防止目录与旧名称表漂移 |
| 文档对齐 | `docs/plugin-api/{v1.md,v1.schema.json,v1.d.ts,README.md}` | 补 `kind`/`apiVersion 2` 全文；修复 schema 中 `allOf` 重复键导致 kind 规则**未生效**的缺陷 |

**job 桥接已接线**：`service/system_task_plugin.go` 的 `RegisterPluginJobHandler` 把 `kind: "job"` 插件绑定到调度器，任务类型固定为 `plugin:<key>`。约定导出：`run`（必需）、`enabled`、`intervalSeconds`、`newPayload`、`taskType`（若声明必须等于自身命名空间）。安全取向是**失败即关闭**：`enabled` 抛错时按未启用处理，`run` panic 只让该任务失败而不拖垮 runner，任务参数投影不含 lease/`locked_by`。`nightly-job` 示例即为可运行作业。

**建议：M1–M2 可以先做，收益确定、风险低。M3–M4 是真正的「全面插件化」，需要一次专门的架构评审。**

### 8.1 M2 各领域落地情况

| 子项 | 结论 | 证据 |
|---|---|---|
| **M2a 系统任务** | 已完成。任务类型从编译期闭集改为开放集合：`model.IsValidSystemTaskType` 接受内置类型或 `plugin:<key>`；`PluginSystemTaskType` 生成插件命名空间；`service.RegisterSystemTaskHandler` 从无返回值改为返回 `error`，校验类型合法性、拒绝重复抢占插件类型，内置类型仍可覆盖。插件无法冒用内置类型（类型即锁与 `active_key` 去重边界）。 | `model/system_task.go`、`service/system_task.go`、`model/system_task_test.go`、`service/system_task_test.go` |
| **M2b OAuth 收敛** | **暂缓**。`/api/status` 的响应字段被前端逐字段消费，收敛需要同批改动前端；且此项属认证流程，按 `AGENTS.md` 必须走 OWASP ASVS 完整校验（枚举防护、会话轮换、state/replay 等），不能作为顺带重构。 | `controller/misc.go:50-73`、`oauth/registry.go:101-132` |
| **M2c authz 资源** | 已完成。`service/authz/registry.go` 的注册表加读写锁，`RegisterResource` 去重并忽略空资源名；重复声明不再让权限清单和角色默认授权翻倍。 | `service/authz/registry.go`、`service/authz/authz_test.go` |
| **M2d 模型来源** | **暂缓**。上游元数据地址已由 `SYNC_UPSTREAM_BASE` 参数化，且只服务上游模型元数据同步。把它做成插件来源会扩大网络出口面（当前插件不允许任意 HTTP），收益低于风险。 | `controller/model_sync.go:43-54` |

### 8.2 开发效率：前端测试耗时

前端是本次改动的最大效率瓶颈：203 个测试文件、约 5.5 万行，基线全量约 **3007 秒**（约 50 分钟），并偶发 jsdom worker `exit 134`（OOM）。

| 尝试 | 做法 | 实测 | 结论 |
|---|---|---|---|
| 限制并发 | `maxWorkers` 限为核数一半 | 20 分钟仍未跑完 | **更慢**。jsdom 用例 CPU 占主导，减 worker 直接降吞吐，已回退 |
| 关闭隔离 | `isolate: false` + 图标缓存按 `globalThis` 分片 | 3007s → 691s（−77%） | 速度达标，但**不可接受**，见下 |

**为什么最终没有采用 `isolate: false`**：共享模块图不仅共享生产代码，也共享**测试文件的模块级 mock 状态**。用 `vi.mocked(...).mockResolvedValue` 在模块作用域设桩的用例，会读到上一个文件留下的桩（`channel-type-badge` 单独跑必过、跟在其他文件后跑必错）。这类失败与执行顺序相关、只在全量时复现，定位成本远高于它省下的时间，因此回退到 `isolate: true`。

**保留的改动**：`src/lib/lobe-icon.tsx` 的图标 memo 从模块级 `const` 改为挂在 `globalThis` 上。这不改变任何可观察行为（rerender 仍复用同一 lazy 实例），但让缓存不再跨越环境边界——这是将来若要安全启用 `isolate: false` 的前置条件。

**后续可选优化**：想要那 −77% 又不动语义，应走「分片 + 并行」而不是共享模块图：用 `vitest --shard` 把 203 个文件切成若干片并发跑，每片仍保持一文件一模块图。本轮未实施。


---

## 9. 可剔除清单（决策表）

> 每条均经实际搜索计数。**风险等级**：低 = 纯死代码；中 = 有外部契约或存量数据；高 = 不确定。
> 引用点数量来自 `Select-String` 实际计数。

### 9.1 Tier 1 — 可立即删除（纯死代码，无外部契约）

| # | 候选 | 证据 | 引用点 | 理由 | 风险 | 工作量 |
|---|---|---|---|---|---|---|
| 1 | 11 条 `RelayNotImplemented` 路由 | `router/relay-router.go:167-179`；实现在 `controller/relay.go:386` | `RelayNotImplemented` 全仓 12（1 定义 + 11 路由） | 全部固定返回 501，无 relay 逻辑；前端 0 调用 | 低 | 0.5 人日 |
| 2 | `bin/migration_v0.2-v0.3.sql`、`migration_v0.3-v0.4.sql` | `bin/`（124B / 481B） | 全仓 0 | one-api 时代 schema（`remain_quota`、`abilities` 无 `priority`），在当前库执行必然报错 | 低 | 0.2 人日 |
| 3 | `bin/time_test.sh` | `bin/time_test.sh`（1248B） | 0 | 依赖外部 curl+bc 的手工脚本，与构建/测试链无关 | 低 | 0.1 人日 |
| 4 | 重复的 `paymentReturnPath` / `PaymentReturnURL` | `controller/return_path.go:9-12` 与 `service/return_path.go:9-12` **函数体逐字相同** | 各 1 定义 | 同 4 行函数复制两份；合并到 `service` 保留薄包装 | 低 | 0.3 人日 |
| 5 | `controller.SearchAllLogs` / `SearchUserLogs` | `controller/log.go:172-178`（函数体只有 `ApiErrorT("This API is deprecated")`）；路由 `api-router.go:327,332` | 2 定义 + 2 路由 | 空实现端点；`/self/search` 还白占一个 `SearchRateLimit()` | 低 | 0.5 人日 |
| 6 | 零调用兼容垫片 3 个 | `relaykit/types/request_meta.go:67-73`（`GetRawData`，标注 Deprecated）、`service/http_client.go:474-478`（`NewProxyHttpClient`）、`service/file_decoder.go:136-150`（`GetFileBase64FromUrl`） | 各 1 定义，外部调用 0 | 明确废弃且无人调用；`relaykit` 是独立 module，属对外 API，需先确认无外部消费方 | 低 | 0.4 人日 |
| 7 | 前端孤立资源 | `web/src/assets/clerk-logo.tsx`、`clerk-full-logo.tsx`；`brand-icons/` 中 12 个未使用图标（docker/facebook/figma/gitlab/gmail/medium/notion/skype/slack/trello/whatsapp/zoom） | 导出各 1，引用 0 | 项目未接入 Clerk；未引用图标仍进构建 | 低 | 0.4 人日 |
| 8 | `makefile` 中 0 引用的 target | `makefile:13-54` | `test`（CI 6 处）、`dev`（2 处）、`dev-web`（1 处）在用；`all`/`build-all-web`/`start-api`/`dev-api-rebuild`/`reset-setup` 0 引用 | 保留 `test`/`dev`/`dev-web` | 低 | 0.3 人日 |
| 9 | `.env.example` 的 `GEMINI_VISION_MAX_IMAGE_NUM` | `.env.example` | Go/TS 全仓 0 处 `GEMINI_VISION` | 唯一「文档有、代码完全不读」的环境变量，误导运维 | 低 | 0.1 人日 |
| 10 | `e2e/` 目录命名 | `e2e/doc_parse_test.go`（`package e2e`，1 个 Test，本地 httptest + 内存 SQLite） | 该包 1 文件 | 名为 e2e 实为单测；建议并入 `controller/` 或 `plugins/` 测试 | 低 | 0.3 人日 |

**Tier 1 合计：约 3.1 人日，收益是约 20 个文件的净减少 + 一处重复实现消除 + 配置文档准确。**

### 9.2 Tier 2 — 需先弃用公告

| # | 候选 | 证据 | 为什么不能直接删 | 风险 |
|---|---|---|---|---|
| 11 | `users.access_token` 全套遗留 | `model/user.go:95,129,136,142,1228-1244`；`middleware/auth.go:254-263`；`model/user_access_token.go:302-329`；`controller/access_token.go:73-91`；前端 `access-tokens-card.tsx:120-161` | **仍在服役**。退役期 `LegacyAccessTokenRetireAt` 是首启 now+30 天的**滚动值**，永远是「未来某点」，需等真实到期且老 token 清零 | 高 |
| 12 | `constant.APITypeAIProxyLibrary` | `constant/api_type.go:11`；`common/api_type.go:22-23`；`controller/model.go:38,100` 两处**专门跳过它** | `GetAdaptor` 无此 case，返回 nil，靠 `model.go` 的 `continue` 绕开。删除需同步 3 处 + 确认 `iota` 值不被持久化 | 中 |
| 13 | 4 个死渠道类型 `ChannelTypeOpenAIMax` / `OhMyGPT` / `AILS` / `FastGPT` | `constant/channel.go:10,11,13,26` + `:142-167`；前端 `constants.ts:48,51,53` 已整行注释 | 前端下拉仍暴露 7/22；存储层 `channels.type` 可能残留旧值 | 中 |
| 14 | `AIProxy` / `API2GPT` / `AIGC2D` 余额查询分支 | `controller/channel-billing.go:648-653 → 221-255,366+` | 无 adaptor 无法新建，但库里若有历史行，删后「更新余额」落到 default 报错 | 中 |
| 15 | `SMTP_*` 兼容别名 | `common/init.go:104-105`（`SMTP_STARTTLS_ENABLE`、`SMTP_INSECURE_SKIP_VERIFY`） | 用旧拼写的部署升级后静默回落默认值，需公告 | 中 |
| 16 | `retiredThemeOptionKey = "theme.frontend"` 归一化 | `model/frontend_option_migration.go:13,26-27,50-65`；`model/option.go:374` | 前端主题已重构；删除后老库残留行仍在但无人读 | 中 |
| 17 | `lib/legacy-route.ts` 的 12 条 `/console/setting?tab=` 映射 | `web/src/lib/legacy-route.ts:28-104`；消费者 `routes/__root.tsx:44` | 纯迁移期重定向，功能正确且在跑；老书签会失效 | 中 |
| 18 | `router/dashboard.go` 的 4 条老计费路由 | `router/dashboard.go:16-21`（`/dashboard/billing/*`、`/v1/dashboard/billing/*`） | 前端 0 调用，但 OpenAI 部分客户端/第三方脚本可能硬编码。**建议先加访问日志观察一个周期** | 中 |

### 9.3 Tier 3 — 不建议删（附理由）

| 候选 | 为什么保留 |
|---|---|
| `pkg/jsplugin`（75 处引用）、`pkg/billingexpr`（35）、`pkg/wsmanager`（4）、`pkg/perf_metrics`（5） | 全部活跃，是本次方案的基础设施 |
| `pkg/ionet` + `controller/deployment.go` + `/api/deployments/*` | 被 **1 个文件**引用（`controller/deployment.go:12`），25 个 handler 挂 `api-router.go:416-439`，前端 8 个文件。**功能完整、非死代码**；是「低使用率特性」，属产品取舍而非代码卫生。**这是最值得你单独决策的一项**（原附带的抓包残留文档已清理） |
| 两套限流 `middleware/rate-limit.go` vs `model-rate-limit.go` | 职责确实不同：前者 IP 级基础设施限流（Redis 固定窗口 Lua），后者用户/分组级模型配额（List 滑窗 + 成功/总数双计数）。**不能当重复实现删**，只能抽公共 store |
| `tokenkit` 的 `Count` vs `Estimate` | 设计意图明确：`Count` 是「精确或降级」统一入口，`Estimate` 是降级算法本体（`tokenkit/tokenizer.go:27`、`tokenkit.go:140`）。不是重复实现 |
| `middleware/header_nav.go:17-37` 的 fallback 分支 | 删掉等于把 pricing/rankings 永久公开 |
| `service/log_info_generate.go` 的 5 个 `Generate*OtherInfo` | Wss/Audio/Claude 三个都只是「调 Text 再补 2 个字段」，可收敛为 options 参数（约 3 人日），但**实际在用，非死代码** |
| `pkg/cachex`（2 个消费者）vs `common/redis.go` vs `model/*_cache.go` | 三层重叠是「收敛目标」不是「删除目标」；统一缓存层是重构工程（5+ 人日） |
| `.github/workflows/` 6 个 | `docker-image-branch.yml`（手动分支）与 `docker-build.yml`（tag）触发条件不同，分属不同发布链路 |
| `go.mod` 抽样 5 个依赖 | `abema/go-mp4`、`go-audio/wav`、`coreos/go-oidc`、`anknown/ahocorasick`、`jinzhu/copier` 各 1–2 处真实 import，**没有可安全移除的直接依赖** |

### 9.4 受治理保护 — 只标注，不可删

按 `AGENTS.md:176-183`，以下**任何情况下都不得修改、删除、替换或移除**：

1. **品牌与归属**：所有文件头的 `Copyright (C) 2023-2026 QuantumNous` + AGPL-3.0 许可块（前端源码几乎全部）
2. **Module 路径**：`module github.com/QuantumNous/new-api`（`go.mod:1`）及 `relaykit` / `tokenkit` 子模块路径
3. **品牌资产**：`web/src/features/about`、`web/src/assets/logo*`、`components/layout/components/{logo,header-logo,system-brand}.tsx`、`LICENSE`、`NOTICE`、`THIRD-PARTY-LICENSES.md`、6 语言 `README*`
4. **品牌文案**：`CHANNEL_PROVIDER_PRESENTATION` 的 provider 描述、`i18n/keys.go` 的 `new_api_error`、`error.type = "new_api_error"`（`controller/relay.go:389`）
5. **构建标识**：`VERSION`、`electron/package.json` 的 `productName`、`docker-compose.yml` 的镜像名
6. **`electron/` 整目录**：桌面分发渠道
7. **`.agents/rules/billing.md`、`.agents/github/ISSUE.md`、`.agents/github/PR.md`**：被 `AGENTS.md:130,185,191` 作为强制门禁引用

### 9.5 未能确认的项（需要你或人工判断）

1. **未运行 `knip` / `bun check` / `go vet` / `go test`**——本环境未保证依赖就绪。前端「未使用文件/导出」全部来自静态扫描。特别地，`knip.config.ts:4` 整目录忽略了 `web/src/components/ui/**`，**无法确认这些 shadcn 组件里有多少真正未被引用**。建议在可跑 bun 的环境执行 `bunx knip --include files,exports` 再定稿前端部分。
2. **`channels.type` 存量数据**——删除死渠道类型前，需在生产库执行 `SELECT type, COUNT(*) FROM channels GROUP BY type`，确认 §9.2 #13/#14 涉及的 7 个 type 值是否还有真实行。
3. **前端 `features/` 的域级死文件**——61 个 `index.ts*` barrel 使静态扫描失效，路由由 `routeTree.gen.ts` 生成（构建产物）。
4. **`_extras` / `_reports`**——全仓未找到该目录；`i18n:sync` 会生成它们，当前工作区不存在（未跑过同步）。
5. **`pkg/ionet` 是否整体下线**——产品决策，见 §9.3。
6. **`/dashboard/billing/*` 是否还有外部消费者**——需生产访问日志。
7. **三层缓存是否 key 空间冲突**——只能确认消费者数量，无法确认语义冲突。

---

## 10. 需要你决策的问题

请逐项给结论，我会据此执行。

| # | 问题 | 选项 |
|---|---|---|
| A | **T0 冻结边界是否认可**？特别是「计费/鉴权/凭证/迁移/中间件链不插件化」 | 认可 / 调整某几条 |
| B | **剔除范围**：Tier 1（10 项，约 3 人日）是否现在执行 | 全部 / 挑选 / 暂缓 |
| C | **Tier 2 各项**（8 项）哪些进入弃用流程 | 逐项勾选 |
| D | **`pkg/ionet` + deployments 全家桶**是否下线（约 3 人日 + 需迁移存量配置） | 保留 / 下线 / 先观察 |
| E | **前端扩展点**是否接受「声明式 UI，拒绝代码注入」 | 接受 / 需要讨论能力更强的方案 |
| F | **M3/M4（渠道注册表 + 插件渠道）**是否立项——这是「全面插件化」的主体，约 25–38 人日 | 立项 / 先做 M1–M2 / 暂缓 |
| G | **L3 进程外插件**（支付、审计导出）是否在路线图内 | 是 / 否 |
| H | 是否需要我把 M1（插件 `kind` + 能力声明 + v1 兼容）写成**可执行的详细技术设计**再动手 | 是 / 否 |

---

## 附：关键证据索引

| 论断 | 证据 |
|---|---|
| 现有协议表只有 3 项 | `pkg/jsplugin/routing.go:89-106` |
| 插件不能发任意 HTTP | `pkg/jsplugin/request.go:14-36`；`relay/channel/task/jsplugin/adaptor.go:827,870` |
| 插件不能读写 DB/env/fs | 无对应 host function，全局仅注入 `utils` 与 `console`（`pkg/jsplugin/utils.go:37-120`） |
| 插件不拿上游凭证 | `pkg/jsplugin/auth.go:22-45`；`adaptor.go:111` |
| 渠道类型是编译期 iota | `constant/api_type.go:3-43`；`constant/channel.go:3-64` |
| 前端渠道表硬编码 | `web/src/features/channels/constants.ts:41-102` |
| 适配器分派是 35 分支 switch | `relay/relay_adaptor.go:50-128` |
| 未知渠道类型静默回退 OpenAI | `common/api_type.go:85-92` |
| 流式能力是硬编码表 | `relay/common/relay_info.go:437-462`（24 项） |
| 11 条未实现路由 | `router/relay-router.go:167-179`；`controller/relay.go:386` |
| 系统任务是唯一「零结构改动」的扩展领域 | `service/system_task.go:34-47,57-64`；`model/system_task.go:313-352` |
| init() 注册模式已被验证两次 | `oauth/github.go:19-20`；`service/authz/resources_channel.go:21` |
