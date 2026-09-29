Status: Draft
Date: 2026-09-30

# 开发 schema 自由迭代，发布库通过 goose 升级

## 1 两种数据库生命周期

开发模式只在空库执行 `backend/go/internal/adapters/sqlite/schema.sql`，已有库允许直接改表，不自动补表；提交时将最终结构写回 schema。发布模式只执行嵌入的 goose SQL 迁移，新库从完整历史创建，已有库按顺序升级。

`schema_meta.database_mode` 区分开发库和发布库，二者不相互转换。首个发布迁移建立新基线，不接管原有 v4 和无迁移历史的数据库；高于程序支持的迁移版本也拒绝打开。此决定替代 ADR 0008 §3 对数据库升级的全面拒绝，其留存、回收及连接/事务决定继续适用。

## 2 只有发布边界冻结迁移

`X.Y.Z-dev` 使用开发模式；发布版和对外 RC 使用 goose。构建元数据不影响模式，程序版本不充当迁移编号。默认开发版本只声明在后端入口的 `main.version`，开发启动和未指定版本的构建均沿用它；发布构建用 `task -d backend/go build RAW_TAG=v0.2.0` 覆盖，`RAW_TAG` 同时支持环境变量。

开发期自由修改 schema 和未发布迁移，不记录每次中间改表。发布前在 `backend/go/internal/adapters/sqlite/migrations/` 整理从上一发布到最终结构的 SQL，以 `-- +goose Up` 开头；无结构变化无需新增迁移。首次分发后，文件名、编号及内容冻结，后续修正追加迁移。

发布前运行 `task test:go` 和 `task lint:go`，验证空库初始化、逐版/跨版升级、数据保留，以及开发 schema 和发布迁移的结构与读写行为一致。

## 3 备份、重建与恢复

已有发布库执行待运行迁移前，用 `VACUUM INTO` 保存含已提交 WAL 内容的一致性备份到数据目录的 `backups/`，验证后再迁移，日志显示备份路径。备份或迁移失败均阻止启动；已成功的迁移在重试时跳过。只支持 Up，备份不自动清理。

重建开发库：先停止后端，再从根目录运行 `task db:rebuild:dev`。命令固定操作 `.dev_data/cache.db`，忽略 `ONSEI_DATA_DIR`，先备份到 `.dev_data/backups/`，再创建新库并替换原文件；配置和媒体文件不变。旧开发库可以重建，goose 管理的发布库拒绝重建。

恢复备份：停止所有使用该库的进程，将当前 `cache.db` 及存在的 `cache.db-wal`、`cache.db-shm` 移到另一目录保留，再把选定备份复制为 `cache.db`，启动对应旧程序。恢复库旁不能残留失败库的 WAL/SHM 文件。

## 拒绝的替代方案

保留手写 SQL，不引入 GORM；开发中间步骤不成为永久迁移历史。独立开发、本地单实例使用现有测试与发布后不可变约定，不增加发布清单、摘要校验、进程锁或额外流水线。
