# OriginBlueprint Agent 指南

这是 AI 编码 agent 进入本项目后应优先阅读的文件。

## 项目关系

- `OriginBlueprint/` 是当前活跃项目。产品和代码修改都应在这里完成。
- 不要假设 clone 后存在任何 sibling 旧项目目录；本仓库必须独立可维护。
- 历史线上文件是 legacy `.vgf` JSON 蓝图。新编辑器必须继续支持打开、显示、编辑、尽可能校验，并在导入导出时避免静默丢失内容。

## 技术栈

- 桌面壳：Go + Wails v2。
- 前端：Vue 3 + TypeScript + Vite。
- 图编辑器：Rete.js v2。
- 节点定义：`nodes/` 下的 JSON 文件。
- 运行时执行：Go，基于序列化后的 `GraphDocument` 快照。

## 修改前必读

做非 trivial 修改前，先阅读唯一权威说明：

- `README.md`：编辑器、节点 JSON、Go API、VM 异步、兼容性、使用禁区和验证要求，也是项目唯一权威使用说明。
- `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`：当前验证蓝图与独立 Go 实现的自动对比结果。

如果修改 `engine/go/blueprint/`，还必须阅读 `engine/go/blueprint/AGENTS.md`。

## 核心架构规则

- `GraphDocument` 是 Go 和 TypeScript 之间持久化交换的契约。
- Go 负责文件持久化、迁移、校验、运行时执行、workspace 访问和 Wails/platform 服务。
- TypeScript 负责 Rete 编辑器构建、画布交互、视觉状态、节点渲染、菜单、选择、鼠标和键盘手势。
- 不要把 Rete 内部结构序列化为保存文件格式。
- 不要在每一帧动画、鼠标移动、缩放步骤或节点拖动更新时调用 Go。只在保存、校验、执行、导入导出或完成编辑等事务边界调用 Go。

## 兼容性规则

- 保留旧 `.vgf` 图内容。未知旧节点或边必须进入 legacy state，而不是丢弃。
- 已知 legacy 节点类在 `legacy.go` 和 `frontend/src/editor/runtimeNodeSchemas.ts` 中映射到当前 `origin.*` type id。
- 如果新增或重命名节点类型，涉及兼容性时必须同步更新迁移、导出逻辑和前端 schema 转换。
- 如果新节点必须能被旧外部解析器使用，需要增加明确的 legacy class 导出映射。
- `.vgf` round-trip 行为风险很高。相关修改必须增加导入、可显示文档形态、校验和 legacy 导出的测试。

## 重要文件

- `graph.go`：`GraphDocument`、校验、稳定的节点端口类型表。
- `legacy.go`：legacy `.vgf` 迁移和导出。
- `node_schemas.go`：运行时加载 `nodes/**/*.json`。
- `execution.go`：桌面工具的文档执行/验证服务；服务器 Go VM 位于 `engine/go/blueprint/`。
- `app.go`：Wails 暴露的文件、workspace 和 platform 服务。
- `frontend/src/platform.ts`：桌面和浏览器能力适配层。
- `frontend/src/App.vue`：应用壳、标签页、打开保存流程、节点库加载。
- `frontend/src/editor/createEditor.ts`：Rete 编辑器、快照恢复、连线、分组和手势。
- `frontend/src/editor/nodeRegistry.ts`：节点 schema 注册和节点工厂。
- `frontend/src/editor/runtimeNodeSchemas.ts`：legacy JSON 节点定义转换。
- `frontend/src/editor/BlueprintNode.vue`：节点视觉布局。
- `frontend/src/editor/BlueprintControl.vue`：内联控件。

## 常用命令

在 `OriginBlueprint/` 下运行：

```powershell
go test ./...
```

在 `OriginBlueprint/frontend/` 下运行：

```powershell
npm run build
npm run test:layout
```

桌面开发：

```powershell
wails dev
```

构建可执行文件：

```powershell
wails build
```

## 测试要求

- 业务规则、迁移、校验、执行和文件格式行为需要 Go 测试。
- 前端编辑器行为在可行时应补 focused tests，鼠标和布局变化还需要人工或视觉检查。
- 兼容性修改需要用代表性的 legacy `.vgf` 文件做 round-trip 测试。
- Go engine 并发修改需要运行 `go test -race ./engine/go/blueprint -count=1`；如果修改 facade 级并发，还要运行 `go test -race ./... -count=1`。
- 汇报完成前，先运行被修改区域的窄测试，并至少运行相关 build/test 命令。

## 已知注意点

- 某些旧 JSON 或旧文档里的中文在不同终端编码下可能显示为乱码。除非用户要求，不要只为了显示正常而重写大型 JSON。
- 不要引用或依赖仓库外的旧项目目录；clone 本仓库后应能独立阅读、构建和维护。
- `.vgf` 和 `.obp` 的保存、导出语义对兼容性敏感。修改前先看 `app.go`、`legacy.go` 和根目录 `README.md` 中的兼容性与禁止用法。
