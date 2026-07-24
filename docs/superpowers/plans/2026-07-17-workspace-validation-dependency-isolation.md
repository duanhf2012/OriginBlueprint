# Workspace Validation Dependency Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent unrelated workspace function files from causing false `target.go` compiler errors while preserving errors from the current graph's real function dependency closure.

**Architecture:** Keep production engine directory loading unchanged. Build a lightweight editor-side index of workspace `.obpf` files, select the iterative dependency closure rooted at function references in the current document, and copy only selected functions into the temporary validation directory. Rebuild structured engine errors with mapped source paths before rendering their messages.

**Tech Stack:** Go 1.x, `encoding/json`, `filepath.WalkDir`, table-driven Go tests, Wails v2.

## Global Constraints

- Do not change `.vgf`, `.obp`, or `.obpf` persistence contracts.
- `target.go` diagnostics may set `blocksRun` but must never set `blocksSave`.
- Do not modify production `engine/go/blueprint` directory-loading semantics.
- Preserve duplicate function-key conflict detection.
- Preserve user-owned changes in `frontend/package.json.md5`, `frontend/wailsjs/go/models.ts`, and `originblueprint.project`.

---

### Task 1: Reproduce unrelated workspace contamination

**Files:**
- Modify: `app_test.go`

**Interfaces:**
- Consumes: `ValidateGraphForWorkspace(content, workspaceRoot, sourcePath)`.
- Produces: regression behavior proving unrelated `.obpf` files do not affect a valid current graph.

- [ ] **Step 1: Write the failing test**

Create a valid current graph with `origin.event.begin`. Add an unrelated `.obpf` containing an unknown executable node and assert that no `engine.compile`, `engine.parse`, or `engine.definition` issue is returned.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestValidateGraphForWorkspaceIgnoresUnreferencedWorkspaceFunction -count=1`

Expected: FAIL because the current implementation copies and compiles the unrelated function.

- [ ] **Step 3: Add dependency-selection tests**

Add tests proving that a directly referenced invalid function and a transitively referenced invalid function remain visible, and that duplicate aliases among referenced functions remain a conflict.

- [ ] **Step 4: Run the focused tests before implementation**

Run: `go test . -run "TestValidateGraphForWorkspace(IgnoresUnreferencedWorkspaceFunction|IncludesReferencedWorkspaceFunction|IncludesTransitiveWorkspaceFunction|PreservesReferencedFunctionAliasConflict)" -count=1`

Expected: the isolation test fails for the contamination reason; dependency-preservation tests document current or desired behavior.

### Task 2: Select the workspace function dependency closure

**Files:**
- Modify: `engine_validation.go`
- Test: `app_test.go`

**Interfaces:**
- Consumes: current `GraphDocument`, workspace root, source path, and workspace `.obpf` documents.
- Produces: `prepareValidationGraphDocuments(...) (currentPath string, err error)` with the same public behavior but a reduced sandbox file set.

- [ ] **Step 1: Define lightweight index records**

Add an internal record containing absolute path, workspace-relative path, aliases, and referenced function keys. Normalize lookup keys with trimmed forward slashes while preserving case-sensitive engine aliases.

- [ ] **Step 2: Index workspace function files**

Walk `.obpf` files once. Always index the relative path; for valid JSON also index `functionId` and `graphName`, and collect `functionId/functionName` from function-call and timer-by-function nodes. Skip the current source so unsaved editor content remains authoritative.

- [ ] **Step 3: Compute the iterative closure**

Seed a queue from the current document's function references. For every key, select every owning record, copy it once, and enqueue its references. Copying every owner preserves duplicate-key conflict diagnostics.

- [ ] **Step 4: Keep referenced malformed files diagnosable**

When a call references a relative `.obpf` path, select that indexed file even if its JSON cannot be decoded; let the existing engine return the structured parse error.

- [ ] **Step 5: Run focused tests**

Run the Task 1 focused command.

Expected: PASS.

### Task 3: Map structured error messages to real paths

**Files:**
- Modify: `engine_validation.go`
- Modify: `app_test.go`

**Interfaces:**
- Consumes: `*blueprint.BlueprintError` and `validationSourceMap`.
- Produces: `ValidationIssue` whose `SourcePath` and `Message` both contain the original workspace path.

- [ ] **Step 1: Strengthen the existing source-path test**

Assert that `issue.Message` contains the original function path and does not contain the temporary graphs directory.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestWorkspaceCompilerIssueCarriesOriginalSourcePath -count=1`

Expected: FAIL because only `ValidationIssue.SourcePath` is currently mapped.

- [ ] **Step 3: Rebuild the structured error**

Copy `BlueprintError`, replace `SourcePath` with `validationSourceMap.originalPath`, and pass the copy to `engineValidationIssue`. Preserve `Cause`, stage, node, PC, error wrapping, and the workspace-function prefix.

- [ ] **Step 4: Run focused tests**

Run: `go test . -run "TestWorkspaceCompilerIssueCarriesOriginalSourcePath|TestGoCompilerIssueNeverBlocksCoreSave" -count=1`

Expected: PASS.

### Task 4: Verification and documentation

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: implemented validation behavior.
- Produces: authoritative documentation and release-ready evidence.

- [ ] **Step 1: Document dependency-scoped Go validation**

Update the editor validation section to state that target compilation includes the current graph and its transitive workspace function dependencies, not unrelated workspace functions.

- [ ] **Step 2: Run formatting and narrow tests**

Run: `gofmt -w engine_validation.go app_test.go`

Run: `go test . -run "TestValidateGraphForWorkspace|TestWorkspaceCompilerIssue|TestGoCompilerIssue" -count=1`

Expected: PASS.

- [ ] **Step 3: Run complete verification**

Run: `go test ./... -count=1`

Run: `go vet ./...`

Run: `go test -race ./... -count=1`

Run from `frontend/`: `npm.cmd run test:layout`

Run from `frontend/`: `npm.cmd run build`

Run: `wails build`

Expected: all commands PASS.

- [ ] **Step 4: Review the final diff**

Confirm no persistence format, core save gate, production engine loader, generated frontend bindings, or user-owned files changed.

