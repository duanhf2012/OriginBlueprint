# 蓝图验证样本

这些文件既用于 OriginBlueprint 的人工可视化检查，也由 Go 自动化测试加载执行。测试会将每个蓝图的实际返回值与独立 Go 参考实现比较。

## 建议打开顺序

1. `01_legacy_all_nodes_showcase.vgf`：确认 legacy 节点、默认值和旧端口在新编辑器中的迁移显示。
2. `02_control_flow_maze.obp`：确认 Sequence、嵌套循环、动态分支、while、break 与任意数组循环的布局和连线。
3. `03_array_data_lab.obp`：确认数组控件、字符串控件、转换节点和局部变量节点。
4. `04_deterministic_algorithm.obp`：确认算术、浮点、比较、Branch、Range、Switch 和固定随机数的端口。
5. 打开 `functions/` 下五个 `.obpf`：确认函数入口/返回的参数名、类型、函数内变量和异步恢复端口显示。
6. `05_function_orchestrator.obp`：确认外部函数调用节点的输入输出端口，以及连续两次调用局部状态函数的可读性。
7. `06_async_delay_resume.obp`：确认所有循环内挂起恢复和函数内挂起的显示与完整连线；再打开 `functions/14_async_delay_function.obpf` 确认可独立传入延迟、整数和标记。
8. `07_async_rpc_resume_to.obp`：确认单一入口依次执行成功、失败两次异步回包，并展示两个 ResumeTo 出口的连线。

## 关键预期

- 函数入口、函数返回和函数调用节点的参数端口名称必须完整显示。
- 所有动态 Sequence、Range 与 Switch 的已连接分支端口必须可见。
- 图中每个分组标题应完整可读，节点不应重叠遮挡端口。
- `13_local_state_isolation.obpf` 的变量属于函数局部状态；`05_function_orchestrator.obp` 连续调用它两次，是后续隔离验证的样本入口。
- `coverage.json` 记录全部当前系统节点的样本位置和阶段覆盖范围。
- `nodes/MockDelayAsync.json` 和 `nodes/MockRpcAsync.json` 是本目录专用测试节点定义，不属于正式系统节点库；其 Go 实现和结果断言位于 `engine/go/blueprint` 的验证测试中。
- `MockDelayAsync` 只表达业务异步节点的 `Yield -> Resume` 语义，不重新引入正式 `Delay`、`Timer` 或 `TimerHandle` 节点。
- `MockDelayAsync` 和 `MockRpcAsync` 节点同时在文档属性中携带测试专用 fallback 端口；这是因为编辑器只扫描根目录 `nodes/`，fallback 仅用于让示例目录中的外部节点和连线可视化，不会将这些节点加入正式模块库。

## 第 2 阶段结果契约

- `01_legacy_all_nodes_showcase.vgf`：只验证 legacy 导入、端口迁移和显示；不作为结果对比图。
- `02_control_flow_maze.obp`：验证嵌套循环、真实 break、Range、Branch、Probability、While 和任意数组遍历。固定输入下会返回循环整数、各分支标记和数组转换字符串。
- `03_array_data_lab.obp`：固定数组应依次返回整数 `4`、长度 `6`、字符串 `green` 和局部变量字符串 `north`。
- `04_deterministic_algorithm.obp`：输入参数决定整数评分分支；固定随机数恒为 `42`，Range/Switch 与浮点转换返回固定文本。后续随机输入阶段会以同一入口参数调用 Go 参考实现。
- `05_function_orchestrator.obp`：验证评分函数、加权数组折叠、嵌套控制函数和两次独立局部状态函数调用的全部输出。
- `06_async_delay_resume.obp`：唯一入口依次验证嵌套 For/ForeachIntArray、ForeachArray、ForLoopWithBreak、While 和函数调用内部的挂起恢复；每次恢复只能继续当前迭代余下语句，随后进入下一迭代。
- `functions/14_async_delay_function.obpf`：Go 测试会对该函数图启动多个独立 Execution，分别传入 10ms、30ms 和 5000ms；验证截止时间顺序，并验证取消 5000ms Execution 后不会恢复。截止时间和取消属于 Execution 调度测试，不应伪装成同一图中的多个同 ID 入口。
- `07_async_rpc_resume_to.obp`：单一入口先从成功分支返回 `314`，再从失败分支返回错误码 `503` 和文本 `mock rpc unavailable`。
- `functions/10_score_kernel.obpf`、`functions/11_array_fold_and_format.obpf`、`functions/12_nested_control_function.obpf`、`functions/13_local_state_isolation.obpf`、`functions/14_async_delay_function.obpf` 分别验证评分、加权累计、真实 break、函数局部变量隔离和函数帧异步恢复。

## 重新生成

在仓库根目录运行：

```powershell
.\scripts\generate-verification-blueprints.cmd
```

请勿在 Windows PowerShell 5 中直接执行 `.ps1`；它可能用系统代码页误读无 BOM 的 UTF-8 中文文本。

## 自动化对比

每个蓝图均已有独立 Go 参考实现。随机对比使用每个文件独立的固定 seed 和 64 组不重复输入，每组重复 3 次；测试会主动拒绝重复输入。异步 Delay 使用虚拟时钟，测试不依赖真实等待。设置 `WRITE_BLUEPRINT_VERIFICATION_REPORT=1` 执行报告测试可更新 `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`。