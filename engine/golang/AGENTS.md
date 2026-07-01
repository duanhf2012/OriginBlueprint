# Go 蓝图引擎 Agent 规则

本目录包含面向服务器运行的 Go 蓝图解析与执行引擎。这里属于高风险运行时代码，修改时必须优先考虑兼容性、线程安全和性能。

## 必读上下文

修改本目录前，先阅读：

- `../../docs/CODEX_BLUEPRINT_ENGINE_RULES_ZH.md`
- `../../docs/BLUEPRINT_ENGINE_TEST_MATRIX_ZH.md`
- 如果涉及 legacy `.vgf` 行为，还要阅读 `../../docs/LEGACY_COMPATIBILITY_ZH.md`

## 硬性规则

- 不要把单次执行的可变状态放到 `CompiledGraph`、`ExecNode` 或 `NodeDefinition` 上；它们是共享只读运行时结构。
- `Graph` 是单次执行 session，不能并发复用。
- 服务器代码应通过 `Blueprint` 调用；`Blueprint` 是对外并发安全 facade。
- 异步 continuation 状态必须只属于被挂起的 `Graph` session。
- 如果节点可能 suspend，在 continuation 完成前，不能把相关执行对象归还池或复用。
- 保持 `.vgf` 兼容性。已删除或未知的 legacy 节点应隐藏或保留，不能静默丢弃。
- 顶层 `nodes/*.json` 是系统节点定义。除非用户明确要求，`nodes/json/**` 业务定义不在处理范围。
- 文件、表格、字典蓝图数据类型已按需求删除。未经用户明确同意，不要恢复。

## 性能规则

- 优先做编译期或加载期预处理，而不是执行期查找。
- `CompileGraph` 返回后，编译结构必须保持不可变。
- 热执行路径上，如果能使用 index 或预计算 binding，就不要使用 string-keyed map。
- 不要在共享节点上缓存 `ExecContext` 或 port 值。
- 对象池必须考虑异步 continuation 生命周期；被挂起的状态绝不能归还池。

## 验证命令

修改 engine 时，先跑窄测试，然后至少运行：

```powershell
go test ./engine/golang -count=1
go test -race ./engine/golang -count=1
```

修改 facade 或线程安全相关代码时，还要运行：

```powershell
go test -race ./... -count=1
```

修改性能敏感路径时，运行：

```powershell
go test ./engine/golang -run '^$' -bench 'BenchmarkBlueprintDo(Shared|Complex)|BenchmarkFunctionCall' -benchtime=3s -benchmem -count=1
```
