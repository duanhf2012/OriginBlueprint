# OriginBlueprint

OriginBlueprint is a desktop-first visual blueprint editor for legacy `.vgf` graphs and native `.obp/.obpf` graph documents. It uses Go + Wails v2 for persistence, migration, validation, runtime services, and platform integration, with Vue 3 + TypeScript + Rete.js v2 for the interactive node editor.

> 中文说明见下方。

## Features

- Node graph editing with pan, zoom, box selection, copy/paste, undo/redo, grouping, alignment, and connection cutting.
- Legacy `.vgf` import with compatibility preservation for unknown legacy nodes and edges.
- Native `.obp` graph documents and `.obpf` function blueprint files.
- JSON-driven node library from `nodes/**/*.json`.
- Workspace file browser, recent files, function library discovery, and node reference search in the desktop build.
- Graph validation for missing entries, invalid ports, unreachable execution flow, and potential execution cycles.
- Image export for selected nodes or the whole graph when a visual snapshot is useful.
- Chinese and English UI menu text.

## Web Compatibility

The frontend can be built with Vite and opened as a web app, but the current product is still desktop-first.

Works in the browser build:

- Create and edit native graph documents.
- Open local `.obp/.obpf/.json` files through the browser file picker.
- Save native graph documents by downloading JSON.
- Load the static node library from `nodes/manifest.json`.
- Export graph images through browser download.

Desktop-only today:

- Legacy `.vgf` migration and legacy export.
- Go-backed graph validation and runtime execution services.
- Workspace directory scanning, recent files, reveal-in-folder, and node reference search.
- Native file dialogs and application window controls.

Before publishing a production web build, the desktop-only services should be replaced by web-safe equivalents such as an HTTP/WASM validation service, browser workspace APIs where available, or a server-side project backend.

## Development

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

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [AI project context, Chinese](docs/AI_PROJECT_CONTEXT_ZH.md)
- [Legacy compatibility, Chinese](docs/LEGACY_COMPATIBILITY_ZH.md)
- [Node JSON format, Chinese](docs/NODE_JSON_FORMAT_ZH.md)
- [Maintenance guide, Chinese](docs/MAINTENANCE_GUIDE_ZH.md)

---

# OriginBlueprint 中文说明

OriginBlueprint 是一个以桌面端为主的可视化蓝图编辑器，用于编辑历史 `.vgf` 蓝图和新的 `.obp/.obpf` 图文档。项目使用 Go + Wails v2 负责文件读写、迁移、校验、运行时服务和系统能力，使用 Vue 3 + TypeScript + Rete.js v2 负责节点画布和高频交互。

## 主要功能

- 节点图编辑：平移、缩放、框选、复制粘贴、撤销重做、节点组、对齐、连线剪切。
- 兼容历史 `.vgf`：未知旧节点和旧边会进入兼容保留区，避免静默丢失内容。
- 支持原生 `.obp` 蓝图和 `.obpf` 函数蓝图文件。
- 从 `nodes/**/*.json` 加载节点库。
- 桌面版支持工程文件浏览器、最近文件、函数库发现和节点引用搜索。
- 蓝图检查：入口缺失、端口错误、不可达执行流和潜在执行循环。
- 支持导出选中节点或整张蓝图为图片，方便文档和讨论。
- 菜单支持中文和英文切换。

## 网页版兼容性

当前前端可以作为 Vite 网页应用构建，但产品能力仍以桌面版为准。

浏览器构建可用：

- 创建和编辑原生图文档。
- 通过浏览器文件选择器打开 `.obp/.obpf/.json`。
- 通过下载方式保存原生 JSON 图文档。
- 从 `nodes/manifest.json` 加载静态节点库。
- 通过浏览器下载导出蓝图图片。

当前仅桌面版可用：

- 历史 `.vgf` 迁移和 legacy 导出。
- Go 侧蓝图校验和运行时执行服务。
- 工程目录扫描、最近文件、资源管理器定位和节点引用搜索。
- 原生文件对话框和窗口控制。

如果未来正式发布网页版，需要为这些桌面专属服务补上 Web 方案，例如 HTTP/WASM 校验服务、可用时接入浏览器文件系统 API，或提供服务端工程后端。

## 开发

桌面开发：

```powershell
wails dev
```

只开发前端：

```powershell
cd frontend
npm install
npm run dev
```

构建和验证：

```powershell
go test ./...
cd frontend
npm run test:layout
npm run build
```

构建桌面可执行文件：

```powershell
wails build
```

## 相关文档

- [架构说明](docs/ARCHITECTURE.md)
- [AI 项目速览](docs/AI_PROJECT_CONTEXT_ZH.md)
- [Legacy 兼容说明](docs/LEGACY_COMPATIBILITY_ZH.md)
- [节点 JSON 格式](docs/NODE_JSON_FORMAT_ZH.md)
- [维护指南](docs/MAINTENANCE_GUIDE_ZH.md)
