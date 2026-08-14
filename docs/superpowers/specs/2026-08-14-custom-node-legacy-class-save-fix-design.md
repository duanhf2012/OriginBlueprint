# 自定义节点 `legacyClass` 保存修复设计

## 背景与结论

编辑器把 legacy runtime schema 的 `name` 转换为 `sourceName` 和 `origin.custom.*` `typeId`，但创建 Rete 节点时没有把 `sourceName` 写入 `BlueprintNode.legacyClass`。保存 `.obp/.obpf` 时，快照只序列化 `node.legacyClass`，导致 Go engine 无法把自定义 `typeId` 映射到已注册的 `IExecNode.GetName()`，并在 parse 阶段报告节点未注册。

Go engine 当前行为符合持久化契约：内置 `typeId` 使用静态映射，自定义节点必须携带 `legacyClass` 和 legacy 端口快照。修复应发生在编辑器，不让 engine 从不可逆的 slug 猜测业务类名。

## 方案比较

### 方案 A：只给新建节点写入 `legacyClass`

在 `fromSchema().create()` 中把自定义 schema 的 `sourceName` 写入节点。改动最小，但已有缺陷 `.obp/.obpf` 在恢复时，空的持久化属性会再次覆盖 schema 默认值，不能自动修复旧文件。

### 方案 B：Go engine 从 `typeId` 推断类名

把 `origin.custom.get-object-info` 反推为 `GetObjectInfo`。该转换无法可靠处理缩写、下划线、数字入口后缀和潜在 slug 冲突，也违背 README 中明确的 `legacyClass` 契约，不采用。

### 方案 C：编辑器写入运行时类名，并在恢复时保留 schema 默认值（推荐）

自定义 runtime schema 创建节点时设置 `legacyClass=sourceName`；恢复文档时，如果持久化内容没有 `legacyClass`，使用当前节点 schema 提供的默认类名。这样同时覆盖新建节点和已经由缺陷版本保存的文件，且不修改 engine 解析语义。

## 设计

### 节点身份映射

`nodeRegistry` 为自定义 runtime schema 暴露稳定的运行时类名查询：

- 仅 `schema.custom === true` 时使用 `schema.sourceName`；
- 已映射到 engine 内置 `typeId` 的 runtime schema 不额外写入 `legacyClass`；
- 新建自定义节点立即设置 `BlueprintNode.legacyClass`。

### 旧文件恢复

`createEditor.applyNodeProperties` 恢复节点属性时采用“持久化值优先、schema 默认值兜底”：

```text
persisted legacyClass ?? schema-derived legacyClass
```

已有缺陷文件虽然没有 `legacyClass`，但编辑器重新打开后可从当前 runtime schema 恢复类名；下一次保存会写出完整契约。显式存在的 `legacyClass` 仍保持原值，避免改变历史兼容节点身份。

### 不修改的范围

- 不修改 Go engine 的 `documentNodeToConfig` 和 Registry 行为；
- 不从 `typeId` 猜测 Go 类名；
- 不修改现有 `.obp/.obpf` 文件；用户打开并保存后由编辑器自然修复；
- 不改变 Go target 诊断不阻止核心保存的现有策略。

## 测试设计

采用 TDD 增加前端单元测试，先确认当前代码失败，再实现：

1. runtime JSON 自定义节点 `GetObjectInfo` 转换后，新建节点的 `legacyClass` 必须为 `GetObjectInfo`；
2. 已映射的内置节点不应被错误标记为自定义 legacy class；
3. 恢复时缺少持久化 `legacyClass`，应返回 schema 派生类名；
4. 恢复时显式持久化类名优先，保证兼容性。

验证范围：

- 新增的聚焦前端测试；
- `npm run test:layout`；
- `npm run build`；
- `go test ./...`；
- `go test ./engine/go/blueprint -count=1`。

## 验收标准

- 新建 `origin.custom.get-object-info` 后保存的文档包含 `"legacyClass": "GetObjectInfo"`；
- 当前缺陷文件重新打开并保存后，所有可由 runtime schema 识别的 `origin.custom.*` 节点均补齐对应 `legacyClass`；
- Go engine 能继续按现有契约将其转换为 `NodeConfig.Class`；
- 内置节点、显式 legacy placeholder 和已有 `legacyClass` 的文档行为不变。
