# 蓝图验证样本

这些文件是 OriginBlueprint 的人工可视化检查样本。它们只用于验证节点显示、端口、连线、分组、函数签名、变量和 legacy 导入形态；当前阶段不会修改引擎，也不会把这些样本作为自动化结果断言。

## 建议打开顺序

1. `01_legacy_all_nodes_showcase.vgf`：确认 legacy 节点、默认值和旧端口在新编辑器中的迁移显示。
2. `02_control_flow_maze.obp`：确认 Sequence、嵌套循环、动态分支、while、break 与任意数组循环的布局和连线。
3. `03_array_data_lab.obp`：确认数组控件、字符串控件、转换节点和局部变量节点。
4. `04_deterministic_algorithm.obp`：确认算术、浮点、比较、Branch、Range、Switch 和固定随机数的端口。
5. 打开 `functions/` 下四个 `.obpf`：确认函数入口/返回的参数名、类型和函数内变量显示。
6. `05_function_orchestrator.obp`：确认外部函数调用节点的输入输出端口，以及连续两次调用局部状态函数的可读性。
7. `06_timer_lifecycle.obp`：确认 Delay、按函数设置定时器、暂停、恢复、状态查询和清除节点的显示与完整连线。

## 关键预期

- 函数入口、函数返回和函数调用节点的参数端口名称必须完整显示。
- 所有动态 Sequence、Range 与 Switch 的已连接分支端口必须可见。
- 图中每个分组标题应完整可读，节点不应重叠遮挡端口。
- `13_local_state_isolation.obpf` 的变量属于函数局部状态；`05_function_orchestrator.obp` 连续调用它两次，是后续隔离验证的样本入口。
- `coverage.json` 记录全部当前系统节点的样本位置和阶段覆盖范围。

## 重新生成

在仓库根目录运行：

```powershell
.\scripts\generate-verification-blueprints.cmd
```

请勿在 Windows PowerShell 5 中直接执行 `.ps1`；它可能用系统代码页误读无 BOM 的 UTF-8 中文文本。

## 后续阶段

第 2 阶段会为每个可执行样本编写独立 Go 参考实现并比较输出。第 3 阶段会在安全输入范围内生成带种子的随机参数。第 4、5 阶段仅在发现差异后总结并修复问题。