# 核心图分析器与保存门禁设计

日期：2026-07-17

## 1. 背景

当前蓝图检查由两部分组成：`graph.go` 对 `GraphDocument` 做结构和启发式流程检查，Go engine 再通过生产编译器做最终确认。现状已经能报告部分不可达节点、可能的执行环和编译错误，但存在以下问题：

- `validateExecutionFlow` 使用普通 DFS 回边检测，只能给出“可能有环”，不能完整表达结构化循环语义。
- 编辑器结构规则与 Go 编译规则分别维护，长期可能漂移。
- 生产编译通常只返回第一个错误，无法一次展示全部问题节点集合。
- 不可达检查主要覆盖可执行节点，未连接纯数据节点和孤立数据岛容易遗漏。
- 将保存门禁绑定到 Go 编译结果会妨碍未来支持其他执行语言。

本设计只优化语言无关的核心图分析器及其正式保存门禁。Go 仍是当前运行目标，但 Go 编译结果不决定蓝图能否保存。

## 2. 目标

1. 建立单一、语言无关的核心图分析入口，一次返回全部可确认问题。
2. 准确计算执行可达性、数据活性、数据依赖环和非法执行环。
3. 区分确定问题与启发式风险，避免结构化循环误报。
4. 由核心诊断明确决定 `blocksSave`，不通过错误文案或 Go 编译失败间接判断。
5. 正式保存被阻止时保留原文件，并生成可恢复但不可直接执行的缓存快照。
6. 保持 `.obp/.obpf/.vgf` 持久化格式和 legacy round-trip 语义不变。

## 3. 非目标

- 不修改 Go VM、Go 节点工厂或 Go 编译器的执行语义。
- 不实现 Lua、JavaScript 等新运行时适配器。
- 不为蓝图文件增加执行语言字段。
- 不让某个运行时的“不支持”诊断阻止正式保存。
- 不在鼠标移动、节点拖动、缩放或动画帧中调用 Go。
- 不尝试静态证明任意结构化循环一定终止；一般程序终止性不能可靠静态判定。

## 4. 诊断契约

扩展现有 `ValidationIssue`，新增字段时保持旧调用方兼容：

```go
type ValidationIssue struct {
    Severity   string   `json:"severity"`
    Code       string   `json:"code"`
    Message    string   `json:"message"`
    NodeID     string   `json:"nodeId,omitempty"`
    NodeIDs    []string `json:"nodeIds,omitempty"`
    SourcePath string   `json:"sourcePath,omitempty"`
    BlocksSave bool     `json:"blocksSave,omitempty"`
    BlocksRun  bool     `json:"blocksRun,omitempty"`
    Target     string   `json:"target,omitempty"`
}
```

约束：

- `Code` 是稳定机器码，保存策略只读取结构化字段，不解析 `Message`。
- `NodeIDs` 对循环问题包含整个强连通分量，并按稳定顺序输出。
- `SourcePath` 标识问题所属文件；只有当前文档的核心诊断可以阻止当前文档保存。
- `Target` 为空表示语言无关的核心问题；例如 `target.go` 只影响对应运行目标。
- `BlocksSave` 只能由核心图分析器或既有源文件保护逻辑设置。
- `BlocksRun` 可由核心分析器和运行时适配器共同设置。

## 5. 分层架构

### 5.1 文档结构层

保留并整理 `graph.go` 中与 `GraphDocument` 契约直接相关的检查：

- schema version、数量和深度安全上限；
- 节点、变量、分组和函数端口 ID 完整性与唯一性；
- 连线端点、端口存在性和端口类型；
- 一个数据输入的生产者数量；
- 一个普通 Exec 输出的目标数量；
- 动态端口范围和函数签名基本契约。

该层只依赖文档和节点 schema，不依赖执行语言。

### 5.2 核心图语义层

新增独立的核心分析单元。输入为已经通过安全解析的 `GraphDocument`、稳定端口定义和控制流语义，输出为 `[]ValidationIssue`。分析器不得修改文档。

内部建立三张只读图：

1. Exec 图：Exec 输出到 Exec 输入的边。
2. 数据依赖图：生产者到消费者的边。
3. 反向数据图：消费者到生产者的边，用于数据活性分析。

控制流语义由节点 schema 注册表提供，不由 Go 工厂名称推断。内建节点至少区分入口、普通节点、分支、结构化循环和带 break 的结构化循环。未知或 opaque legacy 节点缺少可靠语义时只产生“不确定”诊断，不据此设置 `BlocksSave`。

### 5.3 运行时目标层

现有 Go engine 校验继续执行并产生 `Target: "target.go"` 的诊断。它可以阻止 Go 运行或发布，但不能设置核心 `BlocksSave`。未来运行时使用相同适配边界接入。

## 6. 核心分析算法

### 6.1 执行可达性

- 找到所有明确入口节点。
- 从入口沿 Exec 图迭代遍历，得到 `reachableExec`。
- 可执行节点不在集合中时报告 `flow.unreachable-node`。
- 图存在可执行节点但没有入口时报告 `flow.missing-entry`。
- 算法使用显式队列或栈，不使用递归。

### 6.2 数据活性

- 以所有可达执行节点实际消费的数据输入为根。
- 沿反向数据图迭代追踪生产者，得到 `liveData`。
- 纯数据节点不在 `liveData` 且不是入口输出时报告 `flow.unused-data-node`。
- 相互连接但不服务任何可达执行节点的数据岛同样只报告警告。

### 6.3 数据循环

- 对数据依赖图运行迭代版 Tarjan SCC，时间复杂度 `O(V+E)`。
- SCC 节点数大于 1，或单节点存在自环，均为确定的数据依赖循环。
- 每个 SCC 返回一条 `flow.data-cycle`，包含完整、稳定排序的 `NodeIDs`。
- 该诊断设置 `BlocksSave=true`、`BlocksRun=true`。

### 6.4 执行循环

- 先根据语言无关的控制流语义规范化 Exec 图。
- 只有分析器可以证明是规范结构化循环控制边时，才从非法环判定图中排除。
- 带 break 的循环仅在 break 来源处于对应循环体内部且不存在绕过循环体的外部回路时，认可该结构边。
- 对规范化后的 Exec 图运行迭代版 Tarjan SCC。
- SCC 节点数大于 1或存在自环时返回 `flow.exec-cycle`。
- 确定非法环设置 `BlocksSave=true`、`BlocksRun=true`。
- 无法确认节点控制流语义时返回 `flow.possible-cycle`，不得设置 `BlocksSave`。

### 6.5 全量诊断

分析器必须收集所有独立 SCC 和所有不可达/未使用节点，不使用首错返回。输出按 `SourcePath`、`Code`、首个节点 ID 稳定排序，避免问题面板顺序抖动。

## 7. 保存阻断矩阵

### 7.1 始终阻止正式保存

以下当前文档核心问题设置 `BlocksSave=true`：

- `document.decode`、不支持的 schema 或超出安全上限；
- 节点 ID 缺失或重复；
- 悬空连线、端口不存在、确定的端口类型不匹配；
- 同一数据输入存在多个生产者；
- 同一普通 Exec 输出存在非法 fanout；
- 确定的数据依赖环；
- 确定的非结构化 Exec 环；
- 函数文档缺少稳定 `functionId`、存在多个 FunctionEntry、签名端口 ID 重复，或入口/返回节点签名与文档签名不一致；
- 既有加载/恢复过程确认发生节点、连线或属性丢失。

未知节点类型或某运行时不支持节点不自动设置 `BlocksSave`，以保留未来语言和 legacy 扩展能力。

### 7.2 严格模式才阻止

以下问题默认允许保存；项目开启现有 `validateBeforeSave` 时可以阻止：

- `flow.unreachable-node`；
- `flow.missing-entry`；
- `flow.cross-entry-data`；
- legacy placeholder 当前不可执行；

### 7.3 仅警告

- 未连接或未使用的纯数据节点；
- 孤立数据岛；
- 未使用变量和空分组。
- 只能启发式判断、无法证明非法的 `flow.possible-cycle`；该诊断即使在严格模式下也不阻止正式保存。

## 8. 保存数据流

普通保存、另存为和自动保存都使用同一门禁函数：

1. 从编辑器生成稳定 `GraphDocument` 快照。
2. 执行文档结构层和核心图语义层。
3. 合并并去重诊断，运行时目标诊断单独标记。
4. 若当前文档存在 `BlocksSave=true`：
   - 不调用 `SaveGraph` 或 `ForceSaveGraph`；
   - 不允许通过另存为绕过；
   - 保持 dirty 状态；
   - 展开问题面板并高亮问题节点；
   - 原文件和 `.bak` 不发生变化；
   - 原子写入恢复快照。
5. 没有 `BlocksSave` 时，再按 `validateBeforeSave` 决定普通 error 是否阻止保存。
6. 保存成功后清除该文档的诊断状态和恢复快照。

Go 编译错误只影响 `BlocksRun`。当前文件核心合法但 `target.go` 不支持时，允许保存，禁止 Go 运行。

## 9. 恢复快照

恢复快照写入应用配置目录下的 `recovery/`，不写入 workspace，不加入最近文件：

```json
{
  "schemaVersion": 1,
  "sourcePath": "...",
  "tabId": "...",
  "createdAt": "RFC3339",
  "document": {},
  "blockingIssues": []
}
```

- 有源路径时使用规范化路径的 SHA-256 作为恢复键；无路径时使用稳定 tab ID。
- 写入继续使用同目录临时文件和原子替换。
- 每个恢复键最多保留最近 5 份；超过 30 天的快照在启动和成功保存后清理。
- 正式保存成功后删除同一恢复键的全部快照。
- 启动发现比正式文件更新的恢复快照时，提示恢复、另存或删除。
- 恢复快照只能进入受保护编辑标签，不能直接运行或发布。

## 10. 界面行为

- 致命问题显示独立的禁止保存标记。
- 单击循环诊断选择首个节点；双击高亮整个 SCC。
- 问题面板按当前蓝图、workspace 依赖和运行时目标分组。
- 普通 error 使用红点，`BlocksSave` 使用红色禁止标记。
- 保存被阻止时状态栏明确显示原因和恢复快照路径。
- 自动保存遇到阻断问题时只更新恢复快照，不覆盖正式文件。

## 11. 性能与安全边界

- 可达性、数据活性和 SCC 均保持 `O(V+E)`。
- 深图算法全部迭代实现，避免递归栈溢出。
- 复用现有节点、端口、动态输出、签名和深度上限。
- 完整检查只在手动检查、保存、自动保存、运行或发布事务边界执行。
- 未知 schema 或 opaque legacy 内容不得因为分析器不认识而被删除或改写。

## 12. 测试要求

### 12.1 核心算法

- Exec 自环、双节点环、多个独立 SCC。
- 数据自环、多节点数据环、多个独立数据 SCC。
- 合法 ForLoop、While、ForLoopBreak 不误报。
- 不合法 break 回边和绕过循环体的回边仍阻止保存。
- 不可达执行节点、缺少入口、可达数据依赖。
- 未连接纯数据节点和孤立数据岛。
- 深链、深环和上限附近大图不 panic。

### 12.2 保存行为

- 任一 `BlocksSave` 诊断存在时，正式保存 facade 不被调用。
- 另存为、强制保存和自动保存均不能绕过门禁。
- 原文件及 `.bak` 字节不变。
- 恢复快照原子写入、最多保留 5 份并能在启动时发现。
- 修复问题后正式保存成功并清理恢复快照。

### 12.3 兼容性和运行目标

- Go 编译失败但核心图合法时允许保存、禁止 Go 运行。
- workspace 其他文件问题不阻止当前文件保存。
- legacy `.vgf` 已知节点正常分析，opaque 节点不被误判为确定循环。
- 现有 legacy 未知字段和 hidden node/edge round-trip 测试不退化。

### 12.4 完整验证

```powershell
go test ./... -count=1
go vet ./...
go test -race ./... -count=1
cd frontend
npm run test:layout
npm run build
cd ..
wails build
```

## 13. 实施边界

实施应拆分为可独立验证的批次：诊断契约与纯分析算法、保存门禁、恢复快照、界面展示、完整回归。第一批不得修改 `.vgf` 导出格式或 Go VM 执行语义。任何需要新增运行语言或改变发布流程的工作另立设计。
