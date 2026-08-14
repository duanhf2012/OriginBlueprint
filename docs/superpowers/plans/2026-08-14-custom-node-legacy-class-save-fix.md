# Custom Node legacyClass Save Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure custom runtime-schema nodes persist their Go runtime class name and repair older native documents that omitted `legacyClass`.

**Architecture:** Keep Go engine parsing unchanged. Derive a custom node's runtime class from the editor `NodeDefinition`, write it to each new `BlueprintNode`, and use the registry value as a fallback only when a persisted class is absent or blank.

**Tech Stack:** Vue 3, TypeScript, Rete.js v2, Vitest, Go.

## Constraints

- Do not infer runtime class names from slugged type IDs.
- Do not assign `legacyClass` to built-in runtime nodes such as `AddInt`.
- An explicitly persisted non-blank `legacyClass` must override the registry fallback.
- Do not change the Go parser or rewrite existing `.obpf` assets as part of this fix.
- Preserve the unrelated untracked `originblueprint.project` file.

### Task 1: Add a regression test for the editor/runtime contract

**Files:**
- Create: `frontend/tests/runtimeNodeLegacyClass.test.ts`
- Modify: `frontend/package.json`

- [ ] Add a runtime schema fixture containing custom `GetObjectInfo` and built-in `AddInt` definitions.
- [ ] Assert a newly created custom node has `legacyClass === 'GetObjectInfo'`.
- [ ] Assert a newly created built-in node has no `legacyClass`.
- [ ] Assert missing or blank persisted values fall back to `GetObjectInfo`.
- [ ] Assert an explicit persisted value wins over the fallback.
- [ ] Add the test file to the Vitest segment of `npm run test:layout`.
- [ ] Run `npm exec vitest -- run tests/runtimeNodeLegacyClass.test.ts` and confirm it fails for the missing behavior before implementation.

### Task 2: Carry the runtime class through the node registry

**Files:**
- Modify: `frontend/src/editor/nodeRegistry.ts`

- [ ] Add optional `legacyClass` metadata to `NodeDefinition`.
- [ ] Set it to `schema.sourceName` only when `schema.custom` is true.
- [ ] Copy that value onto newly created `BlueprintNode` instances.
- [ ] Export a resolver that trims persisted values, prefers a non-blank explicit value, and otherwise returns the registry metadata for the node type.

### Task 3: Repair omitted values while restoring documents

**Files:**
- Modify: `frontend/src/editor/createEditor.ts`

- [ ] Import the registry resolver.
- [ ] Use it when applying persisted node properties so older documents with an omitted or blank value recover the custom runtime class.
- [ ] Leave all other legacy properties unchanged.

### Task 4: Verify the fix

**Files:**
- Test: `frontend/tests/runtimeNodeLegacyClass.test.ts`
- Test: `frontend/tests/*.test.ts`
- Test: `engine/go/blueprint/...`

- [ ] Run `npm exec vitest -- run tests/runtimeNodeLegacyClass.test.ts` and confirm PASS.
- [ ] Run `npm run test:layout` and confirm PASS.
- [ ] Run `npm run build` and confirm PASS.
- [ ] Run `go test ./engine/go/blueprint -count=1` and confirm PASS.
- [ ] Run `go test ./...` and confirm PASS, or record any unrelated/environment failure precisely.
- [ ] Run `git diff --check` and inspect `git status --short` and the final diff.
- [ ] Commit only the implementation and tests, leaving `originblueprint.project` untouched.
