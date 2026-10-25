# 0000 记录目录

本文件维护 `docs/adr/` 的记录目录：它不记录决定，因此没有 `Status` 行，也不参与记录的状态流转。「状态」一列镜像各记录自己的 `Status:`。规矩见 [AGENTS.md](../../AGENTS.md) 的 ADR charter；领域语言见 [CONTEXT.md](../../CONTEXT.md)；HTTP/SSE 契约见 [api.md](../api.md)。

## 当前架构决策集

| 记录 | 主题 | 状态 |
|---|---|---|
| [0001](0001-library-workbench-and-processing-record.md) | 媒体库即工作台、计划与执行所有权、规划回放与活动约束、数据库自清理与留存、手动运行准入（历史、作品、关系报告与共享来源实体清理 API 已实现；网页入口待实现） | Draft |
| [0002](0002-direct-file-management-and-admission.md) | 直接文件管理、全局准入互斥与作品同级回收目录 | Draft |
| [0003](0003-short-routes-and-directory-identity.md) | 短路由与不透明目录身份 | Draft |
| [0004](0004-task-seam-and-opaque-payloads.md) | Task 接缝、不透明载荷与工作集共享批次 | Draft |
| [0005](0005-declarative-audio-planning.md) | 声明式规划、稀疏继承、无音效匹配与子树隔离 | Draft |
| [0006](0006-execution-sessions-and-component-results.md) | 执行会话、编码池、只读校验与安全发布 | Draft |
| [0007](0007-business-packages-and-dependency-direction.md) | 业务分包与依赖方向 | Draft |
| [0008](0008-deployment-wide-common-settings.md) | 部署级公共设置与唯一写入口（移除独立策略模板） | Draft |
| [0009](0009-work-catalog.md) | 作品目录身份、外部关系报告、库存扫描与用户资料保护（独立报告、扫描复用、确定关系、详情快照与作品／报告／来源清理已实现；收录时间与封面地址去重） | Draft |
| [0010](0010-defer-single-writer.md) | 暂不引入全局单写入口 | Draft |
| [0011](0011-development-schema-and-release-migrations.md) | 开发 schema、当前未发布基线与旧发布库边界 | Draft |
| [0012](0012-metadata-refresh-and-directory-rename-planning.md) | 元数据刷新、外部父链报告、目录重命名与修复、封面投递、需求生命周期及下载回收（有效父链报告补抓已实现） | Draft |
| [0013](0013-derived-media-thumbnails-content-and-facts.md) | 派生媒体：缩略图、文件内容分发与后台媒体事实 | Draft |
| [0014](0014-storage-class-and-concurrency-limits.md) | 存储档与并发上限 | Draft |
| [0015](0015-deployment-and-credentials.md) | 部署形态与凭证投递 | Draft |
| [0016](0016-lyrics-window-hosting.md) | 歌词窗口的宿主：Document PiP 与桌面壳接缝 | Draft |
| [0017](0017-inventory-storage-dirs-and-files.md) | 库存存储：目录表与文件表分离 | Draft |

## 历史编号映射表

在 2026-10-10 架构重组中，为提高 ADR 门槛、去除实现细节并避免过度碎片化，原有 23 篇记录合并为上述 13 篇。此表供历史 commit、PR 与讨论记录回查：

| 旧编号 | 历史主题 | 新归属 / 去向 |
|---|---|---|
| 0001 | 媒体库即工作台：处理记录与当前计划 | [0001](0001-library-workbench-and-processing-record.md) |
| 0002 | 直接文件管理与全局准入互斥 | [0002](0002-direct-file-management-and-admission.md) |
| 0003 | 短路由与目录身份：地址不含名称 | [0003](0003-short-routes-and-directory-identity.md) |
| 0004 | Task 接缝与不透明载荷 | [0004](0004-task-seam-and-opaque-payloads.md) |
| 0005 | 声明式音频规划：目标输出与转换模式 | [0005](0005-declarative-audio-planning.md) |
| 0006 | 稀疏继承与显式编辑 | [0005](0005-declarative-audio-planning.md) |
| 0007 | 执行会话：编码池、提交顺序与组件结果行 | [0006](0006-execution-sessions-and-component-results.md) |
| 0008 | 数据库自清理与会话留存 | [0001](0001-library-workbench-and-processing-record.md) |
| 0009 | 业务分包与依赖方向 | [0007](0007-business-packages-and-dependency-direction.md) |
| 0010 | 部署级公共设置：一次保存同步所有记录 | [0008](0008-deployment-wide-common-settings.md) |
| 0011 | 作品目录：作品号身份、来源数据组与派生关系 | [0009](0009-work-catalog.md) |
| 0012 | YoRHa 视觉底座与三阶段落地 | 退出 ADR，转入前端设计规范 [frontend/web/DESIGN.md](../../frontend/web/DESIGN.md) 与 [frontend/web/AGENTS.md](../../frontend/web/AGENTS.md) |
| 0013 | 开发 schema 自由迭代，发布库通过 goose 升级 | [0011](0011-development-schema-and-release-migrations.md) |
| 0014 | 元数据刷新、作品目录主封面与目录重命名 | [0012](0012-metadata-refresh-and-directory-rename-planning.md) |
| 0015 | 缩略图服务：可丢弃的派生缓存与可替换的缩放引擎 | [0013](0013-derived-media-thumbnails-content-and-facts.md) |
| 0016 | 作品文件内容的只读分发：清单与字节 | [0013](0013-derived-media-thumbnails-content-and-facts.md) |
| 0017 | 库存扫描：作品级调度、完整观察集与按变化合并 | [0009](0009-work-catalog.md) |
| 0018 | 深层作品的同级回收目录与扫描排除 | [0002](0002-direct-file-management-and-admission.md) |
| 0019 | 转换执行的只读校验、安全发布与选中范围生成 | [0006](0006-execution-sessions-and-component-results.md) |
| 0020 | 工作集共享一个文件夹批次，各 Task 独立保存计划 | [0004](0004-task-seam-and-opaque-payloads.md) |
| 0021 | 无音效匹配与其他版本子树隔离 | [0005](0005-declarative-audio-planning.md) |
| 0022 | 成员文件页：选择即目标、移动到成员根与拒绝码 | 决策并入 [0002](0002-direct-file-management-and-admission.md)；接口契约并入 [docs/api/libraries.md](../api/libraries.md#direct-file-management) |
| 0023 | 暂不引入全局单写入口 | [0010](0010-defer-single-writer.md) |
