# 定时器模拟 RPC 异步恢复示例设计

## 目标

提供一个可在编辑器中直接打开的蓝图示例，演示业务自定义节点如何将外部异步结果恢复到原蓝图 Execution，并根据结果选择成功或失败 Exec 输出。示例同时作为 Go engine 的回归测试和使用者阅读入口。

## 范围与边界

- 新增 demo 专用节点定义和 Go 节点实现，不新增根目录 `nodes/` 的系统节点。
- 节点文件只放在 `examples/verification-blueprints/nodes/`，由验证示例的加载/测试路径显式注册。
- 节点通过现有 `TimerScheduler` 模拟 RPC 的延迟回包，通过 `SuspendForResume()` 与 `Continuation.ResumeTo(...)` 恢复原 Execution。
- 不实现真实网络 RPC、超时、重试、协议序列化或新的 Timer 系统能力。
- 不修改 `.vgf` 导入导出、GraphDocument 结构、蓝图绘制或正式系统节点库。

## Demo 节点

节点名：`MockRpcAsync`；中文标题：`模拟 RPC 异步调用`；分类：`示例 / 异步`。

输入：

| 端口 | 类型 | 含义 |
| --- | --- | --- |
| Exec | Exec | 启动模拟调用 |
| DelayMs | Integer | 定时器回包延迟，必须非负 |
| Succeed | Boolean | `true` 走成功，`false` 走失败 |
| SuccessValue | Integer | 成功出口的数据 |
| FailureCode | Integer | 失败出口的错误码 |
| FailureMessage | String | 失败出口的错误信息 |

输出：

| 下标 | 端口 | 类型 | 含义 |
| --- | --- | --- | --- |
| 0 | Succeeded | Exec | 成功执行出口 |
| 1 | Failed | Exec | 失败执行出口 |
| 2 | Value | Integer | 成功值 |
| 3 | ErrorCode | Integer | 失败码 |
| 4 | ErrorMessage | String | 失败信息 |

执行流程：

1. 节点读取并复制所有输入值。
2. 调用 `SuspendForResume()`，然后向当前 Blueprint 的 `TimerScheduler` 注册一次性回调。
3. 节点返回 `ErrExecutionSuspended`，原 Execution 进入 suspended 状态。
4. 定时器到期后：
   - `Succeed=true` 时调用 `ResumeTo(0, SuccessValue, 0, "")`；
   - `Succeed=false` 时调用 `ResumeTo(1, 0, FailureCode, FailureMessage)`。
5. Execution 的 Dispatcher 执行后续节点。若实例释放、Execution 取消或出现重复回调，恢复调用返回错误；demo 节点仅记录/忽略该迟到结果，不能再次推进流程。

定时器回调不持有 TimerScheduler 锁执行蓝图，也不直接在 Scheduler goroutine 中执行后续蓝图；`ResumeTo` 复用现有 Execution Dispatcher 路径。

## 示例蓝图

新增 `examples/verification-blueprints/07_async_rpc_resume_to.obp`：

- 一个“成功调用”入口，连接 `MockRpcAsync(Succeed=true)`，成功出口连接 `追加返回结果(Int)`，失败出口也连接字符串返回，便于观察未命中分支不产生返回。
- 一个“失败调用”入口，连接 `MockRpcAsync(Succeed=false)`，失败出口连接 `追加返回结果(String)`，成功出口保留对应整型返回节点。
- 两个分组分别标明“定时器延迟模拟回包”和“ResumeTo 选择出口”。
- 默认延迟保持很小，仅用于视觉表达；自动化测试使用手动 TimerScheduler，不依赖真实时间。

## 测试与文档

- 在 `engine/go/blueprint` 中新增 demo 节点行为测试，使用手动 `TimerScheduler` 与手动 `ExecutionDispatcher` 验证：触发前 Execution 仍挂起、触发后只走指定成功或失败分支、输出数据正确、Execution 最终完成。
- 增加取消后的迟到 Timer 回调测试，确保不会继续蓝图。
- 增加示例蓝图结构测试：文件可读取、节点与连接完整，且 demo 节点 schema 可被加载。
- 更新 `examples/verification-blueprints/README.md` 和 `coverage.json`，将该示例标记为“异步业务节点接入范例”。

## 兼容与风险

- 因为节点仅位于示例目录，普通项目不会自动获得该节点；这是刻意的隔离，避免误将测试节点投入生产。
- 示例 `.obp` 使用当前原生文档格式，不作为 legacy `.vgf` 导出能力的承诺。
- 节点执行期状态仅保存在单次 Graph session 的 continuation 和 TimerScheduler 回调闭包中，不写入共享 `ExecNode` 或 `CompiledGraph`。
- 真实业务 RPC 节点应把 TimerScheduler 注册替换为 RPC 客户端回调，继续沿用 `SuspendForResume()` / `ResumeTo(...)` 的控制流和错误处理语义。
