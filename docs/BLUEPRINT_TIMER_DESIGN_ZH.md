# 蓝图异步执行与定时器设计

> 状态：第一版已实现并进入验证
> 日期：2026-07-11
> 适用范围：`engine/go/blueprint/`、系统节点定义、前端节点展示与相关测试

## 1. 目标

为 OriginBlueprint 提供适合 Go 服务器使用的延迟与定时执行能力：

- `Delay` 可以挂起当前蓝图执行，时间到后从原位置继续。
- Timer 可以在未来一次或多次调用指定蓝图函数。
- 服务器可以使用非阻塞接口启动含延迟的蓝图。
- 大量等待任务不采用“一个 Delay/Timer 一个 goroutine”的实现。
- 执行、定时器和 Graph 实例具有明确且相互独立的生命周期。
- 删除尚未正式投入使用的旧 Timer 入口和旧 Timer 节点，不继续维护旧运行时语义。

## 2. 非目标

第一阶段不实现以下能力：

- `Set Timer by Event` 或自定义事件委托。
- Timer 事件公共入口。
- Timer Group 和批量取消节点。
- `Set Timer for Next Tick`、`Delay Until`、Cron 等扩展调度能力。
- 分布式定时器、跨进程持久化或进程重启恢复。
- 将 Timer 回调结果自动合并到创建 Timer 的原执行结果中。

## 3. 核心概念

### 3.1 Execution

`Execution` 表示从一次入口调用开始的独立蓝图执行会话。它持有本次执行的上下文、返回结果、挂起点和取消状态。

- 普通节点同步执行。
- 遇到 `Delay` 后，Execution 进入挂起状态。
- Delay 到期后恢复同一个 Execution。
- 到达返回节点后，Execution 进入完成状态。
- 同一个 Execution 只能完成或取消一次。

### 3.2 Delay

`Delay` 是当前 Execution 内部的潜在节点。它不会创建新的蓝图调用，也不会触发函数入口。

### 3.3 Timer

Timer 是属于 `GraphInstance` 的独立调度任务。Timer 到期时创建一次新的函数调用；循环 Timer 每次触发都创建新的调用。

### 3.4 TimerHandle

`TimerHandle` 是控制定时器的运行时句柄，不负责触发执行流。它用于清除、暂停、恢复和查询定时器。

- 默认值是无效句柄。
- 只能在所属 `GraphInstance` 中使用。
- Graph 实例释放后自动失效。
- 句柄运行时值不写入蓝图文件。

### 3.5 TimerScheduler

`TimerScheduler` 是 Delay 和 Timer 共用的调度器。调度器只管理时间和任务状态，到期后将恢复或回调任务提交给蓝图执行器，不在调度循环中长时间执行蓝图。

### 3.6 ExecutionDispatcher

`ExecutionDispatcher` 负责实际运行蓝图任务。`Start`、Delay 恢复和 Timer 回调都通过 Dispatcher 提交，避免在服务器调用 goroutine 或调度器 goroutine 中执行任意长度的蓝图流程。

```go
type ExecutionDispatcher interface {
    Submit(task func()) error
}
```

默认实现使用有界工作队列和固定 worker；宿主可以注入自己的执行器。初始执行提交失败时 `Start` 返回准入错误；Delay 恢复提交失败时必须可靠重试或把 Execution 明确置为失败，绝不能静默丢弃；Timer 回调提交失败时按一次性失败、循环合并的策略处理并记录事件。

## 4. 执行语义

### 4.1 Delay 执行过程

```text
执行到 Delay
  -> 保存 Continuation
  -> 向 TimerScheduler 注册一次性任务
  -> Execution 进入 Suspended
  -> 当前执行调用返回调度层
  -> 时间到后调度器恢复 Continuation
  -> 从 Completed 输出继续执行
  -> 到达返回节点后完成 Execution
```

Delay 属于原 Execution，因此 Delay 后产生的返回值仍然是原调用的结果。

### 4.2 Timer 执行过程

```text
执行到 Set Timer by Function
  -> 保存回调函数引用和参数快照
  -> 注册 Timer
  -> 输出 TimerHandle
  -> 从 Then 立即继续原执行

Timer 到期
  -> 校验 GraphInstance、TimerHandle 和函数引用
  -> 创建一次新的函数执行会话
  -> 从该函数的标准函数入口开始执行
```

Timer 回调与创建 Timer 的 Execution 相互独立。Timer 回调的函数返回值默认忽略，不作为原 `Do` 或 `Execution.Result()` 的结果。

### 4.3 循环 Timer

- `Looping=false`：触发一次后自动失效。
- `Looping=true`：按照间隔重复调用函数，直到清除、Graph 释放或发生不可恢复错误。
- 同一个循环 Timer 的回调默认禁止重入；上一轮仍未完成时，不并发启动下一轮。
- 第一版采用合并策略：上一轮仍在运行时跳过本次到期信号，不补发积压次数。

## 5. Go 对外接口

### 5.1 非阻塞接口

服务器侧优先使用 `Start`：

```go
execution, err := blueprint.Start(ctx, graphID, entranceID, args...)
```

建议的执行句柄：

```go
type Execution interface {
    ID() uint64
    Done() <-chan struct{}
    State() ExecutionState
    IsDone() bool
    Result() (PortArray, error)
    Cancel() bool
}
```

`Start` 是真正的非阻塞准入接口：

- 同步校验 graph、entrance、参数、context 和 Dispatcher 状态。
- 校验通过后将首次执行提交给 ExecutionDispatcher 并立即返回。
- `Start` 不在调用方 goroutine 中执行节点、循环或函数。
- 蓝图是否同步完成由 `Execution.Done/State/Result` 观察，不承诺返回时已经完成。
- 调用方可以保存 Execution、监听 `Done()` 或在业务调度器中轮询状态。

Execution 状态机：

```text
Pending -> Running -> Suspended -> Running -> Completed
    |          |          |           |
    +----------+----------+-----------+-> Canceled
    +----------+----------+-----------+-> Failed
```

- `Result()` 在终态前返回 `ErrExecutionPending`。
- 结果和错误必须先发布，再关闭 `Done()`。
- `Cancel()` 仅在从非终态成功进入 Canceled 时返回 `true`。
- Blueprint 只登记非终态 Execution；完成、失败或取消后立即移出活动表。
- 调用方持有的 Execution 句柄保留终态结果，不要求额外 Release。

### 5.2 阻塞便利接口

保留 `Do`，并新增支持取消的 `DoContext`：

```go
func (b *Blueprint) Do(
    graphID int64,
    entranceID int64,
    args ...any,
) (PortArray, error)

func (b *Blueprint) DoContext(
    ctx context.Context,
    graphID int64,
    entranceID int64,
    args ...any,
) (PortArray, error)
```

语义：

- `DoContext = Start + 等待 Execution.Done()`。
- `Do` 使用 `context.Background()` 调用 `DoContext`。
- 含 Delay 的 `Do` 会阻塞调用 goroutine，直到执行完成或失败。
- MMO、Actor、固定工作池和事件循环不应直接调用可能长期等待的 `Do`，应使用 `Start`。

`Blueprint.Close()` 负责拒绝新任务，并取消该 Blueprint 的全部实例、Execution、Delay 和 Timer。默认 Dispatcher、Scheduler 是进程共享资源，不随单个 Blueprint 关闭；宿主注入的 Dispatcher、Scheduler 同样由宿主管理生命周期。

## 6. 节点设计

### 6.1 删除旧节点

删除以下旧运行时节点和注册：

- `Timer事件入口` / `Entrance_Timer_000003` / `origin.event.timer`
- `创建定时器` / `CreateTimer` / `origin.timer.create`
- `关闭定时器` / `CloseTimer` / `origin.timer.close`

不新增 Timer 专用入口。Timer 回调通过现有函数入口执行。

### 6.2 Delay

显示名称：`延迟`
英文名称：`Delay`
建议分类：`流程控制 / 时间`

| 方向 | 端口 | 类型 | 说明 |
|---|---|---|---|
| 输入 | `Exec` | Exec | 开始延迟 |
| 输入 | `Duration` | Integer | 延迟毫秒数 |
| 输出 | `Completed` | Exec | 延迟完成后继续 |

规则：

- `Duration < 0` 返回参数错误。
- `Duration == 0` 在下一次调度机会恢复，不在当前调用栈递归执行。
- 毫秒转 `time.Duration` 前检查溢出。
- Execution 取消时注销尚未触发的 Delay。

当前内部 `Sleep` 实现应由该节点替代，不保留“一次 Sleep 一个 goroutine”的实现。

### 6.3 Set Timer by Function

显示名称：`按函数设置定时器`
英文名称：`Set Timer by Function`
建议分类：`流程控制 / 定时器`

| 方向 | 端口/控件 | 类型 | 说明 |
|---|---|---|---|
| 输入 | `Exec` | Exec | 注册定时器 |
| 输入 | `Time` | Integer | 触发间隔，单位毫秒 |
| 输入 | `Looping` | Boolean | 是否循环 |
| 输入 | `FirstDelay` | Integer | 首次延迟；`-1` 表示使用 `Time` |
| 输入 | `Function` | FunctionReference | 可搜索下拉选择回调函数 |
| 输入 | 动态参数 | 函数参数类型 | 根据函数签名生成 |
| 输出 | `Then` | Exec | 注册成功后立即继续 |
| 输出 | `TimerHandle` | TimerHandle | 定时器控制句柄 |

界面示意：

```text
┌──────────────────────────────────────┐
│ ◇ 按函数设置定时器                    │
├──────────────────────────────────────┤
│ ▶ Exec                         Then ▶ │
│ ○ 时间（毫秒）          [ 1000      ] │
│ □ 循环                  [ ✓         ] │
│ ○ 首次延迟（毫秒）      [ -1        ] │
│ ƒ 回调函数              [ OnTimer ▼ ] │
│ ○ 动态参数              [ ...       ] │
│                         TimerHandle ⬡ │
└──────────────────────────────────────┘
```

函数选择规则：

- 使用可搜索下拉框，不允许保存不存在的自由文本函数名。
- 按函数分类展示候选项。
- 持久化稳定函数 ID，显示函数名；函数重命名不应破坏引用。
- 函数不存在或签名不匹配时，节点进入明确错误状态。
- 切换函数后重新生成动态参数端口，并尽量保留名称和类型兼容的连线。
- 注册 Timer 时复制参数快照，外部后续修改不能改变已注册回调的参数。
- Integer、Float、Boolean、String 按值复制，Array 深复制；不能安全复制的 Any/宿主对象不能作为第一版 Timer 参数。
- 回调函数可以有输出，但输出会被明确忽略；编辑器应显示提示。
- 一次性 Timer 允许 `Time == 0`，并在下一次调度机会触发。
- 循环 Timer 要求 `Time > 0`；`FirstDelay == -1` 使用 `Time`，`FirstDelay < -1` 非法。
- 所有毫秒值转换前检查 `time.Duration` 溢出。

### 6.4 Timer 控制节点

建议全部放入 `流程控制 / 定时器` 分类。

| 节点 | 输入 | 输出 | 性质 |
|---|---|---|---|
| `Clear Timer` | Exec、TimerHandle、CancelRunningCallback | Then、Success | 执行节点 |
| `Pause Timer` | Exec、TimerHandle | Then、Success | 执行节点 |
| `Unpause Timer` | Exec、TimerHandle | Then、Success | 执行节点 |
| `Is Timer Active` | TimerHandle | Boolean | 纯节点 |
| `Is Timer Paused` | TimerHandle | Boolean | 纯节点 |
| `Is Timer Handle Valid` | TimerHandle | Boolean | 纯节点 |
| `Get Timer Remaining` | TimerHandle | Integer(ms) | 纯节点 |
| `Get Timer Elapsed` | TimerHandle | Integer(ms) | 纯节点 |

查询无效句柄时不 panic：布尔查询返回 `false`，时间查询返回 `-1`。Clear/Pause/Unpause 对无效句柄不终止执行流，继续 `Then`，并通过 `Success=false` 报告；Go API 可以返回更详细的错误。

`CancelRunningCallback=false` 只阻止未来触发；为 `true` 时还取消该 Timer 当前仍未完成的回调 Execution。已经完成的业务副作用不能回滚。

### 6.5 TimerHandle 端口类型

- 增加独立的 socket/port 类型和视觉颜色。
- 不能与 Integer、String 等端口连线。
- 不提供手工输入运行时 ID 的文本框。
- 默认值为 Invalid TimerHandle。
- GraphDocument 只保存端口默认状态和连线，不保存活跃定时器 ID。
- 句柄至少包含 Scheduler 身份、GraphInstance 身份、任务 ID 和代次，避免跨实例操作以及 ID 复用后的旧句柄误命中新任务。

## 7. 生命周期与取消

### 7.1 Execution.Cancel

取消本次 Execution：

- 注销当前挂起的 Delay。
- 使未恢复的 Continuation 失效。
- 阻止后续节点执行。
- Pending/Suspended Execution 立即以明确的取消错误完成。
- Running Execution 采用协作式取消：当前用户节点返回后停止后续流程，再关闭 `Done()`；引擎不会强制终止正在执行的 Go 函数。

### 7.2 Clear Timer

清除指定 Timer：

- 一次性和循环 Timer 均立即失效。
- 与到期并发发生时，通过原子状态或版本号保证回调至多开始一次。
- 已经开始执行的函数回调不由 `Clear Timer` 强制中断。
- `CancelRunningCallback=true` 时取消该 Timer 当前仍未完成的回调 Execution；默认 `false` 保持普通定时器语义。

### 7.3 Graph.Release

释放 Graph 实例时：

- 拒绝新的 Execution 和 Timer。
- 取消所有未完成 Execution 和 Delay。
- 清除该实例的全部 Timer。
- `ReleaseGraph` 本身快速返回；已在运行的节点退出后，对应 `Execution.Done()` 才关闭。
- 当前运行节点返回后不得继续进入后续节点；宿主长耗时节点应自行支持业务 context 或取消信号。
- 任何晚到的恢复或回调都返回 `ErrGraphReleased`，不能进入节点执行。

### 7.4 热重载

第一版采用编译快照策略：已开始的 Execution 固定使用启动时的 `CompiledGraph` 和 runtime state；新 Execution 使用新版本。Timer 保存稳定函数 ID，每次触发解析当前实例版本；函数不存在或签名不兼容时停止 Timer 并报告错误。仅当 GraphInstance 被显式释放时取消全部挂起 Execution 和 Timer，普通节点库刷新不能静默清空线上定时任务。

### 7.5 同实例并发规则

- 保留现有能力：不同 Execution 默认允许并发执行。
- 同一个循环 Timer 的回调不重入。
- 不同 Timer、外部 Start 和 Timer 回调之间可能并发。
- GraphInstance 变量继续保证单次读写的内存安全，但复合读改写不具备事务性。
- 后续可增加可选 Serialized 模式，通过每实例 mailbox 串行执行；第一版不改变现有并发默认值。

## 8. 调度器设计要求

第一版实际接口：

```go
type TimerScheduler interface {
    Schedule(delay time.Duration, callback func()) (ScheduledTaskHandle, error)
    Cancel(handle ScheduledTaskHandle) bool
}
```

Scheduler 只负责一次性到期任务。循环、暂停、恢复、活动状态、剩余时间、已用时间和 `TimerHandle` 归属于每个 `GraphInstance` 的 Timer 注册表；这样 Scheduler 保持通用且不依赖蓝图身份。

实现要求：

- 使用共享最小堆或时间轮，不为每个任务创建等待 goroutine。
- 调度器使用少量固定 goroutine 管理到期任务。
- 到期后将任务提交给可配置的执行器/工作池。
- 调度器回调中不持有全局锁执行蓝图。
- Scheduler 注册、取消和到期必须并发安全；Timer 注册表的暂停、恢复和查询由 `timerMu` 保护。
- TimerHandle 至少包含实例身份和递增代次，防止旧句柄误操作复用 ID。
- 一次性任务触发后及时从注册表删除。
- 回调不得长期持有 GraphInstance 的全局锁。
- 使用单调时钟计算延迟；Wall Clock 调整不能让定时器提前、倒退或重复触发。
- 第一版通过注入手动 Scheduler 控制单元测试时间；独立 Clock 接口保留为后续扩展。
- Pause 保存剩余时间；Resume 从剩余时间继续，之后恢复固定频率调度。
- 循环 Timer 按固定频率计算 deadline；错过的周期合并，不追赶补发。

任务交付规则：

- 首次 Start 提交被拒绝：`Start` 返回 `ErrExecutionRejected`，不创建活动 Execution。
- Delay 恢复提交被拒绝：短暂重试；Dispatcher 已关闭时把 Execution 置为 Failed，关闭 Done 并释放 continuation。
- 一次性 Timer 回调提交被拒绝：Timer 失效并记录失败事件。
- 循环 Timer 回调提交被拒绝或上一轮未完成：合并本次触发，不积压回调。
- Scheduler 和 Dispatcher 均不得在持有注册表锁时调用用户节点、日志实现或宿主回调。

默认可以先实现最小堆调度器；性能测试证明需要时再替换为时间轮，外部接口和节点语义保持不变。

## 9. 函数回调规则

- Timer 回调只能选择可执行的蓝图函数，不能选择入口图本身。
- 每次回调创建新的函数执行 session。
- 每次函数调用的局部变量都是新副本，互不污染。
- GraphInstance 变量仍然共享，并继续使用现有并发保护。
- 函数回调可以包含 Delay；此时该回调 Execution 独立挂起。
- 函数回调返回值默认忽略。
- 回调函数删除、签名失效或加载失败时记录明确错误；一次性 Timer 失效，循环 Timer 自动停止，避免持续报错。
- Timer 需要跟踪当前回调 Execution，以实现不重入和可选的 `CancelRunningCallback`。
- 根 Execution 的取消会传播到它同步调用的函数子 session；Timer 创建的是新的根 Execution，不受创建 Timer 的原 Execution 完成影响。

## 10. 文件格式和兼容边界

- 新节点继续通过 `GraphDocument` 持久化，不序列化 Rete 内部对象。
- 新节点需要稳定 `typeId`、端口 key、端口顺序和 Go/TypeScript 一致的类型定义。
- 旧 Timer 运行时不提供兼容执行路径，也不迁移成新 Timer 语义。
- 但是打开旧 `.vgf` 时仍遵守项目通用规则：未知或已删除节点进入 legacy 保留状态，不能静默丢弃节点和连线。
- 新 Timer 节点如果没有可靠的旧 parser 表达方式，不强行伪装导出为旧 Timer 节点；导出时应给出明确的不兼容诊断。
- 已打开的 `.vgf` 一旦加入 Delay、Timer 或 TimerHandle，保存必须强制“另存为”原生 `.obp`，不得用原生 JSON 静默覆盖原 `.vgf` 文件。

第一版固定 typeId：

```text
origin.flow.delay
origin.timer.set-by-function
origin.timer.clear
origin.timer.pause
origin.timer.unpause
origin.timer.is-active
origin.timer.is-paused
origin.timer.is-valid
origin.timer.remaining
origin.timer.elapsed
```

所有新增标题、描述、端口标签、错误提示和函数选择控件必须从一开始使用现有多语言机制，不在实现末尾补硬编码中文。

## 11. 错误语义

至少需要稳定区分：

- Execution 已取消。
- Graph 已释放。
- Duration/Time 参数非法。
- TimerHandle 无效或不属于当前 GraphInstance。
- 回调函数不存在。
- 回调函数签名与参数不匹配。
- Continuation 已恢复或已失效。
- 调度器已关闭或拒绝任务。
- Execution 尚未完成。
- Dispatcher 已关闭、队列已满或拒绝任务。

错误不能通过 `(nil, nil)` 表示。特别是当前 `Graph.Do` 遇到 `ErrExecutionSuspended` 后返回 `(nil, nil)` 的临时行为必须被 Execution 生命周期替代。

## 12. 分阶段实施计划

状态说明：`[x]` 表示第一版已经实现并有对应测试；`[ ]` 表示明确保留到后续迭代，不影响当前 API 和节点语义。

### 阶段 1：Execution 生命周期

- [x] 增加 Execution 状态机和执行 ID。
- [x] 实现 `Blueprint.Start`。
- [x] 定义可注入 ExecutionDispatcher，并保证 Start 不执行用户节点。
- [x] 实现 `Execution.Done/Result/Cancel/IsDone`。
- [x] 实现 `DoContext`，并让 `Do` 成为阻塞便利封装。
- [x] 保持每次调用的 Graph session 和局部变量独立。
- [x] 增加同步完成、异步完成、取消、释放和并发测试。
- [x] 增加活动 Execution 自动移除和 Blueprint.Close 清理测试。

### 阶段 2：共享调度器与 Delay

- [x] 定义 TimerScheduler 和内部任务模型。
- [x] 支持注入 Scheduler，并实现 Dispatcher 拒绝后的明确失败语义。
- [x] 实现进程共享最小堆调度器及独立调度器关闭流程。
- [x] 将用户可见的 `Sleep` 替换为正式 `Delay` 节点；仅保留内部加载别名。
- [x] 接通 Continuation、Execution 完成和结果传递。
- [x] 验证零延迟、负值、溢出、取消竞争和 Graph 释放。
- [ ] 增加独立 Clock 接口；当前测试通过注入手动 Scheduler 控制时间。

### 阶段 3：TimerHandle 与 Timer 节点

- [x] 增加 TimerHandle Go 端口类型和前端 socket 类型。
- [x] 实现 `Set Timer by Function`。
- [x] 实现清除、暂停、恢复和查询节点。
- [x] 接入稳定函数 ID、可搜索函数下拉框和动态参数端口。
- [x] 验证一次性、循环、暂停恢复、清除竞争及函数调用隔离基础。
- [x] 验证 CancelRunningCallback 和同句柄回调不重入行为。
- [ ] 增加可观测的 Dispatcher 过载计数与 Timer 失败事件；当前失败会停止对应 Timer。

### 阶段 4：删除旧 Timer 机制

- [x] 删除 `Timer事件入口`、`CreateTimer`、`CloseTimer` 的运行时实现。
- [x] 删除旧系统节点注册、前端 schema 和节点 JSON。
- [x] 删除不再成立的旧 Timer 测试和模块接口。
- [x] 保留 legacy 文件通用的未知节点/边数据保护。

### 阶段 5：UI、文档和验证蓝图

- [x] 完成节点中英文名称、描述和端口标签。
- [x] 完成函数搜索下拉、缺失函数验证和工作区“查找所有引用”。
- [x] 更新 Go API、兼容清单和引擎维护文档。
- [x] 重写 Timer 验证蓝图，覆盖 Delay、循环、暂停、恢复、查询和取消。
- [ ] 在验证任务第 2、3 步增加与等价 Go 代码的固定及随机输入结果对比测试。

## 13. 验收标准

功能验收：

- `Start` 不在调用 goroutine 中执行任何蓝图节点，并快速返回。
- `DoContext` 能等待 Delay 后的最终返回值，并可被 context 取消。
- 一万个并发 Delay 不创建一万个等待 goroutine。
- 一次性 Timer 精确触发一次，循环 Timer 不发生同句柄回调重入。
- `Clear/Pause/Unpause` 在并发竞争下行为稳定。
- Timer 回调函数局部变量在每次调用之间完全隔离。
- Graph 释放后没有延迟恢复、Timer 回调或状态泄漏。
- Blueprint.Close 后没有引擎创建的 per-Execution context watcher 或 per-Timer/per-Delay goroutine 泄漏；无法强制终止宿主自身永久阻塞的节点代码。
- Dispatcher 队列拒绝时不存在永久挂起且无法完成的 Execution。
- 旧 Timer 节点不能继续执行，但旧文件内容不会在导入导出时静默丢失。

测试与性能验收：

```powershell
go test ./engine/go/blueprint -count=1
go test -race ./engine/go/blueprint -count=1
go test -race ./... -count=1
go test ./...
```

前端至少运行：

```powershell
cd frontend
npm run build
npm run test:layout
```

新增 benchmark 至少覆盖：

- 大量同步 `Start`。
- 大量挂起和取消的 Delay。
- 大量一次性 Timer 注册、触发和清除。
- 大量循环 Timer 的调度开销。
- Graph 释放时批量清理任务的耗时和内存。

## 14. 最终设计结论

最终采用以下组合：

```text
Delay
+ Set Timer by Function
+ TimerHandle 和控制/查询节点
+ Execution/Start 非阻塞调用
+ Do/DoContext 阻塞便利调用
+ 共享 TimerScheduler
```

`Delay` 恢复原执行，Timer 调用独立函数；TimerHandle 只负责控制，不负责触发流程。该设计既保留普通调用的易用性，也允许服务器框架在不阻塞业务 goroutine 的情况下运行包含延迟的蓝图。
