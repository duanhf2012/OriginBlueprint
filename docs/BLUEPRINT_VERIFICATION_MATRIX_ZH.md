# 蓝图与 Go 实现随机对比报告

本报告由 `TestWriteVerificationMatrixReport` 实际执行后生成，不是手工填写。每个蓝图使用独立 seed 产生 64 组不同随机输入，每组重复执行 3 次；测试会拒绝同一蓝图内的重复输入。蓝图返回值逐端口与独立 Go 参考实现比较。可通过 `ORIGIN_BLUEPRINT_VERIFICATION_SEED_OFFSET` 切换到一轮全新的输入，表中记录的是本轮实际 seed，可用于稳定复现。

- 蓝图文件：12
- 已有对应 Go 参考实现：12/12
- 随机参数组：768
- 实际重复对比执行：2496
- 通过蓝图：12/12
- 不一致蓝图：0

## 文件级结果

| 蓝图文件 | Go 参考实现 | seed | 随机参数组 | 每组重复 | 对比执行数 | 结果 |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| `01_legacy_all_nodes_showcase.vgf` | 有 | 2046332117 | 64 | 3 | 384 | 一致 |
| `02_control_flow_maze.obp` | 有 | 2046332118 | 64 | 3 | 192 | 一致 |
| `03_array_data_lab.obp` | 有 | 2046332119 | 64 | 3 | 192 | 一致 |
| `04_deterministic_algorithm.obp` | 有 | 2046332120 | 64 | 3 | 192 | 一致 |
| `05_function_orchestrator.obp` | 有 | 2046332121 | 64 | 3 | 192 | 一致 |
| `06_async_delay_resume.obp` | 有 | 2046332122 | 64 | 3 | 192 | 一致 |
| `07_async_rpc_resume_to.obp` | 有 | 2046332123 | 64 | 3 | 192 | 一致 |
| `functions/10_score_kernel.obpf` | 有 | 2046332126 | 64 | 3 | 192 | 一致 |
| `functions/11_array_fold_and_format.obpf` | 有 | 2046332127 | 64 | 3 | 192 | 一致 |
| `functions/12_nested_control_function.obpf` | 有 | 2046332128 | 64 | 3 | 192 | 一致 |
| `functions/13_local_state_isolation.obpf` | 有 | 2046332129 | 64 | 3 | 192 | 一致 |
| `functions/14_async_delay_function.obpf` | 有 | 2046332130 | 64 | 3 | 192 | 一致 |

说明：`01_legacy_all_nodes_showcase.vgf` 每组随机参数同时检查整数入口和数组入口，因此对比执行数是其他文件的两倍。异步 Delay 使用虚拟时钟，不依赖真实等待；异步 RPC 使用测试节点的 `Yield -> ResumeTo` 回包，均检查恢复后的最终返回值。

## 本轮检查结论

本轮未发现蓝图执行结果与 Go 参考实现不一致，无新增运行逻辑修正。

## 历史对比检查已修正

1. `03_array_data_lab.obp` 的 `StringSplit` 数据输出未经过执行流，读取时结果尚未生成；已补齐执行连线。
2. `07_async_rpc_resume_to.obp` 原有两个相同入口 ID，加载时存在覆盖风险；已改为单入口依次覆盖成功与失败恢复分支。
3. `functions/13_local_state_isolation.obpf` 返回端重新求值纯 Add，导致一次调用可能返回 `seed*2`；已改为返回本次 Set 后的值，恢复每次调用独立的局部状态语义。
4. `MockDelayAsync`/`MockRpcAsync` 是验证目录专用外部节点，编辑器无法从正式节点库找到时会丢失端口和连线；已在蓝图文档内携带仅用于显示的 fallback 端口定义。

## 失败定位方式

若结果出现不一致，Go 测试错误会输出 `asset`、`seed`、`case`、`repeat`、完整输入、蓝图输出及 Go 期望输出。使用同一 seed 可稳定复现。
