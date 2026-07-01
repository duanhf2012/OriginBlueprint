# 历史功能与兼容性清单

本文档记录 `OriginBlueprint` 已覆盖的历史功能和 legacy `.vgf` 兼容能力。它不要求本地存在旧项目目录；后续维护应以本仓库内的代码、文档、测试和样本为准。

## Editor interaction

- [x] Dark grid canvas
- [x] Wheel zoom around cursor
- [x] Middle mouse canvas pan
- [x] Left mouse node drag
- [x] Left mouse rubber-band selection
- [x] Ctrl multi-selection
- [x] Port drag connection
- [x] Connection selection, Delete/X removal and right-click removal
- [x] Right-click node search menu
- [x] Node library double-click and drag/drop creation
- [x] Delete / X removes selected nodes
- [x] Ctrl+A select all, Ctrl+D deselect all
- [x] Ctrl+C / Ctrl+X / Ctrl+V
- [x] Ctrl+Z / Ctrl+Y
- [x] Ctrl + right-drag cutting line interaction

## Documents

- [x] New graph and multi-tab documents
- [x] Open, save, save as, save all
- [x] Recent graph list
- [x] Workspace and file browser
- [x] Legacy `.vgf` importer (known nodes migrate natively; unsupported nodes are preserved as compatibility placeholders)
- [x] Export selection and graph as image

## Graph editing

- [x] Resizable groups/comments
- [x] Group and ungroup
- [x] Horizontal and vertical center alignment
- [x] Left, right, top and bottom alignment
- [x] Horizontal and vertical distribution
- [x] Straighten selected nodes
- [x] Undo/redo for node movement, connection and group operations

## Node system

- [x] Blueprint node, socket and connection visuals
- [x] Exec and typed data sockets
- [x] Inline text and number controls
- [x] Complete JSON node definition schema from the legacy editor
- [x] Searchable node library loaded from definitions
- [x] Sequence dynamic outputs and editable array controls
- [x] File open/save picker controls inside File nodes
- [x] Type compatibility and automatic converters (Integer/Float to String; unsupported conversions are rejected)

## Variables and runtime

- [x] Versioned graph document and Go validation model
- [x] Boolean, integer, float, string, file, table and dictionary variables
- [x] Array variables and array default values
- [x] Variable Getter and Setter nodes
- [x] Variable rename, type, default value and deletion checks
- [x] Node title and input-default detail editor
- [x] Validation logger with node focusing
- [x] Variable groups and detail editor
- [x] Drag variable to create Getter; Alt+drag creates Setter
- [x] Runtime graph independent from UI
- [x] Background execution and node state display
- [x] Session, loop and branch semantics
- [x] Batched logs and progress events
- [x] File read/write nodes in the Go runtime
- [x] CSV table read/write, row count, headers and inner merge
- [x] Table column selection, sorting, equality filtering, rename, drop and empty-cell filling
- [x] Generic array and table-row foreach execution
- [x] Dictionary set, size and keys operations
- [x] Runtime table viewer with search, pagination, row numbers, copy and CSV export
- [x] Native migration for the legacy file/table/dictionary toolchain

## Explicitly excluded from the current scope

- Runtime debugger: breakpoints, pause, resume and single-step execution
- Persistent Timer scheduling
- Table VIF and Table Column VIF
- OLS, Poisson, Negative Binomial and Zero Inflated regression nodes

## 已完成的历史兼容决策

- [x] Legacy literal/utility aliases: `StringNode` and `Length (Array)`
- [x] Legacy float arithmetic: `AddNode`, `MinusNode`, `MultiplyNode` and `DivideNode`
- [x] Legacy pure integer comparison: `GreaterIntegerNode`
- [x] `WhileNode`
- [x] `ForLoopWithBreak`
- [x] Functional search field in the right-side module library
- [x] Legacy File menu extras: New Window, Clear Recent Files and Quit
- [x] Help/about content
- [x] 基于历史行为记录完成视觉和指针交互对齐

## Optional product enhancements (not old-editor parity)

- [ ] Autosave and crash recovery
- [ ] Full browser runtime: graph execution and workspace access in the Web build
- [ ] User-defined/custom node packages or a node-definition extension mechanism
- [ ] Packaging and smoke tests for macOS and Linux
