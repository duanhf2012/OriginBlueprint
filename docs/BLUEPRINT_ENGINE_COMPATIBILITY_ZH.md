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
- `Init(execDefFilePath, graphFilePath string, module IBlueprintModule, logger ...IBlueprintLogger) error`
- `Create(graphName string) int64`
- `Start(ctx context.Context, graphID int64, entranceID int64, args ...any) (*Execution, error)`
- `Do(graphID int64, entranceID int64, args ...any) (PortArray, error)`
- `DoContext(ctx context.Context, graphID int64, entranceID int64, args ...any) (PortArray, error)`
- `TriggerEvent(graphID int64, eventID int64, args ...any) error`
- `ReleaseGraph(graphID int64)`
- `Close() error`
- `StartHotReload() (func(), error)`
- `GetLogger() IBlueprintLogger`
- `GetGraphName(graphID int64) string`

`Start` 是服务器事件循环推荐入口：它把执行提交到 Dispatcher 并立即返回，后续通过 `Execution.Done()`、`Result()` 和 `Cancel()` 管理生命周期。`Do`/`DoContext` 会阻塞当前 goroutine，直到包含 `Delay` 的完整执行结束，适合命令行、测试或明确允许阻塞的工作协程。

低层 `NewGraph(...).Do(...)` 只支持同步执行；遇到异步节点时返回 `ErrExecutionSuspended`，不会等待，也不会静默返回 `(nil, nil)`。

新增配置与排障入口：

- `SetExecutionDispatcher(dispatcher ExecutionDispatcher)`
- `SetTimerScheduler(scheduler TimerScheduler)`
- `SetTraceLogger(logger BlueprintTraceLogger)`
- `SetTraceEnabled(enabled bool)`

## 已覆盖的替换路径

覆盖测试：

- `engine/go/blueprint/compatibility_test.go`
  - `TestBlueprintLegacyFacadeIntegrationPath`
- `engine/go/blueprint/execution_session_test.go`
  - `TestBlueprintCloseCancelsExecutionsAndRejectsNewWork`
- `engine/go/blueprint/timer_runtime_test.go`
  - `TestReleaseGraphCancelsRuntimeTimers`

覆盖内容：

- 从临时节点目录和蓝图目录执行 `Init`。
- 通过 `RegisterExecNode` 注册服务器自定义节点。
- 通过 `Create` 创建实例。
- 通过 `TriggerEvent` 触发入口并执行完整蓝图。
- `Init` 传入的 logger 可通过 `GetLogger` 取回。
- logger 实现 `BlueprintTraceLogger` 时，可配合 `SetTraceEnabled(true)` 接收节点执行步骤、输入和输出。
- `StartHotReload` 可重新加载图并返回可执行的替换函数。
- `ReleaseGraph` 后再次 `Do` 返回 `ErrGraphNotFound`，不会继续执行实例。
- `ReleaseGraph` 会直接清理实例的 Execution、Delay 和 Timer，不再委托 module 或旧 `cancelTimer` 回调。
- Running Execution 采用协作式取消：当前节点返回后停止后续节点并关闭 `Done`；`ReleaseGraph` 不等待可能长期阻塞的宿主节点。
- 旧 `Timer事件入口`、`CreateTimer`、`CloseTimer`、`CancelTimerId` 和 `IBlueprintModule` timer 方法已删除；这些定时器从未正式投入使用，因此不保留运行时兼容层。

## 接入前人工核对

- 收集服务器当前实际注册的所有自定义节点，逐个确认 `GetName()` 与节点 JSON 中的 `name` 去入口后缀后的类名一致。
- 对照自定义节点的输入输出端口，确认 `port_id`、`type`、`data_type` 与旧定义一致。
- 抽取真实服务器 `.vgf/.obp/.obpf` 文件跑离线加载和执行测试。
- 只有固定后续出口的异步节点使用 `Suspend(nextIndex)`/`Continuation.Resume(...)`；RPC 等需要按结果选择成功、失败出口的节点使用 `SuspendForResume()`/`Continuation.ResumeTo(nextIndex, outputs...)`。
- `ResumeTo` 的 `nextIndex` 是节点输出端口下标，不是第几个已连线节点；目标必须是 Exec 输出端口。`outputs` 按该节点的数据输出端口顺序回填，然后再执行选定分支。
- RPC 回调必须检查 `ResumeTo` 返回的错误。continuation 只允许成功恢复一次，Execution 取消后到达的响应也会被拒绝，回调不得再次推进蓝图。
- Timer 回调应选择蓝图函数，由运行时通过稳定函数 ID 启动独立 Execution；不要自行构造 Timer 事件入口。
- 大规模开启 trace 前必须加业务侧过滤，只对指定 graph/object 开启。
- 灰度期间保留旧库回退开关，先按模块或对象范围逐步替换。

## 推荐验证命令

```powershell
go test ./engine/go/blueprint -run 'TestBlueprintLegacyFacadeIntegrationPath|TestReleaseGraphCancelsRuntimeTimers' -count=1
go test ./engine/go/blueprint -race -count=1
go test ./...
```

## 异步业务节点示例

下面的节点有两个 Exec 输出：下标 `0` 为成功，下标 `1` 为失败；其后的参数按节点数据输出端口顺序传递。`Start` 创建的 Execution 会把恢复任务提交回配置的 Dispatcher，因此 RPC 回调线程不会直接执行后续蓝图节点。

```go
func (n *QueryRoleNode) Exec() (int, error) {
	continuation, err := n.SuspendForResume()
	if err != nil {
		return -1, err
	}

	n.client.QueryRole(n.roleID, func(role Role, callErr error) {
		if callErr != nil {
			// 输出端口 1：Failed；数据输出：错误码、错误信息。
			_ = continuation.ResumeTo(1, errorCode(callErr), callErr.Error())
			return
		}
		// 输出端口 0：Succeeded；数据输出：角色数据。
		_ = continuation.ResumeTo(0, role)
	})
	return -1, blueprint.ErrExecutionSuspended
}
```

生产节点不应直接忽略示例中的恢复错误；应记录重复回调、Execution 已取消或实例已释放等情况。这里省略日志仅为了突出控制流。
