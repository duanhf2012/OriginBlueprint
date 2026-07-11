# Codex 蓝图引擎维护规则

本文档记录 Go 蓝图解析执行引擎的维护规则，供后续 Codex 或其他 agent 修改本库时优先阅读。规则覆盖 `engine/go/blueprint/`，以及和它相关的顶层 `nodes/*.json`、兼容迁移、测试和压测。

## 1. 当前范围

- 当前只实现 Go engine。
- C#、Lua engine 暂不处理。
- `nodes/*.json` 是基础系统节点定义，需要覆盖。
- `nodes/json/**` 是业务节点定义，当前明确忽略，除非用户单独要求。
- RPC 业务节点暂不实现；异步能力通过 Execution、Delay、按函数 Timer 和 continuation 覆盖。
- 文件、表格、字典相关蓝图数据类型和节点已经删除，不得无意恢复。

## 2. 核心架构规则

- `CompiledGraph` 是编译后的共享执行树。
- `ExecNode` 和 `NodeDefinition` 是编译期构建、执行期只读的结构。
- 每个 `Blueprint.Create` 实例拥有自己的变量和 Timer 注册表。
- 每次 `Blueprint.Start`、`Blueprint.Do` 或 `Blueprint.DoContext` 都会创建私有 `Graph` execution session。
- `Graph` 保存本次执行的 transient context、returns、functionResults 和 continuation 状态。
- 不要把单次执行状态写回 `CompiledGraph`、`ExecNode` 或 `NodeDefinition`。
- 不要直接复用同一个 `Graph` 做并发执行；服务器侧并发应通过 `Blueprint.Do`。

## 3. 线程安全规则

- `Blueprint` 是对外并发安全 facade。
- `Blueprint.graphs`、`Blueprint.instances`、热更新替换、创建、释放、查询和执行入口必须受 `RWMutex` 保护。
- `Blueprint.Start` 应在读锁内捕获 `GraphInstance` 快照，然后释放锁并提交 Dispatcher，调用方 goroutine 不执行用户节点。
- `Blueprint.Do` 和 `Blueprint.DoContext` 只作为等待 `Execution.Done()` 的阻塞便利封装；服务器事件循环应使用 `Start`。
- `GraphInstance` 通过可替换的 runtime state 持有 compiled graph、变量表和对应的变量锁；一次执行只捕获一个 state 指针。
- `GraphInstance.runtimeTimers` 通过 `timerMu` 保护；Scheduler 回调不得持有该锁执行蓝图函数。
- Delay 与 Timer 共用可注入的最小堆 Scheduler，不得为每个等待任务创建 goroutine。
- Timer 回调必须通过 Dispatcher 启动独立函数 Execution；循环 Timer 的同一句柄回调不得重入。
- continuation 自身必须保证只 resume 一次。
- 异步挂起后，原 goroutine 不应继续读取可能被回调 goroutine 修改的 session 状态。
- 自定义节点如果持有共享状态，需要由节点实现方自行加锁。

线程安全验证命令：

```powershell
go test -race ./engine/go/blueprint -count=1
```

如果修改 `Blueprint` facade、热更新、实例生命周期或 timer 生命周期，还要跑：

```powershell
go test -race ./... -count=1
```

## 4. 性能规则

- 优先把工作移动到加载/编译期，减少执行期动态成本。
- 蓝图执行流程日志必须默认关闭；关闭时不得格式化输入/输出端口值，只允许极轻量的开关判断。
- 编译期可预处理：
  - 节点连续 index。
  - 数据输入口下标。
  - 默认输入 typed port。
  - exec 边目标输入口。
  - FunctionCall 目标 `*CompiledGraph`。
- 执行期应尽量避免：
  - string-keyed map 热路径查询。
  - 每个端口重复判断 exec/data。
  - 默认值的反复类型转换。
  - 共享节点上的可变缓存。
- 可以考虑 `sync.Pool`，但只允许池化单次执行私有对象。
- 异步 continuation 挂起期间，相关 `Graph`、`ExecContext`、port 状态不能归还池。

性能验证命令：

```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=3s -benchmem -count=1
```

## 5. 异步与函数规则

- 只有一个固定后续执行出口的异步节点，通过 `Suspend(nextIndex)` 捕获 continuation，并由回调调用 `Continuation.Resume(...)`。
- 需要由异步结果选择成功、失败等执行出口的业务节点，通过 `SuspendForResume()` 捕获动态 continuation，并由回调调用 `Continuation.ResumeTo(nextIndex, outputs...)`。
- `ResumeTo` 的 `nextIndex` 是当前节点的输出端口下标，且必须指向 Exec 输出；传入数据端口或越界下标必须报错，并且不得消耗 continuation。
- 动态 continuation 不允许调用 `Resume`/`ResumeAsync`，固定出口 continuation 不允许调用 `ResumeTo`，避免业务回调意外走错分支。
- continuation 只能成功恢复一次。RPC 等宿主回调必须处理恢复错误；超时、取消与正常响应发生竞争时，以第一次成功恢复或 Execution 终止为准。
- Execution 管理的 continuation 必须通过其 Dispatcher 恢复，不得在 RPC 回调 goroutine 内直接继续执行节点链。
- continuation resume 后继续执行的是同一个 suspended `Graph` session。
- 函数调用每次创建 child `Graph` session、独立变量表和独立变量锁；只共享 host module、实例生命周期门禁和 trace。函数变量不得污染调用方或下一次调用。
- `FunctionReturn` 负责收集函数输出，并通过 caller continuation 回到调用点。
- 函数内部异步返回时，caller 必须在函数返回后继续后续节点。
- 递归函数调用必须受 `MaxFunctionCallDepth` 限制。
- 热加载只迁移同名且规范化类型一致的变量值；新变量使用新默认值，删除变量消失，改类型变量重置。进行中的旧 session 继续使用旧 runtime state。

## 6. 兼容性规则

- `.vgf` 是旧编辑器格式，兼容性高风险。
- 旧图里的未知节点和已删除节点不能静默丢弃，应进入 legacy hidden state。
- 如果新增或重命名节点，需要同步考虑：
  - Go document conversion。
  - legacy migration/export。
  - frontend runtime schema。
  - `nodes/*.json` 定义。
  - 测试覆盖。
- 文件、表格、字典相关节点和变量类型已经按当前需求删除；旧图迁移时应隐藏这些节点和边。

## 7. 测试规则

修改 engine 时至少运行：

```powershell
go test ./engine/go/blueprint -count=1
```

修改线程安全时运行：

```powershell
go test -race ./engine/go/blueprint -count=1
```

修改跨包 API、迁移、前端类型或节点定义时运行：

```powershell
go test ./...
cd frontend
npm run test:layout
npm run build
```

测试矩阵详见：

- `docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`

## 8. 新增节点规则

- 顶层 `nodes/*.json` 新增节点后，必须确保 `BuiltinExecNodeFactories` 可注册。
- 新节点必须有行为测试，`TestTopLevelSystemNodeBehaviorCoverage` 应同步更新。
- 有执行流的节点需要测试后续节点能正确继续。
- 有数据输出的纯节点需要测试循环内按需重算语义。
- 有异步行为的节点必须测试 suspend/resume 和 race。

## 9. 不要做的事

- 不要把执行上下文缓存到共享 `ExecNode`。
- 不要为了性能绕过变量锁或 timer 锁。
- 不要在没有 race 测试的情况下声明并发安全。
- 不要恢复 file/table/dictionary 数据类型。
- 不要处理 `nodes/json/**`，除非用户明确要求。
- 不要把旧 `.vgf` 不能识别的内容直接丢弃。
