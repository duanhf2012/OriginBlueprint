package blueprint

import "fmt"

// FunctionEntranceID 是函数图固定使用的入口 ID。
const FunctionEntranceID int64 = 1

// MaxFunctionCallDepth 限制函数递归深度，避免蓝图递归耗尽栈或 CPU。
const MaxFunctionCallDepth = 64

// FunctionEntry 是函数图的入口节点。
type FunctionEntry struct {
	BaseExecNode
}

// FunctionReturn 是函数图的返回节点。
type FunctionReturn struct {
	BaseExecNode
}

// FunctionCall 是调用其他函数图的节点。
type FunctionCall struct {
	BaseExecNode
}

func (n *FunctionEntry) GetName() string {
	return "FunctionEntry"
}

func (n *FunctionEntry) Exec() (int, error) {
	return 0, nil
}

func (n *FunctionReturn) GetName() string {
	return "FunctionReturn"
}

func (n *FunctionReturn) Exec() (int, error) {
	values := make([]any, 0, len(n.ctx.InputPorts)-1)
	n.graph.returns = nil
	for index := 1; index < len(n.ctx.InputPorts); index++ {
		value := portAnyValue(n.ctx.InputPorts[index])
		values = append(values, value)
		n.graph.appendReturn(arrayDataFromAny(value))
	}
	if err := n.graph.completeFunction(values); err != nil {
		return -1, err
	}
	return -1, ErrFunctionReturned
}

func (n *FunctionCall) GetName() string {
	return "FunctionCall"
}

func (n *FunctionCall) Exec() (int, error) {
	if n.graph == nil || n.graph.compiled == nil {
		return -1, fmt.Errorf("function call is not executing")
	}
	functionGraph := n.lookupFunctionGraph()
	if functionGraph == nil {
		return -1, fmt.Errorf("function %s not found", n.functionLabel())
	}
	if n.graph.callDepth >= MaxFunctionCallDepth {
		return -1, fmt.Errorf("maximum function call depth %d exceeded", MaxFunctionCallDepth)
	}

	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}

	child := NewGraph(functionGraph)
	child.name = n.functionLabel()
	child.graphID = n.graph.graphID
	child.module = n.graph.module
	child.instance = n.graph.instance
	child.callDepth = n.graph.callDepth + 1
	child.budget = n.graph.budget
	if n.graph.execution != nil {
		child.execution = n.graph.execution.rootExecution()
		child.functionFrame = newFunctionFrame(child.execution, child)
	}
	// 函数调用与父图共享实例变量锁，保证变量访问语义一致。
	child.trace = n.graph.trace
	child.onFunctionComplete = func(values []any) error {
		return continuation.Resume(values...)
	}

	args := make([]any, 0, len(n.ctx.InputPorts)-1)
	for index := 1; index < len(n.ctx.InputPorts); index++ {
		args = append(args, portAnyValue(n.ctx.InputPorts[index]))
	}

	_, runErr := child.runEntrance(FunctionEntranceID, args...)
	if child.functionFrame != nil {
		child.functionFrame.finish(runErr)
	}
	if runErr != nil && !isFunctionCallStop(runErr) {
		return -1, runErr
	}
	if runErr == ErrFunctionReturned || child.functionCompleted.Load() {
		if n.graph.execution != nil {
			return -1, ErrExecutionSuspended
		}
		return -1, nil
	}
	if runErr == nil && child.onFunctionComplete != nil {
		return -1, fmt.Errorf("function %s completed without FunctionReturn", n.functionLabel())
	}
	return -1, ErrExecutionSuspended
}

func (n *FunctionCall) lookupFunctionGraph() *CompiledGraph {
	if n == nil || n.graph == nil || n.graph.compiled == nil {
		return nil
	}
	if n.node.FunctionGraph != nil {
		return n.node.FunctionGraph
	}
	if n.node.FunctionID != "" {
		if graph := n.graph.compiled.Functions[n.node.FunctionID]; graph != nil {
			return graph
		}
	}
	if n.node.FunctionName != "" {
		return n.graph.compiled.Functions[n.node.FunctionName]
	}
	return nil
}

func (n *FunctionCall) functionLabel() string {
	if n != nil && n.node != nil {
		if n.node.FunctionID != "" {
			return n.node.FunctionID
		}
		return n.node.FunctionName
	}
	return ""
}

func isFunctionCallStop(err error) bool {
	return err == ErrExecutionSuspended || err == ErrFunctionReturned
}

func functionEntryDefinition(inputTypes []string) (*NodeDefinition, error) {
	if err := validateFunctionPortCounts(len(inputTypes), 0, "function input count", "function output count"); err != nil {
		return nil, err
	}
	outPorts, err := functionPorts(inputTypes, true)
	if err != nil {
		return nil, err
	}
	return NewNodeDefinition("FunctionEntry", func() IExecNode { return &FunctionEntry{} }, nil, outPorts), nil
}

func functionReturnDefinition(outputTypes []string) (*NodeDefinition, error) {
	if err := validateFunctionPortCounts(0, len(outputTypes), "function input count", "function output count"); err != nil {
		return nil, err
	}
	inPorts, err := functionPorts(outputTypes, false)
	if err != nil {
		return nil, err
	}
	return NewNodeDefinition("FunctionReturn", func() IExecNode { return &FunctionReturn{} }, inPorts, nil), nil
}

func functionCallDefinition(inputTypes []string, outputTypes []string) (*NodeDefinition, error) {
	if err := validateFunctionPortCounts(len(inputTypes), len(outputTypes), "function input count", "function output count"); err != nil {
		return nil, err
	}
	inPorts, err := functionPorts(inputTypes, false)
	if err != nil {
		return nil, err
	}
	outPorts, err := functionPorts(outputTypes, true)
	if err != nil {
		return nil, err
	}
	return NewNodeDefinition("FunctionCall", func() IExecNode { return &FunctionCall{} }, inPorts, outPorts), nil
}

func functionPorts(types []string, output bool) ([]IPort, error) {
	ports := make([]IPort, len(types)+1)
	ports[0] = NewPortExec()
	for index, typ := range types {
		port, err := newPortFromDataType(typ)
		if err != nil {
			return nil, err
		}
		ports[index+1] = port
	}
	return ports, nil
}

func portAnyValue(port IPort) any {
	p, ok := port.(*Port)
	if !ok || p == nil {
		return nil
	}
	switch p.kind {
	case portKindInt:
		return p.intv
	case portKindFloat:
		return p.floatv
	case portKindString:
		return p.strv
	case portKindBool:
		return p.boolv
	case portKindArray:
		return append(PortArray(nil), p.arrv...)
	case portKindAny:
		return cloneAnyValue(p.anyv)
	case portKindTimerHandle:
		return p.timerv
	default:
		return nil
	}
}

func arrayDataFromAny(value any) ArrayData {
	switch v := value.(type) {
	case int:
		return ArrayData{IntVal: PortInt(v)}
	case int64:
		return ArrayData{IntVal: PortInt(v)}
	case string:
		return ArrayData{StrVal: PortString(v)}
	case bool:
		return ArrayData{BoolVal: PortBool(v)}
	case float64:
		return ArrayData{FloatVal: PortFloat(v)}
	default:
		return ArrayData{}
	}
}
