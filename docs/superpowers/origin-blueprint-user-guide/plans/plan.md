# OriginBlueprint 中文使用手册整合实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用一份与当前源码一致的中文手册完整说明蓝图编辑器、Go 库、通用异步恢复和使用禁区，并删除重复或过时的顶层说明文档。

**Architecture:** `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md` 是唯一用户与集成者入口；自动生成的验证报告和 `docs/superpowers/**` 历史设计继续保留。手册以当前 VM、节点 schema、Facade API 和测试为事实来源，Actor 仅作为通用 Dispatcher 的宿主适配示例。

**Tech Stack:** Markdown、Go、Vue/TypeScript 节点 schema、PowerShell 验证命令。

## Global Constraints

- 只输出中文版。
- Core 不新增 Delay/TimerScheduler，不修改任何运行时行为。
- File、DataFrame/Table、Dict 不得写成当前支持的数据类型。
- 异步统一使用 `BaseExecNode.Yield` 与一次性 `YieldHandle`。
- 通用库不依赖 Actor；Actor 只作为 `ExecutionDispatcher` 的适配示例。
- 删除清单必须与设计稿一致，不删除 `docs/superpowers/**` 和自动生成验证报告。
- 不提交 Git，由用户自行比较和提交。

---

### Task 1: 编写唯一权威中文手册

**Files:**
- Create: `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md`
- Reference: `docs/superpowers/origin-blueprint-user-guide/designs/design.md`
- Reference: `engine/go/blueprint/definition.go`
- Reference: `engine/go/blueprint/runtime.go`
- Reference: `engine/go/blueprint/vm_async.go`
- Reference: `engine/go/blueprint/dispatcher.go`
- Reference: `engine/go/blueprint/execution_session.go`
- Reference: `engine/go/blueprint/blueprint.go`
- Reference: `frontend/src/editor/nodeRegistry.ts`
- Reference: `frontend/src/editor/runtimeNodeSchemas.ts`

**Interfaces:**
- Consumes: `IExecNode.GetName() string`、`IExecNode.Exec() (int, error)`、`BaseExecNode.Yield(int)`、`YieldHandle.Resume(...any)`、`YieldHandle.ResumeTo(int, ...any)`。
- Produces: 面向编辑器用户、Go 节点开发者和宿主集成者的单一阅读入口。

- [x] **Step 1: 创建文档骨架和阅读路线**

写入设计稿第 5 节的 16 个章节，并在首页明确：快速使用读第 3、8、9 节；新增节点读第 5、6、8 节；异步接入读第 10、11、13 节。

- [x] **Step 2: 编写编辑器与节点 schema 章节**

完整说明 legacy runtime schema 的 `name/title/package/description/is_pure/inputs/outputs` 及端口 `name/type/data_type/has_input/default_value/pin_widget/port_id`；完整说明 native schema 的 `id/sourceName/title/category/kind/subtitle/width/inputs/outputs/dynamicOutputs/dynamicBranch` 及稳定 key。给出纯数据、同步流程、多分支、入口、数组、动态分支和异步节点 JSON。

- [x] **Step 3: 编写 Go 节点和执行 API 章节**

给出 `IExecNode`、`BaseExecNode`、节点工厂依赖注入、`RegisterExecNode`、`Init`、`Create`、`Do/DoContext/Start`、`Execution`、`ReleaseGraph/Close` 的完整示例，并说明 `PortArray` 返回语义。

- [x] **Step 4: 编写通用异步章节**

给出定时器 `Resume`、RPC 成功/失败 `ResumeTo`、默认/inline/Actor-aware Dispatcher 示例；明确 Yield 配对、一次性恢复、回调只捕获 handle/普通值、输出顺序、取消和事件循环死锁规则。

- [x] **Step 5: 编写特殊、进阶、禁止用法和检查清单**

覆盖循环与函数挂起恢复、并发 Execution、变量状态、Trace、热加载、低层 Registry/CompiledGraph、性能、测试、排错、上线检查和错误索引；用反例/正确替代表说明禁止用法。

- [x] **Step 6: 检查手册中不存在旧运行时描述**

Run:

```powershell
rg -n "Continuation|TimerScheduler|timerMu|DataFrame|RuntimeTable|\bDict\b" docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md
```

Expected: 仅允许在“已废弃/禁止恢复”说明中出现，不得作为当前 API 或支持能力出现。

### Task 2: 更新唯一入口规则

**Files:**
- Modify: `AGENTS.md`
- Modify: `engine/go/blueprint/AGENTS.md`

**Interfaces:**
- Consumes: `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md`。
- Produces: Agent 和维护者不再读取将被删除的旧说明文档。

- [x] **Step 1: 更新根 AGENTS 必读入口**

将多个旧文档引用替换为新手册和自动验证报告；保留项目职责边界、兼容红线和验证命令，不把已删除文件写回索引。

- [x] **Step 2: 更新 engine AGENTS 必读入口**

用新手册替换 `CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`、`BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md` 和 `LEGACY_COMPATIBILITY_ZH.md` 引用；保留 VM、Yield、并发和测试硬约束。

- [x] **Step 3: 验证两个入口只引用现存文件**

Run:

```powershell
rg -n "docs/.*\.md" AGENTS.md engine/go/blueprint/AGENTS.md
```

Expected: 只引用 `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md`、`docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md` 或实际保留的 `docs/superpowers/**`。

### Task 3: 删除重复和过时说明

**Files:**
- Delete: `docs/AI_PROJECT_CONTEXT_ZH.md`
- Delete: `docs/ARCHITECTURE.md`
- Delete: `docs/BLUEPRINT_CHANGE_SAFETY_ZH.md`
- Delete: `docs/BLUEPRINT_ENGINE_COMPATIBILITY_ZH.md`
- Delete: `docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`
- Delete: `docs/BLUEPRINT_TIMER_DESIGN_ZH.md`
- Delete: `docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`
- Delete: `docs/LEGACY_COMPATIBILITY_ZH.md`
- Delete: `docs/MAINTENANCE_GUIDE_ZH.md`
- Delete: `docs/NODE_JSON_FORMAT_ZH.md`
- Delete: `docs/ORIGIN_NODE_EDITOR_PARITY.md`

**Interfaces:**
- Consumes: 已写入新手册的有效内容。
- Produces: 顶层 `docs` 只保留单一手册和自动验证报告作为普通阅读入口。

- [x] **Step 1: 用 apply_patch 删除清单内 11 个文件**

删除后不得重新创建同名文件。

- [x] **Step 2: 检查顶层 Markdown 清单**

Run:

```powershell
Get-ChildItem docs -File -Filter '*.md' | Sort-Object Name | Select-Object -ExpandProperty Name
```

Expected:

```text
BLUEPRINT_VERIFICATION_MATRIX_ZH.md
ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md
```

### Task 4: 源码一致性和链接验证

**Files:**
- Verify: `docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md`
- Verify: `AGENTS.md`
- Verify: `engine/go/blueprint/AGENTS.md`

**Interfaces:**
- Consumes: 当前 Go 导出 API、节点 schema 字段和文档路径。
- Produces: 可交付的中文手册和零悬空入口。

- [x] **Step 1: 核对手册使用的导出标识符**

Run:

```powershell
rg -n "type (IExecNode|BaseExecNode|YieldHandle|Execution|ExecutionDispatcher)|func (NewInlineExecutionDispatcher|NewActorExecutionDispatcher)|func \(.*\) (Yield|Resume|ResumeTo|RegisterExecNode|Init|Create|Start|Do|DoContext|Result|Cancel|ReleaseGraph|Close)" engine/go/blueprint -g '*.go'
```

Expected: 手册中的所有接口名均有源码定义，签名和返回值一致。

- [x] **Step 2: 扫描被删除文档的悬空引用**

Run:

```powershell
$deleted = 'AI_PROJECT_CONTEXT_ZH|ARCHITECTURE\.md|BLUEPRINT_CHANGE_SAFETY_ZH|BLUEPRINT_ENGINE_COMPATIBILITY_ZH|BLUEPRINT_ENGINE_TEST_MATRIX_ZH|BLUEPRINT_TIMER_DESIGN_ZH|CODEX_BLUEPRINT_ENGINE_RULES_ZH|LEGACY_COMPATIBILITY_ZH|MAINTENANCE_GUIDE_ZH|NODE_JSON_FORMAT_ZH|ORIGIN_NODE_EDITOR_PARITY'
rg -n $deleted . -g '*.md' -g 'AGENTS.md' --glob '!docs/superpowers/**'
```

Expected: 无匹配。

- [x] **Step 3: 检查设计要求覆盖**

Run:

```powershell
rg -n "节点 JSON|IExecNode|RegisterExecNode|Blueprint\.Start|YieldHandle|ResumeTo|ExecutionDispatcher|Actor|禁止|上线前检查" docs/ORIGIN_BLUEPRINT_USER_GUIDE_ZH.md
```

Expected: 每个主题至少有一个独立章节或示例。

- [x] **Step 4: 检查工作区改动**

Run:

```powershell
git status --short -- docs AGENTS.md engine/go/blueprint/AGENTS.md
```

Expected: 新手册、设计/计划、两个 AGENTS 更新和清单内删除；无运行时代码改动。
