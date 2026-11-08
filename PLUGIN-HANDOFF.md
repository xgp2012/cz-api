# 插件化交接单（下一位 AI 从这里开始）

> 生成时间：2026-11-08。基线 commit：`c32c33784`（main）。
> 本文档**自包含**：读它就能接手，不必先读 `PLUGINIZATION.zh_CN.md`（那份是长篇方案，本文是执行清单）。
> 文中每条「已验证」都附带实测命令与结论；标「未验证」的不要当成事实。

---

## 0. 一页结论

**插件化没有完成。** 完成的是「泛化宿主 + 系统任务/job/identity 三种 kind + 渠道目录（只读投影）」；真正让第三方**不动 Go 代码就能新增渠道**的 M4 一步没开始。

| 里程碑 | 状态 | 一句话 |
|---|---|---|
| M0 决策 | ✅ | T0 冻结边界已定（见 §4） |
| M1 泛化宿主 | ✅ 宿主侧 | `kind` + `apiVersion:2`，v1 插件原样兼容 |
| M2a 系统任务 | ✅ | 任务类型从闭集变 `plugin:<key>` 开放集 |
| M2b OAuth 收敛 | ⏸ 暂缓 | 需同批改前端 + 走 OWASP ASVS |
| M2c authz 资源 | ✅ | 注册表加锁去重 |
| M2d 模型来源 | ⏸ 暂缓 | 会扩大插件网络出口面 |
| M3 Step 1 注册表化 | ❌ **没做** | `relay/channel/registry.go` 不存在 |
| M3 Step 2 后端下发目录 | ❌ **没做** | 无 `/api/channel/registry` 路由，无调用方 |
| M3 Step 3 前端驱动 | ❌ 没做 | 前端仍读硬编码 `constants.ts` |
| **M4 插件渠道** | ❌ **没开始** | 无 `openai_chat` 协议、无 provider 桥、无 SSE 逐帧 |
| M5 声明式控制台 | ❌ 没开始 | |
| M6 L3 进程外隔离 | ❌ 没开始 | |

---

## 1. 三个仍然成立的硬编码（任务的真正目标）

这三项是「全面插件化」的判定标准，现在一条没变。**改完这三项才算完成。**

| # | 硬编码 | 位置（实测行号） | 后果 |
|---|---|---|---|
| 1 | 宿主协议表只有 3 项 | `pkg/jsplugin/routing.go:89-110`：`openai_responses` / `openai_video` / `openai_image` | JS 插件**不能**声明 chat / embedding / rerank 渠道 |
| 2 | 分派是平行 switch 链，无注册点 | `relay/relay_adaptor.go:50` `func GetAdaptor(apiType int)`，共 **37 个 `case`**，226 行 | 新增渠道必须改 Go 并重新发版 |
| 3 | 未知类型静默回退 OpenAI | `common/api_type.go`（102 行，`ChannelType2APIType`） | 隐性错误源 |

辅助证据（同样未变）：

- `constant/channel.go`：`ChannelBaseURLs`（:67 起）、`ChannelTypeNames`（:141 起）都是硬编码表，下标即渠道类型。
- `relay/common/relay_info.go` 的 `streamSupportedChannels` 硬编码约 24 项 —— **决定末帧是否带 usage，影响结算精度**，改渠道时必须同步。
- 前端 `web/src/features/channels/constants.ts` 仍硬编码渠道下拉（41-102 行区域）。

---

## 2. 已完成部分的**真实**边界（重要纠偏）

### 2.1 `constant/channel_registry.go` 是「孤儿」

`PLUGINIZATION.zh_CN.md` §8 写「M3 Step 1+2 已完成」，**与代码不符**。实测：

```
Test-Path relay\channel\registry.go        -> False
Select-String router/*.go -Pattern 'channel/registry'  -> 无结果
```

`ChannelCatalog()` / `GetChannelMetadata()` / `IsPluginChannelType()` 在**生产代码里 0 个调用方**，只有自身 `init()` 守卫和 `channel_test.go` 调用。它的实际作用是：

- 提供一份**只读元数据投影**（`Type/Name/BaseURL/RequiresBaseURL/Plugin/Deprecated`），按数值升序返回副本；
- 定义插件保留区间 `ChannelPluginTypeStart = 1000`；
- `init()` 里校验目录与旧名字表不漂移（这是一个有价值的守护测试）。

**它没有替换任何东西**：`GetAdaptor` 的 switch、`ChannelBaseURLs`、`ChannelTypeNames`、`ChannelType2APIType` 都还是原来的写法。

> 接手时请以此为准，不要相信「渠道注册表已完成」的表述。

### 2.2 三种 kind 的现实状态

| kind | 常量 | 是否真的被分派 | 证据 |
|---|---|---|---|
| `task` | `PluginKindTask` | ✅ 完整链路（11 个生产插件） | `relay/channel/task/jsplugin/adaptor.go`（2108 行） |
| `job` | `PluginKindJob` | ✅ 已接线 | `service/system_task_plugin.go`（`RegisterPluginJobHandler`） |
| `identity` | `PluginKindIdentity` | ⚠️ **只在契约层存在** | 生产代码除 `registry.go` 常量与校验外**无任何消费者**；仅 `plugin_contract_test.go` 引用 |

`identity` 目前是「能通过编译与契约校验，但没有东西会去用它」。要做 OAuth 插件必须先补宿主侧的消费点（`oauth/registry.go`），而这一块属认证流程，按 `AGENTS.md` 必须走 OWASP ASVS。

### 2.3 插件契约查询 API（已可用）

`router/api-router.go:266-272`，全部 `RootAuth`、只读：

```
GET /api/plugin            -> controller.ListPlugins
GET /api/plugin/kinds      -> controller.GetPluginKinds
GET /api/plugin/protocols  -> controller.GetPluginProtocols
GET /api/plugin/:key       -> controller.GetPlugin
```

kind/hook 契约的单一来源是 `pkg/jsplugin/registry.go` 的 `TaskDriverHookRequirements`（:617）——`CompilePlugin` 与 `/api/plugin/kinds` 调同一函数，**不要再写第二份 hook 清单**。

### 2.4 示例插件（6 个，可直接抄）

`examples/plugins/`：`echo-minimal`、`stream-sse`、`fanout-batch`、`ordered-json-task`、`artifact-content`、`nightly-job`。
全部由 `pkg/jsplugin/examples_test.go` 用**真实宿主编译器**校验，改契约时这个测试会先红。

---

## 3. 下一个任务：M4（建议按此顺序）

### Step A — 后端下发渠道目录（最小、最低风险，先做这个）

目标：让前端下拉能显示「插件渠道」，且不改前端构建。

1. 新增 `GET /api/channel/registry`（`AdminAuth`，只读），返回 `constant.ChannelCatalog()` + 已注册插件渠道。
2. 前端 `web/src/features/channels/constants.ts` 改为 **运行时拉取 + 静态兜底**（拉取失败回退现有硬编码，保证离线可用）。
3. 补 7 语言 locale（见 `AGENTS.md` 的 i18n 规则：键是英文源串，必须 7 个 locale 同步）。
4. 测试放 `web/src/features/channels/__tests__/`。

**这一步不碰 Go 分派逻辑，风险最低，先跑通「后端驱动前端」的通路。**

### Step B — `openai_chat` 宿主协议（真正的硬骨头）

1. 在 `pkg/jsplugin/routing.go` 的 `hostProtocols` 追加 `openai_chat`，定义 operations（建议 `submit` 复用现有语义）。
2. 新增 provider 原语：URL/头构造、`service.ConvertRequest` 复用、`helper.StreamScannerHandler` 复用、usage 提取、错误归一。
3. **SSE 逐帧回调是唯一难点**：参考 `relay/channel/task/jsplugin/adaptor.go` 的桥接方式，但任务插件是「提交+轮询」，文本渠道是「一请求一流」，语义不同，**不能照抄**。
4. 插件侧仍受 `utils.fetch` 仅 GET/HEAD 的限制 —— **上游转发必须由宿主做**（这正是网关设计），插件只提供「怎么构造请求」的描述。

### Step C — `GetAdaptor` 注册表化

1. 引入 `relay/channel/registry.go`，`Registration{ChannelType, APIType, Name, DefaultBaseURL, SupportsStream, NewAdaptor, PluginKey}`。
2. 内置适配器在各自包 `init()` 里 `Register(...)`。**这个模式在本仓库已被验证两次**：`oauth/github.go:19-20`、`service/authz/resources_channel.go:21`。
3. `GetAdaptor` 改查表；`ChannelType2APIType` 修掉「未知静默回退 OpenAI」，改为显式错误。
4. ⚠️ **绝不重排 `iota`**：`constant/api_type.go` 与 `constant/channel.go` 的值可能已持久化（`channels.type`、任务 `platform`）。只能追加或显式赋值。

### 工作量参考（来自原方案，仅供参考）

| 阶段 | 人日 |
|---|---|
| Step 1 注册表化 | 6–10（37 case 是全量回归面） |
| Step 2 后端下发 + 前端对接 | 4–6 |
| Step 3 插件渠道（含 `openai_chat` + SSE） | 15–22 |
| **合计** | **25–38** |

若只做「声明式透传渠道」（URL 模板 + 固定头 + body 直通，零 JS）：可压到 **4–7 人日**，但只能覆盖 `siliconflow` / `openrouter` / `xinference` 这类 body 完全透传的渠道（现存约 4–5 个可被这样表达）。**如果时间紧，先做这个。**

---

## 4. T0 冻结核心：**不得插件化**（违反即架构事故）

| 领域 | 为什么冻结 |
|---|---|
| 计费换算与额度事务 | 插件只能是「用量事实来源」，价格与扣费由宿主表达式决定 |
| 身份认证与凭证 | 插件**拿不到**上游 key 原值；凭证存取留在宿主 |
| 授权判定（Casbin + PAT） | 插件可声明所需权限，但判定由宿主执行 |
| 核心 relay 管道（重试、限流、日志） | 插件只提供「怎么转」，不提供「怎么调度」 |
| 数据层与迁移 | 插件不得触达宿主表名 |
| 治理与品牌 | 见 §7 |

**安全姿态（不得放宽）**：无 `fetch`/FS/`require`/`async`/`eval`/env；`utils.fetch` 仅 GET/HEAD + host 白名单；未知 meta 字段即拒绝；5s / 8 并发 / 看门狗。

---

## 5. 动手前必读（硬门禁）

| 改动范围 | 必读 |
|---|---|
| 任何改动 | 根 `AGENTS.md`（含新增的 Required Tooling 章节） |
| `web/` | `web/AGENTS.md` + `.agents/skills/shadcn-ui/SKILL.md` |
| 插件契约 | `docs/plugin-api/v1.md` + `v1.schema.json` + `v1.d.ts`（**三者必须一致**） |
| 计费（判定见 `AGENTS.md`） | `.agents/rules/billing.md` **全文**（用 read 工具，不是 grep） |
| Docker / CI | `DEVELOPMENT.zh_CN.md` §17.4–17.6 与 §18.5 |

**`gh` 是必备工具**：远端状态只用 `gh` 取，不要猜。
`gh auth status` 当前 token scope = `gist, read:org, repo, workflow` —— **能读 run 与日志，读不了 package**。

---

## 6. 工程约定（插件化专用）

1. **测试文件不扩散**：一个小特性只扩展现有测试文件；新增最多 1 个，并集中放关键回归用例。测试放模块专属 `__tests__/`（前端）或同级 `_test.go`（后端）。
2. **`relaykit` / `tokenkit` 保持独立可构建**：改后必须在模块内跑 `GOWORK=off go build ./...`。
3. **JSON 一律走 `common.*`**（relaykit 内用 `kitutil.*`），不直接 `encoding/json`。
4. **可选标量必须 `*T` + `omitempty`**，显式零值原样转发上游。
5. **测试判读坑**（Windows）：
   - 看 `--- FAIL` 行，不看包退出码；
   - 大量 `unlinkat ... being used by another process` 是 SQLite 句柄噪声；
   - 本仓库 `*DatabaseMatrix`、passkey、security 用例在 Windows 下**常驻失败**（基线实测 73 个，与改动无关）；
   - 改前先 `git worktree add ../cz-api-base <sha>` 跑基线对比，避免把噪声当回归；
   - 并发跑多包会出现假 `[build failed]`，逐包串行；
   - 删公共符号后 `go clean -cache && go build ./...` 复核。
6. **前端测试**：`isolate: true` 必须保持（`isolate: false` 快 77% 但会让 `lobe-icon` 模块级缓存跨文件泄漏，导致只在全量跑时随机失败）。要提速用 `vitest --shard`。
7. **镜像/CI 效率**：Docker 阶段按输入失效、依赖与编译一律 cache mount、禁 QEMU、`push: true` 必须给 `tags`。已删除上游 Docker Hub 工作流，**不要恢复**。

---

## 7. 受治理保护：**只标注，不可删改**

按 `AGENTS.md` 的 Protected project information：

1. 文件头 `Copyright (C) 2023-2026 QuantumNous` + AGPL-3.0 许可块；
2. module 路径 `github.com/QuantumNous/new-api` 及 `relaykit` / `tokenkit` 子模块路径；
3. `web/src/features/about`、`web/src/assets/logo*`、`web/src/components/layout/components/{logo,header-logo,system-brand}.tsx`、`LICENSE`、`NOTICE`、`THIRD-PARTY-LICENSES.md`、各语言 `README*`；
4. `CHANNEL_PROVIDER_PRESENTATION` 的 provider 描述、`i18n/keys.go` 的 `new_api_error`、`error.type = "new_api_error"`；
5. `VERSION`、`electron/package.json` 的 `productName`、`docker-compose.yml` 的镜像名；
6. `electron/` 整目录。

被要求删除/改名这些内容时：**拒绝并说明受治理保护**。

---

## 8. 可剔除清单（Tier 1）现状

原方案列了 10 项「可立即删除」。**除 2 项外已全部在本仓库落地**：

| # | 项 | 现状（实测） |
|---|---|---|
| 1 | 11 条 `RelayNotImplemented` 路由 | ✅ 已删除（全仓 0 处） |
| 2 | `bin/migration_v0.2-v0.3.sql` 等 | ✅ 已删除（`bin/` 不存在） |
| 3 | `bin/time_test.sh` | ✅ 已删除 |
| 4 | 重复的 `paymentReturnPath` | ⚠️ **两份仍存在**：`controller/return_path.go`、`service/return_path.go` |
| 5 | `SearchAllLogs` / `SearchUserLogs` | ✅ 已删除 |
| 6 | 3 个零调用垫片 | ⚠️ 部分：`relaykit` 的 `GetRawData` 已删；`NewProxyHttpClient` 已改为未导出的 `newProxyHTTPClient`（`service/http_client.go:388`）；`GetFileBase64FromUrl` 已无 |
| 7 | 前端孤立图标 + clerk 标志 | ✅ 已删除（brand-icons 14 个 + 2 个 clerk） |
| 8 | `makefile` 死 target | ✅ 只剩 `dev-api` / `dev-web` / `dev` / `test` |
| 9 | `.env.example` 的 `GEMINI_VISION_MAX_IMAGE_NUM` | ✅ 已删除 |
| 10 | `e2e/` 命名 | ✅ 已改为 `integration/` |

**剩余可做**：#4 的函数去重（约 0.3 人日，风险低）。

Tier 2（需弃用公告，8 项）与 Tier 3（不建议删）见 `PLUGINIZATION.zh_CN.md` §9.2–9.3，本次未动。
**其中 Tier 2 #11 `users.access_token` 遗留仍在服役**，退役期是滚动值，不要急着删。

---

## 9. 需要人决策的问题

| # | 问题 | 建议 |
|---|---|---|
| A | M4 做「完整插件渠道（25–38 人日）」还是「声明式透传（4–7 人日）」 | 先做透传，验证通路后再扩 |
| B | `identity` kind 要不要真正接线（需走 ASVS） | 单独立项，不顺带 |
| C | `pkg/ionet` + deployments 全家桶是否下线 | 产品决策，非代码卫生 |
| D | Tier 2 各项是否进弃用流程 | 逐项勾选 |

---

## 10. 怎么验证你的改动

```bash
# 后端三模块
GOWORK=off go build ./... && GOWORK=off go vet ./...
(cd relaykit && GOWORK=off go build ./... && GOWORK=off go test ./...)
(cd tokenkit && GOWORK=off go build ./... && GOWORK=off go test ./...)

# 前端
cd web && bun check -b && bun run lint && bun run test

# 结果只认同真实运行；CI 用 gh 看
gh run list --limit 3
gh run view <id> --log-failed      # 失败时必看
gh run view <id> --json jobs       # 证明耗时
```

**报结论前必须有一条真实 run 作依据；不许说「应该通过了」。**

---

## 附：关键文件速查

| 用途 | 文件 |
|---|---|
| 插件注册表 / 契约单一来源 | `pkg/jsplugin/registry.go`（2298 行，`PluginKind*` 在 :120-141） |
| 宿主协议表（**要扩这里**） | `pkg/jsplugin/routing.go:89-110` |
| 任务插件桥（参考实现） | `relay/channel/task/jsplugin/adaptor.go`（2108 行） |
| 适配器分派（**要注册表化**） | `relay/relay_adaptor.go:50`（37 case） |
| 渠道常量与名字表 | `constant/channel.go`（226 行）、`constant/api_type.go` |
| 渠道目录（孤儿，待接线） | `constant/channel_registry.go`（138 行） |
| 未知类型回退点 | `common/api_type.go` |
| 流式能力表（影响结算） | `relay/common/relay_info.go` 的 `streamSupportedChannels` |
| job 桥接 | `service/system_task_plugin.go` |
| 插件契约查询 API | `router/api-router.go:266-272` |
| 示例插件 | `examples/plugins/`（6 个） |
| 内置生产插件 | `plugins/tasks/`（11 个） |
| 长方案原文 | `PLUGINIZATION.zh_CN.md` |
| 全架构文档 | `DEVELOPMENT.zh_CN.md`（1436 行） |
