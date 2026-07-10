# Blueprint Persistence and Engine Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 legacy 蓝图静默数据丢失、执行引擎结构校验、实例异步生命周期、热加载变量迁移，并用测试固定函数调用局部变量语义。

**Architecture:** `GraphDocument` 只增加向后兼容的可选 legacy 元数据；legacy 导入导出在 Go 后端完成，前端只透传并在节点删除事务中维护 hidden edge。Go engine 使用显式格式探测、编译期端口校验、实例 lifecycle lease、timer registration token 和可替换 runtime state；函数 Graph 保持每次调用独立变量帧。

**Tech Stack:** Go 1.23、Wails v2、Vue 3、TypeScript、Rete.js v2、Node.js 源码断言测试。

## Global Constraints

- 不修改 Rete 节点绘制、端口视觉和连线交互。
- 不统一 `.obp` 与 `.vgf` 的文件语义。
- 不恢复已删除的业务节点类型。
- 不把同一执行输出多连接判错，也不在本计划中修改其运行语义。
- legacy 无版本 `GraphConfig` 必须继续可加载；显式 `schemaVersion` 只接受整数 `1`。
- 函数变量每次调用新建，与调用方同名变量隔离。
- 所有生产代码修改必须遵循 RED-GREEN-REFACTOR。
- 当前根包基线有 3 个依赖未纳入仓库的业务 schema/样本失败；不得通过放宽规则消除。

---

### Task 1: Legacy defaults, edge identity, ordering, and export preflight

**Files:**
- Modify: `graph.go`
- Modify: `legacy.go`
- Create: `legacy_safety_test.go`
- Create: `testdata/legacy/residual-defaults.vgf`
- Create: `testdata/legacy/interleaved-hidden-edge.vgf`

**Interfaces:**
- Produces: `GraphLegacyResidualDefaults`, `GraphLegacyState.ResidualNodeDefaults`, `GraphLegacyState.HiddenEdgeOrdinals`.
- Produces: `GraphConnection.LegacyEdgeID`, `GraphConnection.LegacyOrdinal`.
- Produces: `canonicalLegacyDefaultIndex(string) (int, bool)` and fail-closed `exportLegacyGraph`.

- [ ] **Step 1: Write failing residual-default round-trip tests**

Add table-driven tests that migrate and export defaults with keys `"13"`, `"-1"`, `"01"`, `"name"` and values `nil`, `false`, `0`, `""`:

```go
func TestLegacyRoundTripPreservesUnmappedDefaultKeys(t *testing.T) {
    input := readLegacyFixture(t, "residual-defaults.vgf")
    document, err := migrateLegacyGraph(input)
    if err != nil { t.Fatal(err) }
    output, err := exportLegacyGraph(document)
    if err != nil { t.Fatal(err) }
    assertLegacyPortDefaultsEqual(t, input, output)
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test . -run '^TestLegacyRoundTripPreservesUnmappedDefaultKeys$' -count=1`

Expected: FAIL because non-canonical/unmapped keys are absent or incorrectly mapped to input `0`.

- [ ] **Step 3: Add optional GraphDocument metadata and canonical-key parsing**

Implement these compatible fields and only map canonical non-negative decimal keys:

```go
type GraphLegacyResidualDefaults struct {
    Class  string                 `json:"class"`
    Values map[string]interface{} `json:"values"`
}

type GraphLegacyState struct {
    ResidualNodeDefaults map[string]GraphLegacyResidualDefaults `json:"residualNodeDefaults,omitempty"`
    HiddenEdgeOrdinals   []int                                  `json:"hiddenEdgeOrdinals,omitempty"`
}

type GraphConnection struct {
    LegacyEdgeID  string `json:"legacyEdgeId,omitempty"`
    LegacyOrdinal *int   `json:"legacyOrdinal,omitempty"`
}

func canonicalLegacyDefaultIndex(raw string) (int, bool) {
    index, err := strconv.Atoi(raw)
    return index, err == nil && index >= 0 && strconv.Itoa(index) == raw
}
```

- [ ] **Step 4: Run residual tests and verify GREEN**

Run: `go test . -run 'TestLegacyRoundTripPreservesUnmappedDefaultKeys' -count=1`

Expected: PASS.

- [ ] **Step 5: Write failing edge-order and export-preflight tests**

Cover visible/hidden interleaving, original edge IDs, unknown visible node, unmappable visible port, duplicate final node ID, and hidden edge with missing endpoint:

```go
func TestLegacyRoundTripPreservesVisibleHiddenEdgeOrder(t *testing.T) {
    input := readLegacyFixture(t, "interleaved-hidden-edge.vgf")
    document, err := migrateLegacyGraph(input)
    if err != nil { t.Fatal(err) }
    output, err := exportLegacyGraph(document)
    if err != nil { t.Fatal(err) }
    if got, want := legacyEdgeIDs(t, output), []string{"visible-1", "hidden-1", "visible-2"}; !slices.Equal(got, want) {
        t.Fatalf("edge ids = %v, want %v", got, want)
    }
}

func TestExportLegacyGraphRejectsUnrepresentableVisibleNode(t *testing.T) {
    _, err := exportLegacyGraph(GraphDocument{Nodes: []GraphNode{{ID: "unknown", TypeID: "origin.unknown"}}})
    if err == nil || !strings.Contains(err.Error(), "unknown") { t.Fatalf("error = %v", err) }
}
```

The connection and dangling-hidden-edge cases use the same pattern and assert the connection array index plus source/target identifiers in the returned error.

- [ ] **Step 6: Run preflight tests and verify RED**

Run: `go test . -run 'TestLegacyRoundTripPreservesVisibleHiddenEdgeOrder|TestExportLegacyGraphRejects' -count=1`

Expected: FAIL because visible edges receive new IDs/order and invalid objects are skipped.

- [ ] **Step 7: Implement ordered export and preflight**

Build all node/spec/edge mappings before marshaling. Preserve original ordinal/edge ID, sort original edges by ordinal, append new visible edges in document order, and return contextual errors instead of `continue`. Validate `HiddenEdgeOrdinals` length when present; for old documents without ordinals assign deterministic append ordinals.

- [ ] **Step 8: Run Task 1 tests and root package tests**

Run: `go test . -run 'TestLegacyRoundTrip|TestExportLegacyGraphRejects' -count=1`

Expected: PASS for focused tests. Record the same 3 known environment-dependent failures if `go test . -count=1` is run.

- [ ] **Step 9: Commit Task 1**

```powershell
git add graph.go legacy.go legacy_safety_test.go testdata/legacy
git commit -m "fix: preserve legacy blueprint residual data"
```

### Task 2: Frontend legacy metadata maintenance and save failure UX

**Files:**
- Modify: `frontend/src/editor/document.ts`
- Modify: `frontend/src/editor/types.ts`
- Modify: `frontend/src/editor/createEditor.ts`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/src/i18n/zh-CN.ts`
- Modify: `frontend/src/i18n/en-US.ts`
- Create: `frontend/tests/legacyPersistence.test.js`
- Modify: `frontend/package.json`

**Interfaces:**
- Consumes: optional `legacyEdgeId`, `legacyOrdinal`, `hiddenEdgeOrdinals` from Task 1.
- Produces: connection metadata round-trip and `pruneHiddenLegacyEdges(nodeIDs)` inside editor mutations.

- [ ] **Step 1: Write failing static frontend tests**

Assert document types, connection snapshot/restore, hidden-edge cleanup inside `deleteSelected`, localized save error strings, and `saveGraph` catch behavior:

```js
assert(documentSource.includes('legacyEdgeId?: string'), 'connections must retain legacy edge ids')
assert(editorSource.includes('pruneHiddenLegacyEdges'), 'node deletion must prune hidden legacy edges')
assert(appSource.includes('catch (error)'), 'saveGraph must keep dirty state on rejected export')
assert(zhCn.includes('保存失败') && enUs.includes('Save failed'), 'save errors must be localized')
```

- [ ] **Step 2: Run the frontend test and verify RED**

Run: `node frontend/tests/legacyPersistence.test.js`

Expected: FAIL because the optional metadata and cleanup helper do not exist.

- [ ] **Step 3: Implement connection metadata passthrough**

Extend `ConnectionSnapshot` and `BlueprintConnection`, copy metadata in restore/snapshot/copy paths, and keep it out of rendering logic:

```ts
export interface ConnectionSnapshot {
  legacyEdgeId?: string
  legacyOrdinal?: number
}
```

- [ ] **Step 4: Implement hidden-edge cleanup as part of deletion transaction**

Filter `currentLegacy.hiddenEdges` and parallel ordinals by deleted node IDs inside the same `mutate` callback. Include the number of hidden edges removed in `callbacks.onStatus`, so undo/redo restores both graph and opaque legacy state.

- [ ] **Step 5: Implement localized save error handling**

Wrap validation/export/save in `saveGraph` with `try/catch`; only clear `tab.dirty` after success. Set status to localized `menuText.status.saveFailed` plus backend detail and keep the existing path/document untouched.

- [ ] **Step 6: Run frontend tests and build**

Run: `npm.cmd run test:layout` from `frontend/`.

Expected: all source assertion tests PASS.

Run: `npm.cmd run build` from `frontend/`.

Expected: TypeScript and Vite build PASS.

- [ ] **Step 7: Commit Task 2 without staging unrelated UI changes**

Stage only Task 2 hunks with explicit path/hunk inspection; preserve the existing module-search/reference changes in the same files.

### Task 3: Version probing, compiler structure checks, and typed assignment

**Files:**
- Modify: `engine/go/blueprint/compiler.go`
- Modify: `engine/go/blueprint/loader.go`
- Modify: `engine/go/blueprint/port.go`
- Modify: `engine/go/blueprint/runtime.go`
- Modify: `engine/go/blueprint/compiler_test.go`
- Modify: `engine/go/blueprint/json_test.go`
- Modify: `engine/go/blueprint/document_test.go`

**Interfaces:**
- Produces: `probeGraphSchemaVersion([]byte) (present bool, version int, err error)`.
- Produces: `assignPortValue(target, source IPort) error` for checked input binding.

- [ ] **Step 1: Write failing version-probe tests**

Cover missing version legacy success and explicit `0`, `2`, `1.5`, `"1"`, `null` failures through both `ParseGraphConfigJSON` and `parseGraphFile`.

- [ ] **Step 2: Run version tests and verify RED**

Run: `go test ./engine/go/blueprint -run 'SchemaVersion|LegacyGraphConfigWithoutSchemaVersion' -count=1`

Expected: unsupported explicit versions are currently accepted or routed as legacy.

- [ ] **Step 3: Implement shared raw JSON version probing**

Use `map[string]json.RawMessage` to distinguish missing from explicit zero, require JSON integer `1`, and call the same helper from both parser entry points.

- [ ] **Step 4: Write failing compiler validation tests**

Add separate tests for empty/duplicate node IDs, empty/duplicate variables, invalid variable type/default, duplicate entrance IDs, source/destination bounds, exec/data mismatch, duplicate data producer, concrete type mismatch, and legal exec fan-out not being newly rejected.

- [ ] **Step 5: Run compiler tests and verify RED**

Run: `go test ./engine/go/blueprint -run 'CompileGraphRejects|CompileGraphAllowsLegacyExecFanout' -count=1`

Expected: malformed graphs compile or overwrite maps.

- [ ] **Step 6: Implement compile-time checks and typed assignment**

Validate nodes/variables before constructing maps; inspect built-in `*Port.kind` for direction/type. Compatibility is exact kind or either side `portKindAny`. Implement `assignPortValue` so target kind never changes, arrays/any values clone, concrete-to-any stores actual value, and any-to-concrete propagates `setAnyValue` errors. Keep custom `IPort.SetValue` behavior unchanged.

- [ ] **Step 7: Run engine tests and race**

Run: `go test ./engine/go/blueprint -count=1`

Expected: PASS.

Run: `go test -race ./engine/go/blueprint -count=1`

Expected: PASS with no race report.

- [ ] **Step 8: Commit Task 3**

```powershell
git add engine/go/blueprint/compiler.go engine/go/blueprint/loader.go engine/go/blueprint/port.go engine/go/blueprint/runtime.go engine/go/blueprint/*_test.go
git commit -m "fix: validate blueprint graph contracts"
```

### Task 4: Instance lifecycle leases, Sleep cancellation, and timer registration

**Files:**
- Modify: `engine/go/blueprint/blueprint.go`
- Modify: `engine/go/blueprint/continuation.go`
- Modify: `engine/go/blueprint/sleep.go`
- Modify: `engine/go/blueprint/system_nodes.go`
- Modify: `engine/go/blueprint/runtime.go`
- Modify: `engine/go/blueprint/blueprint_test.go`
- Modify: `engine/go/blueprint/sleep_test.go`
- Modify: `engine/go/blueprint/timer_test.go`

**Interfaces:**
- Produces: instance `tryAcquireLease() bool`, `releaseLease()`, `markReleasedAndDrainTimers()`.
- Produces: instance-scoped timer registration token/state machine.
- Produces: `ErrGraphReleased` returned by continuation resume after release.

- [ ] **Step 1: Write failing release/continuation tests**

Test `ReleaseGraph` before Sleep expiry, custom continuation resume after release, in-flight lease completion, and release called from an execution chain without deadlock.

- [ ] **Step 2: Run lifecycle tests and verify RED**

Run: `go test ./engine/go/blueprint -run 'ReleaseGraph|Continuation.*Released|Sleep.*Release' -count=1`

Expected: downstream execution still occurs after release.

- [ ] **Step 3: Implement lifecycle lease and central continuation gate**

Acquire a lease in `Blueprint.Do` while holding `Blueprint.mu.RLock`, release when the synchronous execution segment returns, and require a fresh lease in `Continuation.Resume`. `ReleaseGraph` removes the instance, marks it released, closes its signal, drains timers, and never waits for in-flight leases.

- [ ] **Step 4: Replace Sleep AfterFunc with release-aware timer selection**

Use `time.NewTimer` and a goroutine selecting timer expiry versus `instance.releasedCh`; stop and drain safely on release, and rely on `Continuation.Resume` for the final gate.

- [ ] **Step 5: Write failing timer race/state tests**

Cover synchronous `SafeAfterFunc` callback, callback-before-ID-bind, release during bind, concurrent local timers with unique IDs, manual cancel, fired removal, and non-idempotent external cancel called exactly once.

- [ ] **Step 6: Run timer tests and verify RED**

Run: `go test ./engine/go/blueprint -run 'Timer|SafeAfterFunc' -count=1`

Expected: timer map retains fired IDs or registration/cancel counts are wrong.

- [ ] **Step 7: Implement timer token/tombstone state machine**

Create pending registration under the lifecycle lock before external scheduling, capture the internal token in callbacks, bind the public ID afterward, and retain the small tombstone until binding resolves. Store local `*time.Timer` handles on the instance, not on each Graph session. All external module/cancel calls occur after locks are released.

- [ ] **Step 8: Run lifecycle verification**

Run: `go test ./engine/go/blueprint -count=20`

Expected: PASS 20 consecutive runs.

Run: `go test -race ./engine/go/blueprint -count=1`

Expected: PASS with no race report.

- [ ] **Step 9: Commit Task 4**

```powershell
git add engine/go/blueprint/blueprint.go engine/go/blueprint/continuation.go engine/go/blueprint/sleep.go engine/go/blueprint/system_nodes.go engine/go/blueprint/runtime.go engine/go/blueprint/*_test.go
git commit -m "fix: bind async work to blueprint instances"
```

### Task 5: Hot-reload state snapshots and function-local variables

**Files:**
- Modify: `engine/go/blueprint/blueprint.go`
- Modify: `engine/go/blueprint/functions.go`
- Modify: `engine/go/blueprint/init_test.go`
- Modify: `engine/go/blueprint/functions_test.go`
- Modify: `docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`

**Interfaces:**
- Produces: instance runtime state object containing compiled graph, variables, and its own lock.
- Preserves: existing instances for graphs removed from a hot-reload set; new `Create` cannot find removed graphs.

- [ ] **Step 1: Write failing hot-reload migration tests**

Test same-name/same-normalized-type value preservation, new defaults, removed variables, changed-type reset, removed graph behavior, and old suspended session isolation from new sessions.

- [ ] **Step 2: Run hot-reload tests and verify RED**

Run: `go test ./engine/go/blueprint -run 'HotReload.*Variable|HotReload.*RemovedGraph|HotReload.*Suspended' -count=1`

Expected: new variables are missing and old sessions share the replaced instance fields.

- [ ] **Step 3: Implement runtime-state copy/swap**

Group `compiled`, `variables`, and `variableMu` in a state object. `Do` captures one state pointer under `Blueprint.mu.RLock`. During apply, lock in the order `Blueprint.mu -> oldState.variableMu`, deep-clone compatible values, construct a new state/lock, and swap the pointer. Do not call external code while locked.

- [ ] **Step 4: Write failing function-local isolation tests**

Add tests for consecutive calls resetting local variables, caller/function same-name isolation, recursive frame isolation, and async function resume retaining only that call's local state.

- [ ] **Step 5: Run function tests and verify lock-ownership RED**

Run: `go test ./engine/go/blueprint -run 'Function.*Variable|Function.*Recursive|Function.*Async' -count=1`

Expected: repeated-call and same-name semantic tests may already pass; a package-local capture node records `child.variableMu` and fails `childLock != parentLock` because the child currently shares the parent lock. Keep already-passing semantic tests as regression coverage.

- [ ] **Step 6: Remove parent variable-lock sharing and correct docs**

Delete `child.variableMu = n.graph.variableMu`; allow `runEntrance` to initialize the function Graph's own variable map and lock. Update the maintenance rule to state that function calls use fresh local variables per invocation and communicate only through ports.

- [ ] **Step 7: Run engine tests, race, and benchmarks**

Run: `go test ./engine/go/blueprint -count=1`

Run: `go test -race ./engine/go/blueprint -count=1`

Run: `go test ./engine/go/blueprint -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=3s -benchmem -count=5`

Expected: tests/race PASS; median allocation growth <= 5% and median `ns/op` growth <= 15% versus recorded baseline.

- [ ] **Step 8: Commit Task 5**

```powershell
git add engine/go/blueprint/blueprint.go engine/go/blueprint/functions.go engine/go/blueprint/init_test.go engine/go/blueprint/functions_test.go docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md
git commit -m "fix: isolate blueprint runtime variable states"
```

### Task 6: Final compatibility and regression verification

**Files:**
- Modify only files required by failures attributable to Tasks 1-5.

- [ ] **Step 1: Run formatting and focused suites**

Run: `gofmt -w` on modified Go files, then `go test ./engine/go/blueprint -count=1` and focused root legacy tests.

- [ ] **Step 2: Run full required verification**

```powershell
go test -race ./engine/go/blueprint -count=1
go test ./engine/go/blueprint -count=20
go test ./...
go test -race ./... -count=1
go vet ./...
cd frontend
npm.cmd run test:layout
npm.cmd run build
```

Expected: all modified-area tests pass. Report the 3 known root sample/schema failures separately if still present with unchanged output.

- [ ] **Step 3: Run committed and optional external legacy audits**

Run committed fixture audit unconditionally. If `build/bin/vgf` exists, run the external sample audit recursively and report discovered, migrated, exported, and mismatched counts; absence is a documented skip, not a pass.

- [ ] **Step 4: Inspect diff and working tree ownership**

Verify no unrelated `VERSION`, module-library, reference-search, or project settings changes were reverted or staged accidentally.

- [ ] **Step 5: Request final code review**

Review each task commit for compatibility, lock order, error behavior, and test gaps before declaring completion.
