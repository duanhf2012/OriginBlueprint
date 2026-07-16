# OriginBlueprint 中文使用手册

> 本文是 OriginBlueprint 编辑器和 Go 执行库的唯一权威使用说明。行为以当前源码和测试为准。

## 1. 文档定位与阅读路线

OriginBlueprint 包含两个相互配合、但职责不同的部分：

- 蓝图编辑器：创建节点、连线、变量、函数图和蓝图文件。
- Go 执行库：加载节点定义与蓝图文件，编译为 VM 程序并执行。

按目标选择阅读章节：

| 目标 | 建议章节 |
| --- | --- |
| 快速加载并执行已有蓝图 | 3、8、9 |
| 新增普通业务节点 | 5、6、8 |
| 新增 RPC、定时等待等异步节点 | 10、11、13 |
| 理解函数、变量和循环恢复 | 7、11 |
| 生产接入与上线检查 | 9、13、15 |
| 排查错误或查看接口 | 15、16 |

当前能力边界：

- 当前执行器是 Go VM，保存每次 Execution 的 PC、流程栈、循环栈和函数调用栈。
- Core 不提供可执行的内置 Delay/Timer 调度器。
- RPC、定时器、消息队列等业务异步操作通过自定义节点的 `YieldHandle` 接入。
- Actor 不是 Core 依赖，只是宿主事件循环的一种 Dispatcher 适配方式。
- File、DataFrame/Table、Dict 运行时类型已经删除。
- `nodes/Event.json` 中的旧 Delay/Timer 节点仅用于旧文件兼容识别，执行会返回 `ErrUnsupportedAsyncNode`，新业务禁止使用。

## 2. 架构和核心概念

### 2.1 数据链路

```text
nodes/*.json
  -> 编辑器节点库
  -> 用户创建 .obp/.obpf 或编辑旧 .vgf
  -> Go Registry 加载节点定义
  -> CompileGraph 编译共享只读 CompiledGraph
  -> Blueprint.Create 创建图名与生命周期实例
  -> Blueprint.Start/Do 创建带独立局部变量的单次 Execution
  -> VM 执行或 Yield 后等待恢复
```

### 2.2 对象职责

| 对象 | 职责 | 是否可共享 |
| --- | --- | --- |
| `Registry` | 节点名称到 `NodeDefinition` 的映射 | 初始化/编译阶段使用 |
| `NodeDefinition` | 节点工厂、输入输出端口模板 | 编译后只读共享 |
| `CompiledGraph` | 编译后的程序、入口、函数和变量定义 | 可被多个实例共享 |
| `GraphInstance` | `Create` 产生的图名身份和生命周期句柄 | 由 `Blueprint` 管理，不保存普通变量 |
| `Execution` | 一次入口调用的状态、局部变量、结果和取消生命周期 | 每次调用独立 |
| `Graph` | 单次 Execution 的运行上下文 | 禁止跨 goroutine 复用 |
| `YieldHandle` | 一次异步挂起的一次性恢复句柄 | 只能成功恢复一次 |

`Blueprint` 是并发安全的外部 facade。`CompiledGraph`、`ExecNode` 和 `NodeDefinition` 只读共享；单次执行状态不能写回这些共享对象。

## 3. 五分钟快速开始

### 3.1 准备目录

```text
myapp/
  nodes/             节点定义 JSON
  blueprints/        .vgf/.obp/.obpf
```

`Blueprint.Init` 会递归读取节点定义目录中的 `.json`，但会跳过名为 `json` 的子目录；蓝图目录会递归读取 `.vgf`、`.obp`、`.obpf`。

桌面编辑器会合并内嵌节点库、编辑器可执行文件同级的 `nodes/` 和当前工作目录下的 `nodes/`；同一路径的外部文件可覆盖内嵌定义。新增或修改 JSON 后，在编辑器“文件”菜单选择“刷新节点库”，无需重启编辑器。

### 3.2 初始化并执行同步蓝图

```go
package main

import (
    "fmt"

    bp "github.com/duanhf2012/OriginBlueprint/engine/go/blueprint"
)

type HostModule struct{}

func (*HostModule) TriggerEvent(graphID, eventID int64, args ...any) error {
    return nil
}

func main() {
    module := &HostModule{}
    engine := &bp.Blueprint{}
    if err := engine.Init("./nodes", "./blueprints", module); err != nil {
        panic(err)
    }
    defer engine.Close()

    graphID := engine.Create("battle_ai")
    if graphID == 0 {
        panic("blueprint not found")
    }
    defer engine.ReleaseGraph(graphID)

    returns, err := engine.Do(
        graphID,
        bp.EntranceIDIntParam,
        bp.PortInt(10001), // 对象 ID
        bp.PortInt(7),     // 参数 1
        bp.PortInt(9),     // 参数 2
    )
    if err != nil {
        panic(err)
    }
    fmt.Printf("returns=%+v\n", returns)
}
```

`Create` 的参数是图名，不一定是文件名。新格式优先使用文档的 `graphName`；旧格式通常使用去掉扩展名后的文件名。

### 3.3 返回值不是固定为空

`Do`、`DoContext`、`Execution.Result` 返回 `PortArray`。蓝图执行到 `AppendIntReturn`、`AppendStringReturn` 等返回节点时，会按执行顺序追加结果；图中没有返回节点时才会得到空数组。

```go
returns, err := engine.Do(graphID, entranceID, args...)
if err != nil {
    return err
}
if len(returns) < 2 {
    return fmt.Errorf("blueprint returned %d values, want at least 2", len(returns))
}
skillID := returns[0].IntVal
targetID := returns[1].IntVal
```

`ArrayData` 包含 `IntVal`、`FloatVal`、`StrVal`、`BoolVal`。调用方必须按照蓝图返回契约读取对应字段，不要通过“非零字段”猜测类型。

## 4. 蓝图编辑器基础使用

### 4.1 常用操作

- 在节点库双击或拖拽节点到画布。
- 在画布右键搜索节点。
- 从输出端口拖到类型兼容的输入端口创建连线。
- 选中连线后按 Delete/X，或右键删除连线。
- `Ctrl+C/X/V` 复制、剪切、粘贴；`Ctrl+Z/Y` 撤销、重做。
- `Ctrl+A` 全选，`Ctrl+D` 取消选择。
- 变量拖到画布创建 Getter，`Alt+拖拽` 创建 Setter。
- 修改 `nodes/*.json` 后使用“文件 → 刷新节点库”，并检查状态栏是否报告 JSON 错误。
- 使用 `.obp` 保存普通图，使用 `.obpf` 保存函数图；旧 `.vgf` 打开和导出需要保留兼容信息。

### 4.2 连线规则

- Exec 端口只连接 Exec 端口。
- 数据端口应保持相同类型；编辑器只对少量明确转换提供自动转换。
- 数据流描述“取值来源”，执行流描述“执行顺序”。数据连线本身不会保证有副作用节点已经执行。
- 有副作用的数据生产节点必须接入执行流，再由后续节点读取其输出。
- 新格式禁止数据依赖环、非结构化 Exec 环和绕过结构化循环的 break 回边。

### 4.3 变量和函数

- 普通图变量属于单次 `Execution`；每次 `Do/Start` 都从蓝图默认值重新初始化，同一 `graphID` 的不同调用也不会共享。
- 同一 Execution 发生 Yield 后恢复时继续使用原变量槽位，不会重置；函数图每次调用仍有自己的局部变量。
- 当前没有跨 `Do/Start` 共享的全局变量。需要持久状态时放在宿主对象、数据库或缓存中，并通过业务节点显式读写。
- 函数图每次调用拥有独立函数局部变量和调用帧。
- 函数文件应有稳定 `functionId` 和签名；修改已上线函数参数顺序或类型属于兼容变更。
- 函数内发生 Yield 时，恢复后仍在原函数帧继续；函数返回后再回到调用者后续节点。

### 4.4 保存、自动保存与恢复边界

- 打开原生 `.obp/.obpf` 时会先校验原始 JSON，再做编辑器归一化；原始内容存在错误或恢复时丢失节点/连线时，原路径进入保护状态，只允许另存副本。显式“强制保存”会先原子写入同目录 `.bak` 备份。
- 项目设置中的自动保存仅在桌面端生效，支持关闭、1 分钟、3 分钟和 5 分钟。它只覆盖已有路径、已修改、校验通过且不存在兼容性损失的标签；无路径、正在保存、需要转存原生格式或受保护的标签会被跳过。
- 图文件、项目设置和应用配置均使用同目录临时文件加原子替换，避免进程中断留下半份 JSON。最近文件记录失败只记录诊断，不会把已经成功的图保存报告成失败。
- `.vgf` 迁移会在 `legacy` 状态中保留未知根字段、节点字段和边字段；导回 legacy 时仅向身份仍匹配的原节点/原边恢复这些扩展，当前编辑器维护的已知字段优先。
- Undo/Redo 保留最近 100 个完整编辑事务。文本连续输入在失焦时提交为一次事务，布尔值、数组项和动态分支增删也可撤销。
- 桌面校验最终会走与运行时相同的 Go 编译器规则；浏览器模式没有 Go engine，只能返回结构解析结果并报告 `engine.unavailable` 警告，不能作为上线前编译验证。

## 5. 节点 JSON 字段说明

编辑器能读取两种 schema。外部 Go 业务节点推荐使用 legacy runtime schema，因为它能用 `name + port_id` 直接与 `IExecNode.GetName()` 绑定。Native schema 更适合编辑器内建类型；任意新 `id` 不会自动获得 Go 执行实现。

### 5.1 Runtime schema：节点字段

```json
{
  "name": "QueryRoleLevel",
  "title": "查询角色等级",
  "title_en": "Query Role Level",
  "package": "角色",
  "package_en": "Role",
  "description": "读取角色当前等级",
  "description_en": "Read current role level",
  "is_pure": false,
  "width": 280,
  "inputs": [],
  "outputs": []
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是 | 运行时内部名称，必须与 Go `GetName()` 一致；入口节点可带 `_数字ID` 后缀 |
| `title/title_en` | 建议 | 中英文显示标题 |
| `package/package_en` | 建议 | 节点库分类 |
| `description/description_en` | 否 | 节点副标题和用途 |
| `is_pure` | 否 | 兼容字段；实际行为以端口和 Go 实现为准 |
| `width` | 否 | 节点显示宽度 |
| `inputs/outputs` | 是 | 输入、输出端口数组 |

### 5.2 Runtime schema：端口字段

```json
{
  "name": "角色ID",
  "name_en": "Role ID",
  "type": "data",
  "data_type": "Integer",
  "has_input": true,
  "default_value": 0,
  "hide_icon": false,
  "port_id": 1
}
```

| 字段 | 说明 |
| --- | --- |
| `name/name_en` | 端口显示名称 |
| `type` | `exec` 或 `data` |
| `data_type` | `Integer`、`Float`、`Boolean`、`String`、`Array`、`Any`；`TimerHandle` 仅兼容旧文件，不代表 Core 有 Timer |
| `has_input` | 输入数据口是否显示默认值控件 |
| `default_value` | 默认输入值 |
| `pin_widget` | 数组控件可用 `IntegerArrayWdg`、`StringArrayWdg` |
| `hide_icon` | 是否隐藏端口图标 |
| `port_id` | 稳定的端口下标；输入和输出分别从 0 编号，发布后禁止重排 |

端口总数上限为 4096，`port_id` 最大为 4095；同一输入数组或输出数组中禁止重复 `port_id`。

### 5.3 Native editor schema

```json
{
  "id": "origin.example.score-branch",
  "sourceName": "ScoreBranch",
  "title": "评分分支",
  "titleEn": "Score Branch",
  "category": "示例",
  "categoryEn": "Examples",
  "kind": "flow",
  "subtitle": "按评分选择出口",
  "width": 300,
  "inputs": [
    { "key": "exec", "label": "", "type": "exec" },
    { "key": "score", "label": "评分", "type": "data", "data_type": "Integer", "defaultValue": 0 }
  ],
  "outputs": [
    { "key": "low", "label": "低", "type": "exec" },
    { "key": "high", "label": "高", "type": "exec" }
  ]
}
```

| 字段 | 说明 |
| --- | --- |
| `id` | GraphDocument 中稳定的 `typeId` |
| `sourceName` | 原始运行时名称或入口来源名 |
| `title/category/subtitle` | 标题、节点库分类、副标题；带 `En` 后缀的是英文 |
| `kind` | `event`、`flow`、`function`、`variable`；通常可由端口推断 |
| `inputs/outputs` | 使用稳定字符串 `key` 的端口 |
| `defaultValue` | Native schema 的输入默认值，注意是 camelCase |
| `arrayItemType` | 数组项控件类型：`number` 或 `string` |
| `dynamicOutputs` | Sequence 一类动态输出开关 |
| `dynamicBranch` | 动态参数与动态分支出口的映射配置 |

重要：Native schema 能显示不等于 Go engine 能执行。只有当前 document 映射认识该 `id`，或导出文档携带可执行的 legacy class/端口信息，并且 Go 注册了工厂时，才具备执行能力。

## 6. 新建各种节点

### 6.1 纯数据节点

纯数据节点没有 Exec 端口，按下游读取需求求值。Go `Exec` 设置数据输出后返回 `-1, nil`。

```json
{
  "name": "AddBusinessScore",
  "title": "业务评分相加",
  "package": "业务运算",
  "is_pure": true,
  "inputs": [
    { "name": "A", "type": "data", "data_type": "Integer", "has_input": true, "default_value": 0, "port_id": 0 },
    { "name": "B", "type": "data", "data_type": "Integer", "has_input": true, "default_value": 0, "port_id": 1 }
  ],
  "outputs": [
    { "name": "结果", "type": "data", "data_type": "Integer", "port_id": 0 }
  ]
}
```

适合无副作用计算。禁止在纯节点中发 RPC、写数据库或修改宿主状态，因为下游可能多次重新求值。

### 6.2 同步执行节点

```json
{
  "name": "GrantReward",
  "title": "发放奖励",
  "package": "奖励",
  "inputs": [
    { "name": "", "type": "exec", "port_id": 0 },
    { "name": "奖励ID", "type": "data", "data_type": "Integer", "has_input": true, "default_value": 0, "port_id": 1 }
  ],
  "outputs": [
    { "name": "完成", "type": "exec", "port_id": 0 },
    { "name": "数量", "type": "data", "data_type": "Integer", "port_id": 1 }
  ]
}
```

同步完成后 `Exec()` 返回 `0, nil`，表示从输出 Exec 端口 0 继续。

### 6.3 多分支节点

```json
{
  "name": "CheckPermission",
  "title": "检查权限",
  "package": "权限",
  "inputs": [
    { "name": "", "type": "exec", "port_id": 0 },
    { "name": "用户ID", "type": "data", "data_type": "Integer", "port_id": 1 }
  ],
  "outputs": [
    { "name": "无权限", "type": "exec", "port_id": 0 },
    { "name": "有权限", "type": "exec", "port_id": 1 },
    { "name": "权限等级", "type": "data", "data_type": "Integer", "port_id": 2 }
  ]
}
```

`Exec()` 返回 0 或 1 选择对应 Exec 输出。异步节点如果有多个出口，使用 `ResumeTo(0, ...)` 或 `ResumeTo(1, ...)`。

### 6.4 入口节点

入口 runtime `name` 最后一个下划线后的十进制数字是入口 ID：

```json
{
  "name": "Entrance_BattleTurn_000101",
  "title": "战斗回合入口",
  "package": "入口",
  "inputs": [],
  "outputs": [
    { "name": "", "type": "exec", "port_id": 0 },
    { "name": "战斗ID", "type": "data", "data_type": "Integer", "port_id": 1 },
    { "name": "回合数", "type": "data", "data_type": "Integer", "port_id": 2 }
  ]
}
```

Go 工厂的 `GetName()` 返回去掉数字后缀的 `Entrance_BattleTurn`；调用时使用入口 ID `101`，参数按数据输出端口顺序传入。入口 ID 在同一图内必须唯一。

### 6.5 数组和 Any 节点

Go 数组类型为 `PortArray []ArrayData`。整数数组示例：

```go
items := bp.PortArray{
    {IntVal: 10},
    {IntVal: 20},
}
```

`Any` 可承载任意值，但会降低编辑器类型校验能力。跨文件持久化的值应优先使用明确类型；不要把不可序列化、带锁或拥有外部生命周期的 Go 对象写入蓝图持久数据。

### 6.6 动态 Sequence 和动态分支

- `dynamicOutputs: true` 用于 Sequence 一类可增加执行出口的节点。
- `dynamicBranch.controlInput` 指向承载分支值的数组输入 key。
- `defaultOutput` 是不随分支增减的默认出口。
- `outputPrefix/outputStartIndex` 定义动态出口 key。
- `maxBranches` 必须与 Go 编译器支持上限一致。
- `hiddenOutputKeys` 只用于保留旧端口编号占位。

动态输出端口上限为 256。修改动态端口规则时必须同时验证编辑器保存、GraphDocument 转换和 Go 编译。

### 6.7 异步节点

异步节点的 JSON 与普通多分支节点相同，区别在 Go `Exec()` 内调用 `Yield`。只要需要通过 `Resume/ResumeTo` 写数据，就必须把所有 Exec 输出放在输出数组前部，并让数据输出连续排列在后部；恢复参数从第一个数据输出开始逐项写入。完整示例见第 10 节。

## 7. 蓝图、函数、循环和返回语义

### 7.1 循环内异步恢复

当前 VM 会保存循环帧。循环体内节点挂起时，恢复后：

1. 从异步节点选定的 Exec 输出继续。
2. 执行当前循环体余下节点。
3. 当前迭代结束后进入下一迭代。

不会重新执行当前异步节点，也不会额外增加一次循环。比如挂起时 `i=1`，恢复并完成当前循环体后，下一轮是 `i=2`。

### 7.2 函数内异步恢复

函数调用使用 VM CallStack。函数内 Yield 不会丢失调用者：恢复后完成函数，函数返回值映射回调用节点，然后调用者继续后续执行流。

### 7.3 返回结果

`PortArray` 是顶层 Execution 的返回集合。函数返回节点负责函数输出，不等同于顶层 `Append*Return`。业务调用方应为每个入口定义稳定的返回契约，例如：

```text
入口 101：returns[0]=技能ID(Integer)，returns[1]=目标ID(Integer)
```

上线后不要随意改变返回顺序或类型。

## 8. Go 节点实现与注册

### 8.1 IExecNode

```go
type IExecNode interface {
    GetName() string
    Exec() (int, error)
}
```

推荐嵌入 `BaseExecNode` 获取端口和宿主模块访问方法。

### 8.2 纯数据节点实现

```go
type AddBusinessScoreNode struct {
    bp.BaseExecNode
}

func (*AddBusinessScoreNode) GetName() string { return "AddBusinessScore" }

func (n *AddBusinessScoreNode) Exec() (int, error) {
    a, ok := n.GetInPortInt(0)
    if !ok {
        return -1, fmt.Errorf("input 0 is not Integer")
    }
    b, ok := n.GetInPortInt(1)
    if !ok {
        return -1, fmt.Errorf("input 1 is not Integer")
    }
    if !n.SetOutPortInt(0, a+b) {
        return -1, fmt.Errorf("set output 0 failed")
    }
    return -1, nil
}
```

### 8.3 同步流程节点实现

```go
type RewardService interface {
    Grant(rewardID int64) (count int64, err error)
}

type GrantRewardNode struct {
    bp.BaseExecNode
    rewards RewardService
}

func (*GrantRewardNode) GetName() string { return "GrantReward" }

func (n *GrantRewardNode) Exec() (int, error) {
    rewardID, ok := n.GetInPortInt(1)
    if !ok {
        return -1, fmt.Errorf("rewardID is not Integer")
    }
    count, err := n.rewards.Grant(rewardID)
    if err != nil {
        return -1, err
    }
    if !n.SetOutPortInt(1, count) {
        return -1, fmt.Errorf("set count failed")
    }
    return 0, nil
}
```

### 8.4 通过工厂注入依赖

`NodeDefinition.New` 会为每次节点执行创建新对象。工厂必须返回新节点，不能返回共享可变实例：

```go
engine.RegisterExecNode(func() bp.IExecNode {
    return &GrantRewardNode{rewards: rewardService}
})
```

如果节点需要宿主模块，也可通过 `GetBlueprintModule()` 取出 `IBlueprintModule` 并做明确类型断言。优先注入小接口，减少节点对完整服务对象的依赖。

### 8.5 注册顺序

自定义工厂必须在 `Init` 前注册：

```go
engine := &bp.Blueprint{}
engine.RegisterExecNode(func() bp.IExecNode { return &NodeA{} })
engine.RegisterExecNode(func() bp.IExecNode { return &NodeB{client: client} })
if err := engine.Init(nodeDir, graphDir, module, logger); err != nil {
    return err
}
```

`GetName()`、JSON `name` 去掉入口 ID 后缀后的名称、Registry 名称三者必须一致，否则 `Init` 会报告节点尚未注册。

### 8.6 新增一个可执行节点的完整清单

1. 在宿主和编辑器都能读取的 `nodes/*.json` 中声明节点，先确定稳定的 `name`、输入输出顺序和 `port_id`。
2. 在编辑器选择“文件 → 刷新节点库”，把节点放入测试蓝图，连接所有必要的 Exec 和数据端口并保存。
3. 实现同名 `IExecNode`；同步节点设置输出并返回 Exec 出口，异步节点遵循第 10 节的 Yield 契约。
4. 使用返回新对象的工厂调用 `RegisterExecNode`，并且必须早于 `Init`。
5. `Init` 加载与编译节点定义和蓝图；任何“能显示但未注册”“端口类型不匹配”“入口冲突”都应在这一阶段阻止启动。
6. 至少测试默认值、边界值、随机输入和错误路径；异步节点还要测试成功、失败、超时、取消、重复回调和 Dispatcher 拒绝。
7. 发布后把节点 `name`、入口 ID、端口编号、参数顺序和返回顺序视为兼容契约；需要变更时新增版本化节点，不直接重排旧端口。

## 9. Go 图加载与执行场景

### 9.1 Do：普通阻塞调用

`Do` 使用后台 Context，并等待 Execution 完成。适合同步图，或允许当前 goroutine 等待的异步图：

```go
returns, err := engine.Do(graphID, entranceID, args...)
```

### 9.2 DoContext：超时和取消

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

returns, err := engine.DoContext(ctx, graphID, entranceID, args...)
```

必须传入非 nil Context。Context 取消后，Execution 终止；外部 RPC 或定时器资源是否取消仍由业务节点或宿主管理。

### 9.3 Start：事件循环和异步图

`Start` 返回 `Execution`，不阻塞等待完成：

```go
execution, err := engine.Start(ctx, graphID, entranceID, args...)
if err != nil {
    return err
}

go func() {
    <-execution.Done()
    returns, runErr := execution.Result()
    consumeResult(returns, runErr)
}()
```

事件循环、Actor、GUI 线程和单线程服务器中，可能 Yield 的图必须用 `Start`。如果在事件循环中调用 `Do/DoContext` 阻塞，而恢复任务又需要投递回同一事件循环，会造成死锁。

### 9.4 Execution 接口

| 接口 | 说明 |
| --- | --- |
| `ID()` | 本次执行 ID |
| `Done()` | 终态关闭的 channel |
| `State()` | Pending/Running/Suspended/Completed/Canceled/Failed |
| `IsDone()` | 是否已经进入终态 |
| `Result()` | 完成前返回 `ErrExecutionPending`；完成后返回结果副本和错误 |
| `Cancel()` | 请求取消；已经终态时返回 false |

### 9.5 Dispatcher 选择

| Dispatcher | 初始执行 | Yield 恢复 | 适用场景 |
| --- | --- | --- | --- |
| 默认 worker | worker 队列 | worker 队列 | 无线程归属要求的通用服务 |
| `NewInlineExecutionDispatcher` | 调用方立即执行 | 调用 Resume 的 goroutine 立即执行 | 纯同步、测试、明确允许回调线程继续执行 |
| `NewActorExecutionDispatcher(enqueue)` | 当前调用线程立即执行 | `enqueue` 指定的事件队列 | Actor、GUI、游戏主循环、任意线程归属宿主 |
| 自定义 `ExecutionDispatcher` | 由实现决定 | 由实现决定 | 自定义 executor、协程调度器、优先级队列 |

设置必须发生在启动 Execution 前：

```go
engine.SetExecutionDispatcher(dispatcher)
```

### 9.6 实例和关闭

- `Create` 返回 0 表示图不存在或 Blueprint 已关闭。
- `ReleaseGraph` 会释放实例并取消该实例仍未完成的 Execution。
- `Close` 会关闭 Blueprint、释放所有实例并取消全部 Execution；重复调用安全。
- `HotReload` 编译成功后原子替换只读图池；失败时保留旧图和加载配置。
- 已经开始或挂起的 Execution 继续使用启动时捕获的旧编译图和局部变量；之后的 `Start/Do` 获取新图并从新默认值初始化。
- 热加载允许新增、删除变量或修改变量类型，因为普通变量不跨 Execution 迁移。删除图后旧 Execution 可完成，但该 `graphID` 的新调用返回 `ErrGraphNotFound`。
- `Init` 只用于没有实例和活动 Execution 的完整初始化；运行中重复调用返回 `ErrBlueprintInUse`。无实例时重复 Init 会整体替换图池，加载失败不改变当前 module、logger、路径和图池。

## 10. 通用异步节点

### 10.1 原理

```mermaid
flowchart LR
    A["Exec 复制输入"] --> B["Yield(exec 输出)"]
    B --> C["发起 RPC/Timer"]
    C --> D["返回 ErrExecutionSuspended"]
    D --> E["外部 callback"]
    E --> F["Resume 或 ResumeTo"]
    F --> G["ExecutionDispatcher"]
    G --> H["恢复原 VM 上下文"]
```

`Yield` 是 `Exec()` 的终止边界。恢复不会回到 Go 函数中 `Yield()` 后面的下一行，而是从蓝图中选定的 Exec 输出继续。

强制契约：

1. `Yield` 成功后立即返回 `-1, ErrExecutionSuspended`。
2. 未创建 Yield 时禁止返回 `ErrExecutionSuspended`。
3. `Yield(nextPort)` 的 `nextPort` 必须是 Exec 输出端口下标。
4. 一个 handle 只能成功恢复一次。
5. `Resume` 使用 Yield 时指定的出口；`ResumeTo` 可以选择另一个 Exec 出口。
6. 恢复参数从第一个数据输出端口开始，按输出端口顺序写入。
7. callback 不得访问 `BaseExecNode`、输入端口或输出端口。

`Resume/ResumeTo` 返回 nil 只表示恢复任务已被 Dispatcher 接受，不表示后续蓝图已经执行完成。恢复参数数量或类型错误、后续节点失败等错误会使 Execution 进入 Failed，宿主仍须通过 `Done + Result` 取得最终结果。

下列示例中的 `reportAsyncError`、`retryLater` 是宿主应用需要提供的错误上报与延迟重试函数，不属于 OriginBlueprint API。

### 10.2 定时器到期后恢复

```go
type TimerProvider interface {
    After(delay time.Duration, callback func()) (cancel func())
}

type WaitForTimerNode struct {
    bp.BaseExecNode
    timers TimerProvider
}

func (*WaitForTimerNode) GetName() string { return "WaitForTimer" }

func (n *WaitForTimerNode) Exec() (int, error) {
    delayMs, ok := n.GetInPortInt(1)
    if !ok || delayMs < 0 {
        return -1, fmt.Errorf("invalid delayMs %d", delayMs)
    }
    value, ok := n.GetInPortInt(2)
    if !ok {
        return -1, fmt.Errorf("value is not Integer")
    }

    // 从这里开始只把普通值和 handle 交给 callback。
    handle, err := n.Yield(0)
    if err != nil {
        return -1, err
    }
    n.timers.After(time.Duration(delayMs)*time.Millisecond, func() {
        if resumeErr := handle.Resume(value); resumeErr != nil {
            reportAsyncError(resumeErr)
        }
    })
    return -1, bp.ErrExecutionSuspended
}
```

对应输出建议为：`completed(exec, port_id=0)`、`value(Integer, port_id=1)`。`Resume(value)` 写入数据输出 1，然后从 Exec 输出 0 继续。

Core 取消 Execution 时不会自动取消业务 TimerProvider 的底层任务。若计时任务开销较大，宿主必须建立自己的 request/execution 到 timer cancel 的映射；即使未取消，timer 到期后的 `Resume` 也会返回取消或释放错误，不能忽略该错误。

### 10.3 RPC 成功和失败分支

节点输出顺序：

```text
0 succeeded    exec
1 failed       exec
2 value        Integer
3 errorCode    Integer
4 errorMessage String
```

```go
type RPCResponse struct {
    Value int64
}

type AsyncRPC interface {
    Call(requestID int64, callback func(RPCResponse, error))
}

type CallRPCNode struct {
    bp.BaseExecNode
    client AsyncRPC
}

func (*CallRPCNode) GetName() string { return "CallRPC" }

func (n *CallRPCNode) Exec() (int, error) {
    requestID, ok := n.GetInPortInt(1)
    if !ok {
        return -1, fmt.Errorf("requestID is not Integer")
    }
    handle, err := n.Yield(0)
    if err != nil {
        return -1, err
    }

    n.client.Call(requestID, func(response RPCResponse, callErr error) {
        if callErr != nil {
            // 三个参数依次写 value、errorCode、errorMessage。
            if err := handle.ResumeTo(1, int64(0), int64(503), callErr.Error()); err != nil {
                reportAsyncError(err)
            }
            return
        }
        if err := handle.ResumeTo(0, response.Value, int64(0), ""); err != nil {
            reportAsyncError(err)
        }
    })
    return -1, bp.ErrExecutionSuspended
}
```

不要在 callback 中调用 `n.SetOutPort*`。挂起 Context 由 VM 保留，只有 `Resume/ResumeTo` 可以安全写入恢复输出并继续执行。

### 10.4 Dispatcher 提交失败

`Resume/ResumeTo` 可能返回：Execution 已取消、实例已释放、Blueprint 已关闭、handle 已使用、目标端口非法或 Dispatcher 拒绝任务。Dispatcher 拒绝提交时 handle 会恢复为可重试状态；是否重试、退避或记录告警由宿主策略决定。其他错误通常不应重试。

```go
err := handle.Resume(value)
switch {
case err == nil:
case errors.Is(err, bp.ErrExecutionRejected):
    retryLater(func() {
        if retryErr := handle.Resume(value); retryErr != nil {
            reportAsyncError(retryErr)
        }
    })
case errors.Is(err, bp.ErrExecutionCanceled),
     errors.Is(err, bp.ErrGraphReleased),
     errors.Is(err, bp.ErrBlueprintClosed):
    // 生命周期已经结束，只清理外部资源。
default:
    reportAsyncError(err)
}
```

重试时必须保证同一时刻只有一个重试者持有该 handle。

## 11. 特殊宿主和恢复场景

### 11.1 通用事件循环/Actor 适配

`NewActorExecutionDispatcher` 的名字强调 Actor 场景，但参数只是通用 `enqueue func(func())`。任何要求线程归属的事件循环都可使用：

```go
engine.SetExecutionDispatcher(
    bp.NewActorExecutionDispatcher(host.Enqueue),
)
```

其语义是：

- `Start` 的初始执行在当前宿主线程立即开始。
- Yield 后的恢复统一通过 `host.Enqueue` 投递。
- Core 不知道 Actor、service 或 GUI 的具体类型。

事件循环中的完整调用方式：

```go
func (h *Host) RunBlueprint(graphID, entranceID int64, args ...any) error {
    execution, err := h.blueprint.Start(h.ctx, graphID, entranceID, args...)
    if err != nil {
        return err
    }
    go func() {
        <-execution.Done()
        h.Enqueue(func() {
            returns, runErr := execution.Result()
            h.OnBlueprintResult(returns, runErr)
        })
    }()
    return nil
}
```

等待 `Done()` 的 goroutine 只负责把最终处理投递回宿主，不直接修改宿主线程专属状态。

### 11.2 多个异步 Execution

每次 `Start` 都有独立 VM 和 Yield token，可以同时挂起。RPC callback 必须保存各自的 handle，禁止用“节点 ID -> 单个全局 handle”覆盖前一次请求。

### 11.3 取消和回调竞态

- `Cancel` 与 callback 可能并发；以 `Resume` 返回值为准。
- 取消后 callback 仍可能到达，必须容忍取消错误并释放外部资源。
- `ReleaseGraph` 和 `Close` 同样会使后续 Resume 失败。
- 自定义节点若持有共享 client/cache/map，需要自行同步；不要把它放到共享 `ExecNode`。

### 11.4 循环、Sequence 和函数

恢复继续使用原 VM，不重建流程上下文：

- Sequence：恢复分支完成后才进入后续兄弟分支。
- 循环：完成当前迭代余下流程，再进入下一迭代。
- break：恢复后命中 break 时结束循环，再执行 completed。
- 函数：恢复函数帧，函数返回后继续 caller。

## 12. 其他 Go 接口

### 12.1 手工 Registry 和 CompileGraph

不使用目录加载时，可手工构图：

```go
registry := bp.NewRegistry()
registry.Register(bp.NewNodeDefinition(
    "Entrance_IntParam",
    func() bp.IExecNode { return &bp.EntranceIntParam{} },
    nil,
    []bp.IPort{bp.NewPortExec()},
))
registry.Register(bp.NewNodeDefinition(
    "MyNode",
    func() bp.IExecNode { return &MyNode{} },
    []bp.IPort{bp.NewPortExec(), bp.NewPortInt()},
    []bp.IPort{bp.NewPortExec(), bp.NewPortInt()},
))

config := bp.GraphConfig{
    Nodes: []bp.NodeConfig{
        {ID: "entry", Class: "Entrance_IntParam_100"},
        {ID: "work", Class: "MyNode"},
    },
    Edges: []bp.EdgeConfig{
        {SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "work", DesPortID: 0},
    },
}
compiled, err := bp.CompileGraph(registry, config)
if err != nil {
    return err
}
engine.AddCompiledGraph("manual", compiled)
```

手工 Registry 使用者必须自行注册入口定义、系统节点和业务节点。一般业务优先使用 `Blueprint.Init`，避免遗漏文件格式、函数别名和冲突检查。

### 12.2 低层 Graph.Do

```go
returns, err := bp.NewGraph(compiled).Do(entranceID, args...)
```

它不建立完整 Execution 生命周期，只适合同步测试或低层工具。图中任何节点可能 Yield 时禁止使用；生产并发调用优先经过 `Blueprint.Start/Do/DoContext`。

### 12.3 Trace

```go
type TraceLogger struct{}

func (*TraceLogger) TraceBlueprintNode(event bp.BlueprintTraceEvent) {
    log.Printf("step=%d graph=%s node=%s err=%s",
        event.Step, event.GraphName, event.NodeName, event.Error)
}

engine.SetTraceLogger(&TraceLogger{})
engine.SetTraceEnabled(true)
```

Trace 默认关闭；开启后会复制端口值，有额外开销。生产环境只在诊断窗口开启，并注意日志中的业务数据和隐私。

### 12.4 IBlueprintModule 和旧 logger

`IBlueprintModule` 只有 `TriggerEvent`，用于业务事件桥接和让节点通过 `GetBlueprintModule()` 访问宿主。`IBlueprintLogger` 是兼容类型；若实现 `BlueprintTraceLogger`，传给 `Init` 后可配合 Trace 使用。

### 12.5 DiagnosticSink 与结构化错误

同步调用始终检查返回的 `error`。异步调用除等待 `Execution.Done()` 并读取 `Result()` 外，还可以配置可选的终态错误接收器：

```go
type DiagnosticSink struct{}

func (*DiagnosticSink) ReportBlueprintError(event bp.BlueprintError) {
    log.Printf("stage=%s graph=%s graphID=%d entranceID=%d executionID=%d node=%s pc=%d err=%v",
        event.Stage, event.GraphName, event.GraphID, event.EntranceID,
        event.ExecutionID, event.NodeID, event.PC, event.Cause)
}

engine.SetDiagnosticSink(&DiagnosticSink{})
```

`DiagnosticSink` 只接收 `ExecutionFailed`，正常完成、挂起和主动取消不会输出失败事件。未配置时不构造诊断日志；错误对象只在失败分支创建，不影响正常执行热路径。回调仅用于记录或上报，不要在回调中再次阻塞执行所属事件循环。

## 13. 使用约束与禁止用法

### 13.1 必须遵守

- JSON `name` 与 Go `GetName()` 保持一致。
- 新版文档的执行字段采用严格解析；编辑器元数据允许保留，但未知根字段、节点 `values` key、重复变量 ID、缺少入口或不完整函数图会在 `Init` 阶段报错。函数图中确定可达的无返回终止路径也会在编译阶段报错；复杂动态分支和循环无法确定时由运行时 `FunctionReturn` 保护兜底。为兼容历史资源，完全空的旧版 `.vgf` 占位图允许加载，但在补充入口前不能执行，调用会返回 `ErrEntranceNotFound`。
- 工厂每次返回新的节点对象。
- 输入读取和输出设置检查成功状态。
- 发布后保持入口 ID、函数 ID、端口编号、参数顺序和返回顺序稳定。
- Yield 成功后立即返回 `ErrExecutionSuspended`。
- callback 只捕获 handle 和复制后的普通值。
- 所有 `Resume/ResumeTo` 错误都要处理。
- 事件循环宿主使用能恢复线程归属的 Dispatcher。
- 实例不用时 `ReleaseGraph`，进程/模块释放时 `Close`。

### 13.2 禁止用法与替代

| 禁止用法 | 风险 | 正确替代 |
| --- | --- | --- |
| `Exec()` 内 `time.Sleep` | 阻塞 worker/事件循环 | Yield 后由外部 timer callback Resume |
| `Exec()` 内同步等待 RPC/channel | 延迟、死锁、吞吐下降 | 异步 callback + YieldHandle |
| callback 中访问 `BaseExecNode` 或端口 | Context 生命周期和并发错误 | Exec 内复制输入，只保留 handle |
| 一个 handle 恢复两次 | 后续流程重复或 `ErrYieldResumed` | 每个请求独立一次性 handle |
| `ResumeTo` 传数据时忽略输出顺序 | 数据写入错误端口 | Exec 输出在前，数据输出连续并逐项传参 |
| Actor/GUI 中 `Do` 等待异步图 | 恢复任务无法被事件循环处理 | `Start` 后让事件循环继续 |
| 任意 goroutine 直接修改宿主状态 | 数据竞争或线程归属破坏 | Dispatcher/Enqueue 回宿主 |
| 可能 Yield 的图使用 `Graph.Do` | 没有 Execution 生命周期 | `Blueprint.Start/DoContext` |
| 共享一个 `Graph` 并发执行 | VM/Context 状态冲突 | 通过并发安全的 `Blueprint` facade |
| 新业务使用旧 Delay/Timer 节点 | Core 明确不支持执行 | 自定义 Timer/RPC 异步节点 |
| 纯数据节点产生业务副作用 | 按需重算导致副作用重复 | 使用带 Exec 流的节点 |
| 改旧图端口编号只为调整显示 | 线上连线错位 | 保持 `port_id`，只改显示字段 |
| 未知旧节点直接删除 | 保存后线上信息丢失 | 保留 legacy hidden/fallback 信息 |

### 13.3 外部资源生命周期

Core 管理 VM 和 Execution，不自动管理业务 RPC、Timer、订阅或数据库请求。业务节点应明确：

- 如何取消底层请求。
- callback 晚到时如何清理。
- 图释放后是否仍持有大对象。
- Dispatcher 队列满时是否重试。
- 多次 callback 如何去重。

## 14. 进阶使用与性能

- 同一 `CompiledGraph` 可被多个实例和 Execution 共享，不要每次调用重新加载编译。
- 长期业务对象使用 `Create` 一次并复用 graphID；对象销毁时 `ReleaseGraph`。
- Trace 默认关闭。
- 不在节点热路径做反射、大量字符串 map 查找或重复 JSON 解析。
- 大数组和 Any 输出会被复制或保留到当前执行阶段，避免无界对象。
- 单次顶层执行共享 1,000,000 步预算；异步恢复不会重置预算。
- 函数调用深度受限制，禁止递归业务蓝图。
- 新节点端口总数、动态输出、函数签名都受 schema 上限约束，不要通过生成超大节点代替合理拆图。

推荐测试：

```powershell
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
```

修改节点 schema 或编辑器时：

```powershell
go test ./...
cd frontend
npm run test:layout
npm run build
```

## 15. 测试、排错和上线检查

### 15.1 自定义节点最小测试矩阵

- JSON 能加载，工厂名称匹配。
- 默认输入、数据连线和端口类型正确。
- 每个同步 Exec 出口均有覆盖。
- 纯节点在循环中重新求值正确且无副作用。
- 异步成功、失败、取消、重复 callback、Dispatcher 拒绝均有覆盖。
- 异步节点在 Sequence、循环和函数内恢复位置正确。
- race 测试通过。
- 蓝图返回值与独立 Go 参考逻辑对比。

### 15.2 常见问题

| 现象 | 优先检查 |
| --- | --- |
| 编辑器有节点，Init 报未注册 | JSON `name` 与 `GetName()`；是否在 Init 前 Register |
| 节点标题显示成英文 class | schema 是否加载；是否错误写入自定义 label |
| 连线打开后消失 | 端口 key/port_id 是否改变；fallback 端口是否完整 |
| Do 返回空数组 | 图是否经过 Append*Return；是否走到该执行分支 |
| Resume 后走错分支 | `ResumeTo` 的 Exec 输出下标 |
| Resume 数据错位 | 数据输出端口顺序和参数数量 |
| Actor 卡死 | 是否在 Actor 中阻塞调用可能挂起的 Do |
| 取消后仍收到 callback | 外部请求未取消；callback 应处理 Resume 取消错误 |
| 循环恢复后重复当前迭代 | 检查业务节点是否重复发 callback；VM 正常语义不会重跑当前节点 |
| 两次 Do 之间变量重置 | 这是普通变量的预期 Execution-local 语义；跨调用状态应放宿主或持久层 |
| 热加载后仍执行旧逻辑 | 已经开始/挂起的 Execution 固定使用旧版本；下一次 Start/Do 才使用新版本 |

### 15.3 上线前检查清单

- [ ] 新节点 JSON 和 Go 工厂均已部署。
- [ ] `name/GetName/port_id` 已冻结并记录。
- [ ] 入口参数和返回契约已与调用方核对。
- [ ] 未使用内置 Delay/Timer 兼容节点。
- [ ] 异步节点没有阻塞等待，没有捕获节点 Context。
- [ ] 成功、失败、超时、取消和晚到 callback 已测试。
- [ ] 事件循环/Actor 使用 `Start`，Dispatcher 能投递回正确线程。
- [ ] 循环和函数内异步恢复已测试。
- [ ] 旧 `.vgf` 未丢失未知节点、边和端口编号。
- [ ] Go test、race、前端测试和构建通过。
- [ ] 用边界值和固定 seed 随机输入与 Go 参考实现做过对比。

自动对比结果见 `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`。

## 16. API 和错误索引

### 16.1 主要 API

| API | 用途 |
| --- | --- |
| `RegisterExecNode` | 注册业务节点工厂，必须在 Init 前 |
| `Init` | 加载节点定义和蓝图目录 |
| `AddCompiledGraph` | 手工加入编译图 |
| `Create` | 创建图名身份和生命周期实例；普通变量在每次 Start/Do 初始化 |
| `Start` | 非阻塞启动，异步和事件循环首选 |
| `Do/DoContext` | 阻塞等待最终结果 |
| `SetExecutionDispatcher` | 指定执行和恢复调度环境 |
| `HotReload` | 重新加载、编译并安全替换图 |
| `GetGraphName` | 查询实例绑定的图名 |
| `ReleaseGraph` | 释放实例并取消其 Execution |
| `Close` | 关闭 Blueprint 和全部 Execution |
| `SetTraceLogger/SetTraceEnabled` | 配置执行 Trace |
| `SetDiagnosticSink` | 接收同步或异步 Execution 的结构化终态失败 |
| `Yield` | Native 节点挂起当前 VM |
| `Resume/ResumeTo` | 从指定 Exec 输出恢复 |

### 16.2 常见错误

| 错误 | 含义 |
| --- | --- |
| `ErrExecutionPending` | Result 调用时执行尚未结束 |
| `ErrExecutionCanceled` | Execution 被主动取消 |
| `ErrExecutionBudgetExceeded` | 步数或调用深度超过上限 |
| `ErrBlueprintClosed` | Blueprint 已关闭 |
| `ErrBlueprintInUse` | 存在实例或活动 Execution，禁止用 Init 重建运行时 |
| `ErrGraphNotFound` | graphID 不存在 |
| `ErrEntranceNotFound` | 入口 ID 不存在 |
| `ErrGraphReleased` | 图实例已释放 |
| `ErrExecutionRejected` | Dispatcher 拒绝任务 |
| `ErrExecutionSuspended` | Native 节点通知 VM 已成功 Yield；不是业务失败 |
| `ErrYieldInvalid` | Yield/Resume 协议、对象或目标端口非法 |
| `ErrYieldResumed` | 一次性 handle 已经恢复 |
| `ErrUnsupportedAsyncNode` | 执行了 Core 不支持的旧 Delay/Timer 节点 |

出现错误时优先保留图名、graphID、entranceID、节点 ID、节点名称、Execution ID 和错误链；不要只记录一条没有上下文的字符串。

`BlueprintError` 实现 `Unwrap()`，可以继续使用 `errors.Is/As` 判断 `ErrEntranceNotFound`、`ErrExecutionBudgetExceeded` 等根因。解析和编译错误会携带源文件路径；Native 节点、Sequence、循环、函数调用/返回、数据生产者和异步恢复 panic 的执行错误会尽量携带节点与 PC。`BlueprintTraceEvent` 同时提供 `ExecutionID`、`EntranceID`、`PC` 和 `Stage`，并包含 VM 控制节点事件。
