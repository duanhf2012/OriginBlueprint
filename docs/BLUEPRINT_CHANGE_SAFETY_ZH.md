# 蓝图改动安全清单

本文档沉淀蓝图编辑器、旧 `.vgf` 迁移、节点 JSON、前端渲染和 Go 执行链路的改动经验。后续修改蓝图相关代码前，先读这里，再读 `docs/LEGACY_COMPATIBILITY_ZH.md` 和 `docs/NODE_JSON_FORMAT_ZH.md`。

## 核心原则

蓝图不是单纯 UI。任何改动至少同时影响三层合同：

- 数据合同：节点、连线、端口、默认值、变量、分组不能丢。
- 语义合同：入口识别、端口类型、动态分支、导入导出、执行校验不能错。
- 显示合同：节点标题、端口名、颜色、布局、兼容标记不能退化。

只验证其中一层是不够的。比如只验证“节点没有丢”，仍可能出现全部节点变成兼容黄色、标题退化成 `name/class`、右侧动态输出缺失等问题。

## 改动前判断范围

只要涉及下面任一文件或目录，就按蓝图核心链路处理：

- `legacy.go`
- `graph.go`
- `execution.go`
- `node_schemas.go`
- `nodes/**/*.json`
- `frontend/src/editor/runtimeNodeSchemas.ts`
- `frontend/src/editor/nodeRegistry.ts`
- `frontend/src/editor/createEditor.ts`
- `frontend/src/editor/BlueprintNode.vue`
- `frontend/src/editor/BlueprintSocket.vue`
- `frontend/src/App.vue`
- `engine/go/blueprint/**`

开始改动前先回答：

1. 会不会影响旧 `.vgf` 打开？
2. 会不会影响 `.vgf/.obp` 保存或导出？
3. 会不会影响节点标题、颜色、端口名或布局？
4. 会不会影响入口节点、动态分支或隐式入口参数引用？
5. 会不会影响 Go engine 运行时解析？

## 节点显示规则

### 标题来源优先级

节点标题必须按以下优先级处理：

```text
JSON title > 用户显式自定义 label > fallback legacyClass/name
```

注意：

- 旧 `.vgf` 只有 `class`，没有 `title`。
- 如果业务 `nodes/json/*.json` 缺失，迁移只能 fallback 到 `class/name`。
- 不要为了显示方便把 `legacyClass` 写进 `properties.label`。否则以后即使恢复 JSON title，蓝图文件也会继续显示旧英文 name。
- `properties.label` 只用于用户显式改名或确实需要持久化的显示名。

### 颜色来源

节点标题颜色应由节点业务类型决定：

- `event`：入口/事件类颜色。
- `flow`：流程控制颜色。
- `function`：函数/普通业务节点颜色。
- `variable`：变量颜色。

兼容状态不能覆盖业务类型颜色：

- `legacyStyle` 可以显示虚线边框。
- `legacyStyle` 可以显示 `COMPAT` 小标。
- `legacyStyle` 不应覆盖 `--accent`，否则大量 fallback 节点会全变成同一种黄/橙色。

入口节点菱形和参数引用颜色可以由入口源 key hash 得到，但颜色必须是运行时显示计算结果，不要写入蓝图文件。

不要用很小的固定调色板再 `hash % palette.length` 给入口分配颜色。入口数量稍多时很容易撞色，导致不同入口的菱形和参数引用看起来一样。应从完整 hash 做 avalanche 混合后再派生色相、饱和度和亮度；否则相近入口名仍可能落在相近色相上，肉眼看起来几乎一样。

## JSON schema 覆盖规则

旧 `.vgf` 中的 `class` 能否显示为正确 title 和端口名，取决于当前是否加载了对应节点 JSON。

修改 `nodes` 目录时必须检查：

```text
旧蓝图中出现的 class 是否都能从 runtime schema 推导到。
```

经验规则：

- 优先恢复或加载真实 `nodes/json/*.json`。
- fallback 只处理少量确实未知或临时缺定义的运行时节点。
- 如果正常业务蓝图里大量节点变成 `origin.legacy.placeholder`，说明 schema 覆盖出了问题，不能只靠 fallback 糊过去。
- 不要恢复已删除的文件/表格/字典工具链节点，除非有明确需求。
- `nodes/json/common/*` 和顶层 `nodes/*.json` 可能重复，恢复历史 JSON 时要避免旧 common 覆盖当前基础定义。

## fallback 使用规则

fallback 的目标是“不丢图”，不是替代 schema。

可接受 fallback：

- 旧 `.vgf` 中来自 `tools.json_node_loader` 的业务节点，但当前缺少 JSON 定义。
- 迁移时能从旧连线推断出端口，至少可见、可移动、可导出。

不应 fallback：

- 明确删除的旧节点，例如文件、表格、字典相关旧工具节点。
- 已经有 JSON schema 的业务节点。
- 已经有静态 `legacyNodeSpecs` 映射的基础节点。

fallback 节点要求：

- 不进入 `HiddenNodes`，除非完全无法安全保留。
- 保留 `legacyClass`、`legacyModule`、`legacyInputs`、`legacyOutputs`，用于导出回旧 `.vgf`。
- 不把 `legacyClass` 固化为 `properties.label`。
- 前端显示可以有 `COMPAT` 标记，但不能统一改标题颜色。

## port_id 与 key 规则

旧格式使用数字端口：

```text
source_port_id / des_port_id / port_defaultv
```

新格式使用字符串 key：

```text
sourceOutput / targetInput / values
```

维护规则：

- 旧 `port_id` 到新 key 的映射必须稳定。
- 动态分支的旧端口编号不能被随意重排。
- `case0` 这类历史占位输出如果是兼容旧格式需要，应明确写在映射或动态分支规则中。
- 导出旧 `.vgf` 时，必须回到旧解析器期待的端口编号。
- 不要根据当前 UI 行号推断导出端口，必须根据 schema/legacy mapping。

## 动态分支规则

动态分支类节点要同时维护两边：

- 左侧数组输入项。
- 右侧对应 exec 输出端口。

检查点：

- 点击 `+ Item` 后，左侧 item 和右侧输出必须同步增加。
- 删除 item 后，对应多余输出必须移除。
- `maxBranches` 是实际上限；不要再用声明的 `case*` 输出数量制造第二套隐式上限。
- 如果生成输出，使用 `dynamicBranch.outputTemplate` 描述输出类型。
- 旧 EqualSwitch 这类节点仍要保持旧 `port_id` 映射。

## 入口节点规则

普通入口节点有额外约束：

- 同一个蓝图中不允许重复添加相同入口。
- 函数蓝图中不允许拖入普通入口节点。
- 重复或禁止添加时，应在画布操作位置附近显示 toast，而不是只在底部状态栏提示。
- 入口源颜色基于稳定 source key/name hash 计算，不写入蓝图文件。
- 多入口参数引用显示颜色时，只影响显示，不改变保存格式。

## 代表性回归样例

涉及蓝图迁移、节点 schema、显示或执行流时，至少检查这些样例：

- `build/bin/vgf/monsterChoiceskill/choiceskill_easy.vgf`
- `build/bin/vgf/monsterChoiceskill/*.vgf`
- `build/bin/vgf/battle/*.vgf`
- `build/bin/vgf/buffskill/**/*.vgf`
- 新格式 `.obp`
- 函数蓝图 `.obpf`

这些样例覆盖：

- 多种入口节点。
- 业务 JSON 节点 title/port 显示。
- 动态分支。
- 隐式入口参数引用。
- legacy 导入导出。
- Go engine 文档解析。

## 必测合同

后端测试至少覆盖：

- 旧 `.vgf` 打开后，已知业务节点不能进入 `HiddenNodes`。
- 已有 JSON schema 的业务节点不能变成 `origin.legacy.placeholder`。
- 缺 JSON 的运行时业务节点可以 fallback，但不能丢节点和连线。
- fallback 节点导出回旧 `.vgf` 时保留 class、module、端口编号和默认值。
- `choiceskill_easy.vgf` 不应报 `flow.missing-entry`。
- 对真实断开的样例，可以允许 `flow.unreachable-node`，但不能掩盖入口识别失败。

前端测试至少覆盖：

- legacy metadata 不等于 legacy visual style。
- `legacyStyle` 不覆盖节点 kind 颜色。
- 节点标题不能被 `legacyClass` 持久化污染。
- 动态分支新增/删除 item 同步输出端口。
- 重复入口/函数蓝图入口禁止时有画布 toast。
- 入口参数引用颜色来自源入口 key hash。

Go engine 测试至少覆盖：

- `GraphDocument` 中的 legacy placeholder 可以解析出端口映射。
- 动态分支输出端口能被执行/编译层识别。
- 新旧节点 JSON 和执行库注册名称不发生偏移。

## 推荐命令

常规验证：

```powershell
go test .
go test ./engine/go/blueprint
npm.cmd run test:layout -- --runInBand
npm.cmd run build
```

专项验证：

```powershell
go test . -run "TestMigrateBuildBinVGFFilesShowsAllDefinedNodes|TestValidateChoiceskillEasyRecognizesMonsterChoiceSkillEntry|TestChoiceskillEasyUsesRuntimeJsonTitlesInsteadOfFallbackNames" -count=1 -v
```

检查旧图 class 覆盖时，可从 `.vgf` 样例抽取 class，再对照 `runtimeLegacyNodeSpecs` 或 `nodes/**/*.json`。

## 禁止事项

- 不要只为“节点不丢”而让大量业务节点长期 fallback。
- 不要把 `legacyClass` 写入 `properties.label` 作为正常标题。
- 不要让 `legacyStyle` 覆盖所有兼容节点颜色。
- 不要删除或移动 `nodes/json/*.json` 后只跑前端构建。
- 不要只测保存/打开，不测 title、颜色、端口名。
- 不要只改前端 schema，不改 Go 迁移/校验/导出。
- 不要只改 Go 迁移，不检查前端恢复节点的显示效果。
- 不要把 PowerShell 输出乱码误判为 JSON 损坏。优先用编辑器或 UTF-8 工具确认文件内容。

## 出问题时的定位顺序

### 节点丢失

1. 看 `migrateLegacyGraph` 后 `document.Nodes` 数量。
2. 看 `document.Legacy.HiddenNodes` 里是否出现业务 class。
3. 查 `runtimeLegacyNodeSpecs` 是否加载了对应 `nodes/**/*.json`。
4. 查端口数量是否被 max input/output 判断误判越界。

### 节点标题显示成 name/class

1. 查旧 `.vgf` 是否只有 `class`。
2. 查 `nodes/**/*.json` 是否有对应 `name` 和 `title`。
3. 查迁移结果中是否错误写入 `properties.label = legacyClass`。
4. 查前端 `createRestoredNode` 是否走了 `origin.legacy.placeholder`。

### 节点颜色大面积异常

1. 查 `BlueprintNode.vue` 是否用兼容 class 覆盖 `--accent`。
2. 查 `nodeRegistry.ts` 是否正确推断 kind。
3. 查是否大量节点变成 fallback placeholder。

### 连线或端口错乱

1. 查旧边的 `source_port_id/des_port_id`。
2. 查 `legacyNodeSpecs` 和 runtime JSON 的端口顺序。
3. 查 `indexedKey` 和导出时 `legacyKeyIndex`。
4. 动态分支再查 `dynamicBranch` 配置和输出同步逻辑。

## 改动完成说明模板

蓝图核心改动完成后，最终说明至少包含：

```text
影响范围：
- 旧 .vgf 导入：
- 新 .obp/.obpf：
- 节点显示：
- 导出旧格式：
- Go engine：

验证：
- go test .
- go test ./engine/go/blueprint
- npm.cmd run test:layout -- --runInBand
- npm.cmd run build

仍需注意：
- 是否有 fallback 节点：
- 是否有允许存在的 unreachable-node：
```
