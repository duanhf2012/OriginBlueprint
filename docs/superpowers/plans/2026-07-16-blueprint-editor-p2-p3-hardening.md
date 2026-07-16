# Blueprint Editor P2/P3 Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining editor validation, autosave, settings durability, legacy unknown-field, test-governance, and Undo risks without changing known `.vgf` or Go VM execution semantics.

**Architecture:** Keep the desktop structural validator for multi-issue UI diagnostics, then run the production Go loader/compiler as the final gate. Add small pure TypeScript policy/history modules, preserve legacy unknown JSON in opaque field bags, and route all non-graph JSON persistence through the existing atomic writer.

**Tech Stack:** Go 1.23, Wails v2, Vue 3, TypeScript 4.6, Rete.js v2, Vitest 0.34.6.

## Global Constraints

- Do not change known `.vgf` field, port, edge-order, or execution semantics.
- Do not change Go VM execution, scheduling, or hot paths.
- Browser validation must return an explicit `engine.unavailable` warning.
- Autosave never opens a dialog and never overwrites compatibility-limited content.
- Undo history is bounded at exactly 100 full snapshots.
- Preserve the pre-existing `frontend/package.json.md5` and `originblueprint.project` working-tree changes outside commits.

---

### Task 1: Production engine validation and pre-normalization protection

**Files:**
- Modify: `graph.go`
- Modify: `app_test.go`
- Modify: `frontend/src/platform.ts`
- Modify: `frontend/src/App.vue`
- Modify generated: `frontend/wailsjs/go/main/App.d.ts`, `frontend/wailsjs/go/main/App.js`
- Test: `app_test.go`
- Test: `frontend/tests/p2p3Behavior.test.ts`

**Interfaces:**
- Produces: `func (a *App) ValidateGraphForWorkspace(content, workspaceRoot, sourcePath string) ([]ValidationIssue, error)`
- Produces: `validateGraphWithEngine(content, workspaceRoot, sourcePath string) *ValidationIssue`
- Produces: `platform.validateGraph(content: string, workspaceRoot?: string, sourcePath?: string)`
- Consumes: `blueprint.Blueprint.Init`, runtime schema documents, workspace `.obpf` files.

- [ ] **Step 1: Write failing Go tests for production compiler rules**

Add table tests which submit native documents with duplicate producers, native Exec fan-out, a data dependency cycle, duplicate entrance IDs, an unknown executable node, and a mismatched workspace function signature. Each test asserts an `error` whose code is `engine.parse`, `engine.compile`, or `engine.definition`.

- [ ] **Step 2: Verify RED**

Run: `go test . -run 'TestValidateGraphForWorkspace' -count=1 -v`

Expected: FAIL because `ValidateGraphForWorkspace` does not exist.

- [ ] **Step 3: Implement the isolated production-loader gate**

Use an internal structural validation node:

```go
type validationExecNode struct {
    blueprint.BaseExecNode
    name string
}
func (n *validationExecNode) GetName() string { return n.name }
func (n *validationExecNode) Exec() (int, error) { return 0, nil }
```

Create temporary `nodes/` and `graphs/` directories, write the effective schema documents, copy workspace `.obpf` files except `sourcePath`, write current content at its workspace-relative alias when possible, register structural factories, and call `Blueprint.Init`. Always remove the temporary root with `defer os.RemoveAll`.

- [ ] **Step 4: Add failing frontend policy tests for raw-source errors**

In `p2p3Behavior.test.ts`, test a pure helper that marks any raw validation error as source-protected and that browser fallback includes `engine.unavailable`.

- [ ] **Step 5: Integrate open/save validation before normalize**

Call `platform.validateGraph(file.content, workspaceRoot.value, file.path)` before `normalizeDocument`. Store original issues on the tab, set fatal protection after restore, show the logger, and pass workspace/source arguments from test/save flows. Do not clear protection until a successful recovery-copy save.

- [ ] **Step 6: Run focused tests and build**

Run: `go test . -run 'TestValidateGraphForWorkspace|TestValidateGraphService' -count=1`

Run in `frontend/`: `npm.cmd exec vitest -- run tests/p1Behavior.test.ts tests/p2p3Behavior.test.ts && npm.cmd run build`

Expected: PASS.

---

### Task 2: Atomic settings/config and real autosave

**Files:**
- Modify: `app.go`
- Modify: `app_test.go`
- Create: `frontend/src/autoSavePolicy.ts`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/tests/p2p3Behavior.test.ts`

**Interfaces:**
- Produces: `autoSaveIntervalMs(mode: AutoSaveMode): number`
- Produces: `isAutoSaveEligible(tab, requiresNativePersistence): boolean`
- Produces: `autoSaveDirtyTabs(): Promise<void>`
- Changes: `writeAppConfig`, `recordRecent`, `recordExportDirectory` return `error`.

- [ ] **Step 1: Write failing Go atomic persistence tests**

Test that default project settings creation and `SaveProjectSettings` use the injectable atomic-write boundary. Test app-config marshal/write failures are returned and logged by callers rather than ignored.

- [ ] **Step 2: Verify RED and implement atomic settings/config writes**

Run: `go test . -run 'Test(ProjectSettings|AppConfig).*Atomic' -count=1 -v`

Expected RED, then route all three JSON write paths through `writeFileAtomically` and return errors. Preserve successful graph/open operations when only recent-file metadata fails by logging the metadata error.

- [ ] **Step 3: Write failing autosave policy tests**

Assert exact intervals and eligibility rejection for untitled, clean, fatal, restore-loss, native-required legacy, and already-saving tabs.

- [ ] **Step 4: Verify RED and implement the pure policy**

Run: `npm.cmd exec vitest -- run tests/p2p3Behavior.test.ts`

Expected: FAIL because `autoSavePolicy.ts` does not exist, then PASS after minimal implementation.

- [ ] **Step 5: Integrate one managed interval and non-interactive persistence**

Create/clear the interval when settings or workspace change. Share validation/export/atomic-save logic with manual save. Save active and inactive dirty tabs without switching UI tabs; keep failed tabs dirty and summarize failures.

- [ ] **Step 6: Run focused and full-area tests**

Run: `go test . -run 'Test(ProjectSettings|AppConfig)' -count=1`

Run in `frontend/`: `npm.cmd run test:layout && npm.cmd run build`

Expected: PASS.

---

### Task 3: Legacy unknown JSON field round-trip

**Files:**
- Modify: `graph.go`
- Modify: `legacy.go`
- Modify: `frontend/src/editor/document.ts`
- Modify: `legacy_safety_test.go`
- Modify: `vgf_compat_audit_test.go`
- Test fixture: inline minimal JSON in `legacy_safety_test.go`

**Interfaces:**
- Produces: `GraphLegacyState.ExtraRootFields`
- Produces: `GraphLegacyState.ExtraNodeFields`
- Produces: `GraphLegacyState.ExtraEdgeFields`
- Produces helpers to collect and merge unknown `json.RawMessage` fields.

- [ ] **Step 1: Write failing round-trip tests**

Use a legacy fixture containing unknown scalar/object/array fields at root, visible and hidden nodes, and visible and hidden edges. Migrate, marshal/unmarshal `GraphDocument`, export, then compare every unknown JSON value with `reflect.DeepEqual` after decoding.

- [ ] **Step 2: Verify RED**

Run: `go test . -run 'TestLegacyUnknownFieldsRoundTrip' -count=1 -v`

Expected: FAIL because unknown fields disappear.

- [ ] **Step 3: Collect opaque field bags during migration**

Decode each object both into its typed struct and `map[string]json.RawMessage`, remove the known-key set, and store remaining values by root, node ID/class, and edge ordinal.

- [ ] **Step 4: Merge extras during export**

Marshal known output objects to maps, merge only unknown keys, then marshal the final root. Require node ID/class match and existing edge ordinal; never attach extras to a new node or edge.

- [ ] **Step 5: Add TypeScript opaque state types and run compatibility tests**

Run: `go test . -run 'Legacy|VGF' -count=1`

Run in `frontend/`: `npm.cmd run test:layout && npm.cmd run build`

Expected: PASS with no checked-in `.vgf` modifications.

---

### Task 4: Bounded, transaction-aware Undo for inline controls

**Files:**
- Create: `frontend/src/editor/history.ts`
- Modify: `frontend/src/editor/BlueprintControl.vue`
- Modify: `frontend/src/editor/BlueprintNode.vue`
- Modify: `frontend/src/editor/createEditor.ts`
- Modify: `frontend/tests/p2p3Behavior.test.ts`
- Modify: `frontend/tests/p1Safety.test.js`

**Interfaces:**
- Produces: `pushBoundedHistory<T>(stack: T[], value: T, limit = 100): void`
- Produces DOM events: `origin-control-edit-start`, `origin-control-change`, `origin-control-edit-commit`.

- [ ] **Step 1: Write failing bounded-history behavior tests**

Push 101 distinct values, assert length 100 and oldest value 1. Test a small transaction state helper does not commit unchanged focus/blur and commits multiple changes once.

- [ ] **Step 2: Verify RED and implement history helpers**

Run: `npm.cmd exec vitest -- run tests/p2p3Behavior.test.ts`

Expected: FAIL because history helpers do not exist, then PASS.

- [ ] **Step 3: Emit control transaction events before mutation**

Inputs begin on focus and commit on blur. Array and dynamic-branch add/remove buttons begin on pointerdown and commit after their synchronous mutation. Continuous input only emits change.

- [ ] **Step 4: Consume transactions and bound every history push**

Capture one pre-edit snapshot, set a changed flag on change, and push at commit only when changed. Replace every direct `undoStack.push`/`redoStack.push` with the bounded helper; clear pending control state on load, undo, redo, and destroy.

- [ ] **Step 5: Run tests and build**

Run in `frontend/`: `npm.cmd run test:layout && npm.cmd run build`

Expected: PASS.

---

### Task 5: Test governance, review, and final verification

**Files:**
- Modify: `frontend/tests/implicitEntryLinks.test.ts`
- Modify: `frontend/tests/selectionGeometry.test.ts`
- Modify: `frontend/package.json`
- Modify generated Wails bindings if required
- Modify: `README.md` only for user-visible autosave/validation/legacy guarantees.

- [ ] **Step 1: Convert omitted TypeScript scripts to Vitest suites**

Use `describe/it/expect` without changing their behavioral assertions. Add them, `p2p3Behavior.test.ts`, and `legacyPropertyPreservation.test.js` to `test:layout`.

- [ ] **Step 2: Update the authoritative README**

Document actual autosave eligibility, production engine validation, source-protection behavior, bounded Undo, and legacy unknown-field preservation. Do not rewrite unrelated mojibake text.

- [ ] **Step 3: Self-review scope and compatibility**

Inspect the complete diff, search for direct JSON settings/config `os.WriteFile`, direct unbounded history pushes, and changed `.vgf` files. Fix only confirmed issues with a new failing test first.

- [ ] **Step 4: Run full verification**

From root:

```powershell
go test ./... -count=1
go test -race ./engine/go/blueprint -count=1
go test -race ./... -count=1
go vet ./...
wails build
```

From `frontend/`:

```powershell
npm.cmd run test:layout
npm.cmd run build
npm.cmd audit --omit=dev
```

Expected: every command exits 0 and production audit reports 0 vulnerabilities.

- [ ] **Step 5: Commit only task files**

Exclude `frontend/package.json.md5` and `originblueprint.project`. Commit the verified implementation in coherent task commits or one final P2/P3 commit if intermediate commits would capture a temporarily incomplete cross-layer API.

