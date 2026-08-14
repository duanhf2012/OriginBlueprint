# 普通蓝图文件名契约修复设计

## 背景

旧版 `.vgf` 文件不保存 `graphName`。Go engine 加载时以磁盘文件名去扩展名作为蓝图注册名，业务配置中的 `AutoBPFileName` 也使用该名称。

新版普通 `.obp` 文档当前会保存 `graphName`，engine 又优先按该字段注册。编辑器保存时使用带扩展名的标签标题生成 `graphName`，导致 `PetAuto_hama.obp` 被注册为 `PetCmd_hama.obp`，另外两个宠物蓝图被注册为带 `.obp` 后缀的名称。业务调用 `Create("PetAuto_hama")` 因查不到对应键而返回 0。

## 目标

- 普通蓝图 `.vgf` 和 `.obp` 统一以磁盘文件名去扩展名作为唯一运行时名称。
- 普通 `.obp` 持久化文件不包含 `graphName`。
- engine 忽略普通 `.obp` 中历史遗留的 `graphName`，保证旧错误文件也按文件名加载。
- `.obpf` 函数蓝图继续保留函数显示名、`functionId`、函数签名和现有调用别名，不改变函数调用契约。
- 不手改 mp1server 的 `vendor`，也不在没有可用新依赖版本时伪造依赖升级。

## 非目标

- 不改变旧 `.vgf` 的序列化格式。
- 不修改 `AutoBPFileName` 配置值。
- 不重命名蓝图文件。
- 不调整函数蓝图的显示名、分类、签名或引用解析。
- 不顺带修复与文件名契约无关的编辑器节点定义重复问题。

## 方案

### 1. Engine 加载规则

`parseGraphFile` 先根据扩展名区分普通蓝图与函数蓝图：

- `.obpf`：继续使用文档内函数名；为空时回退文件名去扩展名，并保留 `functionId` 与相对路径别名。
- `.obp`：无条件使用文件名去扩展名，忽略文档内 `graphName`。
- `.vgf`：维持现状，使用文件名去扩展名。

这样旧的错误 `.obp` 即使仍含 `graphName: "PetCmd_hama.obp"`，也会注册为 `PetAuto_hama`。

### 2. 编辑器持久化规则

编辑器内部可以继续携带临时 `graphName`，用于未保存标签和现有编辑状态；写盘前统一生成持久化文档：

- 普通蓝图删除顶层 `graphName` 后再序列化。
- 函数蓝图保留 `graphName`，其值是函数显示名而不是文件路径。
- 手动保存、另存为、全部保存、自动保存和恢复副本使用同一个持久化转换函数，防止不同入口重新写回字段。
- 打开不含 `graphName` 的普通 `.obp` 时，以文件名去扩展名作为内存中的显示名称，不把该值再次写入普通文件。

### 3. 现有配置文件清理

从以下普通蓝图删除顶层 `graphName`：

- `PetAuto_hama.obp`
- `PetAuto_xiaozhu.obp`
- `PetAuto_huangshulang.obp`

节点、连接、变量、视图和其他字段保持不变。

### 4. mp1server 验证路径

`TestVMCompilesAllBattleBlueprintFiles` 当前加载历史 `battle/json`，生产代码实际加载 `battle/nodes`。测试改为与生产路径一致的 `battle/nodes + battle/vgf`，用当前 vendored engine 验证删除 `graphName` 后三个普通 `.obp` 均能按文件名编译和注册。

OriginBlueprint engine 的行为修复保留在源仓库并增加单元测试。mp1server 本轮不直接编辑 `vendor`；后续发布 OriginBlueprint 新版本时再通过正常依赖与 vendor 生成链路同步。

## 数据流

1. 编辑器打开 `PetAuto_hama.obp`，从路径得到内存显示名 `PetAuto_hama`。
2. 编辑器写盘时省略普通文档的 `graphName`。
3. server engine 遍历 `PetAuto_hama.obp`，以文件名生成注册键 `PetAuto_hama`。
4. `BattlePet` 从配置读取 `AutoBPFileName = "PetAuto_hama"`。
5. `Blueprint.Create("PetAuto_hama")` 命中已编译蓝图并返回非零实例 ID。

## 测试设计

### OriginBlueprint Go engine

- 普通 `.obp` 内部没有 `graphName` 时，按文件名去扩展名注册。
- 普通 `.obp` 内部包含错误或带扩展名的 `graphName` 时，仍按文件名注册。
- `.obpf` 继续按函数名和现有别名规则加载。

### OriginBlueprint 前端

- 普通文档持久化结果不包含 `graphName`。
- 函数文档持久化结果保留函数 `graphName`。
- 打开缺少 `graphName` 的普通文档时，显示名来自文件名去扩展名。

### mp1server

- `TestVMCompilesAllBattleBlueprintFiles` 使用生产 `nodes` 目录并通过。
- 对照 `PetModelConfig.AutoBPFileName`，确认每个配置名称均能命中加载键。
- 按 battleservice 变更门禁执行目标包测试与最小编译验证。

## 兼容性与风险

- 普通 `.obp` 中自定义的、与文件名不同的 `graphName` 将不再生效，这是本次明确要求的行为变化。
- 文件重命名会同步改变运行时蓝图名称，与旧 `.vgf` 行为一致；调用方配置需要随文件重命名调整。
- `.obpf` 不采用普通蓝图规则，避免破坏函数显示名和函数调用引用。
- 当前 mp1server vendor 在普通 `.obp` 缺少 `graphName` 时已经回退文件名，因此清理数据即可修复当前运行时；源 engine 修改用于强制未来契约并兼容仍含错误字段的旧文件。

## 验收标准

- 三个宠物 `.obp` 文件均不含顶层 `graphName`。
- 编辑器重新保存普通 `.obp` 后不会恢复该字段。
- engine 对普通 `.obp` 始终以文件名去扩展名注册。
- `Create("PetAuto_hama")` 能命中 `PetAuto_hama.obp`。
- 函数蓝图加载与调用测试无回归。
- 前端测试、前端构建、OriginBlueprint Go 测试及 mp1server battle 蓝图编译测试通过。
