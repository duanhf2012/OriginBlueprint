# Go 蓝图执行库接入兼容清单

本文用于服务器项目替换旧 `Origin/util/blueprint` 前核对新 Go engine 的接入风险。

## 旧 `.vgf` 兼容口径

当前兼容目标是：线上旧 `.vgf` 能被新的 Go 解析/执行库正确加载、编译和运行。

验收时以 Go 库行为为准：

- 旧 `.vgf` 可以通过迁移路径生成 `GraphDocument`。
- 已知业务节点、入口节点、函数节点和变量节点不丢失。
- 旧 `port_id`、`port_defaultv`、exec/data 连线能映射到新端口。
- `engine/go/blueprint` 可以编译并执行代表性用例。
- 执行返回值、变量变化、分支命中和异步 continuation 行为符合预期。

不要求新导出的 `.vgf` 与旧编辑器生成的 `.vgf` 字节完全一致。字段顺序、自动 id、保存时间、画布坐标微小差异不作为兼容失败；需要 round-trip 对比时，应使用 canonical 后的语义结构进行比较。

详细测试矩阵见 `docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`。

## 兼容入口

服务器侧优先只依赖以下 facade：

- `RegisterExecNode(factory func() IExecNode)`
- `Init(execDefFilePath, graphFilePath string, module IBlueprintModule, cancelTimer func(*uint64) bool, logger ...IBlueprintLogger) error`
- `Create(graphName string) int64`
- `Do(graphID int64, entranceID int64, args ...any) (PortArray, error)`
- `TriggerEvent(graphID int64, eventID int64, args ...any) error`
- `ReleaseGraph(graphID int64)`
- `CancelTimerId(graphID int64, timerID *uint64) bool`
- `StartHotReload() (func(), error)`
- `GetLogger() IBlueprintLogger`
- `GetGraphName(graphID int64) string`

新增排障入口：

- `SetTraceLogger(logger BlueprintTraceLogger)`
- `SetTraceEnabled(enabled bool)`

## 已覆盖的替换路径

覆盖测试：

- `engine/go/blueprint/compatibility_test.go`
  - `TestBlueprintLegacyFacadeIntegrationPath`
  - `TestBlueprintReleaseGraphCancelsInstanceTimersThroughLegacyCallback`

覆盖内容：

- 从临时节点目录和蓝图目录执行 `Init`。
- 通过 `RegisterExecNode` 注册服务器自定义节点。
- 通过 `Create` 创建实例。
- 通过 `TriggerEvent` 触发入口并执行完整蓝图。
- `Init` 传入的 logger 可通过 `GetLogger` 取回。
- logger 实现 `BlueprintTraceLogger` 时，可配合 `SetTraceEnabled(true)` 接收节点执行步骤、输入和输出。
- `StartHotReload` 可重新加载图并返回可执行的替换函数。
- `ReleaseGraph` 后再次 `Do` 不报错且不会继续执行实例。
- `ReleaseGraph` 会清理实例 timer；有 `IBlueprintModule` 时走 module，没有 module 时走旧 `cancelTimer` 回调。

## 接入前人工核对

- 收集服务器当前实际注册的所有自定义节点，逐个确认 `GetName()` 与节点 JSON 中的 `name` 去入口后缀后的类名一致。
- 对照自定义节点的输入输出端口，确认 `port_id`、`type`、`data_type` 与旧定义一致。
- 抽取真实服务器 `.vgf/.obp/.obpf` 文件跑离线加载和执行测试。
- timer/RPC 类异步节点接入前，确认回调只调用一次 `Continuation.Resume(...)`。
- 大规模开启 trace 前必须加业务侧过滤，只对指定 graph/object 开启。
- 灰度期间保留旧库回退开关，先按模块或对象范围逐步替换。

## 推荐验证命令

```powershell
go test ./engine/go/blueprint -run 'TestBlueprintLegacyFacadeIntegrationPath|TestBlueprintReleaseGraphCancelsInstanceTimersThroughLegacyCallback' -count=1
go test ./engine/go/blueprint -race -count=1
go test ./...
```
