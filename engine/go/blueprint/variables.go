package blueprint

import "fmt"

// GetVariableNode 读取当前 Execution 的局部变量。
type GetVariableNode struct {
	BaseExecNode
}

// SetVariableNode 写入当前 Execution 的局部变量。
type SetVariableNode struct {
	BaseExecNode
}

func (n *GetVariableNode) GetName() string {
	return "GetVariable"
}

func (n *GetVariableNode) Exec() (int, error) {
	index := n.node.VariableIndex
	if index < 0 || index >= len(n.graph.variables) {
		return -1, fmt.Errorf("variable %s not found", n.node.VariableName)
	}
	port := n.graph.variables[index]
	if port == nil {
		return -1, fmt.Errorf("variable %s not found", n.node.VariableName)
	}
	out := n.GetOutPort(0)
	if out == nil {
		return -1, fmt.Errorf("GetVariable output not found")
	}
	out.SetValue(port)
	return -1, nil
}

func (n *SetVariableNode) GetName() string {
	return "SetVariable"
}

func (n *SetVariableNode) Exec() (int, error) {
	in := n.GetInPort(1)
	if in == nil {
		return -1, fmt.Errorf("SetVariable input not found")
	}
	value := in.Clone()
	index := n.node.VariableIndex
	if index < 0 || index >= len(n.graph.variables) {
		return -1, fmt.Errorf("variable %s not found", n.node.VariableName)
	}
	n.graph.variables[index] = value
	out := n.GetOutPort(1)
	if out != nil {
		out.SetValue(value)
	}
	return 0, nil
}
