# Blueprint Parser/VM Diagnostics Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除解析、函数契约、循环边界和执行端口中的静默失败，并为同步与异步执行提供可定位的结构化错误上下文，同时保持现有 `.vgf`、`.obp`、`.obpf` 行为兼容。

**Architecture:** 校验尽量前移到文档转换和 `CompileGraph`；VM 在所有运行期边界保留防御校验。错误继续通过 Go `error` 返回，不绑定宿主日志库；统一用包含阶段、图、入口、Execution、节点、PC 和源文件信息的错误包装补足诊断。编辑器元数据继续允许存在，仅对执行关键字段和值键做严格校验。

**Tech Stack:** Go 1.26、encoding/json、OriginBlueprint Go VM、Go testing/race/vet。

## Global Constraints

- 修改目录固定为 `E:/NewWork/OriginBlueprint/OriginBlueprint`；mp1server 通过 `replace` 直接使用该目录。
- 不恢复旧执行内核，不改变合法蓝图的执行结果和 Yield/Resume 恢复语义。
- 所有行为修复必须先增加失败测试并确认 RED，再实现最小修复并确认 GREEN。
- 不提交代码；用户自行比较并提交。
- 完成后运行引擎全量、Race、Vet、随机差分、Benchmark、mp1server facade 与全量业务蓝图兼容测试。

---

### Task 1: README 唯一入口迁移

**Files:**
- Replace: `README.md`
- Delete: `README_CN.md`
- Move: `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md` -> `README.md`
- Modify: `AGENTS.md`
- Modify: `engine/go/blueprint/AGENTS.md`

**Interfaces:**
- Consumes: 已完成 review 的中文使用手册。
- Produces: 根目录唯一权威 `README.md`，验证报告仍保留在 `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`。

- [ ] **Step 1:** 使用 `apply_patch` 将完整中文手册迁移为根 `README.md`，删除重复的 `README_CN.md` 和原手册文件。
- [ ] **Step 2:** 将手册中的 `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md` 链接按根目录位置校验，将两个 `AGENTS.md` 的权威手册路径更新为 `README.md`。
- [ ] **Step 3:** 运行 `rg -n "ORIGIN_BLUEPRINT_USER_GUIDE_ZH|README_CN" -g '*.md'`，预期无失效权威入口引用。

### Task 2: 解析和默认值严格诊断

**Files:**
- Modify: `engine/go/blueprint/document.go`
- Modify: `engine/go/blueprint/compiler.go`
- Modify: `engine/go/blueprint/loader.go`
- Test: `engine/go/blueprint/document_test.go`
- Test: `engine/go/blueprint/compiler_validation_test.go`

**Interfaces:**
- Produces: 执行关键默认值未知 key 返回错误；重复/空变量 ID 返回错误；编译错误包含源文件路径；普通图/函数图入口契约可验证。

- [ ] **Step 1:** 添加测试，证明未知 `values` key、重复变量 ID、无入口普通图、缺少 FunctionEntry/FunctionReturn 的函数图当前会被接受或延迟到运行期。
- [ ] **Step 2:** 运行对应 focused tests，确认因缺少校验而失败。
- [ ] **Step 3:** 将 `documentDefaults` 改为 `(map[int]any, error)`，未知 key 返回包含 node ID 和 key 的错误；验证变量 ID 非空且唯一。
- [ ] **Step 4:** 在 loader 已知文件类型后执行图契约校验，并用 `file.path` 包装编译错误。
- [ ] **Step 5:** 运行解析、loader 和 legacy compatibility tests，确认合法编辑器元数据及 `.vgf` 不受影响。

### Task 3: VM 执行出口和循环边界

**Files:**
- Modify: `engine/go/blueprint/vm_execution.go`
- Modify: `engine/go/blueprint/vm_flow.go`
- Test: `engine/go/blueprint/vm_execution_test.go`
- Test: `engine/go/blueprint/vm_loop_test.go`

**Interfaces:**
- Produces: `advanceFromPort(portIndex int) error`，只接受 `-1` 或当前节点合法 Exec 输出；循环恰好执行 `vmMaximumLoopIterations` 次可正常完成。

- [ ] **Step 1:** 添加非法负端口、越界端口、数据端口及 99,999/100,000/100,001 循环边界测试。
- [ ] **Step 2:** 运行 focused tests，确认非法端口测试表现为静默成功、100,000 次测试错误超限。
- [ ] **Step 3:** 让 `advanceFromPort` 返回错误并在 Native、Loop、Function、Resume 调用点传播；保留无连线合法 Exec 出口的正常完成语义。
- [ ] **Step 4:** 把循环预算检查移动到 `hasNext` 为 true 之后、开始下一轮之前。
- [ ] **Step 5:** 运行所有 VM、循环、Yield/Resume 测试。

### Task 4: 函数契约和顶层返回类型

**Files:**
- Modify: `engine/go/blueprint/compiler.go`
- Modify: `engine/go/blueprint/runtime.go`
- Modify: `engine/go/blueprint/vm_function.go`
- Modify: `engine/go/blueprint/functions.go`
- Test: `engine/go/blueprint/vm_function_test.go`
- Test: `engine/go/blueprint/document_test.go`

**Interfaces:**
- Produces: 编译期 FunctionCall 输入/输出数量和类型与目标函数一致；运行期返回数量不一致报错；`arrayDataFromAny(value any) (ArrayData, error)` 支持 `PortArray` 并拒绝无法表示的值。

- [ ] **Step 1:** 添加缺失函数、输入/输出数量或类型漂移、函数返回不足、直接函数返回 Array/复杂 Any 的失败测试。
- [ ] **Step 2:** 运行 focused tests，确认当前缺少编译期拒绝或产生空结果。
- [ ] **Step 3:** 为 `CompiledGraph` 保存函数签名，在 `CompileGraph` 绑定 FunctionCall 时逐项核对。
- [ ] **Step 4:** 将运行期 `continue` 改为带函数名、source/target slot 的错误；让顶层返回转换显式成功或报错。
- [ ] **Step 5:** 运行函数、文档 authoring、异步函数和随机差分测试。

### Task 5: 统一执行错误上下文与异步恢复边界

**Files:**
- Create: `engine/go/blueprint/diagnostic.go`
- Modify: `engine/go/blueprint/runtime.go`
- Modify: `engine/go/blueprint/vm_execution.go`
- Modify: `engine/go/blueprint/vm_async.go`
- Modify: `engine/go/blueprint/execution_session.go`
- Modify: `engine/go/blueprint/trace.go`
- Test: `engine/go/blueprint/diagnostic_test.go`
- Test: `engine/go/blueprint/trace_test.go`
- Test: `engine/go/blueprint/vm_async_test.go`

**Interfaces:**
- Produces: `BlueprintError` 实现 `error`/`Unwrap`，字段包含 Stage、SourcePath、GraphName、GraphID、EntranceID、ExecutionID、NodeID、NodeName、PC；异步恢复 panic 转为 ExecutionFailed；TraceEvent 增加 ExecutionID、EntranceID、PC、Stage。

- [ ] **Step 1:** 添加纯数据生产者 panic 归因、Resume 任务 panic、`Graph.Do` 缺入口、`DoContext(nil)` 和 Trace 关联字段测试。
- [ ] **Step 2:** 运行 focused tests，确认错误归因缺失、低层入口静默成功或 nil context panic。
- [ ] **Step 3:** 在解析/编译/执行/恢复边界用 `%w` 和 `BlueprintError` 包装，保持 `errors.Is/As`；在数据节点和 Resume 任务边界 recover。
- [ ] **Step 4:** 统一 `Graph.Do` 与 `Blueprint.Start` 的入口错误；`DoContext` 顶层规范化 nil context。
- [ ] **Step 5:** 为控制指令、Yield、Resume 和终态错误补充关联字段，不记录宿主敏感数据。
- [ ] **Step 6:** 运行 diagnostic、trace、async、dispatcher、cancel/release tests。

### Task 6: 全面回归与性能门禁

**Files:**
- Modify: `README.md`（仅当接口或错误语义与手册不一致时同步）
- Modify: `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`（仅由既有报告测试生成）

**Interfaces:**
- Consumes: Tasks 1-5 的全部行为。
- Produces: 可复现的兼容、并发、性能和业务资产验证结果。

- [ ] **Step 1:** 运行 `gofmt` 后执行 `go test ./engine/go/blueprint -count=1`。
- [ ] **Step 2:** 执行 `go test -race ./engine/go/blueprint -count=1` 和 `go vet ./engine/go/blueprint`。
- [ ] **Step 3:** 执行随机差分测试和 `WRITE_BLUEPRINT_VERIFICATION_REPORT=1 go test ./engine/go/blueprint -run TestWriteVerificationMatrixReport -count=1`。
- [ ] **Step 4:** 执行指定 Blueprint benchmarks，比较修改前后的 alloc/op 与 ns/op，解释差异。
- [ ] **Step 5:** 在 mp1server 执行 `go test ./common/blueprint -count=1`、Battle/GS 全量业务蓝图兼容测试，并确认 `go list` 指向本地 replace。
- [ ] **Step 6:** 检查 `git diff --check`、文档链接、工作区差异，只汇报本任务文件，不覆盖用户已有改动。
