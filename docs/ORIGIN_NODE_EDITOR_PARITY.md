# OriginNodeEditor parity checklist

This document tracks the functional migration from `OriginNodeEditor_old`.

## Editor interaction

- [x] Dark grid canvas
- [x] Wheel zoom around cursor
- [x] Middle mouse canvas pan
- [x] Left mouse node drag
- [x] Left mouse rubber-band selection
- [x] Ctrl multi-selection
- [x] Port drag connection
- [x] Right-click node search menu
- [x] Node library double-click and drag/drop creation
- [x] Delete / X removes selected nodes
- [x] Ctrl+A select all, Ctrl+D deselect all
- [x] Ctrl+C / Ctrl+X / Ctrl+V
- [x] Ctrl+Z / Ctrl+Y
- [ ] Cutting line interaction

## Documents

- [ ] New graph and multi-tab documents
- [ ] Open, save, save as, save all
- [ ] Recent graph list
- [ ] Workspace and file browser
- [ ] Legacy `.vgf` importer
- [ ] Export selection and graph as image

## Graph editing

- [ ] Resizable groups/comments
- [ ] Group and ungroup
- [ ] Horizontal and vertical center alignment
- [ ] Left, right, top and bottom alignment
- [ ] Horizontal and vertical distribution
- [ ] Straighten connected ports
- [ ] Undo/redo for node movement, connection and group operations

## Node system

- [x] Blueprint node, socket and connection visuals
- [x] Exec and typed data sockets
- [x] Inline text and number controls
- [ ] Complete node definition schema
- [ ] Searchable node library loaded from definitions
- [ ] Dynamic ports and node widgets
- [ ] Type compatibility and automatic converters

## Variables and runtime

- [ ] Variable groups and detail editor
- [ ] Drag variable to create Getter; Alt+drag creates Setter
- [ ] Runtime graph independent from UI
- [ ] Background execution and node state display
- [ ] Session, loop and branch semantics
- [ ] Batched logs and progress events
- [ ] Python worker for data-science nodes
