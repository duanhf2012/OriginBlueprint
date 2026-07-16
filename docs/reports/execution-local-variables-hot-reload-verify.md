# Execution-local 变量与热加载快照重构验证报告

## 结论

核心实现、竞态检查、验证蓝图随机差分、OriginBlueprint 全模块编译以及 mp1server 直接调用方均通过。性能基准未出现回退；本次样本的中位数整体更低。

mp1server 全仓 `go build ./...` 仍被与本轮无关的既有缺失符号阻断：`rpc.LoginType_TapTap`、`log.Logger`、`log.MaxSize`。

## 已验证场景

- 每次 `Do/Start` 从默认值创建独立变量槽位，同一 `graphID` 不共享普通变量。
- 同一 Execution 的 Yield/Resume 保留变量、PC、循环栈和函数栈。
- 同一 `graphID` 的并发 Start 使用不同变量对象。
- 热加载后旧挂起 Execution 完成旧版本，新 Start 使用新版本。
- 删除图后旧 Execution 可完成，新 Start 返回 `ErrGraphNotFound`。
- Init 运行中返回 `ErrBlueprintInUse`；失败不改变已发布配置；无实例时完整替换图池。
- 控制流错误携带 dispatch 时的 NodeID 和 PC。
- 函数无 Return、Return 不可达和确定的 `BoolIf` fallthrough 在编译期失败；复杂动态分支/循环不确定时由运行时保护兜底。
- 当前验证蓝图的边界值、固定 seed 随机输入和异步 Delay/RPC 模拟差分全部通过。

## 执行命令

```powershell
go test ./engine/go/blueprint -run 'TestBlueprintVariables|TestBlueprintConcurrentStarts|TestVMYieldResumePreservesExecutionLocalVariables|TestVMHotReload|TestBlueprintInit|TestControlFlowFailure|TestVMFunctionRejects' -count=20
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
go test ./... -count=1
go build ./...
go vet ./engine/go/blueprint
```

以上命令在 `E:/NewWork/OriginBlueprint/OriginBlueprint` 通过。

```powershell
go test ./common/blueprint -count=1
go test ./service/battleservice/battleobject -run '^$' -count=1
go vet ./common/blueprint
```

以上命令在 `E:/NewWork/branch_develop/mp1server` 通过。

## 性能对比

同一机器、每项 5 次、取中位数；单位为 ns/op。

| 基准 | 修改前 | 修改后 | 结果 |
| --- | ---: | ---: | ---: |
| SharedCompiledGraph | 4447 | 4238 | -4.7% |
| ComplexSharedCompiledGraph | 25550 | 20879 | -18.3% |
| ComplexActorDispatcher | 21336 | 18107 | -15.1% |
| ComplexSharedCompiledGraphParallel | 7885 | 3149 | -60.1% |
| FunctionCall | 4724 | 3147 | -33.4% |

基准会受 Windows 调度和机器负载影响，因此本轮只据此判定“没有可见性能回退”，不把单次提速百分比作为稳定承诺。FunctionCall 从 25 allocs/op、2120 B/op 降为 23 allocs/op、2048 B/op；其他四项分配数与基线一致。

## 未通过的全仓门禁

`E:/NewWork/branch_develop/mp1server` 执行 `go build ./...` 失败：

- `service/authservice/AuthService.go:84`: `rpc.LoginType_TapTap` 未定义。
- `middletier/sysmodule/performance/Performance.go`: `log.Logger`、`log.MaxSize` 未定义。

这些文件不依赖本轮变更接口，且 `common/blueprint` 与 `battleobject` 已单独编译通过。
