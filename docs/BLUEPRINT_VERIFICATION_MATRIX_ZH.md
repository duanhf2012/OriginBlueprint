# 蓝图执行对比矩阵

本报告由 Go 测试实际执行生成。每行均已断言蓝图输出与独立 Go 参考逻辑一致；输入采用可复现的零值、正负值和分支边界值。`01_legacy_all_nodes_showcase.vgf` 仅用于 legacy 导入与显示验证，不包含可执行结果契约。

## 02_control_flow_maze.obp

入口的三个整数端口当前未接入执行流，因此十组输入用于确认结果不受未使用入口值污染。

| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对象ID=0, 参数1=0, 参数2=0 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 2 | 对象ID=1, 参数1=1, 参数2=1 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 3 | 对象ID=1, 参数1=10, 参数2=5 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 4 | 对象ID=2, 参数1=-1, 参数2=1 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 5 | 对象ID=7, 参数1=-10, 参数2=-5 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 6 | 对象ID=42, 参数1=2, 参数2=3 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 7 | 对象ID=99, 参数1=11, 参数2=12 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 8 | 对象ID=100, 参数1=100, 参数2=-100 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 9 | 对象ID=-1, 参数1=4, 参数2=8 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |
| 10 | 对象ID=214, 参数1=-50, 参数2=50 | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | [2, 4, 6, 3, 5, 7, 4, 6, 8, "range-branch-true"] | 是 | 无 |

## 03_array_data_lab.obp

入口对象 ID 与数组端口当前未接入执行流，因此十组输入用于确认固定数组与局部变量流程稳定。

| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对象ID=0, 数组=[] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 2 | 对象ID=1, 数组=[1] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 3 | 对象ID=1, 数组=[-1, 2] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 4 | 对象ID=2, 数组=[3, 1, 4] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 5 | 对象ID=7, 数组=[9, 8, 7, 6] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 6 | 对象ID=42, 数组=[] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 7 | 对象ID=99, 数组=[1] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 8 | 对象ID=100, 数组=[-1, 2] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 9 | 对象ID=-1, 数组=[3, 1, 4] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |
| 10 | 对象ID=214, 数组=[9, 8, 7, 6] | [4, 6, 4, "green", "violet"] | [4, 6, 4, "green", "violet"] | 是 | 无 |

## 04_deterministic_algorithm.obp

参数 1、参数 2 参与整数评分、除法、取模和分支。

| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对象ID=0, 参数1=0, 参数2=0 | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 2 | 对象ID=1, 参数1=1, 参数2=1 | [0, 0, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [0, 0, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 3 | 对象ID=1, 参数1=10, 参数2=5 | [8, 1, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [8, 1, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 4 | 对象ID=2, 参数1=-1, 参数2=1 | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 5 | 对象ID=7, 参数1=-10, 参数2=-5 | [-12, -5, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [-12, -5, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 6 | 对象ID=42, 参数1=2, 参数2=3 | [1, 1, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [1, 1, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 7 | 对象ID=99, 参数1=11, 参数2=12 | [13, 6, 42, "score-high", "range-case-3", "switch-case-2", "5"] | [13, 6, 42, "score-high", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 8 | 对象ID=100, 参数1=100, 参数2=-100 | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 9 | 对象ID=-1, 参数1=4, 参数2=8 | [6, 6, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [6, 6, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |
| 10 | 对象ID=214, 参数1=-50, 参数2=50 | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | [-2, -2, 42, "score-low", "range-case-3", "switch-case-2", "5"] | 是 | 无 |

## 05_function_orchestrator.obp

主图入口当前未接入后续函数调用；每行同时列出四个内部函数调用的实际参数和返回值。

| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对象ID=0, 参数1=0, 参数2=0 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 2 | 对象ID=1, 参数1=1, 参数2=1 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 3 | 对象ID=1, 参数1=10, 参数2=5 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 4 | 对象ID=2, 参数1=-1, 参数2=1 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 5 | 对象ID=7, 参数1=-10, 参数2=-5 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 6 | 对象ID=42, 参数1=2, 参数2=3 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 7 | 对象ID=99, 参数1=11, 参数2=12 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 8 | 对象ID=100, 参数1=100, 参数2=-100 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 9 | 对象ID=-1, 参数1=4, 参数2=8 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |
| 10 | 对象ID=214, 参数1=-50, 参数2=50 | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | [30, "gold", 28, "16", 9, "nested-control:complete", 7, 7] | 是 | 评分核心(10, 5, 2) => [30, "gold"]<br>数组折叠与格式化([3, 1, 4, 1, 5], 2) => [28, "16"]<br>嵌套控制流(0, 4) => [9, "nested-control:complete"]<br>局部状态隔离(7) => [7]<br>局部状态隔离(7) => [7] |

## 06_timer_lifecycle.obp

入口参数当前未接入定时器生命周期；每行执行创建、暂停、恢复、查询和清理，并列出定时器回调函数参数。

| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对象ID=0, 参数1=0, 参数2=0 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 2 | 对象ID=1, 参数1=1, 参数2=1 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 3 | 对象ID=1, 参数1=10, 参数2=5 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 4 | 对象ID=2, 参数1=-1, 参数2=1 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 5 | 对象ID=7, 参数1=-10, 参数2=-5 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 6 | 对象ID=42, 参数1=2, 参数2=3 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 7 | 对象ID=99, 参数1=11, 参数2=12 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 8 | 对象ID=100, 参数1=100, 参数2=-100 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 9 | 对象ID=-1, 参数1=4, 参数2=8 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |
| 10 | 对象ID=214, 参数1=-50, 参数2=50 | ["timer-lifecycle-complete"] | ["timer-lifecycle-complete"] | 是 | Set Timer by Function: 局部状态隔离(种子=11) => [11]（循环回调，主图返回不包含回调结果） |

## 07_async_rpc_resume_to.obp

入口参数当前未接入示例 RPC；每行依次执行成功与失败两次异步恢复。

| 组 | 入口输入 | 蓝图输出 | Go 参考输出 | 一致 | 内部函数调用（输入 => 返回） |
| --- | --- | --- | --- | --- | --- |
| 1 | 对象ID=0, 参数1=0, 参数2=0 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 2 | 对象ID=1, 参数1=1, 参数2=1 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 3 | 对象ID=1, 参数1=10, 参数2=5 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 4 | 对象ID=2, 参数1=-1, 参数2=1 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 5 | 对象ID=7, 参数1=-10, 参数2=-5 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 6 | 对象ID=42, 参数1=2, 参数2=3 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 7 | 对象ID=99, 参数1=11, 参数2=12 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 8 | 对象ID=100, 参数1=100, 参数2=-100 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 9 | 对象ID=-1, 参数1=4, 参数2=8 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
| 10 | 对象ID=214, 参数1=-50, 参数2=50 | [314, 503, "mock rpc unavailable"] | [314, 503, "mock rpc unavailable"] | 是 | MockRpcAsync(80ms, true, 314, 0, "") => 成功[value=314]<br>MockRpcAsync(80ms, false, 0, 503, "mock rpc unavailable") => 失败[code=503, message="mock rpc unavailable"] |
