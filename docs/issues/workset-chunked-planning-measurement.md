# 工作集分块暂存、退役清理与规模测量记录

日期：2026-10-30
关联决策：[ADR 0001](../adr/0001-library-workbench-and-processing-record.md)

---

## 1. 测量入口与口径

### 复现入口

后端测量入口位于 `backend/go/internal/adapters/sqlite`（默认跳过，需显式开启环境变量）：

```sh
PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=2000 PREEN_MEASURE_AUDIO=12 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m

PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_SELECTED=20 PREEN_MEASURE_AUDIO=12 \
  go test ./internal/adapters/sqlite -run '^TestMeasureLibraryDeletion$' -count=1 -v -timeout=30m
```

前端虚拟列表测量使用 `frontend/web/e2e/member-list-harness.js`（支持 `?members=1000|2000|5000` 查询参数，默认 5,000）。

### 合成负载与组件装配

- **媒体规模**：每个成员目录包含 12 个 15 字节的合成 FLAC 文件；500 / 1,000 / 2,000 / 5,000 成员分别对应 6,000 / 12,000 / 24,000 / 60,000 个音频文件。
- **模拟与真实边界**：网络请求、音频编码与媒体探测在依赖端口模拟，不注入人工延迟；编码过程仍执行真实文件句柄读写、`fsync`、文件发布、生成凭据校验及库存同步；封面为有效 PNG 字节，第二轮使用不同图像字节。
- **服务与数据库配置**：SQLite（WAL、`foreign_keys`、`busy_timeout=5000`、`IMMEDIATE` 写事务）、库存扫描、准入门、`conversion` 与 `metadata_refresh` 任务、调度器、执行器、封面投递及 HTTP handler 均使用生产实现（第 1–7 节夹具未装配 `rename` 任务；第 8 节起已装配）。规划并发为 1，编码并发为 4，连接池不设额外连接数上限。
- **准入绕过说明**：各档位测量执行时的生产成员上限常量为 500；超过 500 成员的运行仅在测试夹具中调用生产 `ReplaceCurrentWorkset` 写入全量成员与种子草稿以绕过创建数量检查，其余规划、执行、投递、读取、删除与清理均走生产服务路径。

### 计时与统计定义

- **SQL 事务计时**：在 driver 层覆盖主连接池与扫描私有连接池，不包含 `database/sql` 连接池排队时间（口径同 [SQLite 写竞争测量](sqlite-write-contention-measurement.md)）。
  - `BEGIN`：从调用开始至取得写锁并返回。
  - `持锁`（held）：从 `BEGIN` 成功后至 `COMMIT` 或 `ROLLBACK` 返回。
  - `TEMP` 库的 `BEGIN DEFERRED`（`WriteStagingEntries/temp`）单列，不代表主库写锁。
  - 无显式 `BEGIN` 的单语句自动提交（含 `UPDATE ... RETURNING` 直至 `rows` 关闭）仅可观测整句耗时（含锁等待），单独列出，不计入显式事务的 `BEGIN` 或持锁列。
  - 初始化数据库与首次全库扫描不计入下述 SQL 统计表。
- **HTTP 读取计时**：包含 handler 查询、领域视图构造与 JSON 序列化；响应字节数为完整未压缩 JSON body 长度（不含 HTTP header、压缩、网络传输与客户端解析）。每轮每接口请求一次。
- **生成端到端计时**：从发起生成请求至轮询观测到 `completed`（轮询间隔 10 ms）。
- **统计边界**：所有运行均为单次执行，未清理操作系统页缓存，未叠加并发 HTTP 编辑或外部独立锁竞争进程；表中数值均为当次实测最大值，不是 p99。第 2–5 节原有运行的 SQL 尝试失败数均为 0；第 7 节复测另列失败。

---

## 2. 单事务规划持久化与级联替换基线（500 / 1,000 成员）

基线在拆分规划事务与引入退役标记前采集。完整流程为：创建记录 → `conversion` 生成与执行 → `metadata_refresh` 生成与执行 → 完成全部封面投递 → 重新生成并执行这两个操作（各自替换已完整执行的旧计划）→ 再次完成全部封面投递 → 替换当前记录。两轮各 N 个封面投递及两个操作各两次执行均完成（`succeeded`）。含夹具构造的完整测试耗时：500 成员为 27.46 s，1,000 成员为 61.78 s。

### 显式事务路径

| 显式事务路径 | 样本数（500 / 1000） | 500 BEGIN 最大 / ms | 500 持锁最大 / ms | 1000 BEGIN 最大 / ms | 1000 持锁最大 / ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| `CreateScanSession` | 2 / 2 | 0.066 | 2.002 | 0.125 | 2.394 |
| `EnqueueCovers` | 1000 / 2000 | 80.754 | 6.174 | 55.496 | 17.030 |
| `ImportMetadata` | 1000 / 2000 | 3231.123 | 18.250 | 2230.213 | 50.487 |
| `MergeSelectedStaging` | 2 / 2 | 0.244 | 152.882 | 0.113 | 387.885 |
| `NextQueuedExecution` | 9 / 9 | 0.052 | 2.363 | 0.173 | 3.846 |
| `NextQueuedGeneration` | 14 / 14 | 3.442 | 2.348 | 33.385 | 2.165 |
| `PersistOperationRevision` | 4 / 4 | 0.131 | 81.776 | 0.071 | 143.962 |
| `ReplaceCurrentWorkset` | 2 / 2 | 0.006 | 19.697 | 0.048 | 36.295 |
| `SaveExecutionComponentResult` | 2000 / 4000 | 8.524 | 16.060 | 19.317 | 16.973 |
| `SyncObservedInventory` | 2000 / 4000 | 0.210 | 22.716 | 0.226 | 18.776 |
| `WriteStagingEntries/temp` | 13 / 25 | 0.020 | 4.001 | 0.027 | 7.481 |
| `updateEntryBitratesOnce` | 500 / 1000 | 0.405 | 3.755 | 0.398 | 6.377 |

### 自动提交路径

| 自动提交路径（整句耗时，含锁等待） | 样本数（500 / 1000） | 500 最大 / ms | 1000 最大 / ms |
| --- | ---: | ---: | ---: |
| `ClaimCover` | 1002 / 2002 | 4.561 | 16.285 |
| `CreateExecutionGuarded` | 4 / 4 | 3.861 | 4.490 |
| `CreateGeneration` | 4 / 4 | 1.914 | 3.293 |
| `FinishCover` | 1000 / 2000 | 0.459 | 0.555 |
| `FinishExecution` | 4 / 4 | 1.230 | 2.965 |
| `SaveCoverDestination` | 1000 / 2000 | 5.322 | 16.444 |
| `UpdateExecutionPosition` | 2 / 2 | 2.253 | 2.350 |
| `UpdateGenerationProgress` | 2000 / 4000 | 25.458 | 27.238 |
| `UpdateScanSessionStatus` | 4 / 4 | 3.009 | 4.692 |

### HTTP 读取与生成端到端耗时

| 读取接口 | 500 首轮 / ms（字节） | 500 再生成后 / ms（字节） | 1000 首轮 / ms（字节） | 1000 再生成后 / ms（字节） |
| --- | ---: | ---: | ---: | ---: |
| `revision/conversion` | 397.662（13,668,859） | 176.437（2,911,347） | 736.754（27,095,751） | 324.610（5,784,740） |
| `revision/metadata_refresh` | 8.293（490,233） | 8.402（490,233） | 35.023（982,407） | 19.922（982,408） |
| `current`（conversion） | 195.685（122,026） | 115.705（122,025） | 562.177（242,905） | 236.821（242,904） |

| 生成端到端耗时 | 500 首轮 / ms | 500 再生成 / ms | 1000 首轮 / ms | 1000 再生成 / ms |
| --- | ---: | ---: | ---: | ---: |
| `conversion` | 1,399.486 | 1,371.918 | 2,936.994 | 3,012.520 |
| `metadata_refresh` | 517.586 | 487.313 | 1,023.727 | 1,036.996 |

第二轮生成的 `conversion` 计划响应体小于首轮，原因是第一轮执行已将模拟 MP3 输出发布至目录并同步库存，第二轮规划时这些输出已处于满足状态。

---

## 3. 分块暂存、退役替换与分批清理（1,000 成员分步测量）

将规划持久化拆分为 `beginStagingPlan`（写入所属生成的 `staging` 计划行）、`appendStagingPlan`（按表分批写入 `steps` / `roots` / `components`，每批最多 100 行）与 `promoteOperationRevision`（提升为 `ready`、写 revision、切换当前计划并将被替换计划标记为退役）。

### 3.1 仅启用计划分块暂存与旧计划退役（未启用记录替换退役与清理排空）

完整测试耗时 64.10 s。

| 显式事务路径 | 样本数 | BEGIN 最大 / ms | 持锁最大 / ms |
| --- | ---: | ---: | ---: |
| `beginStagingPlan` | 4 | 0.094 | 3.698 |
| `appendStagingPlan` | 84 | 0.074 | 20.727 |
| `promoteOperationRevision` | 4 | 0.045 | 3.385 |
| `ReplaceCurrentWorkset` | 2 | 0.078 | 58.393 |
| `MergeSelectedStaging` | 2 | 0.119 | 661.233 |

| 读取接口 | 首轮 / ms（字节） | 再生成后 / ms（字节） |
| --- | ---: | ---: |
| `revision/conversion` | 803.480（27,338,752） | 287.783（5,823,740） |
| `revision/metadata_refresh` | 17.486（985,409） | 18.274（985,409） |
| `current`（conversion） | 469.257（243,888） | 218.525（243,887） |

生成端到端耗时：`conversion` 首轮 3,491.335 ms / 再生成 2,900.669 ms；`metadata_refresh` 首轮 946.282 ms / 再生成 974.202 ms。

### 3.2 启用记录替换退役与分批清理排空（1,000 成员全路径明细）

将记录替换改为退役旧记录（`replaceCurrentWorkset`，处于封面发布互斥边界内），并在生命周期结束后显式循环调用 `DeleteRetiredPlanBatch(..., 1000)` 直至返回 0（未并行启动后台自动清理 worker，以保留两轮完整已执行计划供末尾统一清理）。清理共执行 35 个非空批次（删除 15,025 行）及 1 次空候选查询；完整测试耗时 72.80 s。

- **生成端到端耗时**：`conversion` 首轮 2,251.443 ms / 再生成 5,157.661 ms；`metadata_refresh` 首轮 1,425.296 ms / 再生成 1,124.199 ms。
- **HTTP 读取耗时与字节数**：
  - `revision/conversion`：首轮 679.038 ms（27,338,752 B）/ 再生成 372.335 ms（5,823,740 B）
  - `revision/metadata_refresh`：首轮 18.366 ms（985,408 B）/ 再生成 18.293 ms（985,408 B）
  - `current`（conversion）：首轮 448.120 ms（243,887 B）/ 再生成 281.851 ms（243,886 B）

| SQL 路径 | 样本数 | BEGIN 最大 / ms | 持锁最大 / ms | 自动提交整句最大 / ms |
| --- | ---: | ---: | ---: | ---: |
| `ClaimCover/autocommit` | 2002 | n/a | n/a | 17.113 |
| `CreateExecutionGuarded/autocommit` | 4 | n/a | n/a | 11.243 |
| `CreateGeneration/autocommit` | 4 | n/a | n/a | 1.977 |
| `CreateScanSession` | 2 | 0.060 | 2.940 | n/a |
| `DeleteRetiredPlanBatch` | 36 | 0.099 | 11.907 | n/a |
| `EnqueueCovers` | 2000 | 111.933 | 139.802 | n/a |
| `FinishCover/autocommit` | 2000 | n/a | n/a | 1.217 |
| `FinishExecution/autocommit` | 4 | n/a | n/a | 7.016 |
| `ImportMetadata` | 2000 | 2643.137 | 29.425 | n/a |
| `MergeSelectedStaging` | 2 | 0.100 | 623.852 | n/a |
| `NextQueuedExecution` | 9 | 53.561 | 4.028 | n/a |
| `NextQueuedGeneration` | 13 | 33.344 | 1.974 | n/a |
| `SaveCoverDestination/autocommit` | 2000 | n/a | n/a | 24.543 |
| `SaveExecutionComponentResult` | 4000 | 81.463 | 35.041 | n/a |
| `SyncObservedInventory` | 4000 | 0.910 | 19.054 | n/a |
| `UpdateExecutionPosition/autocommit` | 2 | n/a | n/a | 2.431 |
| `UpdateGenerationProgress/autocommit` | 4000 | n/a | n/a | 26.707 |
| `UpdateScanSessionStatus/autocommit` | 4 | n/a | n/a | 4.048 |
| `appendStagingPlan` | 84 | 0.357 | 15.943 | n/a |
| `beginStagingPlan` | 4 | 0.096 | 5.424 | n/a |
| `promoteOperationRevision` | 4 | 0.069 | 5.533 | n/a |
| `replaceCurrentWorkset` | 2 | 0.056 | 20.624 | n/a |
| `updateEntryBitratesOnce` | 1000 | 5.369 | 22.387 | n/a |
| `WriteStagingEntries/temp` | 25 | 0.491 | 13.589 | n/a |

---

## 4. 三档规模生命周期与读取测量（1,000 / 2,000 / 5,000 成员）

在 1,000 / 2,000 / 5,000 成员（12,000 / 24,000 / 60,000 音频）三档规模下依次执行完整生命周期测量。每档均完成两个操作各两轮生成与执行、两轮各 N 个封面投递，随后替换记录并调用 `DeleteRetiredPlanBatch(..., 1000)` 排空退役行（未并行运行自动清理 worker）。

### 生命周期写事务路径

| 写事务路径 | 样本数（1000 / 2000 / 5000） | BEGIN 最大 / ms（1000 / 2000 / 5000） | 持锁最大 / ms（1000 / 2000 / 5000） |
| --- | ---: | ---: | ---: |
| `beginStagingPlan` | 4 / 4 / 4 | 0.090 / 0.099 / 0.124 | 2.493 / 7.879 / 6.330 |
| `appendStagingPlan` | 84 / 164 / 404 | 0.078 / 0.226 / 0.130 | 21.249 / 38.615 / 71.541 |
| `promoteOperationRevision` | 4 / 4 / 4 | 0.096 / 0.055 / 0.052 | 4.595 / 13.474 / 52.352 |
| `replaceCurrentWorkset` | 2 / 2 / 2 | 0.026 / 0.049 / 0.048 | 17.752 / 30.535 / 101.365 |
| `DeleteRetiredPlanBatch` | 36 / 51 / 96 | 0.150 / 0.102 / 0.095 | 17.116 / 14.844 / 21.713 |

- **退役清理批次与行数**：三档分别为 35 / 50 / 95 个非空批次（表内样本数另含末次空候选事务），累计删除 15,025 / 30,025 / 75,025 行。
- **其他写路径观测**：选中范围库存合并 `MergeSelectedStaging` 持锁最大分别为 367.638 / 1,103.804 / 4,756.202 ms；`ImportMetadata` 的 `BEGIN` 等待最大分别为 2,230.622 / 3,643.953 / 4,454.661 ms。

### HTTP 读取耗时与响应体积

| 读取接口 | 1000 首轮 / ms（字节） | 1000 再生成 / ms（字节） | 2000 首轮 / ms（字节） | 2000 再生成 / ms（字节） | 5000 首轮 / ms（字节） | 5000 再生成 / ms（字节） |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `revision/conversion` | 635.198（27,338,752） | 333.977（5,823,740） | 1343.610（54,680,752） | 663.028（11,650,740） | 3185.611（136,706,754） | 1566.794（29,131,740） |
| `revision/metadata_refresh` | 22.046（985,409） | 17.170（985,409） | 34.268（1,976,409） | 36.627（1,976,409） | 76.986（4,949,409） | 83.690（4,949,409） |
| `current` | 425.907（243,887） | 278.612（243,887） | 855.735（486,888） | 538.450（486,886） | 2055.337（1,215,888） | 1244.446（1,215,887） |

### 测试总耗时

| 测试流程（含夹具构造与文件读写）/ s | 1000 | 2000 | 5000 |
| --- | ---: | ---: | ---: |
| `TestMeasureWorksetLifecycle` | 63.71 | 150.50 | 553.95 |
| `TestMeasureLibraryDeletion`（全成员） | 35.99 | 88.45 | 339.62 |

---

## 5. 媒体库删除退役与后续清理测量

`TestMeasureLibraryDeletion` 在完成一轮 `conversion` 与 `metadata_refresh` 生成、执行及全部封面投递后，清空前置阶段的 SQL 统计，仅统计 `DELETE /api/v1/libraries/{id}` 及随后的 `DeleteRetiredPlanBatch(..., 1000)` 排空过程。

事务内各阶段耗时由同一删除事务内 driver 的 `Exec` 调用耗时累加得出，不包含事务内 guard 查询、当前记录查询、Go 调度与 `COMMIT` 开销，不代表独立事务的持锁时间。

### 5.1 大库、小工作集（5,000 库目录 / 20 个工作集成员）

媒体库含 5,000 个作品目录、60,000 个音频文件，工作集选择其中 20 个目录。删除前数据库行数为：`inventory_dirs` 5,001 行、`inventory_files` 60,020 行（含 20 条已发布封面）、`work_copies` 5,000 行、`workset_members` 20 行、`cover_deliveries` 20 行。删除后立即校验：`libraries`、`inventory_dirs`、`inventory_files`、`work_copies` 行数为 0，退役记录保留且 `library_id` 为 `NULL`；随后显式调用分批清理，20 个非空批次共清除 175 行（另 1 次空候选查询）。两次独立运行数据如下：

| 指标 / 路径 | 样本数 | 初测 BEGIN 最大 / ms | 初测持锁或阶段耗时 / ms | 复测 BEGIN 最大 / ms | 复测持锁或阶段耗时 / ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| DELETE HTTP 响应 | 1 | — | 145.185 | — | 134.052 |
| `deleteLibrary` 完整事务 | 1 | 0.005 | 144.990 | 0.007 | 133.790 |
| 阶段：记录退役（4 条 UPDATE 合计） | 1 | — | 0.169 | — | 0.244 |
| 阶段：解除副本绑定 | 1 | — | 14.251 | — | 12.929 |
| 阶段：删除整库库存（含文件级联） | 1 | — | 112.970 | — | 101.312 |
| 阶段：删除媒体库行（含退役记录 `library_id` 置空） | 1 | — | 0.192 | — | 0.184 |
| `DeleteRetiredPlanBatch`（含末次空候选） | 21 | 0.062 | 3.220 | — | 2.708 |

初测与复测的完整测试总耗时分别为 5.34 s 与 3.43 s。

### 5.2 全成员工作集媒体库删除（1,000 / 2,000 / 5,000 成员全选）

库目录数与工作集选中成员数均为 N（1,000 / 2,000 / 5,000），完成两个操作各一次执行及 N 个封面投递后删除媒体库，随后排空退役数据：

| 指标 / 路径 | 1000 | 2000 | 5000 |
| --- | ---: | ---: | ---: |
| DELETE HTTP 响应 / ms | 41.344 | 79.158 | 187.260 |
| `deleteLibrary` BEGIN / ms | 0.007 | 0.007 | 0.008 |
| `deleteLibrary` 完整事务持锁 / ms | 41.146 | 78.933 | 186.970 |
| 阶段：记录退役 4 条 UPDATE 合计 / ms | 2.332 | 4.807 | 25.831 |
| 阶段：同步库存删除 SQL / ms | 24.256 | 52.201 | 113.403 |
| 完整持锁减库存删除 SQL / ms | 16.890 | 26.732 | 73.567 |
| 阶段：解除副本绑定 SQL / ms | 2.765 | 4.309 | 14.132 |
| 阶段：删除库行 SQL / ms | 0.713 | 0.956 | 1.758 |
| 后续清理 BEGIN 最大 / ms | 0.075 | 0.097 | 0.151 |
| 后续清理持锁最大 / ms | 8.035 | 11.368 | 20.091 |
| 后续清理事务数（含末次空候选） | 21 | 29 | 53 |
| 后续清理删除总行数 | 8,015 | 16,015 | 40,015 |

每档删除仅包含 1 个完整事务，“完整持锁减库存删除 SQL”为同一样本内其余事务时间的差值合计（包含 guard、记录退役、副本解绑、库行删除、调度与 `COMMIT`）。

---

## 6. 前端成员列表虚拟滚动测量

使用 `frontend/web/e2e/member-list-harness.js` 在 393×852 与 1440×852 两种视口宽度下测试 1,000 / 2,000 / 5,000 成员。字体就绪后，以 120 个 `requestAnimationFrame` 从列表顶部按全列表比例位置逐帧跳转至末行（`N - 1`），统计帧间隔；随后在搜索框输入末尾成员名称，测量直至唯一匹配行出现在 DOM 的耗时。

### 无后台测试重叠时的测量值

| 成员数 / 视口宽度 | 最大 DOM 行数 | frame p95 / ms | frame 最大 / ms | >50 ms 帧数 / 120 | 筛选至唯一结果 / ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1000 / 393 | 26 | 16.8 | 49.9 | 0 | 12.8 |
| 1000 / 1440 | 29 | 16.8 | 83.3 | 1 | 14.0 |
| 2000 / 393 | 27 | 16.8 | 33.4 | 0 | 4.4 |
| 2000 / 1440 | 29 | 16.8 | 33.4 | 0 | 7.7 |
| 5000 / 393 | 27 | 16.7 | 16.8 | 0 | 6.6 |
| 5000 / 1440 | 29 | 33.3 | 33.4 | 0 | 13.6 |

### 与后台负载并发时的观测值

在与 5,000 成员删除夹具准备阶段并发执行的一轮测量中：
- 2,000 成员在 393 / 1440 宽度下的 frame p95 分别为 33.3 / 33.2 ms，frame 最大均为 50.0 ms；
- 1,000 成员在 1440 宽度下出现 1 次 66.7 ms 帧间隔；
- 5,000 成员在 1440 宽度下出现 3 次 >50 ms 帧间隔（最大 166.7 ms）。

### 测量边界

- 脚本通过逐帧设置比例位置跨窗口跳转，不等同于真实滚轮或触控输入，也不包含全量 revision 详情响应的解析与渲染。
- 每组仅采集 120 个帧间隔样本，仅报告 p95 与单次最大值，不用于推导 p99。

---

## 7. 按成员读取改造后的 5,000 × 12 复测

本节在 01–05 的契约与读取路径改造后采集；生产成员上限保持 2,000，5,000 成员仅通过本节原有夹具的仓储写入绕过准入。媒体、SQLite 和依赖模拟边界与第 1 节一致。运行环境仍为 WSL2 上的磁盘运行（非 tmpfs），Go 1.27.3，Chromium 156.0.8078.4。各组运行中每个接口/视口/连接只记一个正式样本（暖页请求另行执行），不是 p99，也不用于声称稳定低于阈值。

### 复现与新增口径

在 `backend/go` 执行（需已安装前端依赖及 Playwright Chromium）：

```sh
GOCACHE=/tmp/preen-go-cache PREEN_MEASURE_WORKSET=1 \
  PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 PREEN_MEASURE_BROWSER=1 \
  PREEN_MEASURE_SNAPSHOT_LIST=1 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m
```

- `PREEN_MEASURE_BROWSER=1` 由 Go 夹具启动真实 loopback HTTP 服务，调用 `frontend/web/e2e/measure-workset-reads.mjs`，后者启动 Vite 的同源 API 代理并加载生产转换页面、`MemberList` 与 `MemberReview`。没有人工 revision、成员结论或组件响应；媒体网络/编码/探测仍按第 1 节模拟。
- 浏览器为 Vite 开发构建，关闭 Vue DevTools；先加载页面和字体，再 reload 清空该文档的 Vue Query 缓存计时，保留浏览器代码/字体缓存与 OS 页缓存。两种视口为 393×852、1440×852；页面请求、解析和真实组件渲染均计入。不是生产压缩 bundle 或冷代码下载成绩。
- 轻量详情请求到渲染：从该 revision 的 `fetch` 开始到首屏出现服务端结论的 DOM；整页导航另从 reload 后的文档时间原点计时，保留 current、operation、workset 等前置请求。浏览器将原生 `Response.json()` 等价拆为真实响应 `text()` + `JSON.parse()` 计时；字节数按 UTF-8 未压缩 body。
- 成员审阅：120 次 rAF 比例滚动后返回顶部，从打开 SQL 选出的组件最多成员到真实 `member-review` 可见；包括浏览器点击/路由、请求、解析和渲染。脚本断言仅新增这一个成员审阅请求，不请求、解析其他成员的组件或文件决定。本负载 5,000 个成员均为 1 个组件，按成员顺序选第一个，审阅渲染 24 个文件决定。
- JS 堆为 Chromium `performance.memory.usedJSHeapSize`（启用精确堆信息），分别在首屏和审阅完成读取；不强制 GC，不代表 retained heap 或峰值。保留第 6 节的 120 次 rAF 比例跳转测量，不等同于滚轮/触摸输入。
- 浏览器采样时没有规划、扫描、执行或封面 worker 在处理任务；dispatcher 保持运行并空闲轮询。列表样本在首轮生成完成、执行开始前；快照样本在首轮转换执行成功后、元数据生成前。没有另一组后台测试与其重叠。夹具未装配媒体事实 SSE，生产页面的该独立订阅收到 503 并退避重试；已按真实页面保留，重试响应仅 65 B。
- HTTP handler 的独立读取仍用 recorder，包含查询、领域视图和 JSON 编码，不含传输。浏览器服务包装器在第一次真实 socket Write 前加入 `Server-Timing`，SSE 仍通过生产 handler 原样逐事件写入、flush；没有缓冲整份响应后再回放。

### 服务端读取

本次完整生命周期尝试运行 618.28 s 后在第二轮 `metadata_refresh` 失败：`COMPONENT_FAILED`，底层 `database is locked (5) (SQLITE_BUSY)`。第一轮转换、元数据与 5,000 封面投递、第二轮转换均成功；第二轮元数据、封面、最终记录替换及退役清理没有完成。不能将本轮记作完整生命周期通过。

`ImportMetadata` 的 7,974 次事务 BEGIN 尝试中有 1 次失败，最大等待 5052.410 ms；同轮 `MergeSelectedStaging` 持锁最大 4092.523 ms。SQL 路径还观测到 `EnqueueCovers` 持锁最大 526.150 ms、`SaveExecutionComponentResult` 最大 620.510 ms；这些是当次各路径最大值，不说明失败等待期间的唯一持锁者。原有第 2–5 节历史运行无 SQL 尝试失败，本次新增样本有此 1 次失败。失败在下表各目标读取采样之后，已取得的读取数据仍按原样保留；提高上限须先解决写竞争。

| 接口 | 首轮 / ms | 首轮 / B | 再生成后 / ms | 再生成后 / B |
| --- | ---: | ---: | ---: | ---: |
| 轻量 `revision/conversion` | 1866.139 | 1,795,039 | 1271.996 | 1,810,025 |
| 组件最多成员的 Plan Review | 19.472 | 28,211 | 22.445 | 6,692 |
| 执行详情首页（`components_from=0&components_limit=200`） | 28.297 | 665,884 | 14.752 | 428,274 |

### 浏览器列表、审阅与堆

| 视口宽度 | revision 请求到结论渲染 / ms | revision handler / ms | JSON.parse / ms | 整页导航 / ms | 打开成员审阅 / ms | 首屏 JS 堆 / B | 审阅后 JS 堆 / B |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 393 | 2124.8 | 2021.539 | 2.5 | 6133.4 | 132.1 | 89,135,370 | 149,207,675 |
| 1440 | 1957.9 | 1849.586 | 2.8 | 5744.6 | 138.5 | 78,086,139 | 71,885,224 |

两次 revision 均为 1,795,039 B；成员审阅均仅 28,211 B，JSON.parse 为 0.1 / 0.2 ms。393 宽度的列表超过 2 s，两个视口成员审阅均低于 500 ms，不能把本轮记为浏览器列表全面达标。整页导航的前置 current handler 为 1596.403 / 1504.956 ms，随后并行的 operation 与 workset handler 分别约 2175 / 2181 ms、1999 / 1994 ms，之后才开始轻量 revision；因此即使只看轻量请求接近 2 s，完整入口仍需 5.7–6.1 s。

| 视口宽度 | 首屏 DOM 行数 | 滚动最大 DOM 行数 | frame p95 / ms | frame 最大 / ms | >50 ms 帧数 / 120 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 393 | 16 | 25 | 27.9 | 36.8 | 0 |
| 1440 | 18 | 27 | 28.6 | 36.9 | 0 |

### 执行快照首次连接与重连

首轮计划为 12 个 encode + 12 个 delete / 成员，`kept_decisions` 为空数组。下表先保留该轮数据，不能用它代替非空保留决定的体积；随后再单独测真实再生成轮。

| 视口 / 连接 | 服务生成与编码 / ms | 传输与 UTF-8 收取 / ms | 帧准备 / ms | JSON.parse / ms | 合计 / ms | JS 堆 / B |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 393 / 首次 | 721.550 | 342.750 | 19.0 | 21.5 | 1104.8 | 202,196,160 |
| 393 / 重连 | 560.969 | 91.431 | 6.7 | 5.9 | 665.0 | 185,447,288 |
| 1440 / 首次 | 670.476 | 181.124 | 6.7 | 22.9 | 881.2 | 156,856,026 |
| 1440 / 重连 | 552.866 | 72.334 | 11.4 | 5.8 | 642.4 | 185,201,313 |

每次快照均为 5,000 组件，JSON payload 16,639,484 B，SSE 帧 16,639,518 B；本轮 reader 连同随后 terminal 事件共收到 16,639,641 B。服务生成/编码取第一次 socket Write 前的 `Server-Timing`；传输与 UTF-8 收取为浏览器取完整首帧耗时减服务计时，含 loopback、Vite 代理、socket 写入、浏览器调度与增量解码；帧准备为合并文本和提取首事件数据。该传输数不是广域网带宽基准。

首轮快照在转换列表文档中主动建立两条独立真实 SSE 连接；第一条收完后关闭 reader，再无 replay id 重连，服务仍发送完整 snapshot。页面自身的 revision/execution 读取可能仍在完成，属于该页面的 HTTP 开销，未与任务执行 worker 并发。四次合计均低于 2 s。补测的非空保留决定将改从生产 execution carrier 加载并记录入口时间；两种页面边界分别记录，不合并挑选成绩。

非空保留样本通过同一个入口单独复现：在第 7 节命令去掉 `PREEN_MEASURE_SNAPSHOT_LIST=1`，增加 `PREEN_MEASURE_KEPT_SNAPSHOT=1`。该模式完成第一次真实转换执行，立即按真实 MP3 库存再生成并执行，随后从生产 `/conversion/execution` 页面采样，不运行元数据/封面任务。此模式运行 292.17 s 并通过；未人为写入 keep 结论。

| 视口 / 连接 | 服务生成与编码 / ms | 传输与 UTF-8 收取 / ms | 帧准备 / ms | JSON.parse / ms | 合计 / ms | JS 堆 / B |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 393 / 首次 | 206.854 | 53.046 | 4.7 | 37.1 | 301.7 | 600,547,197 |
| 393 / 重连 | 214.120 | 30.980 | 5.2 | 6.4 | 256.7 | 676,080,250 |
| 1440 / 首次 | 254.291 | 40.109 | 6.5 | 9.4 | 310.3 | 624,911,223 |
| 1440 / 重连 | 213.264 | 57.036 | 9.5 | 6.3 | 286.1 | 653,269,724 |

四个响应均有 5,000 组件、60,000 个真实保留决定（每组件 12 个），JSON payload 10,699,474 B、SSE 首帧 10,699,508 B，reader 连同 terminal 事件收到 10,699,631 B。该轮执行详情 200 组件首页为 15.235 ms / 428,274 B；首轮转换准备阶段另记录 revision 1924.982 ms / 1,795,039 B、单成员 32.348 ms / 28,211 B、执行详情首页 56.421 ms / 665,884 B，不用它们替换首个完整生命周期尝试的样本。

所有非空快照合计低于 2 s，因此本次没有把“快照生成 + 传输 + 解析”列为超阈值项。计时到 JSON.parse 完成，不包含把 snapshot 合并进 Vue/Pinia 后再次渲染；使用成功执行后的真实会话建立首次/重连连接，未测执行 worker 正在提交时的竞争，也未模拟远程网络。

### 生产执行详情页面的另一项阻碍

非空样本的浏览器先加载执行详情页面和字体，再 reload，以 fresh Vue Query cache 测量页面入口；随后才开始上表的 snapshot-only 请求。两种计时不能相加后声称是同一个生产请求，也不能用 snapshot-only 成绩代表完整执行页入口。

| 视口宽度 | reload 至执行 panel 可见 / ms | 全量 execution JSON / B | execution handler / ms | JSON.parse / ms | 该 JSON 解析完至 panel 可见采样间隔 / ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 393 | 16053.8 | 10,699,474 | 240.701 | 8.3 | 12594.9 |
| 1440 | 17032.6 | 10,699,474 | 214.964 | 21.6 | 12786.9 |

真实页面确实没有读取 revision；但网络记录显示它调用无 `components_from`/`components_limit` 的 execution GET，而非上面单独测的 200 条首页，随后把全部组件交给 `ExecutionPanel`。上述解析后间隔包含组件挂载、布局、GC、浏览器/采样调度等，未进一步归因为某个函数；不是纯 Vue 渲染基准。约 600–676 MB 的 snapshot 读取时 JS 堆也包括这个已挂载页面和临时解析对象。此页面入口远超 2 s，需另行处理完整 execution 读取与大量组件渲染，不在本票扩大实现范围。

### 未达目标后的诊断拆分

仅因上述列表超阈值另跑诊断；不以诊断样本替换原始成绩。复现时在同一命令去掉 `PREEN_MEASURE_BROWSER=1`，改设 `PREEN_MEASURE_READ_BREAKDOWN=1`：只构造同规模夹具、完成首轮生成，停止 dispatcher 后重读一次生产 revision handler，不执行后续生命周期。

查询时间为 driver `QueryContext` + 各次 `Rows.Next` + `Rows.Close` 的耗时总和，不含连接池排队；视图为 `GetRevision` 总时间减查询，含行 Scan、JSON 负载解码、活根校验计算、成员结论组装及连接池排队等剩余服务耗时；适配/编码为 handler 总时间减 service 时间，含 wire DTO 构造与未压缩 JSON 编码/recorder 写入。诊断计时本身有开销。

| 诊断样本 | 查询 / ms | 视图 / ms | 适配与编码 / ms | 合计 / ms | 响应 / B |
| --- | ---: | ---: | ---: | ---: | ---: |
| 5,000 × 12，停止 dispatcher 后的重读 | 760.206 | 1432.783 | 6.961 | 2199.950 | 1,795,039 |

同一个独立诊断运行在停止 dispatcher 前的普通 revision handler 读取为 2376.014 ms，成员审阅 32.439 ms / 28,211 B；不能仅保留前一运行的 1866.139 ms 来宣称稳定通过。约 65% 为视图、35% 为 driver 查询，适配与编码不足 1%。这与当前服务仍读取全量组件、做全根活校验、转换 review 及成员结论仍解码冻结负载的路径一致；这里没有进一步把视图拆成各函数耗时，也没有据此改生产实现。

---

## 8. 三任务当前计划下的 5,000 × 12 基线（读取成本改造前）

本节是读取成本改造的对照基线：测量夹具补装 `rename` 任务，生命周期内每轮在转换与元数据刷新之后为重命名生成当前计划（只规划不执行），三个 operation 都有当前计划后再采样 `current`、`operation`、`workset` 与 revision。生产代码未改动；成员上限仍为 2,000，5,000 成员仅经夹具的仓储写入绕过准入。

### 复现与环境

在 `backend/go` 执行（不开浏览器；未设 `PREEN_MEASURE_BROWSER`）：

```sh
PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m
```

- 运行环境：x86_64 原生 Linux（非 WSL2），Go 1.27.3，GOMAXPROCS 取默认值；数据库与媒体文件位于 tmpfs（内存文件系统）挂载的临时目录，因此 `fsync` 与磁盘读写开销不同于第 4、7 节的磁盘运行，不能与其绝对值直接对比，后续各票须在同一环境重测后再比较。
- 口径：每个接口每轮只请求一次，每档只有这一个样本，不是 p99；其余口径同第 1 节。本次运行无 SQL 尝试失败，测试总耗时 416.31 s。
- 第 1 轮生成重命名计划用时 28,770.112 ms。第 2 轮的重命名输入与第 1 轮未变，生成请求回放第 1 轮的未执行计划（`Created=false`），因此第 2 轮读取的是同一份计划，没有新的生成耗时。
- “首轮”为第 1 轮三个 operation 均有计划后的采样，“再生成后”为第 2 轮（转换与元数据已重新生成并执行，重命名计划为回放）。

### HTTP 读取（三个 operation 均有当前计划）

| 接口 | 首轮 / ms | 首轮 / B | 再生成后 / ms | 再生成后 / B |
| --- | ---: | ---: | ---: | ---: |
| `current`（`/libraries/{id}/operations/conversion/current`） | 10,471.985 | 1,216,532 | 9,067.651 | 1,216,533 |
| `workset`（`/worksets/{id}`） | 10,360.896 | 1,216,520 | 8,948.045 | 1,216,521 |
| `operation`（`/worksets/{id}/operations/conversion`） | 1,502.707 | 818 | 859.107 | 817 |
| `revision/conversion`（轻量，三任务计划后） | 1,862.279 | 1,795,201 | 1,016.572 | 1,810,187 |
| `revision/rename` | 9,079.185 | 1,573,582 | 8,198.746 | 1,573,582 |

同一轮内更早、仅转换与（或）元数据有计划时的采样：`revision/conversion` 1,682.000 ms（1,795,039 B）/ 1,039.332 ms（1,810,025 B），`revision/metadata_refresh` 64.892 ms（1,863,802 B）/ 52.388 ms（1,863,804 B），组件最多成员的 `revision/conversion/member` 28.330 ms（28,211 B）/ 17.533 ms（6,692 B）。与第 4 节（无重命名计划）的 `current` 2,055.337 / 1,244.446 ms 相比，补入重命名计划后 `current` 约增至 5–7 倍；`workset` 与 `current` 响应体积近似（约 1.2 MB），`operation` 仅 818 B 但耗时仍达 0.86–1.5 s。

### 写事务持锁与退役清理

| 写事务路径 | 样本数 | BEGIN 最大 / ms | 持锁最大 / ms |
| --- | ---: | ---: | ---: |
| `beginStagingPlan` | 5 | 0.049 | 2.052 |
| `appendStagingPlan` | 505 | 0.062 | 12.583 |
| `promoteOperationRevision` | 5 | 0.027 | 9.984 |
| `replaceCurrentWorkset` | 2 | 0.020 | 63.759 |
| `DeleteRetiredPlanBatch` | 109 | 0.012 | 4.563 |

- **退役清理**：108 个非空批次（样本数另含末次空候选事务），累计删除 85,031 行。
- 各生成端到端耗时（ms）：转换 7,434.442 / 9,204.601，元数据刷新 486.753 / 496.960，重命名 28,770.112 / 回放。

### 批量输入校验的查询计划

转换的读取时校验用一条查询（`ObservedAudioEntriesByRoot`，成员根以 JSON 数组经 `json_each` 联结）；`EXPLAIN QUERY PLAN` 显示三段（根自身、子树区间、单文件根）都按 `inventory_dirs` 的路径唯一索引搜索，没有扫描 `inventory_dirs` 或 `inventory_files`：

```text
SCAN json_each VIRTUAL TABLE INDEX 1:
SEARCH d USING COVERING INDEX sqlite_autoindex_inventory_dirs_1 (path=?)
SEARCH f USING PRIMARY KEY (dir_ref=?)
SEARCH g USING INDEX sqlite_autoindex_generation_records_1 (path=?) LEFT-JOIN
USE TEMP B-TREE FOR ORDER BY
-- 子树段：SEARCH d USING COVERING INDEX sqlite_autoindex_inventory_dirs_1 (path>? AND path<?)
-- 单文件根段：SEARCH f USING PRIMARY KEY (dir_ref=? AND name=?)
```

`TestObservedAudioByRootSearchesThePathIndexPerRoot` 固定该计划。

---

## 9. 读取成本改造后的 5,000 × 12 终测

本节在第 8 节基线之后的改造（落库 counts 与成员结论、转换批量输入校验、摘要读取只加载计划框架、重命名父目录一次索引）全部落地后，用同一夹具、同一机器重测，与第 8 节逐项对照。成员上限仍为 2,000，5,000 成员仅经夹具的仓储写入绕过准入。

### 复现与环境

服务端（与第 8 节同一命令，在 `backend/go` 执行）：

```sh
PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m
```

浏览器（第 7 节命令，去掉 `PREEN_MEASURE_SNAPSHOT_LIST`）：

```sh
PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 PREEN_MEASURE_BROWSER=1 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m
```

- 运行环境同第 8 节：x86_64 原生 Linux，Go 1.27.3，数据库与媒体位于 tmpfs。浏览器为 `@playwright/test` 1.64.1 匹配的 Playwright Chromium headless shell 156.0.8078.4。
- 口径：每个接口每轮一个样本，不是 p99，不用于声称稳定低于阈值。服务端运行一次（316.82 s，无 SQL 尝试失败）；浏览器运行另跑一次，其 HTTP 读取同样记录在下面“第二样本”列，可见同一接口样本间约 ±5% 的波动。
- 浏览器计时方式同第 7 节：Vite 开发构建，先加载页面再 reload 以清空 Vue Query 缓存；两个视口 393×852、1440×852；整页导航为 reload 后文档时间原点至转换列表显示服务端成员结论（含 health / libraries / current / draft / operation / workset 全部前置请求与渲染）。浏览器采样时只有转换一个 operation 的计划之外，元数据与重命名计划尚未生成，但 `current` 与 `workset` 响应已按三个 operation 的形状返回（约 1.2 MB，随成员数而非计划数增长）；此处的 `current` 耗时因此低于下表三计划后的 `current`，不能互相替代。

### HTTP 读取对照（三个 operation 均有当前计划）

| 接口 | 第 8 节 首轮 / ms | 本节 首轮 / ms | 第二样本 首轮 / ms | 第 8 节 再生成后 / ms | 本节 再生成后 / ms | 第二样本 再生成后 / ms | 响应 / B（本节首轮） |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `current` | 10,471.985 | 556.560 | 522.541 | 9,067.651 | 545.753 | 540.816 | 1,211,533 |
| `workset` | 10,360.896 | 546.542 | 535.503 | 8,948.045 | 549.168 | 535.485 | 1,211,521 |
| `operation` | 1,502.707 | 404.901 | 367.077 | 859.107 | 396.158 | 380.428 | 818 |
| `revision/conversion`（三任务计划后） | 1,862.279 | 435.139 | 407.662 | 1,016.572 | 414.768 | 412.898 | 1,790,201 |
| `revision/rename` | 9,079.185 | 138.200 | 124.482 | 8,198.746 | 126.136 | 128.171 | 1,573,581 |

同一轮其余读取：`revision/conversion`（仅转换有计划）293.834 / 419.514 ms，`revision/metadata_refresh` 55.443 / 48.670 ms，成员审阅 16.867 / 25.153 ms（响应 27,969 / 6,654 B）。

结论：`operation` 与轻量 revision 达到 ≤ 500 ms；`revision/rename` 由 8–9 s 降到约 130 ms；`current` 与 `workset` 由 9–10 s 降到约 520–560 ms，**仍高于 500 ms 约 5–12%**（见下“诊断”）。

### 浏览器整页导航（reload 至转换列表显示成员结论）

| 视口宽度 | 整页导航 / ms | 第 7 节 / ms | revision 请求到结论渲染 / ms | current handler / ms | operation / workset handler / ms | revision handler / ms | 打开成员审阅 / ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 393 | 1295.4 | 6133.4 | 370.4 | 299.4 | 410.7 / 418.8 | 298.7 | 101.6 |
| 1440 | 1226.2 | 5744.6 | 401.3 | 313.7 | 376.4 / 389.4 | 306.8 | 159.2 |

两个视口均 ≤ 2 s，因此**不改前端前置链路**（current 之后仍并行 operation / workset，再串行 revision）。上表是转换单计划状态下的值；三个 operation 都有计划时的实测见下一小节。

#### 三个 operation 均有当前计划时的整页导航（实测）

夹具在第 0 轮重命名计划生成之后，对同一转换 revision 以同一指标再采样一次（reload 至转换列表显示成员结论；转换计划此时已执行，元数据刷新已执行并交付，重命名只生成计划）。同一次浏览器运行中，单计划样本为 393：1280.9 ms、1440：1239.2 ms（与上表同口径，仅为同命令另一次样本，二者差异在运行间波动内）。

| 视口宽度 | 整页导航 / ms | revision 请求到结论渲染 / ms | current handler / ms | operation / workset handler / ms | revision handler / ms | 打开成员审阅 / ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 393 | 1779.4 | 565.8 | 554.9 | 517.6 / 685.6 | 436.5 | 84.7 |
| 1440 | 1901.2 | 654.0 | 614.5 | 493.2 / 652.9 | 435.1 | 139.5 |

两个视口均仍 ≤ 2 s（1440 为 1901.2 ms，余量仅约 100 ms），单样本，不是 p99。较单计划多出约 500–660 ms，来自 `current` / `workset` / `revision` 三者三计划后的读取成本；`workset` handler 在浏览器并发请求下达到 650–690 ms，高于上表串行采样的约 550 ms。

### 写事务持锁与退役清理

| 写事务路径 | 第 8 节 样本数 | 第 8 节 持锁最大 / ms | 本节 样本数 | 本节 持锁最大 / ms |
| --- | ---: | ---: | ---: | ---: |
| `beginStagingPlan` | 5 | 2.052 | 5 | 1.275 |
| `appendStagingPlan` | 505 | 12.583 | 755 | 6.014 |
| `promoteOperationRevision` | 5 | 9.984 | 5 | 9.516 |
| `replaceCurrentWorkset` | 2 | 63.759 | 2 | 72.127 |
| `DeleteRetiredPlanBatch` | 109 | 4.563 | 134 | 6.549 |

- `appendStagingPlan` 的样本数增加 250（每个计划多出结论序列的暂存块），单次持锁最大值未上升；`promoteOperationRevision` 仍只改元数据，持锁无变化。
- 退役清理：133 个非空批次，累计删除 110,031 行（第 8 节 108 个批次、85,031 行）。多出的 25,000 行是 5 份计划 × 5,000 条成员结论行，由显式子表列表分批排空，每批仍受限额约束。
- 本次运行无 SQL 尝试失败。

### 诊断：`current` / `workset` 略高于 500 ms

三个 operation 的读取时代价各自独立相加：`current` ≈ 转换 `operation`（370–405 ms）+ 元数据刷新（约 50–60 ms）+ 重命名（约 125–140 ms 中的摘要部分）的量级，与实测 520–560 ms 吻合；`operation` 的响应仅 818 B，不存在视图/编码代价。转换这一项占大头，用 `PREEN_MEASURE_READ_BREAKDOWN=1`（第 7 节同一诊断入口）对转换 revision 拆分：

| 诊断样本 | 查询 / ms | 视图 / ms | 适配与编码 / ms | 合计 / ms |
| --- | ---: | ---: | ---: | ---: |
| 5,000 × 12，停止 dispatcher 后的 `revision/conversion` | 231.777 | 76.365 | 4.805 | 312.947 |

时间主要在 SQL 查询（成员根下 60,000 个音频条目的批量读取与 5,000 条结论读取），而非 Go 视图计算；批量校验查询已按路径索引搜索（第 8 节查询计划）。进一步降低须评估库存变更计数短路等方案，属于新的优化，**本票不做**，记入 spec 未达项。CPU profile 在该短运行中被生成与暂存合并主导，没有可靠地拆出读取路径，故未引用。

### 9.x 删除工作集视图 `operations[]` 后的复测

按[摘要读取的输入校验代价](workset-summary-validation-spec.md)，工作集视图不再嵌入 operation 视图，`current` / `workset` 不再做读时输入校验。同一命令（本节“复现与环境”的服务端命令）、同一夹具与机器重测一次，耗时 294.03 s，无失败。

| 接口 | 首轮 / ms | 再生成后 / ms | 响应 / B（首轮） | 本节上文首轮 / ms |
| --- | ---: | ---: | ---: | ---: |
| `current` | 17.600 | 17.333 | 1,214,236 | 522–557 |
| `workset` | 14.935 | 14.895 | 1,214,224 | 535–549 |
| `operation`（对照） | 387.738 | 380.574 | 818 | 367–405 |
| `revision/conversion`（三任务计划后） | 401.227 | 400.494 | 1,795,201 | 408–435 |

- `current` 与 `workset` 降到约 15–18 ms，满足 ≤ 500 ms（首轮与再生成后都成立）；其响应体积仍约 1.2 MB，随成员数增长而与计划数无关，主要代价已不在服务端。`operation` 与 revision 不变，仍各自校验一次转换。
- 每个接口一个样本，不是 p99。
- 浏览器三计划整页导航（393 / 1440）**未复测**：本环境 Playwright 1.64.1 要求的 Chromium 修订版未安装（缓存里只有 chromium-1248），未下载，故沿用上文的浏览器数据，不作验收门槛。

---

## 10. 选中范围库存合并分批提交后的 5,000 × 12 复测

`MergeSelectedStaging` 由一个事务改为按成本预算分批提交：每个文件夹的成本为暂存行数 + 旧库存行数 + 1，每批不超过 2,000，文件夹不拆分。同一夹具、同一机器，不启用浏览器（Chromium 修订版仍未安装）。测量驱动新增两项：各路径累计持锁时长；每次超过 1,000 ms 的 `BEGIN` 等待，列出等待窗口内各路径重叠持锁的合计时长（含自动提交写入）。

```sh
PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=45m
```

完整生命周期完成，耗时 286.92 s；无 SQL 尝试失败，无 `SQLITE_BUSY`。

### 写事务持锁

| 写事务路径 | 样本数 | BEGIN 最大 / ms | 持锁最大 / ms | 累计持锁 / ms |
| --- | ---: | ---: | ---: | ---: |
| 选中范围合并批次（`mergeScopeBatch`） | 137 | 0.022 | 62.840 | 7,770.034 |
| `ImportMetadata` | 10,000 | 2,731.862 | 33.451 | 41,931.828 |
| `SaveExecutionComponentResult` | 20,000 | 35.018 | 34.172 | 91,107.203 |
| `EnqueueCovers` | 10,000 | 80.630 | 18.488 | 22,417.210 |
| `SyncObservedInventory` | 20,000 | 1.053 | 13.852 | 14,310.579 |
| `replaceCurrentWorkset` | 2 | 0.029 | 64.395 | 104.195 |
| `promoteOperationRevision` | 5 | 0.005 | 7.884 | 15.515 |
| `appendStagingPlan` | 755 | 0.017 | 4.678 | 895.096 |
| `DeleteRetiredPlanBatch` | 134 | 0.014 | 5.339 | 283.816 |

- 合并：5,000 个范围分为 137 批，单批持锁最大 62.840 ms（改造前单事务 4,092.523 / 4,756.202 ms，第 7 / 4 节）。137 批是生命周期中两次合并之和，累计持锁 7.77 s 也是两次之和，不能与改造前的单次最大持锁直接对照；逐次口径见第 11 节。
- 所有写事务单次持锁 ≤ 1,000 ms，最大为 `replaceCurrentWorkset` 64.395 ms。
- 采样路径名取第一个调用 `BeginTx` 的 sqlite 包帧，合并因此记为 `mergeScopeBatch`。

### `ImportMetadata` 等待归因

本节的重叠持锁含自动提交语句的整句耗时（其中含抢锁等待），区间由记录时刻倒推；第 11 节改为在操作边界取时间戳，并把自动提交语句单列为语句窗口，不作持锁证据。

`ImportMetadata` 的 `BEGIN` 等待最大 2,731.862 ms（第 4 节 4,454.661 ms，第 7 节 5,052.410 ms 并失败），仍高于 1,000 ms。10,000 次中有 40 次超过 1,000 ms（1,029.406–2,731.862 ms），不到 1%，p99 低于 1,000 ms。

| 等待 / ms | `SaveExecutionComponentResult` 重叠 / ms | `EnqueueCovers` 重叠 / ms | `ImportMetadata` 重叠 / ms |
| ---: | ---: | ---: | ---: |
| 2,731.862 | 1,496.349 | 712.615 | 45.249 |
| 1,839.206 | 1,004.425 | 504.676 | 31.160 |

- 40 个等待窗口中，合并批次的重叠持锁均为 0。
- 窗口内其他路径单次持锁都不超过 35 ms，但累计占据窗口的大部分：等待者多次没有抢到写锁，而不是在等某一个长事务。本节只记录重叠，不据此认定 SQLite 忙等待退避或调度的具体机制。

---

## 11. 结果保存按差额更新计数（5,000 × 12）

第 10 节之后：选中范围合并预算改回常量；测量驱动在操作边界取时间戳，自动提交语句单列为语句窗口（`sql_wait_stmt`），不计入持锁归因；新增事务内逐语句与提交耗时、每次合并的墙钟 / 批次数 / 累计持锁。先以此跑基线，再把 `SaveExecutionComponentResult` 的计数从每次全量 `COUNT` / `SUM` 改为同一事务内读旧行、按新旧差额更新，复测。命令同第 10 节，各一个样本，两次运行的机器负载不同。

两次均完成完整生命周期，无 SQL 尝试失败，无 `SQLITE_BUSY`；耗时 288.29 s（基线）/ 245.01 s（改后），差值不只来自本项改动。

### `SaveExecutionComponentResult`（20,000 次）

| 项 | 基线 | 改后 |
| --- | ---: | ---: |
| BEGIN 最大 / ms | 55.149 | 35.296 |
| 持锁最大 / ms | 39.170 | 20.538 |
| 累计持锁 / ms | 90,208.167 | 34,575.534 |
| 结果 upsert 累计 / 最大 ms | 847.669 / 2.304 | 796.749 / 1.959 |
| 读旧行 累计 / 最大 ms | — | 553.236 / 2.019 |
| 计数 `UPDATE` 累计 / 最大 ms | 83,277.468 / 38.970 | 26,914.718 / 12.015 |
| `COMMIT` 累计 / 最大 ms | 5,590.828 / 6.719 | 5,719.275 / 9.014 |

- 基线中计数 `UPDATE` 占该路径累计持锁约 92%：每次保存都重扫该执行的全部结果行，一次执行内累计工作量随组件数平方增长。
- 改后计数 `UPDATE` 不再扫描，仍累计 26.9 s（平均约 1.3 ms）。`plan_executions` 行在计数列之前存有冻结请求 `request_json`，推测每次更新都重写整行；未验证。

### `ImportMetadata` 等待

| 项 | 基线 | 改后 |
| --- | ---: | ---: |
| BEGIN 最大 / ms | 3,431.701 | 1,636.653 |
| 超过 1,000 ms 的次数（共 10,000） | 24 | 9 |

| 运行 | 等待 / ms | 窗口内重叠持锁 |
| --- | ---: | --- |
| 基线 | 3,431.7 | `SaveExecutionComponentResult` 1,873；`EnqueueCovers` 979；`ImportMetadata` 55 |
| 基线 | 2,732.1 | `ImportMetadata` 1,217；`SaveExecutionComponentResult` 500；`EnqueueCovers` 358 |
| 基线 | 2,138.8 | `SaveExecutionComponentResult` 1,167；`EnqueueCovers` 578；`ImportMetadata` 36 |
| 改后 | 1,636.7 | `ImportMetadata` 665；`SaveExecutionComponentResult` 397；`EnqueueCovers` 183 |
| 改后 | 1,529.9 | `ImportMetadata` 827；`EnqueueCovers` 212；`SaveExecutionComponentResult` 122 |
| 改后 | 1,330.8 | `ImportMetadata` 660；`EnqueueCovers` 156；`SaveExecutionComponentResult` 118 |

- 超过 1,000 ms 的等待全部是 `ImportMetadata`；两次运行都没有自动提交语句窗口与这些等待重叠，合并批次的重叠均为 0。
- 改后最长等待中，主要重叠持锁来自其他 `ImportMetadata` 事务本身（单次最大 33 ms），`ImportMetadata` 等待仍高于 1,000 ms。

### 每次合并（同口径对照）

| 合并 | 基线 墙钟 / 批次 / 累计持锁 ms | 改后 墙钟 / 批次 / 累计持锁 ms |
| --- | ---: | ---: |
| 第 1 次 | 5,057.420 / 66 / 3,865.397 | 5,190.527 / 66 / 3,912.054 |
| 第 2 次 | 4,628.035 / 71 / 3,860.991 | 4,691.295 / 71 / 3,880.905 |

- 单次合并分批后的累计持锁约 3.86–3.91 s，与改造前单事务的单次持锁（4.09 / 4.76 s，第 7 / 4 节）同一量级；本组数据不支持“分批增加了总持锁”。墙钟比累计持锁多约 0.8–1.3 s，含装箱前的计数读取与批间等锁。单批持锁最大 67.117 / 65.404 ms。
- 每次合并的批次与持锁按时间窗口匹配 `mergeScopeBatch` 区间统计；两次合并串行，未混淆。

### 冻结请求大小的隔离实验

测量驱动再修两处后复跑一次：回滚与提交分开统计（`sql_rollback` / `sql_commit`），事务内查询计时延长到取行结束（`Rows.Close`）。同轮在每次执行后记录冻结请求 `request_json` 的长度。

`TestMeasureComponentResultByRequestSize`（`PREEN_MEASURE_EXEC_PAYLOAD=1`）只改变执行行的请求载荷，固定为 2,000 次 `SaveExecutionComponentResult`、相同结果内容、同一数据库配置：

| `request_json` / B | 累计 / ms | 平均 / ms | 最大 / ms |
| ---: | ---: | ---: | ---: |
| 10 | 80.491 | 0.040 | 0.375 |
| 65,546 | 107.295 | 0.054 | 0.394 |
| 524,298 | 328.614 | 0.164 | 1.121 |
| 2,097,162 | 1,903.228 | 0.952 | 4.013 |
| 8,388,618 | 15,313.101 | 7.657 | 25.339 |

- 保存耗时随请求载荷显著增长（2 MB 到 8 MB 增长约 8 倍，超过载荷的 4 倍），请求超过约 0.5 MB 后成为主要成本。本实验只证明载荷显著增加保存成本，不确定具体的写放大机制。
- 生命周期中的冻结请求：转换 1,567,824 / 1,562,824 B，元数据刷新 2,305,643 B（各 5,000 个组件）。按上表插值，每次保存约 0.6–1.0 ms，与生命周期中计数 `UPDATE` 平均约 1.33 ms（20,000 次累计 26,661.003 ms）同一量级；余量含并发与缓存差异。结论：计数 `UPDATE` 的剩余成本很可能主要来自冻结请求，拆分存储值得验证；拆分后的收益及对 `ImportMetadata` 等待的影响需另行测量。

复跑（231.99 s，无失败，无 `SQLITE_BUSY`）的其他读数：

- 读旧行计入取行后累计 572.556 ms（上一轮 553.236 ms），原口径基本完整；`SaveExecutionComponentResult` 提交 20,000 次累计 5,422.251 ms，回滚只出现在其他路径的提前返回（各不足 0.1 ms）。
- `ImportMetadata` 的 `BEGIN` 等待最大 4,233.293 ms，超过 1,000 ms 的 5 次；最长窗口内重叠持锁为其他 `ImportMetadata` 事务 2,221.039 ms、`EnqueueCovers` 617.906 ms、`SaveExecutionComponentResult` 385.192 ms。上一轮为 1,636.653 ms / 9 次：单样本最大值波动大，三轮都不足以判断等待改善。
- 合并：两次各 4,191.533 / 3,939.667 ms 累计持锁（66 / 71 批），单批持锁最大 143.658 ms；批内主要成本是过期清理的 `DELETE FROM inventory_files`（两次合计 6,341.736 ms），提交合计仅 3.188 ms。

---

## 12. 冻结请求拆表后的 5,000 × 12 复测

执行的冻结请求 `request_json` 移出 `plan_executions`，存入按执行 ID 关联的 `plan_execution_requests`（随会话同一事务写入、级联删除，退役清理在会话行之前分批删除）；读取接口不变，进度读取不联结请求。测量驱动新增各路径 `BEGIN` 等待 p99 与超过 1,000 ms 的次数（`sql_begin_p99`）。

### 请求大小隔离实验（同第 11 节）

| `request_json` / B | 拆表前平均 / ms | 拆表后平均 / ms |
| ---: | ---: | ---: |
| 10 | 0.040 | 0.039 |
| 65,546 | 0.054 | 0.039 |
| 524,298 | 0.164 | 0.039 |
| 2,097,162 | 0.952 | 0.039 |
| 8,388,618 | 7.657 | 0.039 |

保存耗时不再随请求载荷变化。

### 生命周期

完整生命周期完成，耗时 232.11 s；无 SQL 尝试失败，无 `SQLITE_BUSY`。冻结请求 1,567,824–1,572,824 B（转换）/ 2,315,643 B（元数据刷新），各 5,000 个组件。

| `SaveExecutionComponentResult`（20,000 次） | 第 11 节复跑 | 拆表后 |
| --- | ---: | ---: |
| BEGIN 最大 / p99 ms | 35.980 / — | 53.985 / 0.326 |
| 持锁最大 / ms | 16.199 | 7.099 |
| 累计持锁 / ms | 33,837.604 | 2,969.7 |
| 计数 `UPDATE` 累计 / 最大 ms | 26,661.003 / 11.984 | 623.2 / 3.477 |
| 结果 upsert 累计 / 最大 ms | 691.467 / 1.969 | 787.8 / 6.735 |
| 读旧行 累计 / 最大 ms | 572.556 / 1.734 | 659.6 / 1.959 |
| `COMMIT` 累计 / 最大 ms | 5,422.251 / 9.252 | 432.6 / 1.676 |

- 该路径累计持锁从第 11 节基线的 90.2 s、差额计数后的 33.8–34.6 s 降到约 3.0 s；`COMMIT` 也随之下降（每次提交不再写出含请求的整行页）。

| `ImportMetadata`（10,000 次） | 值 |
| --- | ---: |
| BEGIN 最大 / ms | 1,229.8 |
| BEGIN p99 / ms | 53.507 |
| 超过 1,000 ms 的次数 | 2（1,229.8 / 1,029.8 ms） |

| 等待 / ms | 窗口内重叠持锁 |
| ---: | --- |
| 1,229.8 | `ImportMetadata` 580.4；`EnqueueCovers` 131.9；`SaveExecutionComponentResult` 11.5 |
| 1,029.8 | `ImportMetadata` 727.2；`EnqueueCovers` 154.4；`SaveExecutionComponentResult` 10.6 |

- 最大等待仍高于既定的 1,000 ms 门槛；p99 不替代该门槛。两次超标窗口内的重叠持锁主要来自其他 `ImportMetadata` 事务，`SaveExecutionComponentResult` 只占约 11 ms。仍是单样本。
- 合并：两次各 66 / 71 批，墙钟 4,941.413 / 4,576.610 ms，累计持锁 3,917.805 / 3,932.476 ms，单批最大 65.624 ms，与第 11 节一致。

---

## 13. 关系族按需加载与封面入队索引后的 5,000 × 12 复测

两处改动：关系导入只加载本次涉及的关系族（`applyRelationsGroup`，此前每次导入整表读取关系证据），并新增索引 `cover_deliveries(execution_id, work_code)`。命令同第 10 节，同一夹具与配置，串行连跑 3 次，机器负载未刻意控制。

```sh
PREEN_MEASURE_WORKSET=1 PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=45m
```

3 次均完成完整生命周期（172.12 / 169.28 / 174.20 s，第 12 节 232.11 s），无 SQL 尝试失败，无 `SQLITE_BUSY`。

### `ImportMetadata` BEGIN 等待（10,000 次）

| 运行 | 最大 / ms | p99 / ms | 超过 1,000 ms 次数 | 是否 ≤ 1,000 ms |
| --- | ---: | ---: | ---: | --- |
| 第 12 节 | 1,229.8 | 53.507 | 2 | 否 |
| 1 | 1,329.733 | 33.345 | 2（1,329.7 / 1,131.4） | 否 |
| 2 | 433.060 | 18.435 | 0 | 是 |
| 3 | 1,130.910 | 18.758 | 1 | 否 |

### 持锁

| 路径 | 第 12 节 累计持锁 | 运行 1 / 2 / 3 累计持锁 / ms | 单次持锁最大（1 / 2 / 3）/ ms |
| --- | ---: | ---: | ---: |
| `ImportMetadata` | 41.5 s | 5,551.2 / 5,295.8 / 5,765.7 | 7.846 / 6.435 / 11.992 |
| `EnqueueCovers` | 21.4 s | 3,592.4 / 3,518.2 / 3,746.0 | 3.665 / 4.649 / 2.925 |
| `SaveExecutionComponentResult` | 2.97 s | 2,628.0 / 2,574.9 / 2,685.4 | 1.783 / 2.718 / 1.891 |

语句拆分（累计 ms，运行 1 / 2 / 3；语句计时在事务内，含 `COMMIT` 之外）：

| 语句 | 运行 1 | 运行 2 | 运行 3 |
| --- | ---: | ---: | ---: |
| `ImportMetadata` 关系族查询（`SELECT subject_rj…` ×2、`SELECT declaring_rj…` ×2，第 12 节约 24.8 s 的同类读取） | 1,314.3 | 1,251.3 | 1,356.4 |
| `ImportMetadata` 全部 `SELECT` / `UPDATE` 读取合计 | 1,971.8 | 1,879.4 | 2,054.7 |
| `ImportMetadata` 全部语句合计 | 4,581.9 | 4,378.3 | 4,776.4 |
| `ImportMetadata` `COMMIT` | 347.8 | 345.8 | 361.1 |
| `EnqueueCovers` 封面 `COUNT(*)`（第 12 节 19.1 s） | 349.4 | 329.0 | 366.0 |
| `EnqueueCovers` `SELECT EXISTS` | 800.7 | 759.8 | 831.3 |
| `EnqueueCovers` `INSERT` | 601.0 | 570.9 | 637.5 |
| `EnqueueCovers` `COMMIT` | 1,568.7 | 1,600.1 | 1,627.5 |

- `ImportMetadata` 累计持锁 41.5 s 降到约 5.3–5.8 s；关系族读取由约 24.8 s 降到约 1.3 s。`EnqueueCovers` 累计持锁 21.4 s 降到约 3.5–3.7 s，`COUNT` 由 19.1 s 降到约 0.35 s。
- 当前持锁最大的写路径是 `SyncObservedInventory`（累计约 20.7–20.9 s，单次最大 13.6–16.9 ms），不属本票范围。

### 超过 1,000 ms 的等待窗口

| 运行 | 等待 / ms | 窗口内重叠持锁 |
| --- | ---: | --- |
| 1 | 1,329.7 | `ImportMetadata` 660.0；`EnqueueCovers` 136.5；`SaveExecutionComponentResult` 96.8 |
| 1 | 1,131.4 | `ImportMetadata` 526.6；`EnqueueCovers` 110.8；`SaveExecutionComponentResult` 75.5 |
| 3 | 1,130.9 | `ImportMetadata` 517.1；`EnqueueCovers` 123.3；`SaveExecutionComponentResult` 78.5 |

- 三个窗口的重叠持锁合计约占窗口的 67% / 63% / 64%，主要来自其他 `ImportMetadata` 事务；单次持锁都不超过 12 ms。形态与第 10–12 节一致：等待者多次没有抢到写锁，而不是在等某一个长事务。本节不据此认定具体机制。
- 合并（运行 1 / 2 / 3，两次各 66 / 71 批）：墙钟 5,168.4 / 4,830.5、5,230.5 / 4,853.1、4,983.6 / 4,769.0 ms；累计持锁 4,087.8 / 4,156.4、4,064.1 / 4,127.6、4,057.0 / 4,147.7 ms。

### 结论

门槛为 3 次运行 `ImportMetadata` 的 `BEGIN` 等待最大都 ≤ 1,000 ms。结果：运行 1（1,329.7 ms）与运行 3（1,130.9 ms）超标，仅运行 2（433.1 ms）达标；**未达标**（3 次中 1 次满足）。持锁总量与 p99 已大幅下降（最大等待与超 1 s 次数没有同幅下降：2 / 0 / 1 次，第 12 节 2 次），剩余超标窗口由 `ImportMetadata` 事务之间的竞争主导。按验收口径，进入决定 4（`ImportMetadata` 写事务进程内互斥，且测量须同时报告进程内排队时间与 `BEGIN` 等待）。

## 14. `ImportMetadata` 进程内互斥后的 5,000 × 12 复测

改动：`Repository` 上新增一个普通 `sync.Mutex`，只包住 `ImportMetadata` 的数据库事务（先取锁，再 `db.Begin()`，持有到提交或回滚结束，`catalogChanged()` 在解锁之后）；其他写路径不变。测量新增逐次记录“开始申请 Go 锁 → `db.Begin()` 成功返回”的合计等待，并拆分为 Go 排队与 `BEGIN` 两部分；最大值、p99、超 1,000 ms 次数都按逐次合计计算。命令同第 13 节，串行连跑 3 次，日志在 scratchpad 的 `lifecycle-mutex-run-{1,2,3}.log`。`ImportMetadata` 在测量中的路径名保持不变，便于与第 13 节对照；`sql_begin_p99` 仍只含 SQLite `BEGIN` 部分。

### 逐次结果

3 次均完成完整生命周期，无 SQL 尝试失败，无 `SQLITE_BUSY`。第 13 节同轮日志为对照。

| 项 | 运行 1 | 运行 2 | 运行 3 |
| --- | ---: | ---: | ---: |
| 完成 / 测试耗时 | 是 / 159.7 s | 是 / 177.0 s | 是 / 175.4 s |
| 合计等待 最大 / ms | 43.905 | 47.924 | 44.166 |
| 合计等待 p99 / ms | 19.523 | 22.040 | 20.442 |
| 合计等待超过 1,000 ms 次数 | 0 | 0 | 0 |
| Go 排队 最大 / p99 / ms | 43.782 / 19.019 | 47.797 / 21.623 | 44.059 / 19.648 |
| `BEGIN` 最大 / p99（合计口径）/ ms | 12.805 / 2.982 | 19.415 / 3.221 | 19.497 / 2.534 |
| `ImportMetadata` 累计持锁 / ms（第 13 节） | 4,171.4（5,551.2） | 5,348.5（5,295.8） | 5,159.7（5,765.7） |
| `EnqueueCovers` 累计持锁 / ms（第 13 节） | 2,933.7（3,592.4） | 3,794.0（3,518.2） | 3,763.2（3,746.0） |

### 其他写路径是否变差

`BEGIN` 等待最大 / 累计持锁（括号内为第 13 节同轮，ms；`BEGIN` 最大取自语句行，每次运行一个样本）：

| 路径 | 运行 1 | 运行 2 | 运行 3 |
| --- | --- | --- | --- |
| `EnqueueCovers` 等待最大 | 34.5（55.8） | 34.9（81.3） | 34.7（54.9） |
| `EnqueueCovers` `BEGIN` p99 | 3.51（8.44） | 3.56（3.72） | 3.49（8.33） |
| `SaveExecutionComponentResult` 等待最大 | 19.3（18.9） | 19.0（34.5） | 18.5（34.2） |
| `SaveExecutionComponentResult` 累计持锁 | 2,306.8（2,628.0） | 2,673.3（2,574.9） | 2,648.7（2,685.4） |
| `SyncObservedInventory` 等待最大 | 0.73（0.67） | 0.59（0.86） | 0.68（0.81） |
| `SyncObservedInventory` 累计持锁 | 20,701.7（20,871.1） | 20,335.4（20,731.2） | 20,491.6（20,781.4） |
| 合并批次累计持锁（两次合并） | 3,998.7 / 4,233.5（4,087.8 / 4,156.4） | 4,012.8 / 4,036.3（4,064.1 / 4,127.6） | 4,034.1 / 4,111.6（4,057.0 / 4,147.7） |

- 未见其他写路径变差：`EnqueueCovers` 与 `SaveExecutionComponentResult` 的等待最大持平或更低，其余路径的 p99 与累计持锁在同一量级内波动。每个值只有单样本，不据此断言它们变好。
- 小样本路径 `NextQueuedExecution` / `NextQueuedGeneration`（9 / 17 次）的 p99 约 53 ms 在各次运行中出现在不同路径上（第 13 节也有同样的 53 ms 量级值），未单独调查。

### 最长等待

三次的最长三个合计等待（无超过 1,000 ms 者，取前 3）都在 37–48 ms，其中最长一个几乎全为 Go 排队：

| 运行 | 合计 / ms | Go 排队 / ms | `BEGIN` / ms | `BEGIN` 窗口内其他写路径重叠持锁 |
| --- | ---: | ---: | ---: | --- |
| 1 | 43.905 | 43.782 | 0.123 | 无（仅 `ImportMetadata` 0.005） |
| 1 | 41.439 | 38.075 | 3.364 | `EnqueueCovers` 1.342；`SaveExecutionComponentResult` 0.850 |
| 1 | 37.274 | 31.606 | 5.668 | `EnqueueCovers` 0.713 |
| 2 | 47.924 | 47.797 | 0.128 | 无（仅 `ImportMetadata` 0.006） |
| 2 | 47.336 | 43.056 | 4.279 | `EnqueueCovers` 1.279；`SaveExecutionComponentResult` 0.844 |
| 2 | 45.120 | 38.747 | 6.373 | `EnqueueCovers` 1.390；`SaveExecutionComponentResult` 0.912 |
| 3 | 44.166 | 44.059 | 0.106 | 无（仅 `ImportMetadata` 0.007） |
| 3 | 44.164 | 43.925 | 0.240 | 无（仅 `ImportMetadata` 0.012） |
| 3 | 44.122 | 43.958 | 0.164 | 无（仅 `ImportMetadata` 0.011） |

### 结论

门槛为 3 次运行逐次合计等待（Go 排队 + `BEGIN`）最大都 ≤ 1,000 ms。结果：43.9 / 47.9 / 44.2 ms，超过 1,000 ms 的次数 0 / 0 / 0，**达标**（3 次中 3 次满足）；无 `SQLITE_BUSY`，全部完成，其他写路径没有出现变差。最长的合计等待几乎全是 Go 排队（约 38–48 ms），`BEGIN` 部分最大约 13–19 ms。本节只表明互斥后的合计等待远低于门槛，不对第 13 节超标窗口的具体机制作进一步断言。

## 15. 执行页按成员摘要改造后的 5,000 × 12 复测

本节在执行页改为 `view=members` 摘要、单成员页读取 `member_id` 之后采集，验收见[执行详情页读取与渲染](workset-execution-page-spec.md)。第 7 节“生产执行详情页面的另一项阻碍”中的 16053.8 / 17032.6 ms（393 / 1440）是本节的基线。环境同第 7 节：WSL2 磁盘运行（非 tmpfs），Go 1.27.3，Chromium 156.0.8078.4，Vite 开发构建，5,000 成员 × 12 音频；第二轮真实转换执行由 5,000 个组件构成，每组件 12 个非空保留决定（采样的成员实测 12 个）。**每个视口 / 页面 / 状态只有一个正式样本**，不是 p99，不用于声称稳定低于阈值。

### 复现

在 `backend/go` 执行（命令与第 7 节非空保留模式相同，不需新开关；约 82 s）：

```sh
GOCACHE=/tmp/preen-go-cache PREEN_MEASURE_WORKSET=1 \
  PREEN_MEASURE_MEMBERS=5000 PREEN_MEASURE_AUDIO=12 PREEN_MEASURE_BROWSER=1 \
  PREEN_MEASURE_KEPT_SNAPSHOT=1 \
  go test ./internal/adapters/sqlite -run '^TestMeasureWorksetLifecycle$' -count=1 -v -timeout=30m
```

### 口径与相对第 7 节的变化

- 夹具在第二轮执行开始后（不等待完成）先调用 `measure-workset-reads.mjs` 的 `execution-stream` 阶段，执行完成后再调用原 `execution` 阶段（空闲）。流式阶段的页面采样前后，脚本用 Node 直接读取同一 execution（`view=members`），两次状态都必须为 `running`，否则脚本断言失败；表中记录前 / 后的完成组件数。
- 为让会话在采样窗口内保持 running，Go 夹具在该阶段给每次 `SaveExecutionComponentResult` 的事务开始前加 40 ms 延迟（`PREEN_MEASURE_SAVE_DELAY_MS`，浏览器阶段结束后归零）。延迟只作用于测量驱动，不改生产代码；它让执行变慢，因此流式页面读取时的 SQLite 写入竞争与不加延迟的真实执行不同，仅作“会话确实在运行”的证明。
- 计时：每个页面先 `goto` 加载并等字体，再 reload（清空 Vue Query 缓存）；计到就绪元素可见时的 `performance.now()`。执行页的就绪元素是成员计数 `execution-count-pending`（不是 panel 外壳）；成员页的就绪元素是第一个 `execution-component` 行。所测成员为 5,000 个成员中的第一个（`m0`），12 个保留决定均非空。
- 记录项：请求体字节为未压缩 UTF-8 body，handler 为生产 handler 第一次 socket 写入前的 `Server-Timing`，SSE 为空闲阶段独立打开的 `events?view=members` 首帧（首次与无 replay id 重连各一次）。
- 与第 7 节不同：第 7 节的 snapshot-only 表读取的是全量组件快照；本节同一模式的快照已是成员摘要。

### 验收结果

阈值：单项 reload 至可见 ≤ 2000 ms。

| 视口 | 状态 | 页面 | reload 至可见 / ms | 采样前 → 后完成组件 / 5000 | 页面显示状态 | 达标 |
| --- | --- | --- | ---: | --- | --- | --- |
| 393 | 空闲 | 执行页 | 907.3 | 5000 → 5000（succeeded） | 已完成 | 是 |
| 393 | 流式 | 执行页 | 891.2 | 98 → 120（running） | 执行中 | 是 |
| 393 | 空闲 | 成员页 | 886.6 | 5000 → 5000（succeeded） | 已完成 | 是 |
| 393 | 流式 | 成员页 | 881.8 | 143 → 165（running） | 已完成（`m0` 已落地） | 是 |
| 1440 | 空闲 | 执行页 | 899.4 | 5000 → 5000（succeeded） | 已完成 | 是 |
| 1440 | 流式 | 执行页 | 925.8 | 241 → 264（running） | 执行中 | 是 |
| 1440 | 空闲 | 成员页 | 937.3 | 5000 → 5000（succeeded） | 已完成 | 是 |
| 1440 | 流式 | 成员页 | 957.1 | 288 → 313（running） | 已完成（`m0` 已落地） | 是 |

相对基线（执行页 16053.8 / 17032.6 ms），空闲执行页降为 907.3 / 899.4 ms。

### 读取字节与 handler

页面自身的请求，每个样本一次（ms / B；流式列为 running 会话，空闲列为 succeeded）：

| 请求 | 流式 B | 流式 handler / ms | 空闲 B | 空闲 handler / ms |
| --- | ---: | --- | ---: | --- |
| execution GET `view=members`（393 执行页） | 1,080 | 17.115 | 691 | 28.954 |
| execution GET `view=members`（1440 执行页） | 1,080 | 16.378 | 691 | 28.499 |
| execution GET `member_id=`（393 成员页） | 3,104 | 15.541 | 3,004 | 29.991 |
| execution GET `member_id=`（1440 成员页） | 3,104 | 16.045 | 3,004 | 31.159 |

成员页同时发出 `view=members` 请求（页头 / 状态），393 空闲为 691 B / 29.916 ms，1440 流式为 1,080 B / 17.092 ms。JSON.parse 均 ≤ 0.1 ms。

SSE `view=members` 首帧（空闲，已完成会话）：payload 691 B，SSE 帧 725 B，reader 收到 848 B；服务生成 61.116 / 55.625 ms（393 首次 / 重连），54.985 / 55.678 ms（1440），首帧取完 57.1–63.3 ms，`counts` 合计 5,000，`attention` 0。流式状态下页面自己打开的 SSE 首帧字节未单独记录；同一时刻 execution GET `view=members` 为 1,080 B（含 running 成员的例外条目）。第 7 节同一入口的全量快照为 10,699,474 B。

### 边界

- 单样本，且每个视口各自 reload 一次；没有重复运行，不给出分布。
- 流式样本的“成员页”所选 `m0` 在采样时已经完成（组件在该会话最早的 100 个内落地），因此成员页的流式样本证明的是“会话运行中进入成员页的读取与渲染”，未覆盖“正在运行的成员收到 `member` 事件后重读”的路径；该路径的正确性由单元测试覆盖，本节不测其耗时。
- 页面请求中的 `libraries/.../current`、`worksets/{id}` 各 1.21 MB（handler 约 13–25 ms）和转换 `operation`（流式 handler 354–374 ms，空闲 352–367 ms）仍在页面入口前置，约 0.5 s 的页面入口有一半在 `operation` 读取之后才发出 execution 请求；此次未调查，也不在本票范围。
- 40 ms 保存延迟只用于保持会话运行；空闲 execution GET 的 handler（约 29 ms）反而高于流式（约 16 ms），原因未调查，两者都远低于 2 s 的阈值。
- 开发构建、OS 页缓存、保留代码与字体缓存，loopback 无广域网延迟，没有其它并发 worker，结论条件同第 7 节。

### 结论

8 项（2 视口 × 执行页 / 成员页 × 空闲 / 流式）全部 ≤ 2000 ms，最大 957.1 ms，**达标**；未触发诊断拆分。

### 补测：执行页恢复读取 revision 后

上表采样时，`ConversionPage` 在执行页与成员执行页路由上跳过 revision 读取，旁边的 `MemberList` 因此没有成员结论、全部显示默认徽标（缺陷）。修复为这两个路由也读取轻量 revision 后，同一命令复测一次（72.4 s，通过）。revision 与 execution 读取并行发出，流式时 revision 为 1,810,138 B、handler 373–378 ms、JSON.parse 2.7–3.1 ms：

| 视口 | 状态 | 执行页 / ms | 成员页 / ms |
| --- | --- | ---: | ---: |
| 393 | 空闲 | 844.9 | 847.1 |
| 393 | 流式 | 890.9 | 935.2 |
| 1440 | 空闲 | 879.9 | 981.7 |
| 1440 | 流式 | 987.8 | 957.7 |

8 项最大 987.8 ms，仍全部 ≤ 2000 ms。单样本，口径与边界同上。
