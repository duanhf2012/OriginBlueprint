# Continuation ResumeTo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dynamic continuation API that lets an asynchronous business node choose its success or failure Exec output when the callback completes.

**Architecture:** `SuspendForResume` creates a continuation marked as dynamic. `ResumeTo` validates a selected Exec output and feeds that target through the existing Execution dispatcher state machine; fixed `Suspend/Resume` behavior remains unchanged. Early callbacks persist the selected target beside pending output arguments.

**Tech Stack:** Go 1.x, existing `engine/go/blueprint` runtime, Go testing and race detector.

## Global Constraints

- Preserve all existing `Suspend(nextIndex)`, `Resume`, `ResumeAsync`, Delay and function-call behavior.
- A continuation is resumable exactly once.
- Execution-owned callbacks always resume through `ExecutionDispatcher`.
- Invalid target selection must not consume the continuation.
- Do not add RPC protocols, timeout, retry, frontend nodes or cancellation APIs in this change.

---

### Task 1: Dynamic Continuation Public API

**Files:**
- Modify: `engine/go/blueprint/continuation.go`
- Test: `engine/go/blueprint/execution_session_test.go`

**Interfaces:**
- Consumes: `BaseExecNode.Suspend(nextIndex int)` and `Continuation.Resume(outPortArgs ...any)`.
- Produces: `BaseExecNode.SuspendForResume() (*Continuation, error)` and `Continuation.ResumeTo(nextIndex int, outPortArgs ...any) error`.

- [x] **Step 1: Write failing success/failure branch tests**

Add a test node with two Exec outputs and one Integer data output. Start it through a manual dispatcher, then assert `ResumeTo(0, 41)` runs only Success and `ResumeTo(1, 42)` runs only Failure with the selected value.

- [x] **Step 2: Run the focused test and verify RED**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestContinuationResumeTo(Success|Failure)Branch' -count=1
```

Expected: compilation fails because `SuspendForResume` and `ResumeTo` do not exist.

- [x] **Step 3: Implement the minimal public API**

Add a dynamic flag to `Continuation`, factor Exec-output validation into a shared helper, create `SuspendForResume`, and route `ResumeTo` through an internal target-aware resume method. Return explicit errors when fixed and dynamic APIs are mixed.

- [x] **Step 4: Run focused tests and verify GREEN**

Run the Step 2 command. Expected: PASS.

### Task 2: Execution Dispatcher and Early Callback Semantics

**Files:**
- Modify: `engine/go/blueprint/execution_session.go`
- Test: `engine/go/blueprint/execution_session_test.go`

**Interfaces:**
- Consumes: `Continuation.ResumeTo(nextIndex int, outPortArgs ...any) error`.
- Produces: target-aware continuation scheduling that stores `pendingNextIndex` for callbacks arriving while Execution is Running.

- [x] **Step 1: Write concurrency and validation tests**

Cover an early `ResumeTo` called inside `Exec`, duplicate responses, invalid/non-Exec targets, fixed/dynamic API mixing, and cancellation before recovery.

- [x] **Step 2: Run focused tests**

Run:

```powershell
go test ./engine/go/blueprint -run 'TestContinuationResumeTo|TestContinuationDynamicAPI' -count=1
```

Expected: early callback chooses the wrong/default target or API validation assertions fail.

- [x] **Step 3: Make scheduling target-aware**

Change the internal scheduling signature to accept `nextIndex`, persist it with pending arguments, submit `resumeReservedAt(nextIndex, args...)`, and clear the pending target when the Execution finishes or is canceled.

- [x] **Step 4: Run focused tests and race verification**

```powershell
go test -race ./engine/go/blueprint -run 'TestContinuationResumeTo|TestContinuationDynamicAPI|TestContinuationResumeUsesExecutionDispatcher' -count=20
```

Expected: PASS with no race report.

### Task 3: Documentation and Regression Verification

**Files:**
- Modify: `docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`
- Modify: `docs/BLUEPRINT_ENGINE_COMPATIBILITY_ZH.md`

**Interfaces:**
- Documents the public API produced by Tasks 1 and 2.

- [x] **Step 1: Add a custom asynchronous business-node example**

Document `SuspendForResume`, success/failure `ResumeTo`, `ErrExecutionSuspended`, single-resume behavior and the requirement to handle callback errors.

- [x] **Step 2: Run engine and repository verification**

```powershell
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
go test -race ./... -count=1 -skip 'TestMigrateBuildBinVGFFilesShowsAllDefinedNodes|TestValidateChoiceskillEasyRecognizesMonsterChoiceSkillEntry|TestChoiceskillEasyUsesRuntimeJsonTitlesInsteadOfFallbackNames'
```

Expected: PASS. The three skipped tests depend on local `build/bin/vgf` samples whose matching business node JSON is not present in the repository.

- [x] **Step 3: Review and commit**

Run `git diff --check`, inspect staged scope, and commit the implementation without staging the pre-existing `frontend/package.json.md5` or `originblueprint.project` changes.
