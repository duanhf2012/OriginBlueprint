package blueprint

// nativeRuntime 是 Go 业务节点访问当前 VM 节点上下文的最小边界。
// 现有 BaseExecNode 仍通过 Graph 适配；控制流不再由 Native 节点递归推进。
type nativeRuntime interface {
	input(int) IPort
	output(int) IPort
	returnPort() IPort
	variableName() string
	module() IBlueprintModule
}
