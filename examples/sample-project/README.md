# OriginBlueprint Sample Project

This sample project is intentionally small. Open this folder with **File > Open Workspace Folder** to try the workspace browser, native `.obp` graph loading, and custom node JSON discovery.

## Contents

- `blueprints/getting-started.obp`: a minimal native graph document.
- `functions/calculate-damage.obpf`: a minimal function blueprint document.
- `nodes/SampleNodes.json`: a new-style custom node definition example. Opening this folder loads it automatically from `<workspace>/nodes`, and F5 validation uses the same schema source.
- `originblueprint.project`: workspace UI settings used by the editor.

The custom node is for editor/module-library demonstration. Runtime behavior still needs to be implemented and explicitly registered in Go before a custom node can execute. After editing node JSON, use **File > Refresh Node Library** to reload it.
