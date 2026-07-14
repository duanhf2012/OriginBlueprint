# Codex 蓝图引擎维护规则

本文档记录 Go 蓝图解析执行引擎的维护规则，供后续 Codex 或其他 agent 修改本库时优先阅读。规则覆盖 `engine/go/blueprint/`，以及和它相关的顶层 `nodes/*.json`、兼容迁移、测试和压测。

## 1. 当前范围

- 当前只实现 Go engine。
- C#、Lua engine 暂不处理。
- `nodes/*.json` 是基础系统节点定义，需要覆盖。
- `nodes/json/**` 是业务节点定义，当前明确忽略，除非用户单独要求。
- RPC 等业务异步节点通过 VM `YieldHandle` 接入；Core 不提供 Delay/Timer 调度实现。
- 文件、表格、字典相关蓝图数据类型和节点已经删除，不得无意恢复。

## 2. 核心架构规则

- `CompiledGraph.Program` 是编译后的共享只读 VM 指令与控制流表。
- `ExecNode` 和 `NodeDefinition` 是编译期构建、执行期只读的结构。
- 每个 `Blueprint.Create` 实例拥有自己的变量状态。
- 每次 `Blueprint.Start`、`Blueprint.Do` 或 `Blueprint.DoContext` 都会创建私有 `Graph` execution session。
- `vmMachine` 保存本次执行的 PC、FlowStack、LoopStack、CallStack、节点上下文和返回值。
- 不要把单次执行状态写回 `CompiledGraph`、`ExecNode` 或 `NodeDefinition`。
- 不要直接复用同一个 `Graph` 做并发执行；服务器侧并发应通过 `Blueprint.Do`。

## 3. 线程安全规则

- `Blueprint` 是对外并发安全 facade。
- `Blueprint.graphs`、`Blueprint.instances`、热更新替换、创建、释放、查询和执行入口必须受 `RWMutex` 保护。
- `Blueprint.Start` 捕获 `GraphInstance` 快照后通过 Dispatcher 启动。默认 Dispatcher 异步执行；Actor-aware Dispatcher 的初始执行在当前 Actor 内同步完成，Yield 恢复投递回 Actor 队列。
- `Blueprint.Do` 和 `Blueprint.DoContext` 只作为等待 `Execution.Done()` 的阻塞便利封装；服务器事件循环应使用 `Start`。
- `GraphInstance` 通过可替换的 runtime state 持有 compiled graph、变量表和对应的变量锁；一次执行只捕获一个 state 指针。
- Core 不得重新引入 Delay、TimerScheduler 或 Timer 注册表。需要时间语义时，由业务/stdlib 节点持有宿主调度器并最终调用 `YieldHandle.Resume/ResumeTo`。
- `YieldHandle` 必须保证只 resume 一次。
- 异步挂起后，原 goroutine 不应继续读取可能被回调 goroutine 修改的 session 状态。
- 自定义节点如果持有共享状态，需要由节点实现方自行加锁。

线程安全验证命令：

```powershell
go test -race ./engine/go/blueprint -count=1
```

如果修改 `Blueprint` facade、热更新或实例生命周期，还要跑：

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
- VM 挂起期间，相关 `Graph`、`ExecContext`、port 和各类 VM 栈状态不能归还池。
- `Graph` 可以复用自身的节点 context 帧，但必须隔离重入调用；挂起 context 在 Yield 恢复或 Execution 终止前必须保持占用。
- context 缓存不得长期持有 string、array、any、函数返回值等动态对象；完成、失败或取消时必须清理引用。

性能验证命令：

```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=3s -benchmem -count=1
```

## 5. 异步与函数规则

- 异步节点调用 `Yield(nextPort)` 获取一次性句柄，并返回 `ErrExecutionSuspended`；需要按结果选择分支时调用 `ResumeTo(nextPort, outputs...)`。
- `Yield` 是 Native 节点的终止边界：成功取得句柄后必须立即返回 `ErrExecutionSuspended`，不得依赖恢复后继续执行 `Exec()` 中 Yield 调用之后的 Go 语句。
- `ResumeTo` 的 `nextPort` 是当前节点的 Exec 输出端口下标，`outputs` 按数据输出端口顺序写回。
- Yield 只能成功恢复一次。RPC 等宿主回调必须处理重复恢复、Execution 取消和实例释放错误。
- Yield 恢复必须通过 Execution 启动时捕获的 Dispatcher，不得在 RPC 回调 goroutine 内直接继续执行节点链。
- 恢复继续使用同一个 `vmMachine`，PC、FlowStack、LoopStack、CallStack 与 EvalStack 不得重建。
- 函数调用使用显式 CallStack；每次调用拥有独立函数 Graph/变量状态，返回时将输出映射回 caller 槽位。
- 函数内部异步返回时，caller 必须在函数返回后继续后续节点。
- 递归函数调用必须受 `MaxFunctionCallDepth` 限制。
- 顶层 Execution、嵌套函数、数据节点、结构化循环和 Yield 恢复必须共享同一执行步数预算，异步挂起不得重置预算。
- 热加载只迁移同名且规范化类型一致的变量值；新变量使用新默认值，删除变量消失，改类型变量重置。进行中的旧 session 继续使用旧 runtime state。

## 5.1 加载与编译边界

- 动态节点定义必须在分配大 slice 或构造完整对象前校验计数和 `port_id` 上限。
- 当前单节点总端口上限为 4096，动态 Sequence 输出上限为 256，函数输入和输出分别最多 128。
- 端口 key 需要按运行时规范化规则检查冲突；重复 `port_id`、规范化 key 冲突和函数签名冲突必须报错。
- 新格式图不得包含数据依赖环、非结构化 exec 环或绕过结构化循环的 break 回边。
- legacy 图允许保留历史 exec 环和多出边；执行时必须受总步数预算保护，多出边按稳定的深度优先顺序执行。
- 加载目录时 graph name、function id、function name 和路径别名不得由不同文件静默覆盖，错误信息必须包含冲突双方来源。

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
