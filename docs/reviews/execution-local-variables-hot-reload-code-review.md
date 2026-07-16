# Execution-local 变量与热加载快照代码审查

## 结论

通过。未发现阻塞性问题；实现与已批准设计一致，正常执行热路径没有新增 map 查找、锁或错误对象分配。

## 审查范围

- 变量编译计划、下标绑定和每次 Execution 初始化。
- GraphInstance 身份/生命周期职责。
- Start 与 HotReload 在同一 Blueprint 锁下的版本快照。
- Init 的加载失败、关闭、运行中和重复初始化边界。
- Yield/Resume、函数调用和循环中的变量生命周期。
- 控制流错误 NodeID/PC 归属及 `errors.Is/As` 链。
- 函数 Return 静态校验的兼容性与误报边界。
- mp1server `HotReloadResult` 调用方。

## 审查结果

- 普通变量只存于 `Graph.variables`，不再存在实例级变量 map、迁移锁或热加载迁移路径。
- 变量默认端口在编译/兼容加载阶段生成，Native Getter/Setter 运行时直接使用预绑定索引。
- 每次新入口和每次函数调用初始化变量；Resume 不调用初始化入口。
- 旧 Execution 持有旧 `CompiledGraph` 指针；新 Start 从当前图池获取一次快照，热加载不会修改在途对象。
- GraphInstance 仅保留图名、ID 和生命周期；审查中进一步删除了冗余 module 快照字段。
- Init 在解析前和发布前各检查一次 closed/in-use，只有完整编译成功才一次性写入配置和图池。
- VM handler 错误使用 dispatch 前捕获的 graph/node/PC；已有 `BlueprintError` 复制补字段并保留 Cause 链。
- 函数流校验遇到复杂动态多分支、循环或未绑定递归目标时放行，避免误拒绝线上资产；确定的线性 fallthrough、缺失/不可达 Return 和 BoolIf 分支仍能提前报错。
- mp1server 只读取 `GraphCount`，已删除无语义的实例迁移计数日志。

## 风险结论

- 阻塞性问题：0。
- 建议性问题：0。
- 保留边界：复杂函数流的无返回问题可能仍到运行时才报错，这是为避免静态误报而保留的明确兜底策略。
