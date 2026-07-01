# 节点 JSON 格式

程序启动时会读取 `nodes` 目录，并递归加载其中所有 `.json` 文件。Windows 版从运行目录或可执行程序目录读取，网页版由 Vite 在开发和构建时提供同一份 `nodes` 静态资源。

两个版本都只负责提供原始 JSON 文档，节点格式解析、旧格式转换和模块库注册统一在前端完成，避免网页端和 Windows 端出现两套规则。

外部节点定义统一使用 legacy 节点 JSON 格式。文件内容可以是节点数组、单个节点对象，或 `{ "nodes": [...] }`。

## 示例

```json
[
  {
    "name": "AddInt",
    "title": "加 (int)",
    "package": "运算",
    "description": "加 (int)",
    "is_pure": true,
    "inputs": [
      { "name": "A", "type": "data", "data_type": "Integer", "has_input": true, "port_id": 0 },
      { "name": "B", "type": "data", "data_type": "Integer", "has_input": true, "port_id": 1 }
    ],
    "outputs": [
      { "name": "结果", "type": "data", "data_type": "Integer", "port_id": 0 }
    ]
  }
]
```

## 字段说明

- `name`: 节点内部名称。已知旧节点会映射到当前执行器的 `origin.*` 类型；未知名称会注册为 `origin.custom.*` 自定义节点。
- `title`: 模块库和节点标题显示文本。
- `package`: 模块库分类。
- `description`: 节点副标题。
- `is_pure`: 保留旧格式字段。当前加载器以 `inputs` / `outputs` 中实际声明的端口为准。
- `inputs` / `outputs`: 输入和输出端口列表。
- `port_id`: 旧格式端口编号。已知节点会映射到当前字符串端口 key，未知节点会转换为 `in0` / `out0` 这类 key。
- `type`: `exec` 表示执行端口；其他数据端口写 `data`。
- `data_type`: 数据类型，支持 `Integer`、`Float`、`Boolean`、`String`、`Array`、`File`、`DataFrame`、`Dict`、`Any`。
- `has_input`: 输入端口是否显示默认值控件。
- `pin_widget`: 旧控件名，目前识别 `IntegerArrayWdg` 和 `StringArrayWdg`。

## 注意

只修改标题、分类和端口显示时，不需要重新编译程序；重新启动即可读取运行目录下的 JSON。新增未知 `name` 的节点可以编辑和连线，但执行器不会自动获得业务逻辑。需要执行的节点仍然要映射到已有 `origin.*` 类型，或后续在执行器中实现对应逻辑。

当前阶段新增结点先手写 JSON。后续会补可视化结点设计器，用于生成和维护新的结点定义；在设计器落地前，不要因为单个新结点临时引入另一套定义格式。

结点格式兼容的方向是：旧编辑器产出的结点定义和旧蓝图文件要能被新编辑器读取；新编辑器里的新结点定义不要求被旧编辑器读取。需要给现有蓝图运行解析器使用时，由保存/导出流程负责生成它能识别的旧 `.vgf/.obp` 格式。

## 新格式动态分支

如果结点需要通过“+ Item / -”同时增加左侧参数行和右侧执行出口，可以在新格式 JSON 中配置 `dynamicBranch`。旧格式结点仍保持原样；新结点可以使用下面这种结构：

```json
{
  "id": "origin.flow.equal-switch-new",
  "title": "等于分支== [新]",
  "category": "流程控制",
  "subtitle": "等于比较",
  "inputs": [
    { "key": "exec", "label": "", "type": "exec" },
    { "key": "value", "label": "值", "type": "data", "data_type": "Integer", "defaultValue": 0 },
    { "key": "cases", "label": "值", "type": "data", "data_type": "Array", "defaultValue": [], "arrayItemType": "number" }
  ],
  "outputs": [
    { "key": "otherwise", "label": "否则", "type": "exec" },
    { "key": "case0", "label": "", "type": "exec" },
    { "key": "case1", "label": "", "type": "exec" }
  ],
  "dynamicBranch": {
    "controlInput": "cases",
    "defaultOutput": "otherwise",
    "outputPrefix": "case",
    "outputStartIndex": 1,
    "maxBranches": 4,
    "hiddenOutputKeys": ["case0"]
  }
}
```

- `controlInput`: 左侧动态参数使用的数组输入端口 key。该输入必须是 `type: "array"`，并建议设置 `defaultValue: []`。
- `arrayItemType`: 动态参数输入框类型，`number` 显示数字输入，`string` 显示文本输入。
- `defaultOutput`: 不随 `+ Item` 增减的默认右侧执行出口。
- `outputPrefix` 和 `outputStartIndex`: 右侧动态执行出口的 key 生成规则。例如 `case` + `1` 会生成 `case1`、`case2`。
- `maxBranches`: 最大动态分支数量。右侧输出端口需要在 `outputs` 中预声明到这个数量。
- `hiddenOutputKeys`: 需要保留但不显示的兼容端口。旧端口编号有占位时使用，例如 `case0`。
`kind` 和 `custom` 都是可选字段。新写 JSON 时通常不需要填写：`kind` 会由端口和 `id` 自动推断，`custom` 未填写等价于 `false`。模块库分类只使用 `category`（旧格式使用 `package`）。
