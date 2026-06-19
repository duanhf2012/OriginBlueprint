# 节点 JSON 格式

程序启动时会读取 `nodes` 目录，并递归加载其中所有 `.json` 文件。Windows 版从运行目录或可执行程序目录读取，网页版由 Vite 在开发和构建时提供同一份 `nodes` 静态资源。

两个版本都只负责提供原始 JSON 文档，节点格式解析、旧格式转换和模块库注册统一在前端完成，避免网页端和 Windows 端出现两套规则。

外部节点定义统一使用旧 OriginNodeEditor 格式。文件内容可以是节点数组、单个节点对象，或 `{ "nodes": [...] }`。

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
