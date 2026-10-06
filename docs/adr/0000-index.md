# 0000 记录目录

本文件维护 `docs/adr/` 的记录目录：它不记录决定，因此没有 `Status` 行，也不参与记录的状态流转。「状态」一列镜像各记录自己的 `Status:`，与状态变更在同一次提交中更新。规矩见 [AGENTS.md](../../AGENTS.md) 的 ADR charter；领域语言见 [CONTEXT.md](../../CONTEXT.md)；HTTP/SSE 契约见 [api.md](../api.md)。

| 记录 | 主题 | 状态 |
|---|---|---|
| [0001](0001-library-workbench-and-processing-record.md) | 媒体库即工作台：处理记录与当前计划 | Superseded |
| [0002](0002-direct-file-management-and-admission.md) | 直接文件管理与全局准入互斥 | Accepted |
| [0003](0003-short-routes-and-directory-identity.md) | 短路由与目录身份：地址不含名称 | Accepted |
| [0004](0004-task-seam-and-opaque-payloads.md) | Task 接缝与不透明载荷 | Accepted |
| [0005](0005-declarative-audio-planning.md) | 声明式音频规划：目标输出与转换模式 | Accepted |
| [0006](0006-sparse-inheritance-and-explicit-editing.md) | 稀疏继承与显式编辑 | Accepted |
| [0007](0007-execution-sessions-and-component-results.md) | 执行会话：编码池、提交顺序与组件结果行 | Accepted |
| [0008](0008-database-self-cleanup-and-retention.md) | 数据库自清理与会话留存 | Accepted |
| [0009](0009-business-packages-and-dependency-direction.md) | 业务分包与依赖方向 | Accepted |
| [0010](0010-deployment-wide-common-settings.md) | 部署级公共设置：一次保存同步所有记录 | Accepted |
| [0011](0011-work-catalog.md) | 作品目录：作品号身份、来源数据组与派生关系 | Draft |
| [0012](0012-yorha-design-base.md) | YoRHa 视觉底座与三阶段落地 | Draft |
| [0013](0013-development-schema-and-release-migrations.md) | 开发 schema 自由迭代，发布库通过 goose 升级 | Draft |
| [0014](0014-metadata-refresh-and-directory-rename-planning.md) | 元数据刷新、作品目录主封面与目录重命名 | Draft |
| [0015](0015-thumbnail-service.md) | 缩略图服务：可丢弃的派生缓存与可替换的缩放引擎 | Draft |
| [0016](0016-work-file-content-distribution.md) | 作品文件内容的只读分发：清单与字节 | Draft |
| [0017](0017-inventory-scanning-and-sqlite-merge.md) | 库存扫描：作品级调度、完整观察集与按变化合并 | Draft |
| [0018](0018-work-local-recovery-directories.md) | 深层作品的同级回收目录与扫描排除 | Draft |
| [0019](0019-conversion-execution-and-selected-generation.md) | 转换执行的只读校验、安全发布与选中范围生成 | Draft |
| [0020](0020-shared-workset-batch.md) | 工作集共享一个文件夹批次，各 Task 独立保存计划 | Draft |
