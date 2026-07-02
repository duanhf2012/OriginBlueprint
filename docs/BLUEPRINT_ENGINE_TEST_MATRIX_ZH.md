# Blueprint Engine 测试矩阵

本文档记录当前 Go 版解析执行引擎的测试用例设计、覆盖范围和压测入口。范围覆盖顶层 `nodes/*.json`、业务 `nodes/json/**/*.json`、编辑器保存出的 `.obp/.obpf`、历史 `.vgf` 迁移文件，以及当前 `engine/go/blueprint` 能解析和执行的功能；RPC 业务节点、origin 服务器完整替换接入、C#、Lua 均不在本轮处理。

## 测试目标

- 验证旧蓝图 `.vgf` 能继续加载、编译和执行。
- 验证旧 `.vgf` 与新 Go 解析/执行库兼容；兼容标准是 Go 库能正确加载、编译、执行并得到预期结果，不要求和旧编辑器导出的文件逐字一致。
- 验证新蓝图 `.obp/.obpf` 的函数入口、函数返回、函数调用后续执行。
- 验证由节点定义组装、连线并保存出的 `.obp/.obpf` 能被 Go engine 正确执行。
- 验证异步 continuation 机制能在 sleep/timer 类节点回调后继续当前位置。
- 验证 `nodes/**/*.json` 中所有基础系统节点和已恢复业务节点均有加载覆盖，关键节点有组合行为覆盖。
- 验证已删除的文件、表格、字典相关节点和变量类型不会重新暴露。
- 提供服务器高频执行的 benchmark 入口，用于观察共享编译树、多实例执行、并发执行的耗时和分配。

## 兼容判定口径

本项目当前的旧格式兼容目标是“旧 `.vgf` 能兼容新的 Go 解析和执行库”，而不是“新导出的 `.vgf` 与旧编辑器导出的文件字节完全一致”。

测试中应严格比较语义字段：

- 节点 class/name 到 Go 节点定义的映射。
- 旧 `port_id` 到新 port key 的映射。
- `port_defaultv` 到新 `values` 的转换结果。
- exec/data 连线拓扑。
- 入口、函数、变量、动态分支的执行语义。
- Go engine 执行结果。

测试中不应把这些非语义字段作为失败条件：

- JSON 字段顺序。
- 自动生成的节点 id、连线 id。
- 保存时间。
- 画布坐标的微小差异。
- 编辑器内部展示顺序。

如需对比 `.vgf` round-trip，应先 canonicalize，再比较 class、module、端口编号、默认值和连线拓扑。

## 顶层节点覆盖

覆盖来源：

- `nodes/Base.json`
- `nodes/Entrance.json`
- `nodes/Event.json`
- `nodes/Math.json`
- `nodes/SysFlowControl.json`
- `nodes/Test.json`

业务节点覆盖来源：

- `nodes/json/**/*.json`

覆盖要求：

- 每个业务 JSON 至少进入一次 schema 加载覆盖，防止旧业务节点退化成 `origin.legacy.placeholder`。
- 旧 `.vgf` 中实际出现过的业务 class 必须能映射到 schema、静态 legacy spec，或明确进入允许的 hidden/fallback 清单。
- 代表性业务节点需要进入组合蓝图执行用例，至少覆盖入口参数、业务数据输出、数组/分支连接和函数调用边界。

覆盖测试：

- `engine/go/blueprint/system_nodes_test.go`
  - `TestBuiltinFactoriesCoverAllTopLevelNodeDefinitions`
  - `TestTopLevelSystemNodeBehaviorCoverage`
  - `TestBuiltinMathNodes`
  - `TestBuiltinArrayNodes`
  - `TestBuiltinBranchNodes`
  - `TestBuiltinEntranceTimerAndDebugNodes`
  - `TestBuiltinReturnNodesAppendGraphResults`
- `engine/go/blueprint/flow_integration_test.go`
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

新增蓝图文件生成原则：

- 测试不得直接手写 `.obp/.obpf` JSON 文件。
- 测试应通过“组图动作”生成蓝图：从节点定义创建节点、设置输入默认值、模拟连线、保存文件、重新加载文件。
- 组图动作可以是 headless 测试工具，也可以是少量 Playwright UI 烟测；核心要求是复用编辑器和 Go 侧同一套 schema/端口规则，而不是绕过保存链路。

建议测试产物目录：

- `testdata/generated/obp/`：由测试组图生成的新格式蓝图。
- `testdata/generated/vgf/`：由兼容导出生成的旧格式蓝图。
- `testdata/golden/`：canonical 后的期望结构。
- `testdata/golden/results/`：Go engine 执行结果期望值。

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

函数嵌套流程：

- 主图文件：`.obp`
- 函数文件：`func_a.obpf`、`func_b.obpf`
- 调用关系：主图调用 `func_a`，`func_a` 内部调用 `func_b`
- 预期：函数入参、返回值、局部变量互不污染；`func_b` 返回后继续 `func_a`，`func_a` 返回后继续主图。
- 反例：函数递归或循环依赖应在加载、编译或执行前给出明确错误，不能静默死循环。

业务入口流程：

- 入口：怪物选择技能入口、怪物被攻击入口、怪物回合开始入口、Buff 相关入口。
- 数据流：入口参数连接到业务节点，再进入分支、数组和返回节点。
- 预期：旧 `.vgf` 打开后入口节点不丢，入口参数 port 不乱序；保存为 `.obp` 后 Go engine 仍可按入口参数执行。

旧 `.vgf` 兼容流程：

- 输入：`build/bin/vgf/**/*.vgf` 中的代表性样例，例如 `choiceskill_easy.vgf`。
- 流程：旧 `.vgf` -> Go 迁移 -> 校验 -> 编译 -> 执行；必要时再导出 legacy `.vgf` 做 canonical round-trip。
- 预期：Go 解析/执行库能加载并运行；已知断开的线上样例允许产生明确 validation warning，但不能丢失已知业务节点、入口节点和连线。

异步流程：

- 覆盖文件：`engine/go/blueprint/session_test.go`、`engine/go/blueprint/sleep_test.go`、`engine/go/blueprint/timer_test.go`、`engine/go/blueprint/functions_test.go`
- 预期：节点挂起后保存 continuation，回调时恢复同一执行位置；函数内部异步返回时能回到 caller 并继续 caller 后续节点。

## 删除范围验证

已删除范围：

- 文件节点和文件变量类型
- 表格节点和表格变量类型
- 字典节点和字典变量类型
- `origin.flow.foreach-table-row`

覆盖测试：

- `engine/go/blueprint/document_test.go`
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

- `engine/go/blueprint/benchmark_test.go`
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
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=100x -benchmem -count=1
```

长时间压测：

```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDoComplexSharedCompiledGraph' -benchtime=10s -benchmem -cpu=1,4,8,16 -count=3
```

只测函数调用：

```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkFunctionCall' -benchtime=10s -benchmem -cpu=1,4,8,16 -count=3
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
- 为每个已连接或有默认值的数据输入口预生成 `InputBinding`，执行时直接按绑定复制默认值或上游输出，减少热路径分支和 map 查询。
- 为 `FunctionCall` 预解析目标 `*CompiledGraph`，执行时优先使用编译期指针；递归或后续补挂函数表的场景保留运行期查找 fallback。
- `Graph` 执行上下文 slice 按容量复用并在每次 `Do` 前清空，减少复用 `Graph` 或创建短生命周期 session 时的重复分配。
- 内置 `*Port` 克隆走具体类型快速路径，避免接口分发；自定义 `IPort` 保留原有 `Clone` fallback。
- 执行流程 trace 默认关闭；打开 `Blueprint.SetTraceEnabled(true)` 且设置 `BlueprintTraceLogger` 后，才记录节点步骤、输入端口和输出端口。

## 执行流程日志验证

覆盖测试：

- `engine/go/blueprint/trace_test.go`
  - `TestBlueprintTraceDisabledByDefault`
  - `TestBlueprintTraceLogsNodeStepsInputsAndOutputsWhenEnabled`

预期：

- 默认关闭时不生成任何节点 trace 事件。
- 打开后按实际执行顺序记录 `Step`、图名、图实例 ID、节点 ID、节点名称、执行输入口、下一个执行分支、输入端口值、输出端口值和错误文本。
- 函数子图继承 caller 的 trace 状态，异步 continuation 后续节点仍走同一 trace logger。

## 老接口替换验证

覆盖测试：

- `engine/go/blueprint/compatibility_test.go`
  - `TestBlueprintLegacyFacadeIntegrationPath`
  - `TestBlueprintReleaseGraphCancelsInstanceTimersThroughLegacyCallback`

覆盖内容：

- 使用旧 facade 路径完成 `RegisterExecNode`、`Init`、`Create`、`TriggerEvent`、`Do`、`ReleaseGraph`、`StartHotReload`、`GetLogger`、`GetGraphName`。
- `ReleaseGraph` 会清理实例 timer；有 module 时走 `IBlueprintModule.CancelTimerId`，无 module 时走旧 `cancelTimer` 回调。
- 详细接入清单见 `docs/BLUEPRINT_ENGINE_COMPATIBILITY_ZH.md`。

## 蓝图生成到 Go 执行全链路验证

推荐新增一组专门的 compatibility/golden 测试：

1. 读取 `nodes/**/*.json` 并注册 schema。
2. 使用测试组图器按用例创建节点、设置默认值、连接 exec/data 端口。
3. 保存为 `.obp/.obpf`。
4. 重新加载保存后的文件，验证节点数、端口、默认值、连线拓扑。
5. 对需要旧格式兼容的用例导出 `.vgf`，再用 Go 迁移路径重新读入。
6. 调用 `engine/go/blueprint` 编译并执行。
7. 比较执行返回值、变量变化、trace 关键步骤和错误信息。

建议优先落地这些用例：

- 基础数学和顺序执行：入口 -> `Sequence` -> `AddInt/SubInt/MulInt/DivInt` -> return。
- 动态分支：`EqualSwitch`、`RangeCompare`、数组 item 增删后左右端口同步。
- 数组遍历：`CreateIntArray` -> `ForeachIntArray` -> 累加/返回。
- 业务入口参数引用：多个入口使用不同参数名，保存后来源不混淆。
- 函数调用：主图调用函数、函数调用函数、函数内异步/return 后继续 caller。
- 旧 `.vgf` 样例：`choiceskill_easy.vgf` 和至少一个 battle/buffskill 样例。
- fallback 边界：未知旧节点只在允许清单中 fallback，已知业务节点不得 fallback。

这组测试的失败信息应说明是哪一层失败：

- schema 加载失败。
- 组图保存失败。
- 重新加载后结构不一致。
- legacy 迁移失败。
- Go 编译失败。
- Go 执行结果不一致。
- 显示契约回归，例如标题显示为 `name/class`、大量节点被染成兼容黄色、动态输出端口丢失。

## 当前线程安全边界

- `CompiledGraph`、`ExecNode`、`NodeDefinition` 在编译后按只读共享使用，可被多个 create 实例并发执行。
- `Blueprint` 的 graph 表、instance 表、热更新替换、创建、释放、查询和执行入口使用 `RWMutex` 保护。
- `Blueprint.Do` 在读锁内复制 `GraphInstance` 快照，释放锁后创建本次执行私有的 `Graph` session。
- `GraphInstance` 的变量读写通过 `variableMu` 保护。
- `GraphInstance` 的 timer id 表通过 `timerMu` 保护，避免 timer 回调、取消和释放实例并发读写。
- `Graph` 本身仍是单次执行 session，不应被多个 goroutine 直接并发复用；服务器侧应通过 `Blueprint.Do` 并发调用。

线程安全验证：

```powershell
go test -race ./engine/go/blueprint -count=1
```

## 标准验证命令

Go engine：

```powershell
go test ./engine/go/blueprint -count=1
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
rg -n "origin\.(io|table|dictionary)|foreach-table-row|DataFrame|\bDict\b|RuntimeTable|TableData|FileControl|fileMode|type-file|type-table|type-dictionary" -S graph.go legacy.go execution.go app_test.go engine\go\blueprint frontend\src\editor frontend\src\App.vue frontend\src\style.css frontend\tests nodes --glob "!nodes/json/**"
```

允许命中：

- 删除范围测试中的旧节点字符串。
- 前端普通文件管理能力中的 `file` 文案或变量名。

## 当前不处理项

- RPC 异步业务节点。
- origin 服务器完整替换接入。
- C# engine。
- Lua engine。
