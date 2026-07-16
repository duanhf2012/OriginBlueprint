# 蓝图编辑器 P2/P3 加固设计

## 目标

完成原始审查中尚未关闭的 P2/P3 风险，并把“编辑器检查通过”收紧为“真实 Go 引擎能够解析和编译”。本阶段覆盖：

1. 编辑器结构校验与真实引擎解析/编译规则统一。
2. 原生文档在 normalize 前检查，禁止非法内容被静默修复后覆盖源文件。
3. `1m/3m/5m` 自动保存成为真实功能。
4. 项目设置和应用配置使用原子写入，写入错误不再被忽略。
5. legacy `.vgf` 根、节点和边的未知 JSON 字段可无损往返。
6. 前端遗漏测试进入主测试命令，并以行为测试覆盖新增策略。
7. 内联标量、布尔和数组编辑进入 Undo；历史有明确上限。

P1 已完成的图文件原子保存、强制覆盖备份、恢复损失保护和 Sequence 256 保持不变。

## 方案比较

### 方案 A：增量加固（采用）

保留桌面结构校验器用于生成带 `nodeId` 的可定位问题，同时用真实 Go engine 初始化和编译当前文档及 workspace 函数库，把引擎错误追加为阻断项。保存、自动保存、Undo 和 legacy 扩展字段分别提取小型纯策略或数据边界。

优点是复用生产引擎规则、改动可分批测试，不需要重写 Rete 或 GraphDocument。缺点是结构校验和引擎预编译仍有少量职责重叠，但最终接受标准只有引擎结果。

### 方案 B：完全删除桌面校验器

只返回引擎首个错误。实现更少，但会失去当前多问题列表、不可达节点提示和节点定位，用户修复效率明显下降，因此不采用。

### 方案 C：引入通用 JSON AST/orphan 编辑模型

把所有未知字段和未知节点直接挂到可编辑 AST。长期能力最强，但需要重做 Rete 恢复、属性面板、保存合并和冲突语义，超出本阶段风险预算，因此不采用。

## 1. 真实引擎预编译门禁

新增桌面服务 `ValidateGraphForWorkspace(content, workspaceRoot, sourcePath)`，处理顺序固定为：

1. 对原始内容运行现有结构校验，收集可定位问题。
2. 加载编辑器当前实际使用的内嵌、可执行文件同级和 workspace `nodes/` 定义。
3. 为没有宿主业务实现的 runtime schema 创建只用于结构编译的 no-op 工厂；端口和 class 仍由真实 schema 决定。
4. 在隔离临时目录中写入当前文档和 workspace 的 `.obpf` 函数文档，当前源文件若位于 workspace 内则替换同路径副本，避免函数 ID/路径别名冲突。
5. 调用真实 `blueprint.Blueprint.Init`。因此严格 JSON、未知字段、重复入口、端口多生产者、native Exec fan-out、数据环、非结构化执行环、函数签名和返回路径均由生产解析器/编译器判定。
6. 将 `BlueprintError.Stage` 映射为 `engine.parse` 或 `engine.compile`，尽可能从结构化错误或稳定错误文本提取 `nodeId`。

节点定义加载错误本身是 `engine.definition` error。结构校验问题不会因为存在 legacy placeholder 而跳过整图；引擎错误始终是保存门禁。

浏览器模式没有 Go runtime，只能执行 JSON 语法检查，并明确返回 `engine.unavailable` warning，不能伪装成完整编译通过。

## 2. normalize 前检测

`openGraph` 在确认 `schemaVersion: 1` 后，必须把磁盘原始字符串直接交给 `ValidateGraphForWorkspace`，再调用 `normalizeDocument`。若原始内容包含 error：

- 仍尽可能加载为恢复视图，方便用户查看和修复；
- 标签进入 fatal source-protection 状态；
- 日志面板展示原始问题；
- 普通保存不能覆盖源文件，只能另存恢复副本；
- 自动保存跳过该标签。

这样即使 normalize 为显示目的补默认数组、默认 view 或变量兼容值，也不能把“被修过的结果”静默写回原文件。另存副本成功后，新路径重新按当前文档验证并清除保护。

## 3. 自动保存

新增纯策略模块，固定映射：`off=0`、`1m=60000`、`3m=180000`、`5m=300000`。设置变化或 workspace 切换时重建一个 `setInterval`，组件卸载时清理。

每次 tick 只保存满足全部条件的标签：

- `dirty=true`；
- 已有真实路径，绝不弹出 Save As；
- 不存在 `restoreFatal`、restore loss 或原始文档错误；
- legacy 路径能表达当前文档，不需要强制转为 native；
- 当前没有手动保存或另一轮自动保存。

自动保存复用正常的验证、legacy 导出和后端原子写入，但不切换标签、不弹确认框。活动标签先抓取最新快照；非活动标签使用切换时持久化的 `tab.document`。单个标签失败不阻止其他标签，失败标签保持 dirty，并在状态栏给出汇总。

## 4. 设置和配置原子写入

`LoadProjectSettings` 首次创建默认文件、`SaveProjectSettings` 和 `writeAppConfig` 全部复用 `writeFileAtomically`。`writeAppConfig`、`recordRecent`、`recordExportDirectory` 返回 error；图文件已经成功保存时，最近文件配置失败只记录 Wails 错误而不把图保存误报为失败。项目设置保存失败继续返回给前端并显示状态。

## 5. legacy 未知字段往返

在 `GraphLegacyState` 增加三类不透明字段袋：

- `extraRootFields: map[string]json.RawMessage`
- `extraNodeFields: map[nodeId]LegacyNodeExtras`，同时记录原 class
- `extraEdgeFields: map[legacyOrdinal]map[string]json.RawMessage`

迁移时只收集固定结构没有声明的键。导出时先生成当前已知 legacy 结构，再合并未知字段；已知字段永远由当前编辑结果覆盖。节点扩展只在 node ID 和 class 都匹配时恢复；边扩展只跟随保留下来的原始 ordinal，删除或新建的边不会继承旧扩展。前端把这些字段当作 opaque legacy state 深拷贝，不渲染也不修改。

回归 fixture 必须同时包含根、可见节点、隐藏节点、可见边和隐藏边扩展字段，并验证 migrate→GraphDocument JSON→export 后 JSON 值深度相等。

## 6. Undo 历史

历史上限固定为 100 个完整快照，所有 push 经过同一个有界 helper；超过上限丢弃最旧项，redo 同样有界。仍使用整图快照，避免在本阶段引入命令系统和 legacy 增量合并风险。

`BlueprintControl` 与动态分支控件发出 edit-start、change、edit-commit 三段事件：

- start 在第一次修改前捕获一次快照；
- 连续键入或数组编辑只标 dirty，不重复压栈；
- commit 在 blur/change 或按钮操作结束时压入一次快照；
- 没有实际 change 的 focus/blur 不产生历史。

Undo/Redo、加载文档和销毁编辑器必须清理待提交的控件事务。

## 7. 测试治理

- 用 Vitest 行为测试覆盖自动保存资格、区间映射、有界历史和原始错误保护策略。
- 把 `implicitEntryLinks.test.ts`、`selectionGeometry.test.ts` 转为 Vitest suite 并加入 `test:layout`。
- 把已有 `legacyPropertyPreservation.test.js` 加入 `test:layout`，同时以 Go round-trip 测试作为 legacy 无损的权威证据。
- Go 测试覆盖真实引擎拒绝多生产者、Exec fan-out、数据环、重复入口、未知执行节点和函数签名错误。
- 原子设置/配置测试验证替换成功及失败不破坏原内容。
- Undo 的 DOM 事件接线保留 focused source guard；核心历史边界和事务状态用纯 TypeScript 行为测试。

## 非目标

- 不实现完整 orphan 节点可视化或通用 JSON AST 编辑器。
- 不改变 `.vgf` 已知字段、端口映射、边顺序或执行语义。
- 不为自动保存弹出路径、覆盖或兼容性确认对话框。
- 不把浏览器模式宣称为真实 Go 编译验证。
- 不重写 Undo 为命令模式，也不承诺跨应用重启历史。
- 不修改 Go VM 执行语义、性能热路径或函数调度。

## 验收

1. 原始非法 native 文档不能在 normalize 后静默覆盖源文件。
2. `ValidateGraphForWorkspace` 对生产编译器拒绝的结构返回 error，合法验证 fixture 仍通过。
3. 自动保存按配置周期执行，且不触发 Save As、兼容性覆盖或并发保存。
4. 项目设置和 app config 使用原子替换，错误可观察。
5. legacy 未知字段 fixture 完整往返，现有 legacy 审计不退化。
6. 内联标量、布尔、数组和动态分支编辑一次操作对应一次 Undo，历史不超过 100。
7. `go test ./... -count=1`、`go test -race ./engine/go/blueprint -count=1`、`go vet ./...`、`npm run test:layout`、`npm run build`、`wails build` 全部退出 0。
