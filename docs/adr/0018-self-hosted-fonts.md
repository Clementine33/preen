Status: Accepted — 2026-10-28

# 0018 字体：同源自托管的 CJK 无衬线与衬线

## 背景

前端字体原先通过 `style.css` 顶部的 `@import url(fonts.googleapis.com…)` 引入 7 个家族（BIZ UDPGothic、Noto Sans JP/SC、Noto Serif JP/SC、Shippori Mincho、IBM Plex Mono）。生产构建在 4 倍 CPU 节流下的冷加载暴露三个问题：

- 样式表 `@import` 是渲染阻塞链：浏览器要先取到字体 CSS，页面才能首次绘制；样式表还会阻塞后面脚本的执行，入口脚本约 15 ms 就已下载完，第一个 API 请求却要等到字体 CSS 到达之后才发出（冷加载实测 `/api/v1/libraries` 约 2.1 s，字体 CSS 已缓存时约 0.46 s）。
- 这些家族按 `unicode-range` 切成约 200 个字体切片，到达一片就触发一次整页重排（1700～2000 个节点，每次 50～135 ms）。
- 桌面壳离线时取不到字体，界面退回系统字体栈。

## 决定

1. **三套字体全部随前端打包，同源提供**，通过 `@fontsource` 的 npm 包在 `main.ts` 引入，版本在 `package.json` 中精确锁定：
   - `--font-ui`：Noto Sans SC Variable（可变字重，覆盖界面用到的全部字重），后接 `PingFang SC`、`Microsoft YaHei`、`sans-serif`。
   - `--font-mincho`：Noto Serif SC Variable（可变字重），只用于作品标题和详情页展台标题，用法规则见 [DESIGN.md](../../frontend/web/DESIGN.md)。字体里没有的字形回退到系统衬线。
   - `--font-mono`：IBM Plex Mono 的 latin 子集 400／500／600／700，字体栈不变。
2. **不再有按语言区分的字体**：`--font-ja` 删除，其使用处并入 `--font-ui` 或直接继承。
3. **字体 CSS 保持 `unicode-range` 切片，只取用到的切片**；构建时去掉 `@fontsource` 附带的 woff 回退，只发布 woff2（所有受支持的引擎都读 woff2），避免 dist 与桌面壳内嵌资源翻倍。

## 放弃的方案

- **继续使用 Google Fonts CDN**：渲染阻塞链与样式表阻塞脚本的代价无法在客户端消除，离线也不可用。
- **界面文字只用系统 CJK 字体栈**：浏览器与桌面壳在不同系统上字形、字重不一致，品牌的排版气质不可控。
- **按语言配字体（`--font-ja`、Noto Sans JP、BIZ UDPGothic）**：用户提供的作品名混有简体、繁体和日文，要按作品甚至按音轨判断语言再切换字体，复杂度远高于收益；中日共用汉字在一套简体字体下显示为简体字形，是接受的代价。
- **Shippori Mincho 作明朝体**：日文明朝的字形气质更强，但只覆盖日文汉字，作品名里的简体字和不少繁体字缺字，同一标题内衬线字形混杂；改用覆盖简繁日汉字的 Noto Serif SC。
- **Noto Serif JP 作明朝体或回退**：与 Noto Serif SC 覆盖重叠，多一套切片只换来中日共用汉字的日文字形，与界面字体的取舍不一致。

## 后果

- 首次绘制与首个 API 请求不再等待外部网络；重排只由同源切片的到达触发，且切片到达得更早。
- dist 增加约 11 MB 的 woff2 切片文件（约 200 个文件），主样式表增至约 280 KB（`@font-face` 声明，含全部切片的范围表）；浏览器只下载页面实际用到的切片。作品墙一页 60 个标题在冷加载时约取 50 个衬线切片，与界面切片合计约 6 MB。桌面壳内嵌的前端资源同比增大。
- 简繁日共用汉字一律显示为 Noto Sans SC／Noto Serif SC 的简体字形。
- 新增字重或字体需要同时修改 `main.ts` 的引入与本记录。
