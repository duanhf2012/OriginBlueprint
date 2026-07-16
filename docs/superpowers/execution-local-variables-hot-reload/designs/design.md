# Execution-local 变量与原子热加载设计

日期：2026-07-16  
状态：已批准设计基线，待实施计划

## 1. 文档定位

本设计恢复老版本已经在线运行的变量语义：普通蓝图变量是单次执行内的局部变量，每次 `Do/Start` 重新初始化，不跨调用持久化。当前 VM 版本将其保存在 `GraphInstance` 并跨 Execution 共享，属于重构后的行为偏差。本设计同时据此把热加载收敛为不可变编译图池的原子替换，并补齐 VM 控制节点错误的 `NodeID/PC` 定位和函数返回路径的编译期校验。

本设计实施后，替代以下既有语义：

- `docs/superpowers/specs/2026-07-10-blueprint-persistence-engine-safety-design.md` 第 8 节“热加载变量快照”及其中“顶层实例变量跨 Execution 持久化”的相关内容。
- 当前 README 中“普通图变量属于 Create 实例、同一实例多次 Execution 共享变量”的说明。

旧设计中与 legacy 文件保存、端口校验、异步生命周期、函数调用帧隔离等无冲突内容继续有效。

## 2. 已确认事实

### 2.1 当前实现

当前 `GraphInstance` 通过 `instanceRuntimeState` 保存：

- 当前 `CompiledGraph`；
- 普通变量 map；
- 普通变量读写锁。

同一个 `graphID` 的多次 `Do/Start` 共享该变量 map。热加载会复制变量快照并替换 `instance.state`；已经开始或挂起的旧 Execution 继续引用旧状态，其在交换后的变量写入不会进入新状态。

### 2.2 历史行为基准

用户已确认，VM 重构前实际运行的老版本遵循以下语义：

- 每次执行蓝图时按默认值重新初始化普通变量；
- 变量只在当前一次执行流内有效；
- 同一次执行内的分支、循环和节点可以共享该次变量；
- 下一次 Do 不继承上一次 Do 修改后的变量值。

因此，Execution-local 不是新增业务能力，而是恢复已在线验证过的兼容行为。当前 `TestBlueprintInstanceVariablesPersistAcrossEntrances` 和 README 中的实例持久变量说明不能作为历史兼容基准，应视为近期重构期间引入的错误契约。

### 2.3 当前业务资产

对 mp1server 当前蓝图资产静态扫描确认：

- 10 个 `.vgf` 定义了非空变量；
- 6 个 `.vgf` 实际使用变量 Getter/Setter；
- 使用方式均表现为单次执行内的临时中间结果，例如 `aimNum`、`HpChange`、`rawDamage`、`attacker`；
- 未发现依赖“上一次 Do 写入、下一次 Do 读取”的蓝图；
- `choiceskill_template.vgf` 只有默认值 Getter，没有对应 Setter，不构成跨执行持久状态依赖。

该扫描降低了语义调整风险，但不能替代实施后的全量资产差分验证。

## 3. 目标

1. 恢复历史兼容语义：普通变量只属于一次 `Execution`，每次 `Do/Start` 从当前编译版本的默认值重新初始化。
2. 同一次 Execution 经过循环、函数、Yield/Resume 后继续保留原变量状态。
3. 不同 Execution 的普通变量完全隔离，包括相同 `graphID` 的并发执行。
4. `CompiledGraph` 保持不可变并由 Execution 捕获；热加载成功后只原子替换编译图池。
5. 旧 Execution 使用旧编译图和旧变量完成，新 Execution 使用新编译图和新变量，不迁移、不合并变量。
6. 普通变量新增、删除或改类型不阻塞在线热加载。
7. 所有 VM 控制节点执行错误稳定携带 `NodeID`、`NodeName` 和 `PC`。
8. 函数图的确定性无返回路径尽量在编译阶段拒绝，运行时仍保留最终保护。
9. 正常执行热路径性能不下降；解析、类型绑定和变量索引计算放在编译阶段。

## 4. 非目标

- 本轮不实现跨 Do/Start 共享的全局变量。
- 本轮不提供 Execution 结束后的普通变量读取 API。
- 本轮不把普通变量变化合并回宿主、Actor、数据库或缓存。
- 本轮不修改蓝图文件中的变量定义格式，也不要求重写现有 `.vgf/.obp/.obpf`。
- 本轮不修改入口参数、返回值、循环、函数调用栈和 YieldHandle 的外部契约。
- 本轮不顺带收紧 `Any`、浮点转整数或通用数组转换规则；这些兼容风险单独设计和验证。
- 本轮不实现跨版本 Execution 的业务副作用事务；外部 RPC、数据库和宿主对象仍由业务层保证幂等与顺序。

## 5. 变量作用域

### 5.1 普通图变量

普通图变量采用 Execution-local 语义：

```text
Start/Do
  -> 捕获当前 CompiledGraph
  -> 从编译期变量模板创建 Execution 私有变量
  -> 执行 / Yield / Resume 复用同一份变量
  -> Completed / Failed / Canceled 后释放
```

规则固定为：

- 每次 `Blueprint.Start` 创建全新的普通变量。
- `Blueprint.Do/DoContext` 通过 Start 执行，因此每次调用同样创建全新的普通变量。
- 同一次 Execution 内的 Getter/Setter 读取和修改同一份变量。
- Yield 时变量随 Graph/VM 状态保留；Resume 不重新初始化。
- 循环体挂起恢复、函数调用挂起恢复均不得重置所属作用域的变量。
- 相同 `graphID` 的并发 Execution 使用不同变量容器。
- Execution 终态释放变量引用，不写回 `GraphInstance`。

### 5.2 低层 Graph.Do

低层 `Graph.Do` 每次入口调用也必须重新初始化顶层变量，防止复用同一个 Graph 对象时残留上一次执行状态。

低层 Graph 仍禁止并发复用；该限制与变量是否带锁无关。可能 Yield 的图仍禁止使用低层 `Graph.Do`。

### 5.3 函数变量

函数变量继续保持调用帧局部语义：

- 每次函数调用独立初始化；
- 递归调用的每一层独立；
- 函数挂起恢复保留当前调用帧变量；
- 函数返回后释放；
- 不与调用方普通变量隐式共享。

### 5.4 未来全局变量边界

未来如需跨 Do/Start 持久状态，必须单独设计“全局变量”，至少明确：

- scope 标识和编辑器展示；
- 所属对象和生命周期；
- 并发读改写语义；
- 热加载迁移规则；
- 持久化、快照和恢复方式；
- 对外读取、修改和权限接口。

全局变量不得复用本轮 Execution-local 变量容器，也不得通过恢复旧 `GraphInstance.variables` 隐式实现。

## 6. 编译期变量计划

变量类型解析、默认值转换和节点绑定在编译阶段完成。采用以下内部结构：

```go
type VariablePlan struct {
    Name     string
    Template IPort
}

type CompiledGraph struct {
    // 现有只读字段……
    variablePlans []VariablePlan
}

type ExecNode struct {
    // 现有只读字段……
    VariableIndex int
}

type Graph struct {
    // 单次 Execution 状态……
    variables []IPort
}
```

具体约束：

1. 编译器验证变量名称唯一、类型受支持、默认值可转换。
2. 编译器按稳定顺序生成 `VariablePlan`，并把 Getter/Setter 绑定到固定下标。
3. `CompiledGraph` 返回后，模板、下标和节点计划不可变。
4. Execution 初始化只 clone 模板，不再解析类型字符串或查找变量名称。
5. 无变量图不分配变量 slice。
6. Getter/Setter 使用下标直接访问，不做字符串 map 查找。
7. Setter 使用类型安全赋值保持槽位声明类型，不用输入端口替换槽位类型。
8. Execution 调度保证同一 VM 片段串行运行，普通变量不再需要实例级 `RWMutex`。

变量定义目前只支持引擎内建端口，因此模板 clone 不依赖未知自定义端口语义。如未来允许自定义变量类型，必须要求其 `Clone` 满足 Execution 隔离契约。

## 7. GraphInstance 与 Start

`GraphInstance` 不再保存编译图和变量状态，只保留实例身份与生命周期：

- graph name；
- graphID；
- module/宿主引用；
- released 状态、release 信号和现有生命周期信息。

`Start` 在 `Blueprint` 锁内完成线性化：

1. 验证 Blueprint 未关闭、实例存在且未释放。
2. 通过 `instance.name` 从当前 `b.graphs` 获取最新 `CompiledGraph`。
3. 验证入口存在。
4. 创建 Execution 和顶层 Graph，并捕获该 `CompiledGraph` 指针。
5. 将 Execution 登记到活动表。
6. 解锁后监听 Context 并提交初始任务。

Start 与热加载应用共用同一把 Blueprint 锁，因此只有两种确定结果：

- Start 先取得锁：捕获旧版本；
- 热加载先取得锁：Start 捕获新版本。

不存在一条 Execution 混用新旧 Program、NodePlan 或函数图的状态。

`Create` 仍返回稳定 graphID，用于绑定蓝图名称、宿主 module 和生命周期；它不再创建或持有变量状态。

## 8. 原子热加载

### 8.1 准备阶段

`prepareHotReload` 在 Blueprint 锁外完成：

- 读取节点定义和全部蓝图文件；
- 严格解析；
- 编译普通图和函数图；
- 绑定函数目标；
- 执行函数流契约校验；
- 构建完整的新 `map[string]*CompiledGraph`。

任一文件失败时返回结构化错误，不改变当前图池。

### 8.2 应用阶段

全部成功后短暂持有 Blueprint 锁：

```go
b.graphs = newGraphs
```

应用阶段不再：

- 遍历实例迁移变量；
- clone 旧变量；
- 替换 `instance.state`；
- 等待、取消或修改活动 Execution。

旧 Execution 继续持有旧 `CompiledGraph` 和自己的变量。新 Execution 从新图池取得新 `CompiledGraph` 并初始化新变量。旧版本无引用后由 Go GC 回收，不增加手工引用计数。

### 8.3 删除与重新加入图

新图池删除某个 graph name 时：

- 已经开始的 Execution 继续完成；
- 原 graphID 的下一次 Start/Do 返回 `ErrGraphNotFound`；
- GraphInstance 句柄保留到 `ReleaseGraph`，便于宿主保持生命周期对称；
- 后续热加载重新加入同名图后，该未释放 graphID 可以再次启动新 Execution。

### 8.4 热加载结果

`UpdatedInstances/UnchangedInstances` 依赖旧实例状态迁移语义，实施后没有真实含义，应删除。`HotReloadResult` 只保留本次成功发布的 `GraphCount`。

mp1server 中 `BlueprintModule` 的日志和测试同步删除两个实例计数字段，不保留永远为零的兼容字段。

## 9. Init 原子性

`Init` 必须先在局部变量中完成 Registry、节点定义和全部图的解析编译，成功后再一次性写入：

- module；
- logger/trace logger；
- execDefPath；
- graphPath；
- 完整 graphs map。

失败时 Blueprint 保持调用前状态，禁止出现路径已更新但图池仍是旧版本的部分初始化。

`Init` 定位为无运行实例时的完整初始化入口，规则固定为：

- Blueprint 已关闭时返回 `ErrBlueprintClosed`。
- 存在任意 GraphInstance 或活动 Execution 时返回新增的稳定错误 `ErrBlueprintInUse`，不得用 Init 代替 HotReload。
- 没有实例和活动 Execution 时允许重复 Init；成功后完整替换旧图池，不做逐项 merge。
- Init 在锁外解析编译后，应用前必须重新检查 closed、instances 和 executions，防止解析期间并发 Create/Start 使前置检查失效。
- 应用前复检失败时丢弃本次编译结果，调用前的图池、module 和路径保持不变。

## 10. VM 错误补齐 NodeID

当前 Native 节点错误已经能构造 `BlueprintError`，但 Sequence、Loop、FunctionCall、FunctionReturn 等控制指令错误常直接返回普通 error，最终缺少节点定位。

VM 每次取指令后先捕获：

- dispatch 前的 Graph；
- PC；
- NodePlan/ExecNode。

任一 opcode handler 返回错误时：

1. 如果错误已是 `BlueprintError`，只补充缺失字段，不重复嵌套。
2. 否则按 dispatch 时捕获的 graph/node/PC 构造 `BlueprintError`。
3. 函数返回 handler 即使已经切回调用方，也必须报告产生错误的 callee Return 节点。
4. 最终由 Execution 补充 graphID、entranceID 和 executionID。
5. DiagnosticSink 接收与 `Execution.Result` 同源的结构化错误。

该逻辑只在错误分支构造对象，正常 VM 热循环不增加分配。

解析和编译阶段在已知节点 ID 时也应写入 `BlueprintError.NodeID`；跨文件冲突至少必须携带 stage 和 source path。

## 11. 函数返回路径校验

现有“图中存在 FunctionReturn”不足以证明入口可达，也不足以证明所有可终止路径返回。编译阶段增加 continuation-aware 流分析：

- 从 FunctionEntry 开始分析可达执行流；
- FunctionReturn 是合法函数终点；
- Native 分支、Sequence 后续目标、循环 body/continue/completed、函数调用返回都按 VM 实际 continuation 语义建模；
- 任一路径在空 continuation 下结束且未经过 FunctionReturn，报告对应节点和路径类型；
- 不可达 FunctionReturn 不能满足函数契约；
- 无限循环路径不被误判为“无返回终止”，仍由执行预算保护；
- 递归或互相递归调用继续受函数目标契约和最大调用深度保护。

校验必须保守：只拒绝确定无效的函数，不能因为 Sequence、合法循环或 legacy fanout 的 continuation 建模不足而误拒绝现有合法资产。

运行时 `function ... completed without FunctionReturn` 保留为最终不变量保护，不能因为增加编译检查而删除。

## 12. 兼容性与代码清理

### 12.1 兼容行为恢复

以下当前 VM 行为明确移除：

- 相同 graphID 的不同 Do/Start 共享普通变量；
- 不同入口通过普通变量传递跨调用状态；
- 热加载迁移实例变量当前值。

这些行为不是老版本线上语义，移除后恢复为每次 Do 初始化局部变量。需要跨调用状态的业务必须放到宿主对象或未来明确设计的全局变量中。

### 12.2 文件兼容

- `.vgf/.obp/.obpf` 变量声明格式不变；
- Getter/Setter 节点和端口编号不变；
- 现有蓝图无需重保存；
- 行为变化只发生在 Execution 边界。

### 12.3 删除清单

实施后删除：

- `instanceRuntimeState`；
- `newInstanceRuntimeState`；
- `migrateInstanceRuntimeState`；
- `GraphInstance.state`；
- 顶层实例变量 map 和 `variableMu`；
- 热加载变量迁移代码；
- `TestBlueprintInstanceVariablesPersistAcrossEntrances` 及其他快照迁移断言；
- `HotReloadResult.UpdatedInstances/UnchangedInstances`；
- README 和规则文件中的实例变量持久化说明。

测试不是简单删除，而是替换为新的 Execution-local 契约测试。

## 13. 性能约束

预期变化：

- 无变量图：不得新增变量 map/slice 分配。
- 有变量图：每次 Execution clone 少量编译模板。
- Getter/Setter：由字符串 map + RWMutex 改为 slice index，无锁访问。
- HotReload：取消逐实例变量 clone，应用锁持有时间缩短。
- Start：仍在既有 Blueprint 锁内完成查找，只是从 `instance.state.compiled` 改为 `b.graphs[instance.name]`，不新增锁次数。

基准门禁：

- 无变量同步图的 `allocs/op` 不得增加；
- 常用复杂图的 `ns/op` 中位数回退不得超过 10%；
- 有变量图单独记录初始化成本，并与删除锁/map 查找后的总成本比较；
- 异步恢复不重复初始化变量；
- Race 检查必须覆盖同 graphID 并发执行和热加载并发 Start。

变量运行容器固定采用 slice 下标方案，不保留 Execution 私有字符串 map 作为第二套实现，避免同一语义存在双路径。若基准发现模板 clone 成本异常，只允许优化模板布局和初始化策略，不得恢复实例共享状态。

## 14. 测试设计

### 14.1 普通变量

- 同一次 Execution 中 Set 后 Get 得到新值。
- 同一 graphID 连续两次 Do，第二次读取默认值。
- 同一 graphID 不同入口不共享变量。
- 两个并发 Execution 分别修改变量，结果互不污染。
- 无变量图不创建变量容器。
- Execution 完成、失败、取消后释放变量引用。

### 14.2 Yield、循环和函数

- Yield 前写变量，Resume 后读到相同值。
- 循环体 Yield/Resume 保留变量且循环次数不变化。
- 函数内 Yield/Resume 保留当前函数调用变量。
- 连续两次函数调用从默认值开始。
- 递归函数各层变量隔离。

### 14.3 热加载

- 旧 Execution 挂起后热加载，新 Execution 使用新逻辑，旧 Execution 恢复后仍使用旧逻辑。
- 新旧版本变量同名、改类型、新增、删除时均互不影响。
- 热加载后同一 graphID 的新 Do 从新默认值初始化。
- 删除图后旧 Execution 可完成，新 Start 返回 `ErrGraphNotFound`。
- 同名图重新加入后未释放 graphID 可再次执行。
- 解析或编译失败不改变图池、路径和 module。
- 存在实例或活动 Execution 时调用 Init 返回 `ErrBlueprintInUse`，不改变现有状态。
- 无实例且无活动 Execution 时重复 Init 完整替换图池，不保留已删除的旧图。
- Start 与 apply 并发时，每个 Execution 只捕获一个完整版本。

### 14.4 诊断和函数契约

- Sequence、Range/Array/While/Breakable Loop、FunctionCall、FunctionReturn 错误都包含正确 NodeID 和 PC。
- callee 返回绑定失败报告 callee Return 节点，而不是错误归属到 caller。
- FunctionReturn 不可达时编译失败。
- 分支存在确定 fallthrough 时编译失败。
- Sequence、循环、递归和合法 fanout 不被误拒绝。
- 运行时无返回保护仍有测试。

### 14.5 业务资产与集成

- 当前全部线上 `.vgf` 加载编译通过。
- 变量使用图使用边界值和固定 seed 随机输入，与独立 Go 参考实现多轮对比。
- 全部现有验证蓝图执行结果保持一致。
- mp1server `BlueprintModule` 热加载日志和测试适配新的 HotReloadResult。
- `BattleMonster.go` 等现有 Do 返回值调用方编译与测试通过。

## 15. 验收命令

引擎至少执行：

```powershell
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./engine/go/blueprint -count=20
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex|Parallel)|BenchmarkFunctionCall' -benchmem -count=5
```

mp1server 至少执行：

```powershell
go test ./common/blueprint -count=1
go test ./service/battleservice/battleobject -run '^$' -count=1
```

随后执行项目可用的更宽编译/测试门禁，并单独报告环境依赖失败与本轮回归。

## 16. 风险与缓解

- **近期调用方可能误用当前跨 Do 共享行为**：历史线上语义和当前资产均不依赖该行为；仍需扫描 Go 调用方、执行全量随机差分，并在 README 明确这是兼容行为恢复。
- **每次执行初始化增加分配**：编译变量模板、无变量零分配、下标访问和基准门禁。
- **Resume 误重置变量**：初始化只发生在创建顶层 VM/函数调用帧时，resumeYield 不进入初始化路径。
- **低层 Graph.Do 残留变量**：每次新入口 VM 初始化时强制重建顶层变量。
- **热加载与 Start 竞态**：两者在同一 Blueprint 锁下线性化，Execution 捕获单一不可变版本。
- **删除图改变旧实例语义**：旧执行允许完成，新调用明确返回 ErrGraphNotFound；README 和错误索引同步更新。
- **函数流静态分析误报**：使用 continuation-aware 模型，只拒绝确定 fallthrough，保留运行时兜底并用现有合法资产回归。
- **公开 HotReloadResult 变更**：同步更新 mp1server 调用方、日志和测试；不保留无语义字段。
- **诊断包装影响 errors.Is/As**：BlueprintError 继续实现 Unwrap，已有结构化错误只补字段不重复包装。

## 17. 固定决策

以下决策已经确定，实施计划不得再次引入旧语义：

1. 普通变量是 Execution-local。
2. 全局变量延期，且未来必须显式建模。
3. 热加载原子替换不可变编译图池，不迁移普通变量。
4. 旧 Execution 完成旧版本，新 Execution 使用新版本。
5. 变量增删改类型允许随新编译版本生效。
6. 普通变量不跨 Do/Start 保存。
7. 控制节点错误补齐 NodeID/PC，且不增加成功路径分配。
8. 函数编译校验只拒绝确定无返回路径，运行时兜底保留。
9. Execution-local 是恢复老版本线上语义，不定义为新的不兼容业务行为。
