# Go 蓝图引擎 Agent 规则

本目录包含面向服务器运行的 Go 蓝图解析与执行引擎，属于高风险运行时代码。修改时必须优先考虑兼容性、并发安全和性能。

## 必读上下文

修改本目录前，先阅读：

- `../../../docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md`：当前 Go API、VM、Yield/Resume、并发、兼容性和禁止用法的唯一权威说明。
- `../../../docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`：验证蓝图与独立 Go 参考实现的自动对比结果。

## 硬性规则

- `Program`、`CompiledGraph`、`ExecNode` 和 `NodeDefinition` 是共享只读结构，不得保存单次执行的可变状态。
- `Graph`、PC、Flow/Loop/Call Stack、Native Context 和 Yield token 只能属于单次 Execution，不得并发复用。
- 生产执行只能经过 VM；不得恢复 `ExecNode.Do/doNext`、旧 `Continuation` 或双执行内核。
- 普通 Native 业务节点必须同步完成；RPC 等异步节点通过 `BaseExecNode.Yield` 和一次性 `YieldHandle` 恢复。
- Native 节点成功调用 `Yield` 后必须立即返回 `ErrExecutionSuspended`；恢复发生在所选 exec 出口，不会回到 `Exec()` 的 Go 语句中间。
- Resume 必须经过 Execution 启动时捕获的 Dispatcher；业务宿主负责把恢复投递回所属 Actor。
- VM Core 不实现 Delay/Timer 调度。需要时间语义时由业务或可选 stdlib 节点持有宿主调度器。
- 挂起期间不得释放或复用 Execution 的 Context、Flow/Loop/Call Stack；取消、完成或失败时统一释放引用。
- 保持 `.vgf` 兼容性。已删除或未知的 legacy 节点应隐藏或保留，不能静默丢弃。
- 顶层 `nodes/*.json` 是系统节点定义。除非用户明确要求，`nodes/json/**` 业务定义不在处理范围。
- 文件、表格、字典蓝图数据类型已经按需求删除，未经用户明确同意不得恢复。

## 性能规则

- 优先在编译期或加载期预处理，避免执行期字符串查找。
- `CompileGraph` 返回后，Program 及其 NodePlan、binding、successor 必须保持不可变。
- 热路径优先使用 index 和预计算 binding，不在共享节点上缓存 Context 或端口值。
- 控制流使用显式 VM Frame，不依赖 Go 调用栈保存蓝图恢复上下文。

## 验证命令

修改 engine 时至少运行：

```powershell
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
```

修改 facade 或线程安全相关代码时，还要运行：

```powershell
go test -race ./... -count=1
```

修改性能敏感路径时，运行：

```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex|Parallel)|BenchmarkFunctionCall' -benchtime=3s -benchmem -count=1
```
