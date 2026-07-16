# Blueprint Editor P1 Safety Implementation Plan

> **Goal:** Eliminate the confirmed high-impact silent-loss and validator-crash paths while preserving legacy `.vgf` compatibility.

## Scope

1. Reopening a path that is already open focuses the existing tab and never reloads over unsaved state.
2. Editor restore reports every node/connection it cannot represent. Affected tabs cannot silently overwrite their source file.
3. Compatibility-limited saves default to a recovery copy. Explicit force-overwrite requires a second confirmation, creates a `.bak`, and then replaces the source through the backend safe-write path. A fatal/partial restore never offers force-overwrite.
4. Dynamic sequence outputs support the engine limit of 256 in both editor and validator.
5. Validation rejects hostile sequence/function/legacy-port sizes without allocating from untrusted counts, and execution-flow traversal is iterative.
6. Add focused Go and frontend regression coverage and run the project verification commands.

## Task 1: Backend validation bounds

**Files:** `graph.go`, `app_test.go`

- Add failing tests for negative and over-limit dynamic sequence counts, oversized function signatures, oversized legacy placeholder ports, and a deep execution chain.
- Introduce stable desktop validation limits matching the Go VM (`256`, `128`, `4096`).
- Validate counts before map/slice allocation and return structured issues instead of panicking.
- Replace recursive flow/cycle traversal with iterative traversal.
- Run focused Go tests.

## Task 2: Safe backend persistence

**Files:** `app.go`, new platform-specific atomic replacement helpers if required, `app_test.go`

- Add a failing test proving force-save preserves the previous bytes in `<path>.bak` and writes the new document.
- Route normal graph saves through a same-directory temporary file and safe replacement.
- Add `ForceSaveGraph`, which creates/syncs the backup before replacing the original.
- Ensure failures clean temporary files and do not report success early.
- Run focused persistence tests.

## Task 3: Restore-loss reporting and reopen protection

**Files:** `frontend/src/editor/createEditor.ts`, `frontend/src/editor/document.ts`, `frontend/src/App.vue`, focused frontend test

- Add a restore report containing dropped node and connection identifiers/reasons.
- Make restore cleanup use `try/finally`; propagate catastrophic restore failures separately.
- Capture the report on a tab and preserve it across tab switches.
- When an opened path already exists, focus that tab without replacing its document or dirty flag.
- Add regression assertions for both behaviors.

## Task 4: Compatibility-safe save UX

**Files:** `frontend/src/App.vue`, `frontend/src/platform.ts`, focused frontend test

- Intercept ordinary Save when restore loss exists.
- Offer recovery-copy, explicit force-overwrite, or cancel. Hide force-overwrite after a fatal/partial restore.
- Require a second destructive confirmation before force-overwrite.
- Call the backend force-save API and show the backup path on success.
- Clear compatibility-loss state only after a successful copy or intentional overwrite.

## Task 5: Sequence editor limit

**Files:** `frontend/src/editor/createEditor.ts`, focused frontend test

- Replace the editor-only limit of 12 with the shared product limit of 256.
- Verify restoring and snapshotting output indexes above 12 remains intact.

## Task 6: Verification and review

- Run focused tests after each task.
- Run `go test ./...`, `go test -race ./engine/go/blueprint -count=1`, `npm run test:layout`, and `npm run build`.
- Inspect the final diff for accidental format/legacy behavior changes and confirm the pre-existing dirty files remain untouched.
