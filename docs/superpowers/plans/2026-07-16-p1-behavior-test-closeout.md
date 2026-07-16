# P1 Behavior Test Closeout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace P1 source-string assertions with executable behavior tests, make restore-loss decisions testable without mounting Rete, and restore a trustworthy green project test gate.

**Architecture:** Add two pure TypeScript modules. `documentSafety.ts` owns open/save policy; `restorePlan.ts` converts a `GraphSnapshot` plus a production node-preparation callback into restorable items and a `RestoreLossReport`. `App.vue` and `createEditor.ts` consume these modules so tests exercise the same logic used in production. Go sample tests receive repository-owned runtime schema fixtures through the same parsing path used by production.

**Tech Stack:** Vue 3, TypeScript 4.6, Vite 3, Vitest 0.34.x, Go 1.23.

## Global Constraints

- Do not change `.vgf` import/export or migration semantics.
- Do not add jsdom, Vue Test Utils, Playwright, or Wails GUI automation.
- Keep Sequence compatibility: persisted `0` means default `3`; valid range is `1..256`.
- Fatal restore state never permits force-overwrite.
- Do not stage or rewrite the pre-existing `frontend/package.json.md5` or `originblueprint.project` changes.
- Do not use external sibling repositories or their node definitions.

---

### Task 1: Add executable document safety policy tests

**Files:**
- Create: `frontend/src/documentSafety.ts`
- Create: `frontend/tests/p1Behavior.test.ts`
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/tests/p1Safety.test.js`

**Interfaces:**
- Produces: `graphPathKey(path: string, caseInsensitive: boolean): string`
- Produces: `findOpenTab<T extends { path: string }>(tabs: readonly T[], path: string, caseInsensitive: boolean): T | undefined`
- Produces: `hasRestoreLoss(report?: RestoreLossReport | null): boolean`
- Produces: `compatibilitySaveOptions(input: { fatal: boolean; hasLoss: boolean; formatAllowsForce: boolean }): readonly CompatibilitySaveAction[]`
- Produces: `resolveCompatibilitySaveAction(action: CompatibilitySaveAction, input: { fatal: boolean; hasLoss: boolean; formatAllowsForce: boolean }): 'cancel' | 'recovery-copy' | 'force-source-with-backup'`

- [ ] **Step 1: Install the compatible test runner without upgrading unrelated dependencies**

Run: `npm.cmd install --save-dev vitest@0.34.6 --save-exact`

Verify: `git diff -- frontend/package.json frontend/package-lock.json` contains Vitest and its required dependency resolution only.

- [ ] **Step 2: Write failing policy behavior tests**

Create tests which import `../src/documentSafety` and assert:

```ts
expect(findOpenTab([dirtyTab], 'e:/graphs/test.obp', true)).toBe(dirtyTab)
expect(dirtyTab.document).toBe(originalDocument)
expect(dirtyTab.dirty).toBe(true)
expect(resolveCompatibilitySaveAction('copy', lossInput)).toBe('recovery-copy')
expect(resolveCompatibilitySaveAction('cancel', lossInput)).toBe('cancel')
expect(resolveCompatibilitySaveAction('force', lossInput)).toBe('force-source-with-backup')
expect(compatibilitySaveOptions({ fatal: true, hasLoss: true, formatAllowsForce: true })).toEqual(['copy', 'cancel'])
```

- [ ] **Step 3: Run the test and verify RED**

Run: `npm.cmd exec vitest -- run tests/p1Behavior.test.ts`

Expected: FAIL because `src/documentSafety.ts` does not exist.

- [ ] **Step 4: Implement the minimal pure policy module and use it from App.vue**

Implement the five interfaces above. Replace App-local `graphPathKey` and `hasRestoreLoss` with imports. Use `findOpenTab` inside `openGraph`; use `compatibilitySaveOptions` to populate the dialog and `resolveCompatibilitySaveAction` before calling `platform.saveGraph` or `platform.forceSaveGraph`.

- [ ] **Step 5: Run behavior tests and build**

Run: `npm.cmd exec vitest -- run tests/p1Behavior.test.ts && npm.cmd run build`

Expected: all behavior tests PASS; Vue type checking and Vite build exit `0`.

---

### Task 2: Build and consume a pure restore plan

**Files:**
- Create: `frontend/src/editor/restorePlan.ts`
- Modify: `frontend/tests/p1Behavior.test.ts`
- Modify: `frontend/src/editor/createEditor.ts`
- Modify: `frontend/src/editor/document.ts`

**Interfaces:**
- Produces: `PreparedRestoreNode<T> = { snapshot: NodeSnapshot; node: T; inputKeys: readonly string[]; outputKeys: readonly string[]; alteredNodes?: RestoreAlteredNode[] }`
- Produces: `RestorePlan<T> = { nodes: PreparedRestoreNode<T>[]; connections: ConnectionSnapshot[]; report: RestoreLossReport }`
- Produces: `buildRestorePlan<T>(snapshot: GraphSnapshot, prepare: (node: NodeSnapshot, typeId: string) => PreparedRestoreNode<T> | null): RestorePlan<T>`
- Produces: `normalizeDynamicOutputCount(requested: number): number`

- [ ] **Step 1: Add failing restore behavior tests**

Test unknown nodes, missing endpoints, missing source/target ports, an empty visual graph with populated `legacy` state, invalid dynamic counts, and valid `then12`/`then255` connections. Assertions must inspect `plan.report` and `plan.connections`, not source text.

- [ ] **Step 2: Run the test and verify RED**

Run: `npm.cmd exec vitest -- run tests/p1Behavior.test.ts`

Expected: FAIL because `buildRestorePlan` and `normalizeDynamicOutputCount` are not implemented.

- [ ] **Step 3: Implement the restore planner**

The planner must:

```ts
if (!typeId) report.droppedNodes.push({ id, typeId: '', reason: 'missing-type-id' })
if (!prepared) report.droppedNodes.push({ id, typeId, reason: 'unknown-node-type' })
if (!source || !target) report.droppedConnections.push({ ...connection, reason: 'missing-endpoint' })
if (!source.outputKeys.includes(connection.sourceOutput)) report.droppedConnections.push({ ...connection, reason: 'missing-source-port' })
if (!target.inputKeys.includes(connection.targetInput)) report.droppedConnections.push({ ...connection, reason: 'missing-target-port' })
```

Concatenate `prepared.alteredNodes` into the report and preserve valid connection snapshot objects unchanged.

- [ ] **Step 4: Refactor createEditor.restore to consume the plan**

Move node creation, property application, dynamic output normalization and port-key extraction into the `prepare` callback. Add planned nodes/connections to Rete. Return `plan.report` from `restore` and keep `restoring = false` in `finally`.

- [ ] **Step 5: Run tests and build**

Run: `npm.cmd exec vitest -- run tests/p1Behavior.test.ts && npm.cmd run test:layout && npm.cmd run build`

Expected: behavior tests, existing source/layout tests, type checking and Vite build all PASS.

---

### Task 3: Repair repository-owned Go sample test inputs

**Files:**
- Create: `testdata/runtime_nodes/monster_choices.json`
- Modify: `legacy.go`
- Modify: `app_test.go`

**Interfaces:**
- Produces: `runtimeLegacyNodeSpecsFromDocuments(documents []RuntimeNodeSchemaDocument) map[string]runtimeLegacySpec`
- Produces: `migrateLegacyGraphWithRuntimeSpecs(data []byte, runtimeSpecs map[string]runtimeLegacySpec) (GraphDocument, error)`
- Existing `migrateLegacyGraph` remains the product entry point and delegates using `runtimeLegacyNodeSpecs()`.

- [ ] **Step 1: Preserve the three failing tests as RED evidence**

Run:

```powershell
go test . -run 'TestMigrateBuildBinVGFFilesShowsAllDefinedNodes|TestValidateChoiceskillEasyRecognizesMonsterChoiceSkillEntry|TestChoiceskillEasyUsesRuntimeJsonTitlesInsteadOfFallbackNames' -count=1
```

Expected: the same three tests FAIL before changes.

- [ ] **Step 2: Add a repository-owned runtime node fixture**

Create a minimal legacy runtime JSON schema for the six runtime classes present in the checked-in sample: `Entrance_MonsterChoiceSkill_40300`, `GetObjectInfo`, `GetMinHpTarget`, `GetSkillByType`, `AppendAiChoiceSkillAndTarget`, and `GetTargetsByCamp`. Port ids and types must match the checked-in `choiceskill_easy.vgf`; port `0` on the entrance output is `exec`. `EqualSwitch` continues using its existing static mapping.

- [ ] **Step 3: Extract injectable schema/spec construction**

Make `runtimeLegacyNodeSpecs()` load documents and delegate to `runtimeLegacyNodeSpecsFromDocuments`. Make `migrateLegacyGraph` delegate to `migrateLegacyGraphWithRuntimeSpecs` without changing its output for production inputs.

- [ ] **Step 4: Update tests to follow product format detection**

For `TestMigrateBuildBinVGFFilesShowsAllDefinedNodes`, first decode `GraphDocument`; native schema-version-1 samples assert non-empty `typeId` and skip legacy round-trip assertions. True legacy samples retain existing migration and round-trip assertions.

For the two choiceskill tests, load the repository fixture, build runtime specs with `runtimeLegacyNodeSpecsFromDocuments`, and call `migrateLegacyGraphWithRuntimeSpecs`.

- [ ] **Step 5: Run focused and full Go tests**

Run the focused command from Step 1, then `go test ./... -count=1`.

Expected: both commands PASS with no skipped replacement for the three repaired tests.

---

### Task 4: Final verification and scope audit

**Files:**
- Modify: `frontend/package.json` only if needed to include `vitest run tests/p1Behavior.test.ts` in `test:layout`
- Modify: `frontend/tests/p1Safety.test.js` to remove source assertions superseded by behavior tests

- [ ] **Step 1: Run all required verification**

Run from repository root:

```powershell
go test ./... -count=1
go test -race ./engine/go/blueprint -count=1
go vet ./...
```

Run from `frontend/`:

```powershell
npm.cmd run test:layout
npm.cmd run build
```

Expected: every command exits `0`.

- [ ] **Step 2: Audit the diff**

Run: `git diff --check` and `git status --short`.

Verify: no `.vgf` production file changed; no autosave, Undo/Redo, or engine compile-rule work entered the diff; `frontend/package.json.md5` and `originblueprint.project` remain outside the task scope.
