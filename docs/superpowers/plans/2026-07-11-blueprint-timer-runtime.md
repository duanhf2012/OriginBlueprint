# Blueprint Timer Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现真正非阻塞的蓝图 Execution、共享 Delay/Timer 调度器、按函数设置定时器节点及完整的前端编辑体验。

**Architecture:** `Blueprint.Start` 只负责校验和向 `ExecutionDispatcher` 准入，`Execution` 持有单次 Graph session 的终态结果。`TimerScheduler` 只管理 deadline，通过 Dispatcher 恢复 Delay 或启动独立函数回调；Timer 属于 GraphInstance，Delay 属于 Execution。

**Tech Stack:** Go 1.x、Wails v2、Vue 3、TypeScript、Rete.js v2、JSON 节点定义。

## Global Constraints

- 不把单次执行可变状态写入 `CompiledGraph`、`ExecNode` 或 `NodeDefinition`。
- `Start` 不在调用方 goroutine 中执行用户节点。
- 不采用一个 Delay/Timer 一个 goroutine。
- 旧 Timer 运行时不兼容，但 legacy 文件内容不能静默丢失。
- 所有新增节点 UI 从第一版支持多语言。
- 每项生产代码先写失败测试，再实现最小代码。

---

### Task 1: Execution 生命周期与 Dispatcher

**Files:**
- Create: `engine/go/blueprint/execution_session.go`
- Create: `engine/go/blueprint/dispatcher.go`
- Create: `engine/go/blueprint/execution_session_test.go`
- Modify: `engine/go/blueprint/blueprint.go`
- Modify: `engine/go/blueprint/runtime.go`
- Modify: `engine/go/blueprint/continuation.go`

**Interfaces:**
- Produces: `ExecutionState`、`Execution`、`ExecutionDispatcher`、`Blueprint.Start`、`Blueprint.DoContext`、`Blueprint.Close`。
- Preserves: `Blueprint.Do(graphID, entranceID, args...) (PortArray, error)`。

- [ ] 写测试证明 `Start` 返回前不执行测试节点，Dispatcher 放行后才执行。
- [ ] 写测试证明 `Result` 在完成前返回 `ErrExecutionPending`，完成后返回结果。
- [ ] 写测试证明取消、Graph 释放和 Dispatcher 拒绝产生稳定终态。
- [ ] 运行 `go test ./engine/go/blueprint -run 'TestBlueprint(Start|Execution|DoContext)' -count=1`，确认因接口缺失而失败。
- [ ] 实现状态机、活动表、Dispatcher 和阻塞便利接口。
- [ ] 运行同一命令确认通过，再运行 `go test ./engine/go/blueprint -count=1`。

### Task 2: 共享 Scheduler 与 Delay

**Files:**
- Create: `engine/go/blueprint/scheduler.go`
- Create: `engine/go/blueprint/scheduler_test.go`
- Modify: `engine/go/blueprint/sleep.go`
- Modify: `engine/go/blueprint/sleep_test.go`
- Modify: `engine/go/blueprint/continuation.go`
- Modify: `engine/go/blueprint/system_nodes.go`

**Interfaces:**
- Consumes: `ExecutionDispatcher`、`Execution`。
- Produces: `TimerScheduler`、内部 `ScheduledTaskHandle`、正式 `Delay` 节点。

- [ ] 写 fake clock/手动 Scheduler 测试，证明多个 Delay 不创建等待 goroutine。
- [ ] 写零延迟、负值、溢出、取消、释放和恢复结果测试。
- [ ] 运行窄测试确认失败。
- [ ] 实现单 worker 最小堆 Scheduler 和可靠 Dispatcher 提交。
- [ ] 将 Sleep 改为 Delay，并通过 Scheduler 恢复 Continuation。
- [ ] 运行 `go test ./engine/go/blueprint -run 'Test(Scheduler|Delay|ReleaseGraph)' -count=1`。

### Task 3: TimerHandle 与 Timer 运行时节点

**Files:**
- Create: `engine/go/blueprint/timer_runtime.go`
- Create: `engine/go/blueprint/timer_nodes.go`
- Create: `engine/go/blueprint/timer_runtime_test.go`
- Modify: `engine/go/blueprint/port.go`
- Modify: `engine/go/blueprint/definition.go`
- Modify: `engine/go/blueprint/compiler.go`
- Modify: `engine/go/blueprint/system_nodes.go`

**Interfaces:**
- Produces: `TimerHandle`、Set/Clear/Pause/Unpause/Query 节点。
- Timer 回调通过稳定 FunctionID 创建新的根 Execution，返回值忽略。

- [ ] 写 TimerHandle 类型隔离和跨实例拒绝测试。
- [ ] 写一次性、循环不重入、暂停恢复、清除竞争测试。
- [ ] 写 `CancelRunningCallback` 和函数局部变量隔离测试。
- [ ] 运行窄测试确认失败。
- [ ] 实现 TimerRegistry 和全部运行时节点。
- [ ] 运行 Timer/函数/并发窄测试。

### Task 4: GraphDocument、节点 JSON 与前端

**Files:**
- Modify: `nodes/Entrance.json`
- Modify: `nodes/Event.json`
- Modify: `nodes/SysFlowControl.json`
- Modify: `graph.go`
- Modify: `legacy.go`
- Modify: `engine/go/blueprint/document.go`
- Modify: `frontend/src/editor/runtimeNodeSchemas.ts`
- Modify: `frontend/src/editor/nodeRegistry.ts`
- Modify: `frontend/src/editor/types.ts`
- Modify: `frontend/src/editor/BlueprintControl.vue`
- Modify: `frontend/src/App.vue`
- Modify: existing frontend i18n resources discovered during implementation
- Test: `frontend/tests/`

**Interfaces:**
- Uses fixed typeId from `docs/BLUEPRINT_TIMER_DESIGN_ZH.md`。
- Reuses function call node metadata: stable `functionId`、display name、signature and dynamic ports。

- [ ] 写 Go 文档转换和端口类型失败测试。
- [ ] 写前端 schema、socket、函数选择和多语言测试。
- [ ] 删除旧 Timer 入口/CreateTimer/CloseTimer 的可创建 schema。
- [ ] 实现新节点 JSON、稳定端口 key 和前端控件。
- [ ] 运行 `go test ./...`、`npm run test:layout`、`npm run build`。

### Task 5: 旧运行时清理与验证资产

**Files:**
- Modify: `engine/go/blueprint/loader.go`
- Modify: `engine/go/blueprint/blueprint.go`
- Modify: `engine/go/blueprint/system_nodes.go`
- Modify: `engine/go/blueprint/timer_test.go`
- Modify: `docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`
- Modify: `docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`
- Modify: `examples/verification-blueprints/`

**Interfaces:**
- Removes: `IBlueprintModule.SafeAfterFunc`、旧 `CancelTimerId` 路径、`Entrance_Timer`、`CreateTimer`、`CloseTimer`。
- Keeps: generic legacy hidden node/edge preservation。

- [ ] 写 legacy 导入保留但不执行旧 Timer 节点的测试。
- [ ] 删除旧运行时和不再成立的兼容测试。
- [ ] 更新 Timer/Delay 验证蓝图及对应结果说明。
- [ ] 运行全量 Go、race、前端构建和布局测试。

### Task 6: 性能、泄漏与最终 Review

**Files:**
- Modify: `engine/go/blueprint/benchmark_test.go`
- Modify: `docs/BLUEPRINT_TIMER_DESIGN_ZH.md`

- [ ] 增加 Start、批量 Delay、Timer 注册/取消和 Graph 释放 benchmark。
- [ ] 验证一万个等待任务不会增加一万个 goroutine。
- [ ] 运行 `go test -race ./engine/go/blueprint -count=1`。
- [ ] 运行 `go test -race ./... -count=1`。
- [ ] 运行相关 benchmark 并记录基线。
- [ ] 对最终 diff 做代码 review，修复所有 P0/P1 问题。
