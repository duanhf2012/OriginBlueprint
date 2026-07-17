# Core Graph Analyzer Save Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a language-neutral core graph analyzer that emits exhaustive structured diagnostics and prevents formal persistence only when the current graph has a confirmed high-risk core defect.

**Architecture:** Keep document-contract checks in the facade, move reachability/liveness/cycle analysis into a focused iterative analyzer, and mark core issues with explicit `BlocksSave` metadata. Runtime target diagnostics remain separate. All formal persistence paths share one frontend gate; blocked writes create atomic recovery snapshots through a dedicated Go service.

**Tech Stack:** Go, Wails v2, Vue 3, TypeScript, Vitest, Rete.js v2.

## Global Constraints

- `GraphDocument` remains the persisted Go/TypeScript contract; do not serialize Rete internals.
- Do not change `.vgf`, `.obp`, or `.obpf` file format semantics.
- Unknown and opaque legacy content must remain preserved and must not become a confirmed cycle merely because its flow semantics are unknown.
- Go runtime compilation may set `BlocksRun`, but must never set `BlocksSave`.
- Confirmed core defects use structured fields; never parse localized `Message` text to make persistence decisions.
- Reachability, liveness, and cycle algorithms are iterative and `O(V+E)`.
- Do not invoke Go on mouse movement, zoom, animation frames, or node dragging.
- Preserve the user-owned `frontend/package.json.md5` modification and untracked `originblueprint.project`.

## File Map

- Create `core_graph_analyzer.go`: graph construction, reachability, liveness, SCC, and structured-loop normalization.
- Create `core_graph_analyzer_test.go`: analyzer behavior and deep-graph tests.
- Modify `graph.go`: diagnostic contract, structural blocker classification, port semantics, analyzer delegation.
- Modify `engine_validation.go`: mark Go compiler diagnostics target-specific and run-only.
- Create `recovery.go` and `recovery_test.go`: atomic snapshot persistence, retention, listing, reading, deletion.
- Modify `app.go`: expose recovery methods to Wails.
- Create `frontend/src/saveGate.ts`: pure persistence-decision policy.
- Modify `frontend/src/platform.ts` and `frontend/src/App.vue`: recovery APIs, shared save gate, startup recovery UX.
- Modify frontend tests, package test gate, Wails bindings, and `README.md`.

---

### Task 1: Structured Diagnostic Contract and Structural Save Blocking

**Files:**
- Modify: `graph.go`
- Modify: `app_test.go`
- Modify: `frontend/src/platform.ts`
- Modify: `frontend/src/editor/document.ts`

**Interfaces:**
- Produces Go `ValidationIssue{SourcePath, BlocksSave, BlocksRun, Target}`.
- Produces the matching TypeScript optional fields.
- Produces `coreIssueBlocksSave(code string) bool`.

- [ ] **Step 1: Write the failing blocker-matrix test**

```go
func TestCoreIssueBlocksSaveUsesExplicitLanguageNeutralCodes(t *testing.T) {
    blocking := []string{"node.missing-id", "node.duplicate-id", "connection.dangling", "connection.missing-port", "connection.type-mismatch", "connection.multiple-producers", "flow.exec-fanout", "flow.data-cycle", "flow.exec-cycle"}
    for _, code := range blocking {
        if !coreIssueBlocksSave(code) { t.Errorf("%s should block save", code) }
    }
    nonBlocking := []string{"flow.unreachable-node", "flow.missing-entry", "flow.possible-cycle", "node.legacy-placeholder", "engine.compile"}
    for _, code := range nonBlocking {
        if coreIssueBlocksSave(code) { t.Errorf("%s should not block save", code) }
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./... -run TestCoreIssueBlocksSaveUsesExplicitLanguageNeutralCodes -count=1`

Expected: build failure because `coreIssueBlocksSave` is undefined.

- [ ] **Step 3: Implement the contract and explicit classification**

Use this exact Go shape and matching TypeScript fields:

```go
type ValidationIssue struct {
    Severity   string   `json:"severity"`
    Code       string   `json:"code"`
    Message    string   `json:"message"`
    NodeID     string   `json:"nodeId,omitempty"`
    NodeIDs    []string `json:"nodeIds,omitempty"`
    SourcePath string   `json:"sourcePath,omitempty"`
    BlocksSave bool     `json:"blocksSave,omitempty"`
    BlocksRun  bool     `json:"blocksRun,omitempty"`
    Target     string   `json:"target,omitempty"`
}
```

Implement `coreIssueBlocksSave` as an explicit switch. After structural validation, set `BlocksSave` only for listed core error codes. Do not make every error blocking by default.

- [ ] **Step 4: Verify and commit**

Run: `go test ./... -run "TestCoreIssueBlocksSave|TestValidateGraph" -count=1`

Run: `cd frontend; npm run build`

Expected: both exit 0.

Commit:

```powershell
git add graph.go app_test.go frontend/src/platform.ts frontend/src/editor/document.ts
git commit -m "feat: classify core save-blocking diagnostics"
```

---

### Task 2: Iterative Core Graph Analyzer

**Files:**
- Create: `core_graph_analyzer.go`
- Create: `core_graph_analyzer_test.go`
- Modify: `graph.go`
- Modify: `app_test.go`

**Interfaces:**
- Consumes: `GraphDocument`, `map[string]GraphNode`, `map[string]portDefinition`.
- Produces: `analyzeCoreGraph(document GraphDocument, nodes map[string]GraphNode, ports map[string]portDefinition) []ValidationIssue`.
- Replaces: old `validateExecutionFlow` internals.

- [ ] **Step 1: Write failing reachability/liveness/fanout tests**

```go
func TestCoreAnalyzerReportsReachabilityAndLiveness(t *testing.T) {
    issues := validateGraph(graphWithEntryPrintUnreachableExecAndUnusedLiteral())
    assertIssue(t, issues, "flow.unreachable-node", false)
    assertIssue(t, issues, "flow.unused-data-node", false)
}

func TestCoreAnalyzerBlocksMultipleProducersAndExecFanout(t *testing.T) {
    issues := validateGraph(graphWithDuplicateProducerAndFanout())
    assertIssue(t, issues, "connection.multiple-producers", true)
    assertIssue(t, issues, "flow.exec-fanout", true)
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./... -run "TestCoreAnalyzerReportsReachabilityAndLiveness|TestCoreAnalyzerBlocksMultipleProducersAndExecFanout" -count=1`

Expected: FAIL because the new diagnostics are absent.

- [ ] **Step 3: Implement iterative graph construction, reachability, and liveness**

Build edges only when endpoints and ports are known. Count data producers by target node/input and Exec fanout by source node/output. Traverse Exec forward from every entry. Starting from data inputs consumed by reachable executable nodes, traverse the reverse data graph to mark live pure producers. Report pure nodes outside that set as `flow.unused-data-node` warnings.

- [ ] **Step 4: Write failing exhaustive SCC tests**

Cover data self-loop, two-node data cycle, multiple disjoint data SCCs, Exec self-loop, multiple disjoint Exec SCCs, valid `ForLoopBreak`, invalid external break, and a 20,000-node deep graph.

```go
func TestCoreAnalyzerReturnsEveryConfirmedCycle(t *testing.T) {
    issues := validateGraph(graphWithTwoExecSCCsAndTwoDataSCCs())
    assertIssueCount(t, issues, "flow.exec-cycle", 2)
    assertIssueCount(t, issues, "flow.data-cycle", 2)
    for _, issue := range cycleIssues(issues) {
        if !issue.BlocksSave || len(issue.NodeIDs) == 0 { t.Fatalf("issue = %#v", issue) }
    }
}
```

- [ ] **Step 5: Verify SCC tests RED**

Run: `go test ./... -run "TestCoreAnalyzerReturnsEveryConfirmedCycle|TestCoreAnalyzerAllowsStructuredLoopBreak|TestCoreAnalyzerHandlesDeepGraph" -count=1`

Expected: FAIL because exhaustive SCC analysis is absent.

- [ ] **Step 6: Implement iterative SCC and structured-break normalization**

Use iterative Tarjan or Kosaraju and return stable sorted components. A one-node component is cyclic only with a self-edge. Exclude a break edge only when its target is a known `origin.flow.for-loop-break` break input and its source is proven to belong exclusively to that loop body using the existing compiler-neutral reachability rule. Opaque legacy flow semantics yield `flow.possible-cycle` warnings, never confirmed blockers.

- [ ] **Step 7: Verify and commit**

Run:

```powershell
gofmt -w graph.go core_graph_analyzer.go core_graph_analyzer_test.go app_test.go
go test ./... -run "CoreAnalyzer|ValidateGraph|DeepExecutionFlow|Legacy" -count=1
```

Expected: exit 0.

Commit:

```powershell
git add graph.go core_graph_analyzer.go core_graph_analyzer_test.go app_test.go
git commit -m "feat: analyze core graph reachability and cycles"
```

---

### Task 3: Separate Runtime-Target Diagnostics

**Files:**
- Modify: `engine_validation.go`
- Modify: `app_test.go`

**Interfaces:**
- Produces Go compiler issues with `Target: "target.go"`, `BlocksRun: true`, `BlocksSave: false`, accurate `SourcePath`.

- [ ] **Step 1: Write failing target-separation tests**

```go
func TestGoCompilerIssueNeverBlocksCoreSave(t *testing.T) {
    issues, err := NewApp().ValidateGraphForWorkspace(graphWithUnknownGoExecNodeJSON(), "", "graph.obp")
    if err != nil { t.Fatal(err) }
    issue := requireIssue(t, issues, "engine.definition")
    if issue.Target != "target.go" || !issue.BlocksRun || issue.BlocksSave { t.Fatalf("issue = %#v", issue) }
}
```

Add a workspace-function case proving an error from another file carries that file's source path and does not block saving the current file.

- [ ] **Step 2: Verify RED**

Run: `go test ./... -run "TestGoCompilerIssueNeverBlocksCoreSave|TestWorkspaceCompilerIssueCarriesSource" -count=1`

Expected: FAIL because target metadata is absent.

- [ ] **Step 3: Implement and verify target metadata**

Set target/run metadata in `engineValidationIssue`. Preserve structured `BlueprintError.SourcePath` and map temporary validation paths back to source/workspace paths. Never set `BlocksSave` from engine errors.

Run: `go test ./... -run "ValidateGraphForWorkspace|GoCompilerIssue|WorkspaceCompilerIssue" -count=1`

Expected: exit 0.

- [ ] **Step 4: Commit**

```powershell
git add engine_validation.go app_test.go
git commit -m "feat: separate Go target validation from save safety"
```

---

### Task 4: Atomic Recovery Snapshot Service

**Files:**
- Create: `recovery.go`
- Create: `recovery_test.go`
- Modify: `app.go`

**Interfaces:**
- Produces `SaveRecoverySnapshot(sourcePath, tabID, documentJSON, issuesJSON string) (RecoverySnapshotResult, error)`.
- Produces `ListRecoverySnapshots() ([]RecoverySnapshotResult, error)`.
- Produces `ReadRecoverySnapshot(path string) (string, error)`.
- Produces `DeleteRecoverySnapshot(path string) error`.
- Produces `DeleteRecoverySnapshots(sourcePath, tabID string) error`.

- [ ] **Step 1: Write failing service tests**

Test deterministic keying, atomic-write injection, newest-five retention, 30-day expiry, refusal to access paths outside the recovery root, and cleanup by source/tab key.

```go
func TestRecoverySnapshotsRetainNewestFiveAtomically(t *testing.T) {
    app := newRecoveryTestApp(t)
    for index := 0; index < 7; index++ {
        if _, err := app.SaveRecoverySnapshot("graph.obp", "tab", fmt.Sprintf(`{"version":%d}`, index), `[]`); err != nil { t.Fatal(err) }
    }
    got, err := app.ListRecoverySnapshots()
    if err != nil { t.Fatal(err) }
    if len(got) != 5 { t.Fatalf("snapshots = %d, want 5", len(got)) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./... -run RecoverySnapshot -count=1`

Expected: build failure because recovery APIs are undefined.

- [ ] **Step 3: Implement recovery storage**

Use the application config directory plus `recovery/`. Key by SHA-256 of normalized source path, falling back to tab ID. Store schema version 1, RFC3339 timestamp, source path, tab ID, raw document, and issues. Use `a.writeAtomically`; ensure every caller-provided read/delete path resolves inside the recovery root. Keep five snapshots per key and clean files older than 30 days.

- [ ] **Step 4: Verify and commit**

Run:

```powershell
gofmt -w recovery.go recovery_test.go app.go
go test ./... -run RecoverySnapshot -count=1
go test -race ./... -run RecoverySnapshot -count=1
```

Expected: exit 0.

Commit:

```powershell
git add recovery.go recovery_test.go app.go
git commit -m "feat: persist atomic graph recovery snapshots"
```

---

### Task 5: Frontend Save Gate and Recovery UX

**Files:**
- Create: `frontend/src/saveGate.ts`
- Modify: `frontend/src/platform.ts`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/tests/p2p3Behavior.test.ts`
- Create: `frontend/tests/coreGraphSaveGate.test.js`
- Modify: `frontend/package.json`
- Regenerate: `frontend/wailsjs/go/main/App.d.ts`
- Regenerate: `frontend/wailsjs/go/main/App.js`

**Interfaces:**
- Produces `saveGateDecision(issues, strict): { blocked: boolean; blockingIssues: ValidationIssue[] }`.
- Ensures manual save, save-as, force-save, save-all, and autosave share one gate.

- [ ] **Step 1: Write failing pure policy tests**

```ts
it('blocks core blockers but never target-only errors', () => {
  expect(saveGateDecision([{ severity: 'error', code: 'flow.exec-cycle', blocksSave: true }], false).blocked).toBe(true)
  expect(saveGateDecision([{ severity: 'error', code: 'engine.compile', target: 'target.go', blocksRun: true }], false).blocked).toBe(false)
  expect(saveGateDecision([{ severity: 'error', code: 'flow.unreachable-node' }], false).blocked).toBe(false)
  expect(saveGateDecision([{ severity: 'error', code: 'flow.unreachable-node' }], true).blocked).toBe(true)
  expect(saveGateDecision([{ severity: 'warning', code: 'flow.possible-cycle' }], true).blocked).toBe(false)
})
```

- [ ] **Step 2: Verify RED**

Run: `cd frontend; npm exec vitest -- run tests/p2p3Behavior.test.ts`

Expected: FAIL because `saveGateDecision` is undefined.

- [ ] **Step 3: Implement the pure policy**

Core `blocksSave` always blocks. In strict mode, non-target errors block except `flow.possible-cycle`. Target-only diagnostics never block persistence.

- [ ] **Step 4: Write and verify a failing integration guard**

`coreGraphSaveGate.test.js` must assert that manual and automatic save call one shared validation/gate helper before persistence; blocked paths call `platform.saveRecoverySnapshot`; successful paths call `platform.deleteRecoverySnapshots`; save-as and force-save cannot bypass.

Run: `cd frontend; node tests/coreGraphSaveGate.test.js`

Expected: FAIL because shared wiring is absent.

- [ ] **Step 5: Wire persistence gate and recovery UI**

Add `validateForPersistence(tab, document)`. It calls validation, stores diagnostics, computes the gate decision, highlights all blocking node IDs, writes a recovery snapshot when blocked, keeps dirty state, and returns false before formal persistence. Group diagnostics by current graph, workspace dependency, and runtime target; show a fatal tab marker when `BlocksSave` is present. On startup, list recovery snapshots and show Restore/Keep/Delete actions. Restore opens a dirty protected tab without changing the source. Successful formal save deletes snapshots for the source/tab key.

- [ ] **Step 6: Regenerate bindings and verify**

Run:

```powershell
wails build
cd frontend
npm run test:layout
npm run build
```

Expected: exit 0. Restore `frontend/package.json.md5` to the user's pre-task value and do not stage it.

- [ ] **Step 7: Commit**

```powershell
git add frontend/src/saveGate.ts frontend/src/platform.ts frontend/src/App.vue frontend/tests/p2p3Behavior.test.ts frontend/tests/coreGraphSaveGate.test.js frontend/package.json frontend/wailsjs/go/main/App.d.ts frontend/wailsjs/go/main/App.js
git commit -m "feat: block unsafe graph persistence with recovery"
```

---

### Task 6: Documentation, Self-Review, and Full Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README**

Document that only language-neutral core diagnostics set `BlocksSave`; Go diagnostics are target-specific. Document `flow.exec-cycle`, `flow.data-cycle`, `flow.unreachable-node`, `flow.unused-data-node`, recovery retention, and restore behavior.

- [ ] **Step 2: Review the complete diff against the approved spec**

Search for message-string parsing, Go issues setting `BlocksSave`, recursive graph traversal, formal-save bypasses, and unexpected `.vgf` format changes. Confirm unknown legacy content remains opaque and preserved.

- [ ] **Step 3: Run fail-fast full verification**

```powershell
go test ./... -count=1
go vet ./...
go test -race ./... -count=1
cd frontend
npm run test:layout
npm run build
npm audit --omit=dev
cd ..
wails build
git diff --check
```

Expected: all commands exit 0; audit reports 0 production vulnerabilities; Wails creates `build/bin/OriginBlueprint.exe`.

- [ ] **Step 4: Commit documentation and verify scope**

```powershell
git add README.md
git commit -m "docs: document core graph save safety"
```

After the commit, `git status --short` must show only the pre-existing user-owned `frontend/package.json.md5` modification and untracked `originblueprint.project`.
