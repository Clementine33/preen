# 0000 记录目录

本文件维护 `docs/adr/` 的记录目录：它不记录决定，因此没有 `Status` 行，也不参与记录的状态流转。「状态」一列镜像各记录自己的 `Status:`。规矩见 [AGENTS.md](../../AGENTS.md) 的 ADR charter；领域语言见 [CONTEXT.md](../../CONTEXT.md)；HTTP/SSE 契约见 [api.md](../api.md)。

## 当前架构决策集

| 记录 | 主题 | 状态 |
|---|---|---|
| [0001](0001-library-workbench-and-processing-record.md) | 媒体库即工作台、计划与执行所有权、规划回放与活动约束、数据库自清理与留存、手动运行准入（历史、作品、关系报告与共享来源实体清理 API 已实现；网页入口待实现） | Accepted |
| [0002](0002-direct-file-management-and-admission.md) | 直接文件管理、全局准入互斥与作品同级回收目录 | Accepted |
| [0003](0003-short-routes-and-directory-identity.md) | 短路由与不透明目录身份 | Accepted |
| [0004](0004-task-seam-and-opaque-payloads.md) | Task 接缝、不透明载荷与工作集共享批次 | Accepted |
| [0005](0005-declarative-audio-planning.md) | 声明式规划、稀疏继承、无音效匹配与子树隔离 | Accepted |
| [0006](0006-execution-sessions-and-component-results.md) | 执行会话、编码池、只读校验与安全发布 | Accepted |
| [0007](0007-business-packages-and-dependency-direction.md) | 业务分包与依赖方向 | Accepted |
| [0008](0008-deployment-wide-common-settings.md) | 部署级公共设置与唯一写入口（移除独立策略模板） | Accepted |
| [0009](0009-work-catalog.md) | 作品目录身份、外部关系报告、库存扫描与用户资料保护（独立报告、完整观察与目录直属文件沿用、确定关系、详情快照与作品／报告／来源清理已实现；收录时间与封面地址去重） | Accepted |
| [0010](0010-defer-single-writer.md) | 暂不引入全局单写入口 | Accepted |
| [0011](0011-development-schema-and-release-migrations.md) | 开发 schema、当前未发布基线与旧发布库边界 | Accepted |
| [0012](0012-metadata-refresh-and-directory-rename-planning.md) | 元数据刷新、外部父链报告、目录重命名与修复、封面投递、需求生命周期及下载回收（有效父链报告补抓已实现） | Accepted |
| [0013](0013-derived-media-thumbnails-content-and-facts.md) | 派生媒体：缩略图、文件内容分发与后台媒体事实 | Accepted |
| [0014](0014-storage-class-and-concurrency-limits.md) | 存储档与并发上限（扫描统一 32 worker 重叠元数据访问等待） | Accepted |
| [0015](0015-deployment-and-credentials.md) | 部署形态、凭证投递与桌面壳（Wails v3 子进程托管、便携数据位置） | Accepted |
| [0016](0016-lyrics-window-hosting.md) | 歌词窗口的宿主：Document PiP 与桌面壳原生窗口（Windows 透明与锁定穿透，Linux 对齐 PiP） | Accepted |
| [0017](0017-inventory-storage-dirs-and-files.md) | 库存存储：目录表与文件表分离、会话临时暂存、Library 库根互不包含 | Accepted |
| [0018](0018-self-hosted-fonts.md) | 字体：同源自托管的 CJK 无衬线与衬线（Noto Sans SC Variable、Noto Serif SC Variable、IBM Plex Mono） | Accepted |
| [0019](0019-outbound-proxy-follows-the-system.md) | 出站代理：跟随系统、手动或直连（Windows WinHTTP 含 PAC；Linux KDE／GNOME／环境变量） | Accepted |
