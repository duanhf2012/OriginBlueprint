# 蓝图验证样本生成实施计划

> **适用于 agentic worker：** 必须按任务逐项执行，并使用复选框追踪进度。

**目标：** 生成一套可在编辑器中打开检查的 `.vgf`、`.obp` 与 `.obpf` 蓝图样本，为后续解析与执行结果对照奠定固定输入文件。

**架构：** 新增一个独立的 PowerShell 生成脚本，脚本只写入 `examples/verification-blueprints/`，并从当前已确认的 `GraphDocument` 节点 ID 和端口 key 构建 JSON。样本覆盖说明和覆盖矩阵同目录保存；不修改 Go 引擎、前端绘制、迁移或存档逻辑。

**技术栈：** PowerShell、JSON、现有 Go `GraphDocument` 格式、legacy `.vgf` 格式。

## 全局约束

- 仅执行验证计划的第 1 阶段，不添加 Go 对照测试或随机执行测试。
- 只新增 `scripts/`、`examples/verification-blueprints/` 和本计划文件；不得修改解析器、编辑器、存档或节点定义。
- 文件内容必须使用当前 `engine/go/blueprint/document.go` 中的节点 ID、端口 key 与函数签名规则。
- 随机数节点使用相同的最小值和最大值；概率节点仅使用 `0` 或 `10000`。

---

### 任务 1：实现可重复的样本生成器

**文件：**
- 新增：`scripts/generate-verification-blueprints.ps1`
- 新增：`examples/verification-blueprints/01_legacy_all_nodes_showcase.vgf`
- 新增：`examples/verification-blueprints/02_control_flow_maze.obp`
- 新增：`examples/verification-blueprints/03_array_data_lab.obp`
- 新增：`examples/verification-blueprints/04_deterministic_algorithm.obp`
- 新增：`examples/verification-blueprints/05_function_orchestrator.obp`
- 新增：`examples/verification-blueprints/06_timer_lifecycle.obp`
- 新增：`examples/verification-blueprints/functions/10_score_kernel.obpf`
- 新增：`examples/verification-blueprints/functions/11_array_fold_and_format.obpf`
- 新增：`examples/verification-blueprints/functions/12_nested_control_function.obpf`
- 新增：`examples/verification-blueprints/functions/13_local_state_isolation.obpf`

**输入：** `engine/go/blueprint/document.go` 的 `documentNodeSpecs`、`functionEntrySpec`、`functionReturnSpec` 与 `functionCallSpec`。

**产出：** 11 个可视化蓝图文件，涵盖入口、控制流、数组、算术、字符串、函数、变量和定时器。

- [ ] 编写生成器：为 native 图、函数图和 legacy 图提供 JSON 节点、连线、分组、变量和视图对象构造函数。
- [ ] 编写生成器：为每个函数使用同一份完整签名写入函数入口、返回和调用节点。
- [ ] 运行：`powershell -ExecutionPolicy Bypass -File scripts/generate-verification-blueprints.ps1`
- [ ] 检查：运行 `Get-ChildItem examples/verification-blueprints -Recurse -File`，应得到 13 个文件（11 个蓝图/函数文件、README、coverage）。

### 任务 2：记录覆盖边界和人工验收方式

**文件：**
- 新增：`examples/verification-blueprints/README.md`
- 新增：`examples/verification-blueprints/coverage.json`

**输入：** 任务 1 生成的文件和 `documentNodeSpecs` 中所有顶层系统节点。

**产出：** 中文的打开顺序、预期可视结果和按节点 ID 记录的覆盖矩阵。

- [ ] 在 `README.md` 列出每个样本的入口、关键结构、预期可见布局与第 2 阶段的执行目标。
- [ ] 在 `coverage.json` 为每个顶层节点 ID 标明样本路径和 `visual`、`execution` 或 `async` 覆盖级别。
- [ ] 说明 legacy 图以视觉导入/导出为主，定时器图只在第 2 阶段执行。

### 任务 3：第 1 阶段自检

**文件：**
- 检查：`examples/verification-blueprints/**/*.vgf`
- 检查：`examples/verification-blueprints/**/*.obp`
- 检查：`examples/verification-blueprints/**/*.obpf`

**输入：** 任务 1 和任务 2 生成的全部文件。

**产出：** JSON 语法正确、覆盖矩阵完整、仓库既有测试未受影响的样本集。

- [ ] 运行 PowerShell `ConvertFrom-Json` 检查每个 JSON 文件；预期全部解析成功。
- [ ] 使用 PowerShell 对比 `coverage.json` 的节点 ID 与 `engine/go/blueprint/document.go` 的 51 个 `origin.*` 系统节点；预期无缺失项。
- [ ] 运行 `go test ./engine/go/blueprint -count=1`；预期通过，证明样本新增没有影响引擎。
- [ ] 运行 `git diff --check`；预期无空白错误。

## 自检

- 规格覆盖：三个任务分别覆盖文件生成、覆盖说明和格式/回归检查。
- 占位符检查：本计划没有未确定的路径、节点 ID 或验收命令。
- 契约一致性：生成器只使用 `document.go` 已定义的 native 节点和函数端口命名规则。
