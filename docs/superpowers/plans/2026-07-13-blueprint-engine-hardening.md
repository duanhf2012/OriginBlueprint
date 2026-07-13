# Go 蓝图引擎正确性与性能加固实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Go 蓝图解析执行器已确认的正确性和资源安全问题，并降低复杂图执行分配。

**Architecture:** 顶层 Execution 与函数子图共享轻量执行作用域；编译器负责结构和规模校验，运行时负责统一预算；执行语义修复后再引入执行级上下文复用。所有改动限定在 `engine/go/blueprint/` 和对应文档测试，不修改前端绘制及持久化契约。

**Tech Stack:** Go、标准库 testing/race/benchmark、现有 GraphDocument/CompiledGraph/Execution API。

## Global Constraints

- 保持 legacy `.vgf` 未知内容兼容，不静默丢边。
- 不改变现有 `GraphDocument` JSON 字段。
- 所有生产代码修改必须先有能够稳定失败的回归测试。
- 异步生命周期改动必须通过 race 测试。

---

### Task 1: 嵌套函数异步执行作用域

**Files:** `execution_session.go`, `runtime.go`, `functions.go`, `sleep.go`, `function_test.go`, `sleep_test.go`

- [ ] 写函数内 Delay/RPC 继承调度器、取消和实例释放的失败测试。
- [ ] 引入共享执行作用域，函数 Graph 只继承作用域而不共享函数帧状态。
- [ ] 运行函数、定时器和 race 窄测试。

### Task 2: 执行预算与环检测

**Files:** `compiler.go`, `runtime.go`, `execution_session.go`, `compiler_test.go`, `runtime_test.go`

- [ ] 写数据环、任意执行环和结构化循环预算测试。
- [ ] 编译期拒绝数据依赖环，运行时统一计步并返回稳定预算错误。
- [ ] 验证 For/While/Break 正常图不被误拒绝。

### Task 3: Schema 资源限制

**Files:** `definition.go`, `document.go`, `definition_test.go`, `document_test.go`

- [ ] 写巨大/重复端口 ID、巨大动态输出及函数签名测试。
- [ ] 增加集中限制常量和上下文化校验错误。
- [ ] 运行解析、定义和 GraphDocument 测试。

### Task 4: While 条件刷新与 legacy 扇出

**Files:** `native_nodes.go`, `runtime.go`, `compiler.go`, `native_nodes_test.go`, `compiler_test.go`

- [ ] 写循环体修改变量后退出及 legacy 多分支全部执行的失败测试。
- [ ] 每轮重算 While 条件生产链；legacy 保序执行全部目标，新格式拒绝扇出。
- [ ] 运行控制流和 legacy 回归测试。

### Task 5: 函数名称冲突

**Files:** `loader.go`, `loader_test.go`

- [ ] 写图名、函数 ID、路径别名冲突测试。
- [ ] 加入来源感知的冲突检测并返回确定性错误。
- [ ] 运行 loader 与热加载测试。

### Task 6: Continuation 与随机整数边界

**Files:** `continuation.go`, `system_nodes.go`, `execution_session_test.go`, `system_nodes_test.go`

- [ ] 写数据端口 Suspend 和全 int64 范围随机数失败测试。
- [ ] 严格验证执行口并实现无溢出闭区间采样。
- [ ] 运行 Continuation、系统节点和 race 测试。

### Task 7: 执行上下文分配优化

**Files:** `runtime.go`, `execution_session.go`, `benchmark_test.go`, `runtime_test.go`

- [ ] 固化当前复杂图、函数和异步重入的分配基线测试。
- [ ] 增加执行级上下文帧复用与完成后的引用释放，不使用 `sync.Pool`。
- [ ] 比较优化前后 ns/op、B/op、allocs/op，并运行 race。

### Task 8: 综合验证与自审

**Files:** `docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`, 本次所有修改文件

- [ ] 运行 gofmt、窄测试、包测试、race、vet、全仓测试和 benchmark。
- [ ] 检查兼容性、错误信息、异步生命周期和性能回退风险。
- [ ] 更新测试矩阵并总结仍存在的仓库级失败。
