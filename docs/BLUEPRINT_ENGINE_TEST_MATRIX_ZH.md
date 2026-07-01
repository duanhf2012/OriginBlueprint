# Blueprint Engine 测试矩阵

本文档记录当前 Go 版解析执行引擎的测试用例设计、覆盖范围和压测入口。范围只包含顶层 `nodes/*.json` 与当前 engine/golang 功能；`nodes/json/**`、RPC 业务节点、origin 服务器完整替换接入、C#、Lua 均不在本轮处理。

## 测试目标

- 验证旧蓝图 `.vgf` 能继续加载、编译和执行。
- 验证新蓝图 `.obp/.obpf` 的函数入口、函数返回、函数调用后续执行。
- 验证异步 continuation 机制能在 sleep/timer 类节点回调后继续当前位置。
- 验证顶层 `nodes/*.json` 中所有基础系统节点均有加载覆盖和行为覆盖。
- 验证已删除的文件、表格、字典相关节点和变量类型不会重新暴露。
- 提供服务器高频执行的 benchmark 入口，用于观察共享编译树、多实例执行、并发执行的耗时和分配。

## 顶层节点覆盖

覆盖来源：

- `nodes/Base.json`
- `nodes/Entrance.json`
- `nodes/Event.json`
- `nodes/Math.json`
- `nodes/SysFlowControl.json`
- `nodes/Test.json`

覆盖测试：

- `engine/golang/system_nodes_test.go`
  - `TestBuiltinFactoriesCoverAllTopLevelNodeDefinitions`
  - `TestTopLevelSystemNodeBehaviorCoverage`
  - `TestBuiltinMathNodes`
  - `TestBuiltinArrayNodes`
  - `TestBuiltinBranchNodes`
  - `TestBuiltinEntranceTimerAndDebugNodes`
  - `TestBuiltinReturnNodesAppendGraphResults`
- `engine/golang/flow_integration_test.go`
  - `TestLegacyBlueprintFileRunsComplexBranchAndNestedLoopFlow`
  - `TestLegacyBlueprintFileBreaksForLoopAndContinuesCompletedFlow`
  - `TestNativeBlueprintFileCallsFunctionAndContinuesFlow`

节点行为覆盖清单：

- 入口节点：`Entrance_IntParam`、`Entrance_ArrayParam`、`Entrance_Timer`
- 基础数组节点：`CreateIntArray`、`CreateStringArray`、`GetArrayInt`、`GetArrayString`、`GetArrayLen`、`AppendIntegerToArray`、`AppendStringToArray`
- 返回节点：`AppendIntReturn`、`AppendStringReturn`
- 数学节点：`AddInt`、`SubInt`、`MulInt`、`DivInt`、`ModInt`、`RandNumber`
- 流程节点：`Sequence`、`Foreach`、`ForeachIntArray`、`BoolIf`、`GreaterThanInteger`、`LessThanInteger`、`EqualInteger`、`RangeCompare`、`EqualSwitch`、`Probability`
- 事件节点：`CreateTimer`、`CloseTimer`
- 测试节点：`DebugOutput`

## 组合流程用例

复杂旧蓝图流程：

- 入口：`Entrance_IntParam_000001`
- 执行流：`Sequence`
- 分支：`EqualInteger`、`RangeCompare`、`EqualSwitch`
- 嵌套循环：外层 `Foreach`，内层 `ForeachIntArray`
- 数据流：`CreateIntArray`、`AddInt`
- 结果：`AppendIntReturn`、`AppendStringReturn`
- 预期：循环返回顺序稳定，分支命中后继续后续节点。

循环 break 流程：

- 外层节点：`ForLoopBreak`
- 条件：`CompareGreaterInteger` + `BoolIf`
- break 入口：通过 exec 连接跳到 `ForLoopBreak` 的 break input
- 预期：当前循环停止，然后执行 completed 分支。

函数流程：

- 函数文件：`.obpf`
- 主图文件：`.obp`
- 函数节点：`origin.function.entry`、`origin.function.return`、`origin.function.call`
- 预期：函数返回值写回调用点，调用节点之后的执行流继续执行。

异步流程：

- 覆盖文件：`engine/golang/session_test.go`、`engine/golang/sleep_test.go`、`engine/golang/timer_test.go`、`engine/golang/functions_test.go`
- 预期：节点挂起后保存 continuation，回调时恢复同一执行位置；函数内部异步返回时能回到 caller 并继续 caller 后续节点。

## 删除范围验证

已删除范围：

- 文件节点和文件变量类型
- 表格节点和表格变量类型
- 字典节点和字典变量类型
- `origin.flow.foreach-table-row`

覆盖测试：

- `engine/golang/document_test.go`
  - `TestRemovedFileTableDictionaryDocumentNodesAreUnsupported`
- `frontend/tests/removedDataTypes.test.js`
- `app_test.go`
  - `TestMigrateLegacyHidesRemovedFileTableAndDictionaryNodes`

预期：

- 新文档定义中不能再找到 `origin.io.*`、`origin.table.*`、`origin.dictionary.*`。
- Go engine 不能编译已删除的旧 class。
- 前端变量类型只保留 boolean、integer、float、string、array。
- 旧蓝图迁移时，删除节点进入 `Legacy.HiddenNodes`，相关边进入 `Legacy.HiddenEdges`。

## 性能与压测入口

压测代码：

- `engine/golang/benchmark_test.go`
  - `BenchmarkBlueprintDoSharedCompiledGraph`
  - `BenchmarkBlueprintDoComplexSharedCompiledGraph`
  - `BenchmarkBlueprintDoComplexSharedCompiledGraphParallel`
  - `BenchmarkFunctionCall`

压测图特征：

- 编译图只创建一次，所有实例共享同一棵 compiled graph。
- `BenchmarkBlueprintDoComplexSharedCompiledGraph` 创建 4096 个实例，串行轮询执行。
- `BenchmarkBlueprintDoComplexSharedCompiledGraphParallel` 创建 65536 个实例，通过 `b.RunParallel` 并发执行。
- 复杂图包含 sequence、嵌套循环、数组、整数运算、范围分支和返回结果。

快速 smoke：

```powershell
go test ./engine/golang -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=100x -benchmem -count=1
```

长时间压测：

```powershell
go test ./engine/golang -run '^$' -bench 'BenchmarkBlueprintDoComplexSharedCompiledGraph' -benchtime=10s -benchmem -cpu=1,4,8,16 -count=3
```

只测函数调用：

```powershell
go test ./engine/golang -run '^$' -bench 'BenchmarkFunctionCall' -benchtime=10s -benchmem -cpu=1,4,8,16 -count=3
```

解读指标：

- `ns/op`：单次执行耗时，越低越好。
- `B/op`：单次执行分配字节数，服务器高频场景重点关注。
- `allocs/op`：单次执行分配次数，后续优化应优先降低复杂流程和函数调用的分配。
- `-cpu`：观察不同并发度下是否出现明显退化。

## 当前编译期预处理

加载蓝图并执行 `CompileGraph` 时，Go engine 会提前完成以下分析，减少 `Do` 阶段的动态消耗：

- 为每个 `ExecNode` 分配连续 `Index`，`Graph` 执行上下文使用 slice 按下标访问；手工构图的未索引节点保留 fallback。
- 将节点默认输入从 `map[int]any` 预转换为 typed `IPort`，执行时直接复制 port 值。
- 为 `NodeDefinition` 预计算数据输入口下标，执行时只遍历真实数据口。
- 为 `FunctionCall` 预解析目标 `*CompiledGraph`，执行时优先使用编译期指针；递归或后续补挂函数表的场景保留运行期查找 fallback。

## 当前线程安全边界

- `CompiledGraph`、`ExecNode`、`NodeDefinition` 在编译后按只读共享使用，可被多个 create 实例并发执行。
- `Blueprint` 的 graph 表、instance 表、热更新替换、创建、释放、查询和执行入口使用 `RWMutex` 保护。
- `Blueprint.Do` 在读锁内复制 `GraphInstance` 快照，释放锁后创建本次执行私有的 `Graph` session。
- `GraphInstance` 的变量读写通过 `variableMu` 保护。
- `GraphInstance` 的 timer id 表通过 `timerMu` 保护，避免 timer 回调、取消和释放实例并发读写。
- `Graph` 本身仍是单次执行 session，不应被多个 goroutine 直接并发复用；服务器侧应通过 `Blueprint.Do` 并发调用。

线程安全验证：

```powershell
go test -race ./engine/golang -count=1
```

## 标准验证命令

Go engine：

```powershell
go test ./engine/golang -count=1
```

项目 Go 全量：

```powershell
go test ./... 
```

前端结构测试：

```powershell
npm run test:layout
```

前端构建：

```powershell
npm run build
```

删除范围残留扫描：

```powershell
rg -n "origin\.(io|table|dictionary)|foreach-table-row|DataFrame|\bDict\b|RuntimeTable|TableData|FileControl|fileMode|type-file|type-table|type-dictionary" -S graph.go legacy.go execution.go app_test.go engine\golang frontend\src\editor frontend\src\App.vue frontend\src\style.css frontend\tests nodes --glob "!nodes/json/**"
```

允许命中：

- 删除范围测试中的旧节点字符串。
- 前端普通文件管理能力中的 `file` 文案或变量名。

## 当前不处理项

- `nodes/json/**` 业务节点。
- RPC 异步业务节点。
- origin 服务器完整替换接入。
- C# engine。
- Lua engine。
