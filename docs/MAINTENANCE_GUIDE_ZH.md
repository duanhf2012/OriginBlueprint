# OriginBlueprint 工程学习与维护指南

本文面向熟悉 Go、但不熟悉 Vue、TypeScript、Wails 和 Rete.js 的维护者。目标不是系统教授前端，而是帮助你快速判断代码位置、理解调用链，并安全地修改工程。

## 1. 工程是什么

OriginBlueprint 是一个跨平台桌面蓝图编辑器：

- Go：文件、工作区、旧格式迁移、校验和蓝图执行。
- Wails v2：把 Go 方法暴露给前端，并将 Vue 页面包装成桌面程序。
- Vue 3：窗口中的菜单、面板、对话框和状态管理。
- TypeScript：带类型检查的 JavaScript，负责编辑器交互。
- Rete.js v2：节点、端口、连线以及画布的基础数据结构和插件体系。
- Vite：前端开发服务器和打包工具。

可以把它理解为：Go 程序启动一个 WebView，WebView 中运行 Vue/Rete.js，双方通过 Wails 桥接调用。

```text
Windows / macOS / Linux
        |
        v
Wails 桌面窗口
   |             |
   v             v
Go 后端       Vue + Rete.js 前端
文件/校验      界面/画布/即时交互
迁移/执行
```

## 2. 快速开始

### 2.1 环境

需要安装：

- Go
- Node.js 和 npm
- Wails v2 CLI
- Windows 下的 WebView2 Runtime（通常系统已有）

安装 Wails：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

检查环境：

```powershell
wails doctor
```

### 2.2 日常开发

在 `OriginBlueprint` 目录运行：

```powershell
wails dev
```

它会启动 Go 后端、Vite 开发服务器和桌面窗口。修改 Vue/CSS 后通常会热更新；修改 Go 后 Wails 会重新编译。

只检查前端：

```powershell
cd frontend
npm run build
```

运行 Go 测试：

```powershell
go test ./...
```

生成正式 Windows 程序：

```powershell
wails build
```

输出位于 `build/bin/OriginBlueprint.exe`。也可以双击根目录的 `run.bat`，它会运行现有程序，缺少程序时自动构建。

## 3. 目录地图

```text
OriginBlueprint/
  main.go                       Wails 程序入口和窗口配置
  app.go                        桌面能力：打开、保存、工作区、最近文件等
  graph.go                      GraphDocument 数据结构、端口定义和校验
  execution.go                  蓝图运行时
  legacy.go                     旧 OriginNodeEditor .vgf 格式迁移
  app_test.go                   Go 回归测试
  wails.json                    Wails 构建配置
  run.bat                       Windows 一键启动/构建
  docs/
    ARCHITECTURE.md             代码归属原则
    ORIGIN_NODE_EDITOR_PARITY.md 旧编辑器功能复刻清单
  frontend/src/
    main.ts                     Vue 前端入口
    App.vue                     主窗口、菜单、面板和业务编排
    platform.ts                 Wails 桌面与浏览器环境的适配层
    style.css                   全局布局和外观
    editor/
      document.ts               GraphDocument 的 TypeScript 对应类型
      types.ts                  Rete 节点、连线和自定义控件类型
      nodeRegistry.ts           前端节点注册表和节点外观定义
      createEditor.ts           Rete 编辑器、交互、历史记录和序列化
      BlueprintNode.vue         节点外观
      BlueprintSocket.vue       端口外观
      BlueprintConnection.vue   连线外观
      BlueprintControl.vue      输入框、数组和文件选择控件
  frontend/wailsjs/             Wails 自动生成，通常不要手工修改
  build/                        图标、安装配置和构建结果
```

## 4. 从 Go 的角度理解前端

### 4.1 TypeScript

TypeScript 可以近似理解为“带静态类型提示的 JavaScript”。它最终仍会编译成浏览器执行的 JavaScript。

常见语法对应关系：

| TypeScript / Vue | 类似的 Go 概念 |
| --- | --- |
| `interface GraphDocument` | `type GraphDocument struct` |
| `type A = 'x' \| 'y'` | 字符串枚举约束 |
| `async function` / `Promise` | 异步调用，概念上类似等待 goroutine 结果 |
| `ref(value)` | 可被 Vue 观察并触发界面更新的变量 |
| `computed(() => ...)` | 依赖变化时自动重新计算的只读值 |
| `onMounted(...)` | 窗口组件初始化回调 |
| `v-if` | 条件渲染 |
| `v-for` | 模板中的循环 |
| `@click="fn"` | 注册点击事件 |
| `:value="x"` | 将表达式绑定到 HTML 属性 |

不要把 `ref` 理解成 Go 指针。它是 Vue 的响应式容器，TypeScript 中通过 `.value` 读写，模板中会自动解包。

### 4.2 Vue 单文件组件

`.vue` 文件通常包含三部分：

```vue
<script setup lang="ts">
// 状态和函数
</script>

<template>
  <!-- HTML 结构 -->
</template>

<style scoped>
/* 只影响本组件的 CSS */
</style>
```

本工程的 `App.vue` 较大，可以先这样理解：

- `<script>`：类似窗口 Controller，保存状态并调用 Go 或编辑器。
- `<template>`：描述窗口显示什么。
- CSS：描述颜色、尺寸和布局。

### 4.3 Rete.js

Rete.js 不是蓝图业务运行时，它只负责编辑器中的节点图。主要对象：

- `NodeEditor`：保存节点和连接。
- `AreaPlugin`：画布位置、缩放、拖动和视口。
- `ConnectionPlugin`：创建连线的交互。
- `VuePlugin`：用 Vue 组件绘制节点、端口和连线。
- `ClassicPreset.Node/Input/Output/Socket`：节点图的基础对象。

真正的蓝图执行在 Go 的 `execution.go` 中，Rete 对象不会直接执行节点逻辑。

### 4.4 Wails

`main.go` 中通过 `Bind` 注册 `App`：

```go
Bind: []interface{}{app},
```

因此 `App` 的公开方法会生成前端绑定，例如：

```go
func (a *App) SaveGraph(path, content string) (string, error)
```

前端可通过生成的 Wails 方法调用它。工程没有让各个 Vue 文件直接依赖 Wails，而是统一经过 `frontend/src/platform.ts`，这样前端也能作为普通网页启动。

修改公开 Go 方法签名后，应运行：

```powershell
wails generate module
```

`wails dev` 和 `wails build` 也会重新生成绑定。`frontend/wailsjs` 是生成代码，不要在其中实现业务。

## 5. 核心数据模型

### 5.1 GraphDocument

`GraphDocument` 是整个工程最重要的协议，定义在 `graph.go`，前端对应类型位于 `frontend/src/editor/document.ts`。

它包含：

- 文档格式版本。
- 图名称。
- 节点及节点位置、输入默认值和扩展属性。
- 连线。
- 分组/注释框。
- 变量和变量组。
- 画布缩放及平移状态。

保存时不应序列化 Rete 内部对象，而应转换为 `GraphDocument`。这样文件格式不受 Rete.js 内部版本变化影响。

重要约定：

- `node.id`、`connection.id` 必须稳定且唯一。
- `typeId` 是节点类型的永久标识，不要随意改名。
- 端口 `key` 是连接和执行时使用的协议字段，也不要仅为了显示效果改名。
- 节点标题可以变化，业务判断必须使用 `typeId`。
- 新字段应考虑旧文件缺失该字段时的默认值或迁移方式。

### 5.2 两份类型为什么都存在

Go 结构用于保存、校验和执行；TypeScript 接口用于前端编译期检查。JSON 是它们之间的传输格式。

修改文档结构时必须同步检查：

1. `graph.go` 中的 Go struct 和 JSON tag。
2. `frontend/src/editor/document.ts` 中的接口。
3. `createEditor.ts` 中的 `getDocument` 和 `loadDocument`。
4. `App.vue` 中的 `normalizeDocument`。
5. `legacy.go` 是否需要迁移。
6. `app_test.go` 是否需要增加往返测试。

## 6. 主要调用链

### 6.1 程序启动

```text
main.go
  -> NewApp()
  -> wails.Run(... Bind: app)
  -> 加载 frontend/dist
  -> frontend/src/main.ts
  -> 挂载 App.vue
  -> App.vue onMounted
  -> createBlueprintEditor(...)
```

### 6.2 保存蓝图

```text
用户点击 Save
  -> App.vue saveGraph()
  -> editor.getDocument()
  -> Rete 当前状态转换为 GraphDocument
  -> JSON.stringify(document)
  -> platform.saveGraph()
  -> Wails 调用 App.SaveGraph()
  -> Go 写入磁盘并记录最近文件
```

### 6.3 打开蓝图

```text
用户点击 Open
  -> App.vue openGraph()
  -> platform.openGraph()
  -> App.OpenGraph() 读取文件
  -> 必要时 MigrateLegacyGraph()
  -> normalizeDocument()
  -> editor.loadDocument()
  -> 创建 Rete 节点、连线和分组
```

### 6.4 运行蓝图

```text
用户点击 Test/F5
  -> App.vue testGraph()
  -> editor.getDocument()
  -> platform.validateGraph(JSON)
  -> graph.go validateGraph()
  -> graph.go validateExecutionFlow()
  -> App.vue 在底部 Test Results 面板展示问题，点击问题定位节点
```

### 6.5 拖动节点为什么不调用 Go

拖动、缩放和画连线会产生大量鼠标事件。如果每次都跨 Wails 调用 Go，会增加序列化和进程桥接开销，造成卡顿。

因此这些操作完全在 `createEditor.ts` 中完成。操作结束后，保存、校验或运行时再生成完整 `GraphDocument` 交给 Go。这条性能边界应长期保持。

## 7. Go 文件详解

### 7.1 `main.go`

只负责应用启动：窗口标题、尺寸、背景色、资源嵌入和绑定 `App`。它应保持精简，不要放蓝图业务。

### 7.2 `app.go`

这是 Wails 服务层，负责操作系统相关能力：

- 打开和保存图文件。
- 原生文件/目录选择器。
- 工作区文件列表。
- 最近文件配置。
- 导出 PNG。
- 新窗口和退出程序。

它类似 HTTP 项目中的 handler/service 边界。复杂蓝图规则不要继续堆在这里，应放入独立 Go 模块，再由 `App` 调用。

### 7.3 `graph.go`

包含持久化模型、节点端口定义以及 `ValidateGraph`。

`nodePorts` 是 Go 运行时认识节点端口的权威表之一。连接校验依赖它检查节点是否存在、端口是否存在、类型是否兼容。

新增节点时，如果它要保存、校验或运行，必须检查这里。

### 7.4 `execution.go`

这是蓝图解释执行器。当前前端不暴露本地运行入口，右上角 `Test` 调用 `ValidateGraph` 做结构与流程可达性检查；`execution.go` 保留底层执行语义和测试覆盖，便于后续恢复运行能力或验证兼容执行逻辑。

- `runNode`：执行带 Exec 流程的节点。
- `follow`：沿指定 Exec 输出继续运行。
- `input`：取得节点输入，优先读取连线来源，否则读取默认值。
- `output`：按需计算纯数据节点输出。
- `flush`：批量发送执行状态，避免高频跨桥通信。

理解执行器时，先区分两种节点：

1. 流程节点：有 Exec 端口，在 `runNode` 中处理。
2. 纯数据节点：没有执行流，被其他节点读取时在 `output` 中计算。

执行器有步骤上限和 context 检查，用于阻止错误图导致无限执行。新增循环节点时必须保留这两个保护。

### 7.5 `legacy.go`

将旧 `.vgf` JSON 转换为新 `GraphDocument`。

`legacyNodeSpecs` 维护旧节点名称到新 `typeId` 和端口顺序的映射。不能识别的节点会尽量保留为兼容占位节点，避免打开旧文件时直接丢数据。

旧格式迁移原则：

- 尽量只做单向导入，不让新代码依赖旧结构。
- 端口映射按旧文件的索引转换成稳定 key。
- 已经发布的迁移行为应由测试固定下来。

## 8. 前端文件详解

### 8.1 `App.vue`

主界面和功能编排中心，负责：

- 多标签文档。
- 菜单和快捷键。
- 模块库搜索。
- 文件打开、保存、迁移和规范化。
- 变量面板。
- 校验日志和执行结果。
- 调用 `BlueprintEditorHandle` 操作画布。

这里适合放“用户点击按钮后依次做什么”，不适合放复杂节点算法，也不适合放每次鼠标移动的底层逻辑。

### 8.2 `platform.ts`

所有桌面能力的统一入口。桌面模式调用 Wails 生成方法，网页模式使用浏览器 API 或降级行为。

新增 Go 桌面能力时，通常按以下顺序：

1. 在 `App` 增加公开方法。
2. 重新生成 Wails 绑定。
3. 在 `DesktopApp` 类型中声明方法。
4. 在 `platform` 对象中包装它。
5. Vue 只调用 `platform.xxx()`。

不要在新 Vue 组件中直接访问 `window.go.main.App`。

### 8.3 `nodeRegistry.ts`

定义模块库中有哪些节点，以及每个节点如何创建 Rete 对象。每项通常包含：

- `id`：稳定 `typeId`。
- `title`：界面标题。
- `category`：模块库分类。
- `kind`：可选。决定节点视觉类型；未填写时由端口和 `id` 自动推断。
- `inputs` / `outputs`：端口 key、标签和数据类型。
- `defaultValue`：未连接时的默认值。

`nodeDefinitions` 是用户可新建的节点；`allNodeDefinitions` 还包含仅用于打开旧文件的隐藏节点。

### 8.4 `createEditor.ts`

最复杂的前端文件，也是画布行为的中心。它负责：

- 创建和连接 Rete 插件。
- 节点拖动、画布平移和缩放。
- 框选、多选、剪切线和删除。
- 复制、粘贴、撤销和重做。
- 分组、对齐和分布。
- 文档序列化与加载。
- 选中节点回调和运行状态高亮。

它向 `App.vue` 返回 `BlueprintEditorHandle`。可以把这个接口看成 Go package 对外导出的接口；`App.vue` 应尽量只通过它操作编辑器内部。

修改本文件时要重点手测鼠标操作和 500 个节点场景。不要在 `pointermove`、缩放或动画回调中调用 Go。

### 8.5 四个 `Blueprint*.vue`

- `BlueprintNode.vue`：节点框、标题、输入输出布局。
- `BlueprintSocket.vue`：不同数据类型端口的颜色和形状。
- `BlueprintConnection.vue`：连线路径、选中状态和执行状态。
- `BlueprintControl.vue`：文本、数字、布尔、数组、文件路径输入。

调整视觉效果通常从这些文件和 `style.css` 入手，不需要修改 Go。

## 9. 如何新增一个节点

以下以“两个整数取最大值”的纯数据节点为例，建议按顺序修改。

### 9.1 确定稳定协议

先确定：

```text
typeId: origin.math.max-integer
inputs: a(integer), b(integer)
outputs: value(integer)
```

这些 key 一旦写入用户文件就应视为公开协议。

### 9.2 注册前端节点

在 `nodeRegistry.ts` 增加定义，使模块库可以创建节点。前端定义控制显示标题、端口和默认输入控件。

### 9.3 注册 Go 端口

在 `graph.go` 的端口定义中增加同一个 `typeId` 及完全一致的端口 key/type。否则 `ValidateGraph` 会报告未知节点或端口。

### 9.4 实现执行逻辑

纯数据节点在 `execution.go` 的 `output` 中处理：读取 `a`、`b`，根据请求的输出 key 返回结果。

如果节点带 Exec 流程，则主要在 `runNode` 中处理，完成后通过 `follow` 进入对应输出。

### 9.5 兼容旧节点（如需要）

如果旧编辑器有对应节点，在 `legacy.go` 增加映射，明确旧输入/输出索引对应的新 key。

结点演进统一遵循“老结点冻结，新结点承载改进”的原则：

- 已经能从旧 `.vgf/.obp` 打开的老结点，不直接改变 `typeId`、端口 key、旧端口编号映射和导出格式。
- 如果需要调整交互、布局、动态端口机制或数据结构，新增一个现代版本结点，并在模块库中放在同名老结点后面，标题追加 `[新]`。
- 兼容方向是单向的：老编辑器、老工程、老 `.vgf/.obp` 文件必须能在新编辑器中兼容打开；新结点不要求能回到老编辑器中打开或编辑。
- 新结点可以使用更清晰的内部 `typeId`、前端机制和后续可视化结点设计器生成的定义格式；如果它需要导出给现有蓝图运行解析器，则导出 `.vgf/.obp` 时仍必须映射回旧解析器能识别的 class、输入端口编号、输出端口编号和默认值格式。
- 老结点继续负责打开历史文件；新结点用于后续新蓝图逐步替换。等外部解析器完成升级、历史蓝图迁移完成后，再考虑隐藏或删除老结点。
- 在可视化结点设计器完成前，新增结点先手写 JSON；手写 JSON 应尽量沿用现有 `nodes` 目录规则，避免提前引入第二套难以维护的定义来源。
- 每新增一个兼容型新结点，都要添加导入/导出测试，至少验证：新结点导出的旧 class 正确、动态端口编号正确、默认值字段位置正确、重新打开后连线不丢失。

### 9.6 添加测试

至少增加：

- 校验合法节点和连接。
- 执行结果。
- 涉及旧格式兼容时，补充 `.vgf/.obp` 导入、导出或 round-trip 测试。
- 如果涉及旧格式，再加迁移测试。

最后运行：

```powershell
gofmt -w *.go
go test ./...
npm run build --prefix frontend
wails build
```

## 10. 常见修改应该去哪里

| 需求 | 主要文件 |
| --- | --- |
| 改窗口大小、标题 | `main.go` |
| 改打开/保存方式 | `app.go`、`platform.ts`、`App.vue` |
| 改 JSON 文件格式 | `graph.go`、`document.ts`、`createEditor.ts`、迁移和测试 |
| 新增节点 | `nodeRegistry.ts`、`graph.go`、`execution.go`、测试 |
| 改节点颜色和布局 | `BlueprintNode.vue`、`BlueprintSocket.vue`、`style.css` |
| 改连线外观 | `BlueprintConnection.vue` |
| 改拖动/缩放/选择 | `createEditor.ts` |
| 改菜单或面板 | `App.vue` |
| 改蓝图执行规则 | `execution.go` |
| 改旧 `.vgf` 导入 | `legacy.go` |
| 增加系统文件对话框 | `app.go` 和 `platform.ts` |

## 11. 调试方法

### 11.1 Go 后端

优先把不依赖 Wails UI 的逻辑写成普通 Go 函数，这样可以直接用 `go test`。执行器目前就是这种结构，`executeGraph` 可在测试中直接调用。

常用命令：

```powershell
go test ./...
go test ./... -run TestExecuteGraph -v
```

### 11.2 前端

前端类型错误和构建错误：

```powershell
npm run build --prefix frontend
```

运行 `wails dev` 后可使用 WebView 开发者工具检查 DOM、CSS 和控制台。出现界面不更新时，先检查：

- 修改的是 `ref.value`，还是错误替换了非响应式对象。
- 异步函数是否遗漏 `await`。
- Vue 模板引用的字段是否存在。
- Rete 节点更新后是否请求了重新渲染。

### 11.3 Go 与前端桥接

若前端提示方法不存在：

1. 确认方法是 `App` 的公开方法（首字母大写）。
2. 确认 `main.go` 绑定了同一个 `App`。
3. 重新运行 `wails dev` 或 `wails build` 生成绑定。
4. 检查 `platform.ts` 的 `DesktopApp` 声明和包装方法。

## 12. 测试与发布检查

一次功能修改完成后至少执行：

```powershell
gofmt -w app.go graph.go legacy.go execution.go app_test.go
go test ./...
npm run build --prefix frontend
wails build
git diff --check
```

涉及画布交互时还应人工检查：

- 节点拖动、框选和 Ctrl 多选。
- 中键平移和滚轮缩放。
- 创建、选择、切断和删除连线。
- 复制、粘贴、撤销、重做。
- 保存后重新打开，节点位置和默认值是否一致。
- 大图拖动/缩放是否仍流畅。

涉及执行器时检查：

- 正常结果。
- 输入未连接时的默认值。
- 类型错误和缺失端口能否被校验发现。
- 循环能否停止，取消是否生效。
- 大量日志是否仍批量发送。

## 13. 维护规则

1. Go 是持久业务规则、文件兼容、校验和执行的权威实现。
2. Vue/TypeScript 负责显示和高频交互，不要把鼠标移动逐帧发给 Go。
3. `GraphDocument`、`typeId` 和端口 key 是协议，修改前先考虑已有文件。
4. 前端调用系统能力必须经过 `platform.ts`。
5. 不要手工维护 `frontend/wailsjs` 中的业务逻辑。
6. 新业务规则优先添加 Go 测试。
7. 不要为了整理目录一次性大范围重构；在功能修改时逐步拆分。
8. 遇到不熟悉的前端代码，先从 `BlueprintEditorHandle` 和调用链阅读，不必先掌握整个 Rete.js。

## 14. 建议的学习顺序

只熟悉 Go 时，建议按以下顺序阅读：

1. `graph.go`：先理解保存的数据是什么。
2. `app_test.go`：通过测试了解当前行为。
3. `execution.go`：理解蓝图如何真正运行。
4. `app.go`：理解桌面文件能力。
5. `legacy.go`：理解旧文件如何进入新模型。
6. `frontend/src/editor/document.ts`：对照 Go 数据结构。
7. `frontend/src/platform.ts`：理解 Go 如何被前端调用。
8. `frontend/src/App.vue` 的 `<script>`：理解用户操作调用链。
9. `nodeRegistry.ts`：理解节点如何显示。
10. `createEditor.ts`：最后再深入画布和 Rete.js 细节。

按照这个顺序，可以先建立熟悉的 Go 业务模型，再逐渐进入前端，不需要一开始就通读大型 Vue 文件。
