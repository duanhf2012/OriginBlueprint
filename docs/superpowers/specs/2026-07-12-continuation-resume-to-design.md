# Continuation 动态执行出口设计

## 目标

为异步业务节点提供运行时选择执行出口的能力。典型场景是业务层自定义 RPC 节点：请求成功后走 `Success` Exec，失败后走 `Failure` Exec，并把响应或错误数据写入节点的数据输出端口。

本次只扩展 Go 蓝图引擎的 Continuation API，不新增通用 RPC 节点、节点 JSON、RPC 客户端、超时策略或业务协议。

## 公共 API

新增动态挂起入口：

```go
func (n *BaseExecNode) SuspendForResume() (*Continuation, error)
```

新增动态恢复入口：

```go
func (c *Continuation) ResumeTo(nextIndex int, outPortArgs ...any) error
```

使用方式：

```go
continuation, err := node.SuspendForResume()
if err != nil {
    return -1, err
}

rpcClient.Call(request, func(response Response, callErr error) {
    if callErr != nil {
        _ = continuation.ResumeTo(failureExecIndex, callErr.Code, callErr.Message)
        return
    }
    _ = continuation.ResumeTo(successExecIndex, response)
})

return -1, blueprint.ErrExecutionSuspended
```

现有 `Suspend(nextIndex)`、`Continuation.Resume(...)` 和 `ResumeAsync(...)` 保持兼容，Delay 与现有异步函数调用不改变行为。

## 行为规则

1. `SuspendForResume` 创建动态出口 Continuation，不预先绑定后续 Exec。
2. `ResumeTo` 的 `nextIndex` 必须对应当前节点已声明的 Exec 输出端口；未连接的合法出口允许恢复并正常结束该分支。
3. 数据参数继续使用现有 `applyOutputArgs` 规则，按节点数据输出端口顺序写入，不因选择哪个 Exec 出口而改变。
4. 属于 `Execution` 的恢复必须通过该 Execution 的 Dispatcher，不能在 RPC 回调 goroutine 中直接执行后续蓝图。
5. 回调早于异步节点返回时，Execution 保存 Continuation、目标出口和输出参数；节点返回 `ErrExecutionSuspended` 后再提交恢复任务。
6. 每个 Continuation 只能成功预约一次恢复。第二次响应返回 `ErrContinuationResumed`，不得重复执行后续节点。
7. 图已释放、Execution 已取消或 Dispatcher 拒绝任务时，沿用现有明确错误和终态语义。
8. 固定出口 Continuation 调用 `ResumeTo` 返回明确的 API 使用错误，避免悄悄覆盖 `Suspend(nextIndex)` 的约定。

## 内部状态调整

Execution 当前保存早到恢复的 `pending` 和 `pendingArgs`。新增目标出口字段，使早到的 `ResumeTo` 在异步节点退出后仍能恢复到调用时选择的分支。

Continuation 增加“固定出口/动态出口”标记。固定出口仍使用创建时的 `nextIndex`；动态出口只接受 `ResumeTo`。

恢复实现统一收敛到带目标出口的内部方法，避免 `Resume` 与 `ResumeTo` 形成两套并发状态机。

## 错误边界

- 非法或非 Exec 的 `nextIndex`：恢复失败，但不消费 Continuation，调用方可以改用合法出口重试。
- 固定 Continuation 调用 `ResumeTo`：返回固定出口错误，不消费 Continuation。
- 动态 Continuation 调用普通 `Resume`：返回“必须指定出口”的错误，不消费 Continuation。
- 输出参数类型或数量错误：沿用现有端口赋值错误；Continuation 已经预约，不允许改写并二次恢复，避免部分写入后的重复执行。
- RPC 业务失败不是 Execution 失败，应由 `Failure` Exec 和错误数据输出表达。

## 测试范围

- 成功回调选择第一个 Exec，并传递响应数据。
- 失败回调选择第二个 Exec，并传递错误数据。
- 回调早于节点返回时仍通过 Dispatcher 选择正确分支。
- 非法出口、数据输出端口和固定/动态 API 混用返回明确错误。
- 重复回调只执行一次。
- Execution 取消、Graph 释放后不恢复。
- 现有 Delay、固定 Continuation、异步函数和 race 测试保持通过。

## 非目标

- `Continuation.Fail(error)`。
- RPC timeout、retry、熔断、请求 ID 或协议序列化。
- 对外暴露 Execution 取消钩子。
- 新增模块库 RPC 节点或前端 UI。

