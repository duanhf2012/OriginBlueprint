# OriginBlueprint 中文使用手册整合设计

## 1. 目标

建立一份面向蓝图编辑者、Go 节点开发者和宿主集成者的唯一权威中文手册：

- 从编辑器角度解释节点定义、端口字段和常见节点类型的创建方式。
- 从 Go 角度解释节点实现、注册、图加载、实例创建、同步与异步执行。
- 完整说明 `YieldHandle`、`Resume`、`ResumeTo`、RPC callback 和定时器 callback。
- 将 Actor 作为宿主 Dispatcher 的一个适配示例，不把通用库绑定到 Actor。
- 集中说明必须、建议和禁止用法。
- 删除相互冲突、已废弃或重复的顶层说明文档，消除多入口问题。

## 2. 行为基准

新手册只以以下事实为准：

1. 当前 `engine/go/blueprint` 源码及其测试。
2. 当前前端节点 schema、`GraphDocument` 和节点加载实现。
3. 当前 `nodes/*.json` 与验证蓝图。

旧文档只用于提取仍然有效的背景，不继承与代码冲突的说明。具体基准包括：

- 执行器为 VM：`Program + PC + FlowStack + LoopStack + CallStack`。
- 业务异步节点使用 `BaseExecNode.Yield` 和一次性 `YieldHandle`。
- Core 不实现 Delay/Timer 调度器。
- `nodes/Event.json` 中旧 Delay/Timer 类型只用于兼容识别，执行时不受支持。
- File、DataFrame/Table、Dict 相关运行时类型已经删除。
- `Blueprint` 是并发安全 facade；低层 `Graph` 只属于一次执行，不能跨 goroutine 复用。

## 3. 输出文件

新建：

- `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md`

保留：

- `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`：自动生成的验证结果。
- `docs/superpowers/**`：历史设计和实施记录，不作为用户入口。

更新：

- `AGENTS.md`
- `engine/go/blueprint/AGENTS.md`
- `README.md`
- `README_CN.md`

两个 `AGENTS.md` 统一将新手册列为当前行为与使用基准；中英文 README 统一只保留新手册和自动验证报告两个文档入口，不再引用被删除文档。

## 4. 删除清单

- `docs/AI_PROJECT_CONTEXT_ZH.md`
- `docs/ARCHITECTURE.md`
- `docs/BLUEPRINT_CHANGE_SAFETY_ZH.md`
- `docs/BLUEPRINT_ENGINE_COMPATIBILITY_ZH.md`
- `docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`
- `docs/BLUEPRINT_TIMER_DESIGN_ZH.md`
- `docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`
- `docs/LEGACY_COMPATIBILITY_ZH.md`
- `docs/MAINTENANCE_GUIDE_ZH.md`
- `docs/NODE_JSON_FORMAT_ZH.md`
- `docs/ORIGIN_NODE_EDITOR_PARITY.md`

## 5. 新手册目录

1. 文档定位与阅读路线
2. OriginBlueprint 架构和核心概念
3. 五分钟快速开始
4. 蓝图编辑器基础操作
5. 节点 JSON 字段完整说明
6. 新建各种节点
   - 纯数据节点
   - 同步执行节点
   - 多分支节点
   - 入口节点
   - 数组和 Any 节点
   - 动态 Sequence/动态分支节点
   - 异步节点
7. 蓝图、函数图、变量、连线和返回值
8. Go 节点实现与注册
9. Go 图加载、实例创建和执行
10. 通用异步执行
    - Yield/Resume 状态流
    - 定时器到期恢复
    - RPC 成功/失败 ResumeTo
    - Dispatcher 与宿主线程模型
    - Actor Dispatcher 适配示例
11. 特殊场景
    - 通用事件循环/Actor Dispatcher 适配
    - 循环体挂起恢复
    - 函数内挂起恢复
    - 并发 Execution
    - 取消、释放、关闭和回调竞态
12. 其他 Go 接口
    - Registry、CompiledGraph、低层 Graph.Do
    - Trace、IBlueprintModule 和兼容 logger
13. 使用约束与禁止用法
14. 进阶使用和性能建议
15. 测试、排错和上线检查清单
16. API 与错误索引

每个主要章节都包含“最小示例、适用场景、注意事项、禁止用法”四类内容，避免字段参考与实际操作脱节。

### 5.1 自检后补充的重点内容

- 明确区分“编辑器能显示节点”和“Go engine 能执行节点”：未知 JSON `name` 可以进入编辑器，但只有绑定并注册对应 `IExecNode` 后才能执行。
- 分别说明 legacy runtime schema 与 native editor schema。外部可执行 Go 节点优先使用带 `name/port_id` 的 runtime schema；native schema 主要用于编辑器内建类型，不能假设任意新 `id` 会自动获得 Go 执行映射。
- 给出节点端口顺序契约：端口下标由 `port_id` 决定；异步多分支节点应把 Exec 输出放在前面、数据输出连续放在后面；`Resume/ResumeTo` 参数按数据输出顺序写入。
- 说明 `Exec()` 返回值：同步流程节点返回下一个 Exec 输出端口下标；纯数据节点没有后续执行出口时返回 `-1`；异步挂起返回 `-1, ErrExecutionSuspended`。
- 说明节点工厂的依赖注入方式：通过闭包为每次执行创建新节点，并注入 RPC client、timer provider 或宿主服务接口。
- 增加完整接入链路：节点 JSON、`GetName()`、`RegisterExecNode`、`Init`、`Create`、`Start/Do`、`Result`、`ReleaseGraph/Close`。
- 增加编辑器节点库来源和“刷新节点库”操作，并把 JSON 声明、画布验证、Go 工厂注册、初始化编译和发布兼容检查串成完整新增节点清单。
- 说明 `PortArray/ArrayData`、入口参数和返回端口的类型关系，避免调用方误以为 `Do` 永远返回空数组。
- 增加默认 worker、inline、自定义 Dispatcher、Actor-aware Dispatcher 的选择表。
- 明确事件循环死锁风险：可能挂起的图不得在 Actor/GUI/单线程事件循环中调用阻塞式 `Do/DoContext`；必须使用 `Start` 并让事件循环继续处理恢复任务。
- 增加 Execution 状态、`Done`、`Result`、`Cancel`、Context 取消、Dispatcher 拒绝以及实例释放后的处理方式。
- 增加 Trace、热加载、函数局部变量、实例变量、并发 Execution 与共享 `CompiledGraph` 的边界。
- 所有异步示例不得在 callback 中访问 `BaseExecNode` 或节点端口，只捕获一次性 `YieldHandle` 和复制后的普通值。

## 6. 异步示例设计

异步章节先定义通用模型：

```text
Native Exec
  -> 复制输入参数
  -> Yield(execOutput)
  -> 发起外部异步操作
  -> 返回 ErrExecutionSuspended
  -> 外部 callback
  -> Resume 或 ResumeTo
  -> Execution 启动时捕获的 Dispatcher
  -> 恢复原 VM 的 PC/Flow/Loop/Call Stack
```

提供三类可编译风格示例：

1. 注入通用 `TimerProvider`，到期 callback 中调用 `Resume`。
2. RPC callback 根据成功/失败调用 `ResumeTo`。
3. 自定义宿主事件循环/Actor 使用 `NewActorExecutionDispatcher(enqueue)` 将恢复任务投递回所属执行环境。

Actor 示例明确为 Dispatcher 的一种适配，不进入 Core API，也不要求其他宿主实现 Actor。

RPC 与定时器示例使用文档内定义的最小接口，不要求 Core 新增依赖：

```go
type AsyncRPC interface {
    Call(request Request, callback func(Response, error))
}

type TimerProvider interface {
    After(delay time.Duration, callback func()) (cancel func())
}
```

示例节点通过工厂闭包注入这些接口，说明生产环境可以替换为 goroutine、网络库、Origin service、Actor timer 或其他事件循环实现。

## 7. 约束表达方式

规则按三个等级展示：

- 必须：违反后会造成编译失败、执行错误、数据竞争或兼容破坏。
- 建议：用于性能、可维护性和排错。
- 禁止：当前引擎明确不支持，或者有高概率导致线上问题。

重点禁止项：

- 在 `Exec()` 内阻塞等待 RPC、调用 `time.Sleep` 或等待 channel。
- Yield 成功后不返回 `ErrExecutionSuspended`，或未 Yield 却返回挂起错误。
- 一个 `YieldHandle` 恢复多次。
- callback 中继续访问节点输入输出端口或保存 `BaseExecNode`。
- 使用低层 `Graph.Do` 执行可能 Yield 的图。
- 跨 goroutine 复用同一个 `Graph`。
- 在错误线程直接修改宿主线程专属状态。
- 在 Actor、GUI 或单线程事件循环中使用阻塞式 `Do/DoContext` 等待可能挂起的图。
- 新业务蓝图使用旧 Delay/Timer 兼容占位节点。
- 发布后随意调整 `port_id`、入口 ID、函数 ID 或变量类型。

## 8. 兼容性说明

手册保留必要的 `.vgf` 兼容知识，但不展开历史实现细节：

- 未知旧节点和连线不能静默丢失。
- `port_id` 是旧图兼容契约。
- 新增可执行节点需要同时考虑编辑器 schema、Go `GetName()`、注册工厂和行为测试。
- 新格式动态分支与旧格式端口编号的对应关系必须稳定。

## 9. 验证

文档完成后执行：

1. 扫描新手册中的 Go 标识符，逐项与当前源码核对。
2. 扫描已删除文档名，确保普通文档和 `AGENTS.md` 不再引用。
3. 检查所有代码示例的端口下标、输出顺序和错误处理。
4. 检查无 `Continuation`、`TimerScheduler`、可执行内置 Delay/Timer 等过时说法。
5. 执行文档相关链接和文件存在性检查。

## 10. 非目标

- 不实现新的运行时功能。
- 不恢复内置 Delay/TimerScheduler。
- 不修改蓝图文件格式或节点执行语义。
- 不删除 `docs/superpowers/**` 历史设计记录。
- 不提交 Git，由用户自行比较和提交。
