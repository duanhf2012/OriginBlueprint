# OriginBlueprint Agent Guide

This file is the first stop for AI coding agents working on this project.

## Project Relationship

- `OriginBlueprint/` is the active project. Make product and code changes here.
- `../OriginNodeEditor/` is the legacy Python/PyQt editor used as compatibility reference only.
- Do not modify `../OriginNodeEditor/` unless the user explicitly asks for legacy-editor changes.
- Online files produced by the old editor are legacy `.vgf` JSON graphs. The new editor must continue to open, display, edit, validate where possible, and export them without silently losing content.

## Stack

- Desktop shell: Go + Wails v2.
- Frontend: Vue 3 + TypeScript + Vite.
- Graph editor: Rete.js v2.
- Node definitions: JSON files under `nodes/`.
- Runtime execution: Go, using serialized `GraphDocument` snapshots.

## Read Before Editing

Read these docs before making non-trivial changes:

- `docs/AI_PROJECT_CONTEXT_ZH.md`: fast project map for agents.
- `docs/LEGACY_COMPATIBILITY_ZH.md`: `.vgf` and old-node compatibility rules.
- `docs/ARCHITECTURE.md`: Go/frontend ownership boundary.
- `docs/NODE_JSON_FORMAT_ZH.md`: node JSON format.
- `docs/ORIGIN_NODE_EDITOR_PARITY.md`: parity checklist with the old editor.

## Core Architecture Rules

- `GraphDocument` is the durable interchange contract between Go and TypeScript.
- Go owns file persistence, migration, validation, runtime execution, workspace access, and Wails/platform services.
- TypeScript owns Rete editor construction, canvas interaction, visual state, node rendering, menus, selection, and pointer/keyboard gestures.
- Do not serialize Rete internals as the saved file format.
- Do not call Go on every animation frame, pointer move, zoom step, or node drag update. Call Go only at transaction boundaries such as save, validate, execute, import/export, or completed edits.

## Compatibility Rules

- Preserve old `.vgf` graph content. Unknown old nodes or edges must be retained in legacy state rather than discarded.
- Known legacy node classes map to current `origin.*` type IDs in `legacy.go` and `frontend/src/editor/runtimeNodeSchemas.ts`.
- If you add or rename a node type, update both migration/export logic and frontend schema conversion when compatibility is involved.
- If a new node must be usable by the old external parser, add an explicit export mapping to a legacy class.
- Treat `.vgf` round-trip behavior as high risk. Add tests for import, displayable document shape, validation, and legacy export.

## Important Files

- `graph.go`: `GraphDocument`, validation, stable node-port type table.
- `legacy.go`: legacy `.vgf` migration and export.
- `node_schemas.go`: runtime loading of `nodes/**/*.json`.
- `execution.go`: Go runtime execution semantics.
- `app.go`: Wails-facing file/workspace/platform services.
- `frontend/src/platform.ts`: desktop/browser capability abstraction.
- `frontend/src/App.vue`: app shell, tabs, open/save flow, node library loading.
- `frontend/src/editor/createEditor.ts`: Rete editor, snapshot/restore, connections, groups, gestures.
- `frontend/src/editor/nodeRegistry.ts`: node schema registration and node factory.
- `frontend/src/editor/runtimeNodeSchemas.ts`: legacy JSON node definition conversion.
- `frontend/src/editor/BlueprintNode.vue`: node visual layout.
- `frontend/src/editor/BlueprintControl.vue`: inline controls.

## Commands

Run from `OriginBlueprint/`:

```powershell
go test ./...
```

Run from `OriginBlueprint/frontend/`:

```powershell
npm run build
npm run test:layout
```

Desktop development:

```powershell
wails dev
```

Build executable:

```powershell
wails build
```

## Testing Expectations

- Business rules, migration, validation, execution, and file-format behavior need Go tests.
- Frontend editor behavior should get focused tests when practical, plus manual/visual checks for pointer and layout changes.
- Compatibility changes need round-trip tests against representative legacy `.vgf` files.
- Before reporting completion, run the narrow tests for the touched area and at least the relevant build/test command.

## Known Caution Points

- Some Chinese text in old JSON/doc files may look garbled in terminals depending on encoding. Do not rewrite large JSON files just to normalize display unless the user asks.
- The project root containing both `OriginBlueprint/` and `OriginNodeEditor/` may not be a git repository. Check before using git-based workflows.
- Existing docs may mention `OriginNodeEditor_old`; in this workspace the sibling legacy directory is `OriginNodeEditor`.
- Saving/export semantics around `.vgf` and `.obp` are compatibility-sensitive. Review `app.go`, `legacy.go`, and `docs/LEGACY_COMPATIBILITY_ZH.md` before changing them.
