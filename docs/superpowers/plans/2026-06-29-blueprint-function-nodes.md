# Blueprint Function Nodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make function library items create real call nodes and add explicit function entry/return node factories for function graphs.

**Architecture:** Keep normal node schemas in `nodeRegistry.ts`, but add dedicated factories for function call, entry, and return nodes because their ports are driven by function signatures rather than static JSON schemas. Store function metadata in `GraphDocument.nodes[].properties` so nodes survive save/load/copy/paste without serializing Rete internals.

**Tech Stack:** Vue 3, TypeScript, Rete.js, existing Node-based frontend tests.

---

### Task 1: Guard Function Node Wiring

**Files:**
- Create: `frontend/tests/functionNodes.test.js`
- Modify: `frontend/package.json`

- [x] **Step 1: Write the failing test**

Create `frontend/tests/functionNodes.test.js` that reads `App.vue`, `createEditor.ts`, `nodeRegistry.ts`, and `document.ts`, then asserts the function node factories, editor API, document properties, and module-library interactions are wired.

- [x] **Step 2: Run test to verify it fails**

Run: `node tests/functionNodes.test.js` from `frontend/`.
Expected: FAIL because `createFunctionCallNode`, `addFunctionCallNode`, and function metadata properties do not exist.

- [x] **Step 3: Add the test to the existing script**

Update `frontend/package.json` so `npm run test:layout` also runs `node tests/functionNodes.test.js`.

### Task 2: Add Function Node Factories

**Files:**
- Modify: `frontend/src/editor/document.ts`
- Modify: `frontend/src/editor/types.ts`
- Modify: `frontend/src/editor/nodeRegistry.ts`

- [x] **Step 1: Extend document metadata**

Add `FunctionNodeRole`, `FunctionNodeSource`, `FunctionNodeMetadata`, and optional function metadata fields to `NodeProperties`.

- [x] **Step 2: Add node fields**

Add matching function metadata fields to `BlueprintNode`.

- [x] **Step 3: Implement factories**

Add `createFunctionCallNode`, `createFunctionEntryNode`, and `createFunctionReturnNode`. Function call nodes have exec input/output plus signature-driven data inputs/outputs; entry nodes have exec output and signature input params as outputs; return nodes have exec input and signature output params as inputs.

### Task 3: Wire Editor Creation and Persistence

**Files:**
- Modify: `frontend/src/editor/createEditor.ts`

- [x] **Step 1: Add editor API**

Expose `addFunctionCallNode`, `addFunctionEntryNode`, and `addFunctionReturnNode` on `BlueprintEditorHandle`.

- [x] **Step 2: Restore function nodes**

Teach restore and paste to recreate `origin.function.call`, `origin.function.entry`, and `origin.function.return` from saved properties.

- [x] **Step 3: Snapshot metadata**

Serialize function metadata from `BlueprintNode` into `NodeProperties`.

### Task 4: Enable Module Library Function Items

**Files:**
- Modify: `frontend/src/App.vue`

- [x] **Step 1: Convert placeholder click into creation**

Make click/double-click and pointer-drag for function module items call `editor.addFunctionCallNode(...)` rather than only updating the status bar.

- [x] **Step 2: Add function graph terminal creation**

When creating a workspace `.obpf`, seed the document with function entry and return nodes. Add helper actions for the active function graph so the user can recreate terminals if deleted.

### Task 5: Verify

**Files:**
- No new files.

- [x] **Step 1: Run frontend function-node test**

Run: `node tests/functionNodes.test.js`.
Expected: PASS.

- [x] **Step 2: Run frontend test script**

Run: `npm run test:layout`.
Expected: PASS.

- [x] **Step 3: Run frontend build**

Run: `npm run build`.
Expected: PASS.
