Status: Draft — 2026-11-16

# 0020 线格式契约：由 Go 类型生成 JSON Schema

## 背景

HTTP/SSE 的线格式现在有三处各自维护的描述：Go 侧的 json 标签（分散在 `adapters/httpapi` 和直接被序列化的领域类型里），前端手写的 `lib/api/types.ts`，以及 `docs/api/*.md` 的文字说明。三者之间没有机器检查：后端 handler 测试用零散的键断言检查响应，前端测试自己 mock fetch，`client.ts` 用 `response.json() as T` 直接信任响应。改了一侧，另一侧的测试照样能过。代码越来越多由 AI 生成，这种不被察觉的漂移风险正在变大。

[0015](0015-deployment-and-credentials.md) 规定前端产物由同一个服务同源托管、随同一个包发布，所以新加载的页面总是和服务同一版本。唯一的例外是服务升级时仍然开着的旧标签页：旧版 JS 会继续请求新后端，同源托管不会替换正在运行的页面。除此之外，漂移都发生在开发和构建阶段，应当在 CI 里拦住，而不是等到运行时。

## 决定

1. **Go 是线格式的唯一来源。** 线上出现的每个 JSON 请求体、响应体（包括错误信封）和 SSE 事件载荷（包括 error 事件）都是 `adapters/httpapi` 里的具名类型。领域类型不直接编码到线上，也不直接从请求解码，需要时在 httpapi 里转换或包装。`arch` 测试守住这条边界。
2. **结构由生成的 JSON Schema 表达。** 从 httpapi 的具名类型生成 JSON Schema，入库到 `docs/api/schema/`。schema 描述实际的序列化结果：自定义 `MarshalJSON` 的类型用手写的 schema 覆盖，生成时不对结果做美化。CI 重新生成后必须零 diff。
3. **不透明载荷按 Task kind 分别建 schema。** [0004](0004-task-seam-and-opaque-payloads.md) 中以 `json.RawMessage` 传输的 Task 载荷和设置，外层 schema 只能看到一个不透明字段。每个 Task kind 和 `schema_version` 的组合要有自己的载荷 schema，由各 Task 包的具名类型生成；外层 schema 按 kind 引用对应的载荷 schema。
4. **职责划分。** schema 负责字段、类型、可选与可空；`docs/api/*.md` 负责语义、行为、错误码、准入与版本规则，并链接到对应的 schema，不再重复字段的类型说明。
5. **后端测试按 schema 校验。** httpapi 测试和 e2e 测试用同一个辅助函数，把请求体、成功响应、错误响应和 SSE 事件载荷对照 schema 校验。键断言只保留在检查语义值的地方。
6. **前端 wire 类型由 schema 生成。** 生成的类型取代 `types.ts` 里的请求和响应 DTO。只有路径参数、查询参数、客户端契约和配置类型仍然手写。客户端对数据的整形（例如 `classifier_tags` 的归一化）放在 wire 类型到视图类型的一层，组件只使用视图类型。
7. **运行时校验只在开发和测试环境启用，契约由调用点指定。** 泛型参数在运行时不存在，所以每个 API 方法要显式传入 schema 标识：HTTP 按状态码分别对应成功 schema 和错误信封，SSE 按事件名对应载荷 schema。生产构建不包含这段代码。服务升级时旧标签页和新后端之间的错配风险，接受下来，由用户刷新页面消除，不在生产环境做校验。

## 放弃的方案

- **前端手写 zod schema**：会成为 Go 类型、TS 类型之外的第三个来源，而且后端测试读不到它，拦不住后端漂移；放到生产环境运行，能拦住的只有升级时旧标签页的错配，代价却是作品墙这类大列表每次都要多解析一遍；这种错配更适合靠刷新页面解决。
- **Go 直接生成 TS 类型（例如 tygo），不产出 schema**：前端能得到类型，但后端测试和脚本客户端没有可以执行校验的产物；自定义 `MarshalJSON` 的实际形状也无处表达。
- **完整的 OpenAPI 描述（包括路由、参数和认证）**：覆盖面大，但路由、准入和错误语义已经由 `docs/api/*.md` 承担，再全部迁过去成本高、收益低；只把结构机器化就能解决漂移问题。
- **以 TS 类型为来源反向约束后端**：线格式由后端决定，以前端为来源会让后端测试依赖前端构建，与 0015 中“后端构建和测试不依赖前端产物”的考量相冲突。
- **维持现状，靠键断言和人工同步**：现在大约有 300 处零散断言，覆盖不完整、无法审计，前后端之间也没有任何联系。

## 后果

- httpapi 会多出一批具名 DTO 和转换代码；匿名的请求和响应结构，以及直接编码或解码的领域类型，都需要先迁移，迁移完成前 schema 只覆盖已迁移的端点。
- 新增一个 Task kind 或提升载荷的 `schema_version` 时，必须同时提供它的载荷 schema。
- 服务升级时还开着的旧标签页，在刷新前可能把新响应按旧结构解析。
- 生成器和校验器会引入新的依赖：Go 侧一个 JSON Schema 生成库，前端开发依赖里一个 schema→TS 生成器和一个校验器。
- 可空性会被如实暴露。httpapi DTO 的数组字段从不为 `null`，空集合是 `[]`；存储的 Task 载荷可能仍含 `null` 数组，它们的 schema 如实保留可空。约定见 [common.md](../api/common.md#wire-schemas)。
- 脚本客户端可以直接使用入库的 schema。

## 实施约束

- 阶段 2 使用 invopop/jsonschema 生成 Draft 2020-12，Go 测试使用 santhosh-tekuri/jsonschema/v6 校验；两者都只被测试/生成代码引用，后端构建不依赖前端。
- schema 按具名类型分别入库。HTTP 状态码和 SSE 事件绑定单独生成；Task 外壳按 kind/version 生成独立 schema 并引用 Task 包的载荷类型。请求与响应分别建模。
- 编码差异如 PascalCase FileTuple、兼容 classifier_tags、缺少 operation_type 的 session_snapshot 均如实保留。metadata_refresh 的 TaskResult 用现有 stage 判别，不增加线上字段。

- 阶段 3 使用 json-schema-to-typescript 16.0.0；相对引用先做等价重定位，metadata 结果使用显式 oneOf，使生成 TS 可表达判别。存储中原样透传的旧 classifier_tags 载荷有独立兼容 schema，不改规范编码或请求形状。
- 前端 wire.gen.ts 只由 schema 生成；视图类型以生成类型为基础，仅收窄展示状态或表达客户端整形。session_snapshot 的操作身份由订阅上下文补入缓存视图，原始 SSE schema 仍没有该字段。

- 阶段 4 使用 ajv 8.20.0 的 standalone ESM 生成物。schema ID、HTTP 状态和 SSE 事件绑定同时生成，调用点显式选择契约；执行完整、成员汇总和单成员视图各有独立入口。
- JSON 请求/响应和 SSE 解析在开发、Vitest 与使用 Vite 的 Playwright 中校验。调用点的编译期条件同时移除校验入口和契约元数据；生产构建逐模块检查，禁止渲染校验器、绑定元数据及 AJV。
