# Execution-local Variables and Atomic Hot Reload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The user requires inline execution in the existing directories; do not create a worktree, branch, commit, or vendor directory.

**Goal:** Restore ordinary blueprint variables to per-Execution local state, atomically publish immutable compiled graph pools, and complete control-node diagnostics and function-flow validation without regressing runtime performance.

**Architecture:** `CompiledGraph` owns immutable variable templates and each variable node owns a compile-time slot index. Every top-level Execution and function call clones those templates into its private Graph state; hot reload only swaps `Blueprint.graphs`, so old executions retain old programs while new executions capture the new pool. VM handler errors are wrapped at dispatch boundaries, and function fallthrough is rejected conservatively during load/compile while the runtime guard remains.

**Tech Stack:** Go, existing OriginBlueprint VM, Go testing/race/vet/benchmarks, mp1server local replace integration.

## Global Constraints

- Modify `E:/NewWork/OriginBlueprint/OriginBlueprint` directly and update affected mp1server callers under `E:/NewWork/branch_develop/mp1server`.
- Do not create commits, branches, worktrees, or vendor output; the user will compare and commit manually.
- Ordinary variables are Execution-local; global variables are explicitly out of scope.
- Yield/Resume and loop/function suspension must retain the current Execution/call-frame variables.
- Put parsing, type conversion, variable binding, and flow validation in compile/load stages; keep the success runtime path allocation-light.
- Preserve old Execution program references across hot reload; new Start/Do captures the current graph pool.
- Retain runtime `FunctionReturn` fallthrough protection even after compile-time validation.
- Do not tighten Any/numeric/array conversion semantics in this change.
- Every production behavior change follows RED → GREEN → REFACTOR.

---

## File Map

- `engine/go/blueprint/compiler.go`: compile variable templates, stable indices, and function contracts.
- `engine/go/blueprint/runtime.go`: store Execution-local variable slots and reset them for each top-level Graph.Do.
- `engine/go/blueprint/variables.go`: index-based Getter/Setter without instance locks.
- `engine/go/blueprint/blueprint.go`: remove instance runtime state and simplify hot reload graph-pool publication.
- `engine/go/blueprint/execution_session.go`: Start captures the current graph pool and initializes an independent Graph.
- `engine/go/blueprint/loader.go`: transactional Init and loaded function-flow validation.
- `engine/go/blueprint/diagnostic.go`, `vm_execution.go`: dispatch-time NodeID/PC error wrapping.
- `engine/go/blueprint/*_test.go`: variable, hot reload, lifecycle, diagnostic, function-flow, race, and benchmark coverage.
- `common/blueprint/BlueprintModule.go`, `BlueprintModule_test.go`: adapt `HotReloadResult` logging and tests.
- `README.md`, `engine/go/blueprint/AGENTS.md`: document restored local-variable semantics and atomic hot reload.

---

### Task 1: Lock the Execution-local variable contract with failing tests

**Files:**
- Modify: `engine/go/blueprint/variables_test.go`
- Modify: `engine/go/blueprint/vm_async_test.go`
- Modify: `engine/go/blueprint/benchmark_test.go`

**Interfaces:**
- Consumes: existing `Blueprint.Do`, `Blueprint.Start`, `YieldHandle.Resume`.
- Produces: regression tests that require per-Execution initialization and resume preservation.

- [ ] **Step 1: Replace the persistence test with a failing reset test**

Create this test using the existing registry and `testReadVariable` helper:

```go
func TestBlueprintVariablesResetForEveryExecution(t *testing.T) {
    // Entry 1 sets Count=44. Entry 2 reads Count.
    // Run both with the same graphID.
    // The second execution must read the configured default, not 44.
}
```

- [ ] **Step 2: Add a failing concurrent isolation test**

Use a manual dispatcher and two Start calls on the same graphID. Give each execution a different entrance argument, set/read the same variable, then assert each result contains only its own value.

- [ ] **Step 3: Add a failing Yield/Resume preservation test**

Use a node that sets a variable, yields, resumes, and reads it in the same Execution. Assert Resume preserves the value while a later Start sees the default.

- [ ] **Step 4: Verify RED**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestBlueprintVariablesResetForEveryExecution|TestBlueprintVariablesAreIsolatedAcrossConcurrentExecutions|TestBlueprintVariablesSurviveYieldResume' -count=1
```

Expected: the reset/isolation tests fail because current GraphInstance state is shared; the resume test documents the behavior that must remain green.

---

### Task 2: Compile variable plans and move variables into Graph

**Files:**
- Modify: `engine/go/blueprint/compiler.go`
- Modify: `engine/go/blueprint/runtime.go`
- Modify: `engine/go/blueprint/variables.go`
- Test: `engine/go/blueprint/compiler_test.go`
- Test: `engine/go/blueprint/variables_test.go`

**Interfaces:**
- Produces: private `VariablePlan`, `CompiledGraph.variablePlans`, `ExecNode.VariableIndex`, `Graph.variables []IPort`, `Graph.initializeVariables()`.
- Consumes: `IPort.Clone`, `assignPortValue`, existing dynamic Get_/Set_ compilation.

- [ ] **Step 1: Add compiler tests for stable variable indices and templates**

Test two variables and assert Get/Set nodes bind to their exact index, templates carry converted defaults, duplicate/unknown variables still fail compilation, and a graph without variables has zero plans.

- [ ] **Step 2: Verify compiler tests fail**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestCompilerBuildsVariablePlans|TestCompilerBindsVariableNodesByIndex' -count=1
```

Expected: compile failure because the plan/index fields do not yet exist.

- [ ] **Step 3: Implement immutable variable plans**

Use the design signatures:

```go
type VariablePlan struct {
    Name     string
    Template IPort
}

func cloneVariablePlans(plans []VariablePlan) []IPort
```

Build plans in `CompileGraph`, bind every variable node to a validated index, and never mutate plan templates after compile.

- [ ] **Step 4: Implement Execution-local slot initialization**

Change Graph variables to `[]IPort`. Initialize from compiled templates whenever a new top-level entrance VM or function child Graph is created. Do not enter initialization from `resumeYield`.

- [ ] **Step 5: Implement index-based Getter/Setter**

Getter copies the indexed slot into its output. Setter assigns into the existing typed slot with `assignPortValue`, then copies it to the data output. Return a node error when an index/slot is invalid.

- [ ] **Step 6: Verify GREEN and refactor**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestCompilerBuildsVariablePlans|TestCompilerBindsVariableNodesByIndex|TestBlueprintVariables' -count=1
go test ./engine/go/blueprint -run 'TestVMYield|TestVM.*Loop|TestVMFunction' -count=1
```

Expected: all selected tests pass; remove old map/mutex helpers only after tests are green.

---

### Task 3: Make graph instances identity-only and hot reload pool-only

**Files:**
- Modify: `engine/go/blueprint/blueprint.go`
- Modify: `engine/go/blueprint/execution_session.go`
- Modify: `engine/go/blueprint/init_test.go`
- Modify: `engine/go/blueprint/vm_lifecycle_test.go`

**Interfaces:**
- Produces: `GraphInstance` without `state`, Start lookup through `b.graphs[instance.name]`, pool-only `hotReloadPlan.apply`.
- Consumes: Task 2 Execution-local initialization.

- [ ] **Step 1: Add failing hot reload generation tests**

Cover:

```go
func TestVMHotReloadOldExecutionUsesOldProgramAndNewExecutionUsesNewProgram(t *testing.T)
func TestVMHotReloadAllowsVariableSchemaChangesAcrossExecutions(t *testing.T)
func TestVMHotReloadRemovedGraphRejectsNewStartButOldExecutionCompletes(t *testing.T)
func TestVMHotReloadReaddedGraphAllowsExistingInstanceToStart(t *testing.T)
```

- [ ] **Step 2: Verify RED**

Run the four tests. Expected failures must show current instance state snapshot/missing-graph retention behavior.

- [ ] **Step 3: Remove instance runtime state**

Delete `instanceRuntimeState`, migration helpers, and variable locks. `Create` records name/id/module only. `Start` retrieves the current compiled graph by name while holding `b.mu` and constructs an independent Graph.

- [ ] **Step 4: Reduce hot reload apply to graph-pool publication**

After a complete successful compile, apply only:

```go
b.mu.Lock()
b.graphs = p.graphs
b.mu.Unlock()
```

Do not visit instances or executions.

- [ ] **Step 5: Verify GREEN and race-sensitive lifecycle tests**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestVMHotReload|TestBlueprintConcurrent|TestVMCancel|TestVMRelease' -count=20
go test -race ./engine/go/blueprint -run 'TestVMHotReload|TestBlueprintConcurrent' -count=10
```

Expected: all pass with no race report.

---

### Task 4: Make Init transactional and simplify HotReloadResult

**Files:**
- Modify: `engine/go/blueprint/loader.go`
- Modify: `engine/go/blueprint/blueprint.go`
- Modify: `engine/go/blueprint/init_test.go`
- Modify: `common/blueprint/BlueprintModule.go`
- Modify: `common/blueprint/BlueprintModule_test.go`

**Interfaces:**
- Produces: `ErrBlueprintInUse`, `HotReloadResult{GraphCount int}`.
- Consumes: complete compiled graph maps from `loadGraphDir`.

- [ ] **Step 1: Add failing Init atomicity tests**

Cover failed Init leaving module/paths/graphs unchanged, repeat Init replacing rather than merging while idle, and Init returning `ErrBlueprintInUse` when any instance/execution exists.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestBlueprintInitIsTransactional|TestBlueprintRepeatInitReplacesGraphPool|TestBlueprintInitRejectsInUse' -count=1
```

- [ ] **Step 3: Implement two-phase Init**

Snapshot factories under lock, compile outside the lock, then reacquire and recheck closed/instances/executions before replacing all configuration fields and the graph map. Return `ErrBlueprintInUse` without partial writes.

- [ ] **Step 4: Remove obsolete HotReloadResult fields**

Keep only `GraphCount`. Update mp1server logging and tests to stop referencing instance counters.

- [ ] **Step 5: Verify engine and mp1server integration**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestBlueprintInit|TestBlueprintHotReload' -count=1
go test ./common/blueprint -count=1
```

Expected: both modules compile and all selected tests pass.

---

### Task 5: Add dispatch-time NodeID/PC diagnostics

**Files:**
- Modify: `engine/go/blueprint/diagnostic.go`
- Modify: `engine/go/blueprint/vm_execution.go`
- Modify: `engine/go/blueprint/diagnostic_test.go`

**Interfaces:**
- Produces: `wrapVMDispatchError(graph *Graph, node *ExecNode, pc PC, err error) error`.
- Consumes: existing `BlueprintError`, `newBlueprintNodeError`, `enrichExecutionError`.

- [ ] **Step 1: Add table-driven failing control-node diagnostic tests**

Force errors in Sequence, each loop family, FunctionCall and FunctionReturn. Assert `errors.As` finds BlueprintError with exact NodeID and PC. Add a return-binding error test proving the callee Return node is retained after the machine restores the caller.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestVMControlErrorsIncludeNodeAndPC|TestVMFunctionReturnErrorKeepsCalleeNode' -count=1
```

Expected: NodeID/PC assertions fail for current plain errors.

- [ ] **Step 3: Wrap errors at VM dispatch**

Capture graph/node/PC before invoking each opcode handler. On error, fill missing fields of an existing BlueprintError or create a new node error. Keep the normal success switch free of allocations.

- [ ] **Step 4: Verify GREEN and errors.Is/As compatibility**

Run the diagnostic tests plus existing diagnostic suite. Assert root sentinel errors remain discoverable with `errors.Is`.

---

### Task 6: Validate deterministic function fallthrough during compile/load

**Files:**
- Modify: `engine/go/blueprint/compiler.go`
- Modify: `engine/go/blueprint/loader.go`
- Modify: `engine/go/blueprint/vm_function_test.go`
- Modify: `engine/go/blueprint/parser_hardening_test.go`

**Interfaces:**
- Produces: conservative `validateFunctionFlow(*CompiledGraph) error` run after VM Program compilation/binding.
- Consumes: `Program.Nodes`, successors, control kinds, function targets.

- [ ] **Step 1: Add failing function-flow tests**

Cover unreachable FunctionReturn, a branch with direct fallthrough, a valid Sequence continuation, zero-iteration loop completion returning, recursive function acceptance, and the retained runtime fallthrough guard.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestCompileRejectsFunction|TestCompileAcceptsFunction|TestVMFunctionWithoutReturnFailsAtRuntime' -count=1
```

Expected: invalid functions currently compile.

- [ ] **Step 3: Implement conservative continuation-aware validation**

Analyze reachable execution with structured continuations. Report only paths proven to exhaust an empty continuation without FunctionReturn. Treat unresolved recursion/nontermination as runtime-protected, not compile errors. Include offending node ID in the compile BlueprintError.

- [ ] **Step 4: Verify GREEN against all assets**

Run the focused tests and all parser/loader/legacy compatibility tests. Any existing asset rejection must be reviewed as either a real invalid function or a validator false positive; do not weaken unrelated parser rules.

---

### Task 7: Remove stale semantics and update authoritative documentation

**Files:**
- Modify: `README.md`
- Modify: `engine/go/blueprint/AGENTS.md`
- Create: `E:/NewWork/branch_develop/mp1server/docs/refactor/execution-local-variables-hot-reload.md`

**Interfaces:**
- Produces: one consistent public contract.

- [ ] **Step 1: Update variable and hot reload documentation**

State that ordinary variables reset per Execution, survive Yield/Resume, and never persist across Do. Explain that global state belongs to host storage until explicit global variables are designed.

- [ ] **Step 2: Update hot reload and error documentation**

Document immutable pool replacement, old/new Execution version capture, removed-graph behavior, transactional Init, simplified HotReloadResult, and control-node NodeID/PC diagnostics.

- [ ] **Step 3: Scan for stale contracts**

Run:

```powershell
rg -n '同一实例的多次 Execution|跨.*Do|UpdatedInstances|UnchangedInstances|实例变量迁移|instanceRuntimeState' README.md engine/go/blueprint docs -g '*.go' -g '*.md'
```

Expected: only historical design context explicitly marked as superseded may remain.

---

### Task 8: Full verification, business differential, and performance gate

**Files:**
- Modify tests only when a newly reproduced defect first has a failing regression test.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: fresh evidence for correctness, compatibility, race safety, and performance.

- [ ] **Step 1: Run format and static checks**

```powershell
gofmt -w engine/go/blueprint/blueprint.go engine/go/blueprint/compiler.go engine/go/blueprint/diagnostic.go engine/go/blueprint/execution_session.go engine/go/blueprint/loader.go engine/go/blueprint/runtime.go engine/go/blueprint/variables.go engine/go/blueprint/benchmark_test.go engine/go/blueprint/compiler_test.go engine/go/blueprint/diagnostic_test.go engine/go/blueprint/init_test.go engine/go/blueprint/parser_hardening_test.go engine/go/blueprint/variables_test.go engine/go/blueprint/vm_async_test.go engine/go/blueprint/vm_function_test.go engine/go/blueprint/vm_lifecycle_test.go
go vet ./...
```

In mp1server run `gofmt -w common/blueprint/BlueprintModule.go common/blueprint/BlueprintModule_test.go`.

- [ ] **Step 2: Run full engine verification**

```powershell
go test ./... -count=1
go test -race ./... -count=1
go test ./engine/go/blueprint -count=20
```

- [ ] **Step 3: Run business asset and random differential tests**

Run the existing verification fixture, random asset, async, loop, and function suites. Confirm every online `.vgf` compiles and repeated fixed-seed inputs match the independent Go implementations.

- [ ] **Step 4: Run mp1server integration verification**

```powershell
go test ./common/blueprint -count=1
go test ./service/battleservice/battleobject -run '^$' -count=1
```

- [ ] **Step 5: Run benchmarks five times**

```powershell
go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex|Parallel)|BenchmarkFunctionCall' -benchmem -count=5
```

Compare medians with the recorded pre-change baseline. No-variable allocs/op must not increase and common runtime ns/op regression must remain within 10%.

- [ ] **Step 6: Review the final diff**

Confirm no vendor output, executable artifact, unrelated file, stale instance-variable code, or unhandled generated-file requirement was introduced. Do not stage or commit.
