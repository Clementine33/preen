Status: Draft — 2026-10-26

# 0016 歌词窗口的宿主：Document PiP 与桌面壳接缝

## 背景

Lyrics Window（见 [CONTEXT.md](../../CONTEXT.md)）要在任何页面跟随正在播放曲目的 Timed Lyrics，并能控制播放器。浏览器侧没有一种悬浮窗口能同时覆盖所有引擎；桌面壳选用 Wails v3，其多窗口都是独立 webview，与主页面不共享 JS realm。

## 决定

1. **浏览器只走 Document Picture-in-Picture（Chromium 116+）**：窗口内容是普通 Vue 组件，在同一 realm 内挂载并直接读写响应式状态。检测不到该能力时入口隐藏，不提供替代实现。
2. **维护链路 = Chromium + 桌面壳**：Firefox、Safari 不在支持范围；桌面版由壳提供原生窗口。
3. **宿主接缝在 `DesktopAdapter`**：平台标识中性化为 `'browser' | 'desktop'`；`openLyricsWindow(frame, onCommand, onClose)` 返回关闭函数，用户关窗时回调 `onClose`，不支持时返回 null。本轮桌面实现只占位。
4. **窗口契约按跨 realm 设计**：窗口组件只接收可序列化的快照（作品、曲名、当前行、暂停与前后曲可用性），只发出 `prev | toggle | next` 三种命令；它不读 Pinia、不用路由。当前行在主窗口计算后推送，桌面壳以后用事件通道承载同一契约即可。
5. **窗口是单行滚动的纯展示 + 悬停浮现的顶部控制栏**：固定深色底，不做键盘控制、不做点行跳转。Document PiP 在手机和平板上不可用，功能只面向桌面，因此沿用桌面歌词的惯例，控制栏不占歌词区高度。

## 放弃的方案

- **canvas → captureStream → `<video>` 的视频 PiP**：Chromium 与 Safari 可用，但窗口内容是图像，无法交互、需自绘文本排版，后台需另行驱动重绘；为多覆盖一个 Safari 承担一条独立渲染链路，不值得。Firefox 依然不可用。
- **`window.open` 弹窗兜底**：不能置顶，易被拦截，与"悬浮"语义不符。
- **adapter 返回 `Window`/`Document` 让调用方自行挂载**：桌面壳的独立 webview 给不出同 realm 对象，接缝会在桌面落地时被推翻。
- **逐字卡拉 OK 填色、液态玻璃背景**：真实库里的 Timed Lyrics 以只有行级时间的翻译歌词为主，逐字填色没有数据来源；Document PiP 窗口不透明，玻璃只能伪造，真正的系统材质只在桌面壳可得，收益不抵维护。

## 后果

- 非 Chromium 浏览器用户没有 Lyrics Window。
- 桌面壳落地时需要：一个承载窗口组件的入口、主窗口到窗口的快照推送与命令回传通道；契约本身不变。
- Wayland 没有窗口置顶协议，Linux 桌面壳的 Lyrics Window 不能保证悬浮在其他窗口之上。
- 若改为支持 Safari 或放宽契约（例如窗口内跳转、键盘控制），重新评估本记录。
