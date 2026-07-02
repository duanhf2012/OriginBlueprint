# Blueprint Engine Compatibility Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add regression tests proving editor-authored `.obp/.obpf` files and legacy `.vgf` files can be loaded, compiled, and executed by `engine/go/blueprint`.

**Architecture:** Keep production code changes minimal. Add Go test helpers that simulate editor authoring operations (`AddNode`, `SetValue`, `Connect`, `Save`) instead of hand-writing graph JSON, then load those files through the same `loadGraphDir` path used by the engine.

**Tech Stack:** Go tests, `engine/go/blueprint`, existing JSON node definitions under `nodes/*.json`, representative legacy `.vgf` files under `build/bin/vgf`.

---

### Task 1: Legacy `.vgf` Go Compatibility Smoke

**Files:**
- Modify: `engine/go/blueprint/legacy_compatibility_test.go`

- [ ] Add a test that loads representative legacy `.vgf` files through `loadGraphDir`.
- [ ] Assert that the Go parser/loader returns either a compiled graph or a clear unsupported-node error.
- [ ] Keep byte-for-byte `.vgf` comparison out of scope.
- [ ] Run:

```powershell
go test ./engine/go/blueprint -run TestRepresentativeLegacyVGFFilesAreHandledByGoLoader -count=1 -v
```

### Task 2: Editor-Style `.obp` Authoring Harness

**Files:**
- Modify: `engine/go/blueprint/document_authoring_test.go`

- [ ] Add a test-only authoring helper with operations `AddNode`, `SetValue`, `Connect`, `SaveOBP`, and `SaveOBPF`.
- [ ] Use the helper to generate a graph with entry, sequence, math, dynamic branch, arrays, and return nodes.
- [ ] Load the saved `.obp` through `loadGraphDir`.
- [ ] Execute it with `NewGraph(...).Do(...)` and assert returned values.
- [ ] Run:

```powershell
go test ./engine/go/blueprint -run TestAuthoredOBPFromNodeOperationsRunsInGoEngine -count=1 -v
```

### Task 3: Function In Function

**Files:**
- Modify: `engine/go/blueprint/document_authoring_test.go`

- [ ] Use the authoring helper to save `functions/Double.obpf`.
- [ ] Use the authoring helper to save `functions/FormatDouble.obpf`, which calls `Double`.
- [ ] Use the authoring helper to save `main.obp`, which calls `FormatDouble`.
- [ ] Load all files through `loadGraphDir`.
- [ ] Execute the main graph and assert the nested function return value.
- [ ] Run:

```powershell
go test ./engine/go/blueprint -run TestAuthoredFunctionCanCallAnotherAuthoredFunction -count=1 -v
```

### Task 4: Schema Coverage Guard

**Files:**
- Modify: `engine/go/blueprint/document_authoring_test.go`

- [ ] Assert every top-level executable schema in `nodes/*.json` can be converted to the document execution contract or is listed as intentionally non-executable.
- [ ] If `nodes/json/**/*.json` exists, assert the files parse and their node names are visible for migration coverage.
- [ ] Run:

```powershell
go test ./engine/go/blueprint -run TestNodeJSONDefinitionsHaveCompatibilityCoverage -count=1 -v
```

### Task 5: Regression Verification

**Files:**
- No new files unless failures identify missing production support.

- [ ] Run focused Go tests.
- [ ] Run full Go engine tests.
- [ ] Run project-level Go tests if engine tests pass.
- [ ] Run frontend layout/build checks only if touched frontend code or display contracts.

```powershell
go test ./engine/go/blueprint -count=1
go test .
npm.cmd run test:layout -- --runInBand
npm.cmd run build
```

