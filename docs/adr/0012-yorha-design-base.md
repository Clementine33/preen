# 0012 YoRHa 视觉底座与三阶段落地

Date: 2026-09-30

Status: Draft

应用此前是一套 shadcn + Linear 风格的主题（`--brand` 紫、Space Grotesk、8px 圆角）。用户在 Work 页原型的多轮评审中确立了 YoRHa 视觉语言，并明确它不能只停留在 Work 页成为「局部孤岛」，也不是把整套像素平铺到 Workset 工坊流程上。本记录确立：以「系统底座（tokens/原语）同构，业务心智（HUD/表单）分层」的方式分三个阶段落地。

**唯一视觉真相源**是已评审的原型 `work-wall-v5-2-yorha-refined.html`（open-design 项目 dorganizer-plan-7686，仓外文件）：其中每一个 token、尺寸与时长都是用户已确认的决定，不是起点建议。改动任何颜色前必须复验对比度（正文对 ≥ 4.5:1）。

## 1 三阶段

- **阶段 1（本次）**：`theme-yorha.css` 承载 YoRHa 原生 tokens 与原语类；`style.css` 把 shadcn 语义层与角色 tokens 全部改为指向 YoRHa 变量的间接引用 —— 现有全部页面不改 markup 即刻换上 YoRHa 色板/字阶/直角圆角。
- **阶段 2（本次）**：Work 页作为旗舰按原型完整落地（作品墙 + 检视舱 + 底边播放条 + 媒体库下拉），路由 `/works`、`/works/:rj`。
- **阶段 3（后续任务，另开 issue）**：Workset 外壳与组件「战术化换肤」：只动容器观感、表格终止线、表单控件圆角、按钮反色与选中高亮，**不推翻**既有业务交互流程（目录树、计划审阅、批量编辑表单）。

## 2 命名与不可逾越的保留色

- 原型的 `--accent`（陶土红）在本仓更名为 **`--now`**：`--accent` 在 shadcn 语义里是悬停面，两个语义不能共用一个名字。陶土红只归「正在播放」（NOW 标、播放中音轨标记、状态点、浅色激光），**绝不挪作他用**。
- 状态角色（success/warning/danger/info）保持字面色值，不并入 YoRHa 色板：它们是领域编码（状态永远不只靠颜色传达），其对比度记录见 `style.css` 注释。原 encode-state 的琥珀即存活于 warning 角色。
- 密度 token（`--row-h` 等）与 `--motion-*` 保持不变；YoRHa 动效时长（`--dur-*`、`--ease-*`）与其并行，页面按自己的语言取用。

## 3 字体三层矩阵

用户显式确认的选型，**覆盖原型的双字体方案**（原型只有 IBM Plex Mono + Zen Kaku Gothic New）：

- `--font-mincho`：Shippori Mincho（回退 Noto Serif JP → Noto Serif SC）—— 作品标题/详情展台/全站 h1（`--font-heading` 重映射至此）。
- `--font-ui`：BIZ UDPGothic（回退 Zen Kaku Gothic New）—— 正文、元数据、表单控件。
- `--font-mono`：IBM Plex Mono —— 标签（`.lbl`）、RJ 号、时间等遥测数字，全部 `tabular-nums`。

经 Google Fonts `@import` 切片加载；离线回退系统字体。此前的 Space Grotesk 实际上从未有任何加载来源，直接废弃。

## 4 已知边界

- 全局 `prefers-reduced-motion` 下一切过渡瞬时生效（原型原样）。
- 颗粒（grain）只铺底材，永不覆盖封面；45° 排线只用于空状态；双终止线只出现在外缘（顶栏下、底条上）。
- 播放条当前挂在 WorksPage 内（28px 行）；音频后端落地后可升级为 AppShell 的全局行。
- 控件边界另设 `--wire-control`（浅 `rgba(20,21,24,.5)`、深 `rgba(222,215,198,.45)`，各底色上 ≥ 3:1，记录见 `style.css`）：原型的 `--wire-mid` 只有约 2.4:1，作装饰线足够，作输入框与控件边界不达 WCAG 1.4.11。这是唯一不来自原型的 token。
- 键盘归属：文本框与展开的菜单（`data-own-keys`）收下自己的全部按键；Enter / Space 落在链接或按钮上时保持其原生含义，不再同时打开选中作品。作品墙只在键盘移动后夺取焦点，筛选结果到达、页面加载、地址恢复都不移动焦点。
