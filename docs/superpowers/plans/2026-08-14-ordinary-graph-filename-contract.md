# Ordinary Graph Filename Contract Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make ordinary `.obp` and `.vgf` blueprints use the disk filename stem as their runtime identity, stop persisting `graphName` for ordinary documents, and preserve function metadata for `.obpf` files.

**Architecture:** The Go loader owns runtime identity and derives ordinary graph names from `filepath.Base(path)`. The frontend keeps `graphName` only as transient editor state for ordinary tabs, removes it at every persistence boundary, and preserves it for function blueprints. Existing server data is normalized, and the battle compatibility test loads the same `nodes` directory as production and verifies configured names can be created.

**Tech Stack:** Go 1.26, TypeScript 4.6, Vue 3, Vitest, Wails.

---

### Task 1: Lock the Go loader naming contract

**Files:**
- Modify: `engine/go/blueprint/loader_conflict_test.go`
- Modify: `engine/go/blueprint/loader.go`

**Step 1: Write the failing tests**

Add table-driven tests around `parseGraphFile`:

- `PetAuto_hama.obp` containing `"graphName":"PetCmd_hama.obp"` must return `PetAuto_hama`.
- `CommandAimChange.obpf` containing `"graphName":"CommandAimChange"` must return `CommandAimChange` and `isFunction=true`.

Use valid minimal schema-version-1 documents so the tests exercise the real native document parser.

**Step 2: Run the focused test and verify RED**

Run: `go test ./engine/go/blueprint -run TestParseGraphFileUsesFilenameForOrdinaryDocuments -count=1`

Expected: FAIL because the current loader returns the persisted ordinary `graphName`.

**Step 3: Implement the smallest loader change**

In `parseGraphFile`, initialize `name` from the filename stem. Only when the extension is `.obpf` should the loader mark the graph as a function and replace `name` with a non-empty trimmed `document.GraphName`. Keep function aliases unchanged.

**Step 4: Format and verify GREEN**

Run:

```powershell
gofmt -w engine/go/blueprint/loader.go engine/go/blueprint/loader_conflict_test.go
go test ./engine/go/blueprint -run 'TestParseGraphFileUsesFilenameForOrdinaryDocuments|TestParseGraphFilePreservesFunctionGraphName' -count=1
```

Expected: PASS.

**Step 5: Commit**

```powershell
git add engine/go/blueprint/loader.go engine/go/blueprint/loader_conflict_test.go
git commit -m "fix(engine): 普通蓝图使用文件名注册"
```

### Task 2: Define and test the frontend persistence boundary

**Files:**
- Create: `frontend/src/graphPersistence.ts`
- Create: `frontend/tests/graphPersistence.test.ts`
- Modify: `frontend/package.json`

**Step 1: Write the failing Vitest tests**

Test a pure serialization helper with these cases:

- Ordinary `.obp` serialization has no own `graphName` property.
- Legacy `.vgf` serialization has no own `graphName` property.
- Function `.obpf` serialization preserves `graphName`, `functionId`, and `functionSignature`.
- Windows and slash-separated paths derive the correct filename stem for transient editor state.

Add the new test file to `test:layout`.

**Step 2: Run the focused test and verify RED**

Run: `npm exec vitest -- run tests/graphPersistence.test.ts`

Expected: FAIL because `graphPersistence.ts` does not exist.

**Step 3: Implement the pure helper**

Create helpers that:

- recognize `.obpf` paths case-insensitively;
- derive a filename stem from either Windows or slash-separated paths;
- clone an ordinary `GraphDocument` while omitting `graphName`;
- preserve the full function document;
- serialize the resulting persisted form with optional indentation.

Do not weaken the internal `GraphDocument` type: `graphName` remains required in memory.

**Step 4: Run the focused test and verify GREEN**

Run: `npm exec vitest -- run tests/graphPersistence.test.ts`

Expected: PASS.

**Step 5: Commit**

```powershell
git add frontend/src/graphPersistence.ts frontend/tests/graphPersistence.test.ts frontend/package.json
git commit -m "test(frontend): 锁定普通蓝图持久化契约"
```

### Task 3: Route all editor saves through the persistence boundary

**Files:**
- Modify: `frontend/src/App.vue`
- Modify: `frontend/tests/graphPersistence.test.ts`

**Step 1: Extend behavior tests before wiring**

Add assertions that the exact serialized JSON passed to validation, recovery snapshots, native saves, and legacy export can share the same helper contract without mutating the in-memory document.

**Step 2: Wire open and save flows**

In `App.vue`:

- import the persistence helpers;
- remove the local duplicate `isFunctionBlueprintPath`;
- after opening a non-function file, set the in-memory `document.graphName` from the file path stem so stale historical metadata is not displayed or propagated;
- use the serialized persisted form in validation, recovery snapshots, auto-save, manual save, and legacy export;
- preserve `.obpf` function metadata;
- create new ordinary `.vgf` files without `graphName` in persisted JSON;
- leave transient editor calls such as `editor.getDocument(...)` unchanged.

**Step 3: Run focused and frontend tests**

Run:

```powershell
npm exec vitest -- run tests/graphPersistence.test.ts tests/runtimeNodeLegacyClass.test.ts
npm run test:layout
npm run build
```

Expected: PASS.

**Step 4: Commit**

```powershell
git add frontend/src/App.vue frontend/tests/graphPersistence.test.ts
git commit -m "fix(frontend): 普通蓝图保存时移除内部名称"
```

### Task 4: Add the server regression and normalize production blueprints

**Files:**
- Modify: `service/battleservice/blueprintnode/vm_compatibility_test.go`
- Modify: `mp1config/Server/BluePrint/battle/vgf/petAuto/PetAuto_hama.obp`
- Modify: `mp1config/Server/BluePrint/battle/vgf/petAuto/PetAuto_xiaozhu.obp`
- Modify: `mp1config/Server/BluePrint/battle/vgf/petAuto/PetAuto_huangshulang.obp`
- Create: `docs/fixes/2026-08-14-blueprint-filename-contract-fix.md`

**Step 1: Write the failing server regression**

Change the compatibility test to load `battle/nodes`, matching production. After successful initialization, assert `Create` returns a non-zero VM ID for `PetAuto_hama`, `PetAuto_xiaozhu`, and `PetAuto_huangshulang`; close each created VM through the existing public lifecycle API.

**Step 2: Run the focused test and verify RED**

Run: `go test ./service/battleservice/blueprintnode -run TestVMCompilesAllBattleBlueprintFiles -count=1`

Expected: FAIL because the vendored loader currently registers the three native documents under their persisted `graphName` values.

**Step 3: Normalize the three data files**

Remove only the top-level `graphName` field from the three `.obp` files. Preserve all nodes, connections, variables, formatting, and unrelated submodule changes.

**Step 4: Run the focused test and verify GREEN**

Run: `go test ./service/battleservice/blueprintnode -run TestVMCompilesAllBattleBlueprintFiles -count=1`

Expected: PASS because the current vendored loader already falls back to the filename stem when `graphName` is absent.

**Step 5: Record the fix and commit only owned paths**

Document the symptom, root cause, compatibility contract, changed files, and verification evidence. Commit the three blueprint files inside the `mp1config` submodule without staging existing Excel or PetModelConfig changes. Then commit the server test, fix note, and updated submodule pointer in the parent repository.

### Task 5: Run the final verification matrix

**Files:**
- Verify only; no expected source changes.

**Step 1: Verify OriginBlueprint**

Run:

```powershell
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
go test ./... -count=1
Set-Location frontend
npm run test:layout
npm run build
Set-Location ..
git diff --check
git status --short
```

**Step 2: Verify mp1server**

Run:

```powershell
go test ./service/battleservice/blueprintnode -run TestVMCompilesAllBattleBlueprintFiles -count=1
go test ./service/battleservice/... -count=1
git diff --check
git status --short
git -C mp1config diff --check
git -C mp1config status --short
```

**Step 3: Inspect final diffs**

Confirm:

- ordinary engine registration never trusts `document.GraphName`;
- `.obpf` names and aliases are unchanged;
- every ordinary editor persistence path omits `graphName`;
- only the three intended pet files changed in `mp1config`;
- pre-existing unrelated workspace changes remain unstaged and unmodified.

**Step 4: Report**

Report the final behavior, commit IDs, exact PASS/FAIL commands, any environment limitations, submodule branch/commit state, and remaining end-to-end risk.
