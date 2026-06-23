# OriginNodeEditor 兼容说明

这份文档专门说明 `OriginBlueprint` 如何兼容旧版 `OriginNodeEditor` 导出的 `.vgf` 文件。修改导入、导出、节点定义、端口映射、运行时前，请先读这里。

## 兼容目标

线上已经存在旧编辑器导出的 `.vgf` 文件。新编辑器必须做到：

- 能打开旧 `.vgf`。
- 已知旧节点能显示为新节点，并能继续编辑。
- 旧端口默认值、连线、变量、分组尽量保留。
- 未知旧节点不能静默丢失。
- 导出给旧外部解析器使用时，输出应保持 legacy JSON 形状。

兼容目标不是让旧 `OriginNodeEditor` 读取所有新功能。方向是：新编辑器读旧文件，并在需要时导出旧系统能识别的文件。

## 旧 `.vgf` 形状

旧 `.vgf` 本质是 JSON，主要字段如下：

```json
{
  "graph_name": "Graph",
  "time": "...",
  "nodes": [
    {
      "id": "...",
      "class": "AddInt",
      "module": "tools.json_node_loader",
      "pos": [12, 34],
      "port_defaultv": { "0": 1, "1": 2 }
    }
  ],
  "edges": [
    {
      "source_node_id": "...",
      "source_port_index": 0,
      "source_port_id": 0,
      "des_node_id": "...",
      "des_port_index": 1,
      "des_port_id": 1
    }
  ],
  "groups": [],
  "variables": []
}
```

关键点：

- 节点类型由 `class` 表示。
- 端口连接依赖 `source_port_id/des_port_id` 或 port index。
- 输入默认值放在 `port_defaultv`，key 是旧端口编号。
- 老图没有新格式的 `schemaVersion`。

## 新 `GraphDocument` 中的保留方式

迁移后，新文档使用 `GraphDocument`：

```text
nodes[]            可见的新节点
connections[]      可见的新连线
groups[]           新分组
variables[]        新变量
legacy             legacy 保留区
```

`legacy` 字段用于 round-trip：

```text
legacy.format       通常是 "vgf"
legacy.time         旧时间字段
legacy.hiddenNodes  未知或无法安全映射的旧节点
legacy.hiddenEdges  与隐藏节点相关的旧边
legacy.groups       旧分组快照
legacy.variables    旧变量快照
```

如果一个旧节点无法安全显示，不要删除它。放进 `legacy.hiddenNodes`，导出时再恢复。

## 导入路径

核心代码在 `legacy.go`：

```text
MigrateLegacyGraph
  -> migrateLegacyGraph
  -> runtimeLegacyNodeSpecs
  -> legacyNodeSpecs/static mapping + nodes/**/*.json runtime mapping
  -> GraphDocument
```

导入步骤：

1. 尝试把文件解析成 `legacyGraph`。
2. 建立旧变量到新变量 ID 的映射。
3. 扫描边，记录每个节点实际用到的最大输入/输出端口。
4. 对每个旧节点查找映射：
   - 静态映射在 `legacyNodeSpecs`。
   - 运行时 JSON 节点通过 `nodes/**/*.json` 推导。
   - `Get_变量名`、`Set_变量名` 会映射成变量 getter/setter。
5. 如果端口超出已知 spec，认为不安全，隐藏保留。
6. 旧 `port_defaultv` 按端口 index 转换成新节点 `values`。
7. 旧边按端口 index 转换成新 `connections`。
8. 分组按节点位置估算新分组矩形。

## 导出路径

核心代码在 `legacy.go`：

```text
ExportLegacyGraph
  -> exportLegacyGraph
  -> legacyGraph JSON
```

导出步骤：

1. 通过 `runtimeLegacyNodeSpecs` 建立 typeId 到旧 class 的反向映射。
2. 变量 getter/setter 导出为 `Get_变量名` / `Set_变量名`。
3. 部分新节点显式导出为旧 class：
   - `origin.flow.equal-switch-new` -> `EqualSwitch`
   - `origin.array.create-integer-new` -> `CreateIntArray`
   - `origin.array.create-string-new` -> `CreateStringArray`
4. 节点 `values` 按新端口 key 转回旧 `port_defaultv` index。
5. 新连接按端口 key 转回旧端口编号。
6. `legacy.hiddenNodes` 和可恢复的 `legacy.hiddenEdges` 会重新写回。

## 前端显示路径

打开文件后，前端流程在 `App.vue`：

```text
openGraph
  -> JSON.parse
  -> schemaVersion == 1 ? normalizeDocument : platform.migrateLegacyGraph
  -> editor.loadDocument
```

编辑器恢复在 `frontend/src/editor/createEditor.ts`：

```text
loadDocument
  -> restore
  -> createVariableNode / createLegacyNode / createNode
  -> addConnection
```

节点定义注册在：

```text
frontend/src/platform.ts
  -> loadNodeSchemas
  -> parseNodeSchemaDocument
  -> registerNodeSchemas
```

旧节点 JSON 转换在 `frontend/src/editor/runtimeNodeSchemas.ts`。这里的 `legacyNodeSpecs` 应尽量与 Go 的 `legacy.go` 保持一致，否则会出现 Go 能迁移但前端节点库显示不一致的问题。

## 已知节点映射位置

需要同时关注这些地方：

- Go 导入/导出映射：`legacy.go` 中的 `legacyNodeSpecs`。
- Go 校验端口类型：`graph.go` 中的 `graphNodePorts`。
- 前端旧 JSON 转换：`frontend/src/editor/runtimeNodeSchemas.ts` 中的 `legacyNodeSpecs`。
- 节点库定义：`nodes/json/**/*.json`。
- 前端节点工厂：`frontend/src/editor/nodeRegistry.ts`。
- 运行时语义：`execution.go`。

如果只改其中一处，常见结果是：

- 节点能显示但校验失败。
- 节点能校验但不能执行。
- 新节点能用但导出旧 `.vgf` 丢失。
- 旧 `.vgf` 能导入但前端节点库搜索不到。

## `.vgf` 与 `.obp` 注意点

当前保存逻辑有兼容历史：

- 前端 `App.vue` 中，原路径是 `.vgf` 时会主动调用 `ExportLegacyGraph`。
- 后端 `app.go` 中，`graphContentForPath` 对 `.vgf` 和 `.obp` 都可能导出 legacy JSON。
- 默认保存名曾偏向 `.vgf`，这是为了旧外部解析器兼容。

因此，改 `.obp` 语义要非常小心。如果希望 `.obp` 成为纯新格式，建议单独做一次设计，至少覆盖：

- `Save As` 默认扩展名。
- `.vgf` 是否只作为导出旧格式。
- `.obp` 是否只保存 `GraphDocument`。
- 最近文件和工作区过滤。
- 旧外部解析器如何获得 legacy 文件。
- 对现有 `.obp` 文件的兼容策略。

## 兼容测试建议

改兼容逻辑时优先补 Go 测试。已有测试集中在 `app_test.go`：

- 旧图迁移成新文档。
- 未知节点隐藏但 round-trip 保留。
- legacy 导出不包含 `schemaVersion/typeId`。
- 变量 getter/setter 导出为旧 class。
- 动态分支节点端口 round-trip。
- 文件/表格/字典节点迁移和执行。

建议测试模式：

```text
旧 .vgf JSON
  -> migrateLegacyGraph
  -> validateGraph
  -> exportLegacyGraph
  -> json.Unmarshal legacyGraph
  -> 断言节点数、边数、class、端口编号、默认值
```

如果有线上样本，优先把脱敏后的 `.vgf` 放进测试样本目录，再写批量迁移测试。

## 新增兼容节点 Checklist

新增或修改一个需要兼容旧图的节点时，按这个顺序检查：

1. 旧节点 class 是什么？
2. 旧节点 module 是否需要保留？
3. 输入端口旧编号到新 key 的映射是什么？
4. 输出端口旧编号到新 key 的映射是什么？
5. `port_defaultv` 应进入哪些 `values` key？
6. 节点是否需要执行？如果需要，`execution.go` 是否支持？
7. `graph.go` 是否知道该节点的端口类型？
8. `runtimeNodeSchemas.ts` 是否能把旧 JSON 转成同样的 typeId/key？
9. 导出时是否要转回旧 class？
10. 是否有迁移、导出、校验、执行测试？

## 不要做的事

- 不要为了显示方便改旧端口编号。
- 不要把未知旧节点直接丢掉。
- 不要把 legacy 保留区当作可随意清理的缓存。
- 不要只改前端节点 JSON，而不改 Go 校验/导出/运行时。
- 不要在保存时输出 Rete 内部结构。
- 不要假设中文显示乱码就代表 JSON 已损坏。

## 快速定位问题

旧 `.vgf` 打不开：

- 看 `App.vue openGraph` 是否走到 `MigrateLegacyGraph`。
- 看 `legacy.go` 的 JSON struct 是否匹配输入。
- 加测试直接调用 `migrateLegacyGraph`。

旧节点不显示：

- 查 `legacy.go legacyNodeSpecs` 是否有 class。
- 查 `runtimeLegacyNodeSpecs` 是否从 `nodes/**/*.json` 推导到了 class。
- 检查实际边使用的端口是否超过 spec，超过会隐藏。

连线丢失：

- 检查 `source_port_id/des_port_id` 是否是数字或可转数字。
- 检查 `indexedKey` 和 spec 端口顺序。
- 检查节点是否被隐藏，相关边可能进入 `legacy.hiddenEdges`。

导出后旧解析器不认：

- 检查导出 class 是否为空。
- 检查 module 是否是旧系统期望值。
- 检查 `port_defaultv` key 是否为旧端口编号。
- 检查输出 JSON 是否意外包含 `schemaVersion` 或 `typeId`。
