# OriginBlueprint

[中文说明](README_CN.md)

OriginBlueprint is a desktop-first visual blueprint editor built with Go, Wails v2, Vue 3, TypeScript, and Rete.js v2. It is designed to edit legacy `.vgf` blueprints while also supporting native `.obp` graph files and `.obpf` function blueprint files.

![OriginBlueprint editor preview](docs/assets/OriginBlueprint.png)

## What It Does

- Edit node graphs with pan, zoom, box selection, copy/paste, undo/redo, node groups, alignment, and connection cutting.
- Open and preserve legacy `.vgf` files, including unknown legacy nodes and edges where possible.
- Save native `.obp` graph documents and `.obpf` function blueprint files.
- Load custom node definitions from JSON files under `nodes/`.
- Browse a workspace folder, open blueprints from the file tree, reveal files in Explorer/Finder, and refresh the node library.
- Validate graph structure, missing ports, type mismatches, missing entry nodes, unreachable execution flow, and possible execution cycles.
- Export selected nodes or the whole graph as a PNG image.
- Switch UI language between English and Chinese.

## File Types

| File | Purpose |
| --- | --- |
| `.vgf` | Legacy blueprint JSON. The editor can migrate and preserve legacy content. |
| `.obp` | Native OriginBlueprint graph document. |
| `.obpf` | Native function blueprint document. |
| `originblueprint.project` | Workspace-level editor settings such as layout sizes, language, and UI preferences. |
| `nodes/**/*.json` | Node definition files loaded into the module library. |

## Quick Start

Desktop development:

```powershell
wails dev
```

Frontend-only development:

```powershell
cd frontend
npm install
npm run dev
```

Build and verify:

```powershell
go test ./...
cd frontend
npm run test:layout
npm run build
```

Build the desktop executable:

```powershell
wails build
```

## Basic Usage

1. Open the application.
2. Use **File > Open Workspace** to select a project directory.
3. Open `.vgf`, `.obp`, or `.obpf` files from the file browser.
4. Drag nodes from the module library into the canvas.
5. Connect compatible ports. Execution ports represent control flow; data ports carry typed values.
6. Use the variables panel to create variables and variable groups.
7. Use **Test** to validate the graph.
8. Save with `Ctrl+S`, or use **Save As** to choose a new file.

Useful shortcuts:

| Shortcut | Action |
| --- | --- |
| `Ctrl+S` | Save current graph |
| `Ctrl+Shift+S` | Save As |
| `Ctrl+A` | Select all |
| `Ctrl+D` | Deselect all |
| `Ctrl+C / Ctrl+X / Ctrl+V` | Copy, cut, paste |
| `Ctrl+Z / Ctrl+Y` | Undo, redo |
| `Ctrl+G` | Create a group, or ungroup when a group is selected |
| `Ctrl+Shift+Q` | Reveal the current or selected blueprint file in the system file manager |
| `Ctrl+Alt+R` | Export selected nodes as PNG |
| `Ctrl+Shift+R` | Export the whole graph as PNG |

## Project Directory Rules

A typical workspace can look like this:

```text
ProjectRoot/
  originblueprint.project
  nodes/
    MyGameplayNodes.json
    combat/
      DamageNodes.json
  blueprints/
    battle.vgf
    skill.obp
  functions/
    calculate_damage.obpf
```

Rules:

- Open the workspace root folder, not an individual blueprint file, when you want file browser, node library, and project settings support.
- Put custom node JSON files anywhere under `nodes/`. The loader scans recursively.
- Keep blueprint files in any folder you prefer. The file browser shows `.vgf`, `.obp`, and `.obpf` files.
- `originblueprint.project` belongs to the workspace root. It stores editor preferences for that project.
- Avoid editing generated build output. Treat `frontend/dist/` and packaged app output as disposable.

## Custom Node JSON

Use the current explicit node-definition format for new nodes. It uses stable node IDs and named port keys, which are easier to maintain than legacy numeric port IDs.

Legacy node JSON can still be imported for compatibility, but new documentation and new projects should use the format below.

### Node Example

```json
{
  "id": "origin.example.clamp-integer",
  "title": "Clamp Integer",
  "category": "Math",
  "subtitle": "Clamp a value between min and max.",
  "inputs": [
    { "key": "value", "label": "Value", "type": "data", "data_type": "Integer", "defaultValue": 0 },
    { "key": "min", "label": "Min", "type": "data", "data_type": "Integer", "defaultValue": 0 },
    { "key": "max", "label": "Max", "type": "data", "data_type": "Integer", "defaultValue": 100 }
  ],
  "outputs": [
    { "key": "result", "label": "Result", "type": "data", "data_type": "Integer" }
  ]
}
```

Field notes:

- `id`: Stable node type ID. Do not rename it after users save graphs with this node.
- `title`: Display name in the module library and node header.
- `category`: Module library category.
- `subtitle`: Optional description shown as secondary text.
- `inputs` / `outputs`: Port definitions.
- `key`: Stable port key used by graph documents and runtime validation.
- `label`: Port display text.
- `type`: Use `exec` for execution-flow ports and `data` for value ports.
- `data_type`: Supported common values include `Integer`, `Float`, `Boolean`, `String`, `Array`, and `Any`.
- `defaultValue`: Optional inline default value for input data ports.
- `arrayItemType`: Optional input item control type for array ports, usually `number` or `string`.

Guidelines:

- Keep `id` stable after users start saving graphs with that node.
- Use lowercase, descriptive port keys such as `exec`, `value`, `result`, `true`, and `false`.
- Use `exec` ports only for control-flow order.
- Use data ports for values and set `defaultValue` when a user should be able to type a value directly on the node.
- Adding a JSON node makes it visible and editable. It does not automatically implement runtime behavior in Go.

## Dynamic Branch Nodes

Some flow nodes need a `+ Item` control that adds matching parameter rows and execution outputs. Use `dynamicBranch` for this pattern:

```json
{
  "id": "origin.flow.equal-switch-example",
  "title": "Equal Switch",
  "category": "Flow",
  "inputs": [
    { "key": "exec", "label": "", "type": "exec" },
    { "key": "value", "label": "Value", "type": "data", "data_type": "Integer", "defaultValue": 0 },
    { "key": "cases", "label": "Cases", "type": "data", "data_type": "Array", "defaultValue": [], "arrayItemType": "number" }
  ],
  "outputs": [
    { "key": "otherwise", "label": "Otherwise", "type": "exec" },
    { "key": "case1", "label": "", "type": "exec" },
    { "key": "case2", "label": "", "type": "exec" }
  ],
  "dynamicBranch": {
    "controlInput": "cases",
    "defaultOutput": "otherwise",
    "outputPrefix": "case",
    "outputStartIndex": 1,
    "maxBranches": 2
  }
}
```

## Go Integration

The editor keeps persistent graph data in the `GraphDocument` contract defined in `graph.go`. Do not serialize Rete.js internals as a file format.

Important Go files:

| File | Responsibility |
| --- | --- |
| `graph.go` | `GraphDocument`, validation, stable built-in port definitions. |
| `legacy.go` | Legacy `.vgf` migration and export compatibility. |
| `node_schemas.go` | Loading `nodes/**/*.json` documents. |
| `execution.go` | Go runtime execution semantics. |
| `app.go` | Wails-facing file, workspace, project, image export, and platform services. |

When adding a runtime node:

1. Add or update the JSON node definition under `nodes/`.
2. If the node must validate as a known runtime node, add its stable port definition in `graph.go`.
3. If it should execute in the Go runtime, implement its behavior in `execution.go`.
4. If it must import/export old `.vgf` files, update the mapping rules in `legacy.go` and `frontend/src/editor/runtimeNodeSchemas.ts`.
5. Add focused Go tests for validation, migration, execution, or round-trip behavior.

Minimal execution flow:

```text
GraphDocument JSON
  -> Go validation / migration
  -> executeGraph(...)
  -> ExecutionEvent logs, results, variables, and node states
```

The frontend owns live canvas interaction. Go owns durable file compatibility, validation, migration, and execution rules.

## Web Compatibility

The frontend can be built as a Vite web app, but the product is currently desktop-first.

Works in the browser build:

- Create and edit native graph documents.
- Open local `.obp`, `.obpf`, or JSON files through the browser file picker.
- Save native graph documents by downloading JSON.
- Load the static node library from `nodes/manifest.json`.
- Export graph images through browser download.

Desktop-only today:

- Native workspace directory scanning.
- Recent files and reveal-in-folder.
- Legacy `.vgf` migration and export through Go services.
- Go-backed validation and runtime services.
- Native file dialogs and Wails window controls.

## Compatibility Notes

- Unknown legacy nodes and edges should be preserved instead of silently dropped.
- `.vgf` round trips are compatibility-sensitive. Test representative legacy files when changing migration or export behavior.
- New node IDs, port keys, and data types should be treated as stable once saved into graph documents.
- If a new node must be consumed by an old external parser, provide an explicit legacy export mapping.

## More Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Node JSON format, Chinese](docs/NODE_JSON_FORMAT_ZH.md)
- [Legacy compatibility, Chinese](docs/LEGACY_COMPATIBILITY_ZH.md)
- [Blueprint change safety, Chinese](docs/BLUEPRINT_CHANGE_SAFETY_ZH.md)
- [Engine test matrix, Chinese](docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md)
