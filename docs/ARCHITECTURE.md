# OriginBlueprint Architecture

## Core Principle

OriginBlueprint uses Go for domain logic and platform capabilities, and Vue/TypeScript for presentation and high-frequency editor interaction.

The goal is to keep business rules maintainable in Go without putting frame-by-frame canvas work across the Wails bridge.

## Go Responsibilities

Put the following code in Go:

- Graph document definitions, validation, and version migration.
- Opening, saving, importing, and exporting blueprint files.
- Legacy `.vgf` conversion.
- Workspace, recent-file, project, and resource management.
- Node metadata and configuration loading.
- Variable definitions, data types, and connection compatibility rules.
- Blueprint validation, compilation, execution, and debugging services.
- Search or batch operations over large document sets.
- Operating-system integration and other Wails desktop capabilities.

Go should be the authoritative implementation of persistent business rules.

## Vue and TypeScript Responsibilities

Put the following code in Vue/TypeScript:

- Rete.js editor construction and rendering.
- Node, socket, connection, group, and selection visuals.
- Canvas pan, zoom, drag, resize, box selection, and connection gestures.
- Menus, panels, tabs, dialogs, and other presentation state.
- Keyboard and pointer event handling.
- Hover, focus, selection, preview, and other temporary UI state.
- Immediate visual feedback required during an interaction.

TypeScript owns the live Rete.js representation, but it does not define persistent business truth independently of the graph document contract.

## Performance Boundary

Never call Go once per animation frame, pointer-move event, zoom step, or node-drag update.

High-frequency interactions stay entirely in the frontend. Send a consolidated update to Go only at a meaningful boundary, such as:

- A drag or resize operation finishes.
- A connection is created or removed.
- A command transaction completes.
- The user saves, validates, compiles, or executes the graph.

For example, node dragging is rendered and updated in TypeScript. Its final position may be synchronized after pointer release.

## Data Ownership

`GraphDocument` is the interchange contract between Go and TypeScript. It must be serializable and versioned.

- Go owns file-format compatibility, durable validation, and migrations.
- TypeScript owns the current editable in-memory view used by Rete.js.
- File persistence must use the document contract instead of serializing Rete.js internal objects.
- New schema fields should be optional or migrated explicitly when compatibility matters.
- Shared identifiers and enum values must remain stable across both languages.

Avoid maintaining unrelated Go and TypeScript models that can silently diverge. When practical, generate Wails bindings and TypeScript-facing types from Go definitions. Frontend-only display fields may remain local.

## Command Flow

Use this general data flow:

```text
Pointer/keyboard input
        |
        v
Vue + Rete.js interaction
        |
        v
Frontend command/transaction completes
        |
        v
GraphDocument update or Wails service call
        |
        v
Go validation, persistence, compilation, or execution
```

Go should return structured results and errors. Vue decides how those results are presented to the user.

## Placement Checklist

Before adding a feature, ask:

1. Does it run continuously while the user drags, zooms, or draws? Keep it in TypeScript.
2. Is it temporary visual or selection state? Keep it in TypeScript.
3. Must the rule remain identical in desktop, web, CLI, or tests? Put it in Go when the target can use Go services.
4. Does it involve persistence, migration, validation, compilation, or execution? Put it in Go.
5. Does it require an operating-system API? Put it behind a Go/Wails service.
6. Would moving it to Go introduce frequent bridge calls? Keep the interactive portion in TypeScript and call Go only at transaction boundaries.

## Web Compatibility

Frontend editor code must not directly depend on Wails globals. Access desktop functions through the platform abstraction in `frontend/src/platform.ts`.

- Desktop implementation calls Go through Wails.
- Web implementation uses browser APIs or a future HTTP/WebSocket backend.
- Core canvas behavior must work without the desktop backend.

Go-only domain services may later be exposed through HTTP or WebSocket when a web deployment needs the same behavior.

## Code Organization Direction

As the project grows, prefer these ownership boundaries:

```text
OriginBlueprint/
  internal/
    graph/       document model and validation
    migration/   schema and legacy migrations
    compiler/    compilation and execution preparation
    workspace/   files, projects, and resources
  app.go         thin Wails-facing application service
  frontend/src/
    editor/      Rete.js and high-frequency canvas behavior
    components/  Vue presentation components
    stores/      frontend session and UI state
    platform.ts  desktop/web capability abstraction
```

Do not move existing code only to match this proposed layout. Refactor into it when a feature is being changed or when a module has become difficult to maintain.

## Runtime Boundary

Blueprint execution is implemented in Go and operates only on a serialized `GraphDocument` snapshot. The Vue/Rete editor never executes node business logic.

- `StartGraph` creates a cancellable background session.
- Go evaluates data dependencies on demand and advances control through Exec connections.
- Execution progress is emitted in batches so loops do not call the frontend once per animation frame.
- Vue applies node state highlights and renders logs, results, and variable snapshots.
- `StopGraph` cancels the active Go context; execution also has a hard step limit to contain malformed graphs.

## Review Rule

New business rules should include Go tests. New editor interactions should include focused frontend tests where practical and a manual interaction check for pointer behavior. Changes that cross the document contract should test serialization or round-trip behavior.
