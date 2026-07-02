# OriginBlueprint AI 项目上下文

这份文档面向刚拉取工程的 AI/coding agent。目标是在少读代码的前提下，快速理解项目边界、数据流、兼容要求和安全改动方式。

## 一句话定位

`OriginBlueprint` 是当前唯一维护的蓝图编辑器项目。后续功能只在本仓库中演进，但必须兼容历史线上已经使用并导出的 `.vgf` 图文件。

## 目录关系

当前仓库应当能独立 clone、构建和维护，不依赖任何 sibling 旧项目目录：

```text
OriginBlueprint/
  OriginBlueprint/      当前唯一应该修改和维护的项目
```

如果需要理解历史兼容行为，应以本仓库中的文档、测试、`legacy.go`、`nodes/` 定义和脱敏 `.vgf` 样本为准；不要引用仓库外的旧项目目录。

## 技术栈

- Go：Wails 应用后端、文件读写、迁移、校验、运行时执行、平台能力。
- Wails v2：桌面壳和 Go/前端桥接。
- Vue 3 + TypeScript：应用界面和编辑器交互。
- Rete.js v2：蓝图节点编辑器。
- Vite：前端开发与构建。
- JSON：节点定义和旧 `.vgf` 图文件都基于 JSON。

## 架构边界

项目的核心原则是：Go 管持久业务规则，TypeScript 管高频视觉交互。

Go 负责：

- `GraphDocument` 数据结构、版本、校验。
- 打开、保存、导入、导出。
- 旧 `.vgf` 迁移和导出。
- 节点定义 JSON 的读取。
- 变量、数据类型和连接兼容规则。
- 蓝图运行时执行和日志/进度事件。
- 工作区、最近文件、文件选择器、Wails 平台能力。

TypeScript 负责：

- Rete 编辑器创建和渲染。
- 节点、端口、连线、分组、选择框等视觉。
- 拖拽、缩放、框选、剪线、复制粘贴、撤销重做。
- 菜单、面板、多标签、弹窗和临时 UI 状态。
- 将当前 Rete 视图序列化为 `GraphDocument` 快照。

不要让前端直接定义另一套持久化真相。保存文件时必须经过 `GraphDocument`，不要保存 Rete 内部对象。

## 核心文件地图

Go 侧：

- `graph.go`：`GraphDocument`、节点/变量/连接校验、稳定端口类型表。
- `legacy.go`：旧 `.vgf` 到新 `GraphDocument` 的迁移，以及新文档导出旧 `.vgf`。
- `node_schemas.go`：从运行目录和可执行文件目录读取 `nodes/**/*.json`。
- `execution.go`：图运行时。执行逻辑只读序列化后的 `GraphDocument`。
- `app.go`：Wails 暴露给前端的应用服务，包含打开/保存/最近文件/工作区/导出图片。
- `main.go`：Wails 应用入口。
- `app_test.go`：迁移、导出、校验、运行时和工作区相关测试。
- `node_schemas_test.go`：节点 JSON 加载和格式约束测试。

前端侧：

- `frontend/src/App.vue`：应用壳。负责多标签、打开/保存、变量面板、运行、校验、菜单和工作区。
- `frontend/src/platform.ts`：桌面/浏览器能力抽象。前端不要直接依赖 Wails 全局对象。
- `frontend/src/editor/createEditor.ts`：Rete 编辑器核心，包含快照/恢复、连接、分组、选择、剪线、撤销重做。
- `frontend/src/editor/nodeRegistry.ts`：节点注册、socket 映射、节点实例创建。
- `frontend/src/editor/runtimeNodeSchemas.ts`：旧节点 JSON 格式转换成新节点 schema。
- `frontend/src/editor/document.ts`：前端版 `GraphDocument` 类型。
- `frontend/src/editor/BlueprintNode.vue`：节点 UI 布局。
- `frontend/src/editor/BlueprintControl.vue`：输入控件、数组控件、文件控件。
- `frontend/src/editor/implicitEntryLinks.ts`：入口参数隐式连线/显示逻辑。
- `frontend/tests/`：轻量前端行为和布局测试。

节点定义：

- `nodes/json/**/*.json`：节点库定义。支持 legacy JSON 格式，也支持带 `id/category/inputs/outputs` 的新格式。

文档：

- `docs/ARCHITECTURE.md`：架构边界。
- `docs/NODE_JSON_FORMAT_ZH.md`：节点 JSON 格式。
- `docs/ORIGIN_NODE_EDITOR_PARITY.md`：历史功能和兼容性清单。
- `docs/LEGACY_COMPATIBILITY_ZH.md`：旧 `.vgf` 兼容专题。
- `docs/BLUEPRINT_CHANGE_SAFETY_ZH.md`：蓝图迁移、节点 JSON、显示和执行链路改动安全清单。

## 文件格式和数据流

新编辑器内部统一使用 `GraphDocument`：

```text
GraphDocument
  schemaVersion
  graphName
  nodes
  connections
  groups
  variables
  variableGroups
  view
  legacy
```

打开文件：

```text
用户选择 .obp/.vgf/.json
  -> app.go OpenGraph 读取文本
  -> App.vue JSON.parse
  -> 如果 schemaVersion == 1：normalizeDocument
  -> 否则桌面端调用 MigrateLegacyGraph
  -> legacy.go migrateLegacyGraph
  -> editor.loadDocument
  -> createEditor.restore 创建 Rete 节点和连线
```

保存文件：

```text
editor.getDocument
  -> GraphDocument 快照
  -> App.vue 根据路径决定是否先 ExportLegacyGraph
  -> app.go SaveGraph
  -> graphContentForPath 可能再次导出 legacy
  -> 写入磁盘
```

这里的 `.vgf/.obp` 保存语义非常敏感。当前实现偏向兼容旧外部解析器。改动前先读 `docs/LEGACY_COMPATIBILITY_ZH.md`。

## 节点系统

节点来源有两类：

1. 旧格式节点 JSON：

```json
{
  "name": "AddInt",
  "title": "...",
  "package": "...",
  "inputs": [
    { "name": "A", "type": "data", "data_type": "Integer", "port_id": 0 }
  ],
  "outputs": [
    { "name": "Result", "type": "data", "data_type": "Integer", "port_id": 0 }
  ]
}
```

2. 新格式节点 JSON：

```json
{
  "id": "origin.math.add-integer",
  "title": "...",
  "category": "...",
  "inputs": [
    { "key": "a", "label": "A", "type": "data", "data_type": "Integer" }
  ],
  "outputs": [
    { "key": "result", "label": "Result", "type": "data", "data_type": "Integer" }
  ]
}
```

前端在 `runtimeNodeSchemas.ts` 中把旧格式转换为统一 schema，再由 `nodeRegistry.ts` 注册成可创建节点。Go 在 `legacy.go` 中也有旧 class 到新 `origin.*` 的映射，用于 `.vgf` 文件迁移和导出。

如果改节点类型，请同时考虑：

- 节点 JSON 是否要新增或调整。
- `legacy.go` 是否需要旧 class 映射或导出映射。
- `runtimeNodeSchemas.ts` 是否需要前端旧 JSON 映射。
- `graph.go` 的 `graphNodePorts` 是否需要补充校验端口。
- `execution.go` 是否需要运行时逻辑。
- 是否需要迁移/导出/校验/执行测试。

## 运行时

运行时在 Go 中执行，不在 Vue/Rete 中执行业务逻辑。

大致流程：

```text
App.vue testGraph
  -> editor.getDocument
  -> ValidateGraph
  -> graph.go validateGraph / validateExecutionFlow
  -> 底部 Test Results 面板展示问题，点击问题可定位节点
```

运行时只依赖 `GraphDocument`，这让 UI 和执行逻辑保持分离。当前前端不暴露本地运行入口；`execution.go` 保留底层执行语义和 Go 测试，便于后续恢复运行能力或验证兼容执行逻辑。

## 兼容性红线

- 打开旧 `.vgf` 不能静默丢节点、边、变量或分组。
- 不认识的旧节点应进入 `legacy.hiddenNodes`，不应直接消失。
- 与隐藏节点相关的边应进入 `legacy.hiddenEdges`，导出时尽量恢复。
- 已知旧节点要迁移为可见、可编辑的新节点。
- 新增节点如果需要给旧外部解析器使用，必须能导出成旧 parser 认识的 class/port。
- 端口顺序和端口编号是兼容关键，不要只按显示顺序猜测。

## 常用命令

在 `OriginBlueprint/` 下：

```powershell
go test ./...
wails dev
wails build
```

在 `OriginBlueprint/frontend/` 下：

```powershell
npm run build
npm run test:layout
```

`npm run build` 会先运行 `vue-tsc --noEmit`，再进行 Vite 构建。

## 改动建议

做功能前先判断归属：

- 持久化、迁移、校验、运行时、外部文件格式：优先 Go。
- 画布、节点视觉、鼠标键盘交互、临时状态：优先 TypeScript/Vue。
- 既影响文件又影响显示：先定义或扩展 `GraphDocument`，再分别改 Go 和前端。

推荐工作顺序：

1. 读相关 docs 和核心文件。
2. 找到现有相似测试。
3. 先写或补测试。
4. 小范围修改。
5. 跑相关测试和构建。
6. 在最终说明中明确兼容影响。

## 当前已知注意点

- 部分中文内容在 PowerShell 输出里可能显示为乱码，这通常是编码显示问题，不代表 JSON 一定损坏。
- 当前工作区顶层可能不是 git 仓库，使用 git 前先确认。
- 不要假设本仓库外存在旧项目目录；如果需要旧格式样本，应使用仓库内已提交的脱敏样本或测试内联样本。
- `.obp` 当前保存/导出行为与 legacy 兼容有耦合。若要让 `.obp` 成为纯新格式扩展，需要专门设计迁移策略。
