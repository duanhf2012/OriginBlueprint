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
	return -1, ErrControlNodeRequiresVM
}

func (n *FunctionCall) GetName() string {
	return "FunctionCall"
}

func (n *FunctionCall) Exec() (int, error) {
	return -1, ErrControlNodeRequiresVM
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

func arrayDataFromAny(value any) (ArrayData, error) {
	switch v := value.(type) {
	case int:
		return ArrayData{IntVal: PortInt(v)}, nil
	case int64:
		return ArrayData{IntVal: PortInt(v)}, nil
	case string:
		return ArrayData{StrVal: PortString(v)}, nil
	case bool:
		return ArrayData{BoolVal: PortBool(v)}, nil
	case float64:
		return ArrayData{FloatVal: PortFloat(v)}, nil
	default:
		return ArrayData{}, fmt.Errorf("top-level function return type %T cannot be represented by PortArray", value)
	}
}
