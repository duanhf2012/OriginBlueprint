package blueprint

import "fmt"

// GetVariableNode 读取当前 Execution 的局部变量或当前 GraphInstance 的共享变量。
type GetVariableNode struct {
	BaseExecNode
}

// SetVariableNode 写入当前 Execution 的局部变量或当前 GraphInstance 的共享变量。
type SetVariableNode struct {
	BaseExecNode
}

func (n *GetVariableNode) GetName() string {
	return "GetVariable"
}

func (n *GetVariableNode) Exec() (int, error) {
	index := n.node.VariableIndex
	if index < 0 || index >= len(n.graph.compiled.variablePlans) {
		return -1, fmt.Errorf("variable %s not found", n.node.VariableName)
	}
	var port IPort
	if n.node.VariableScope == VariableScopeInstance {
		if n.graph.instance == nil {
			return -1, fmt.Errorf("instance variable %s requires a Blueprint graph instance", n.node.VariableName)
		}
		port = n.graph.instance.getVariable(n.node.InstanceVariableKey)
	} else if index < len(n.graph.variables) {
		port = n.graph.variables[index]
	}
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
	if index < 0 || index >= len(n.graph.compiled.variablePlans) {
		return -1, fmt.Errorf("variable %s not found", n.node.VariableName)
	}
	if n.node.VariableScope == VariableScopeInstance {
		if n.graph.instance == nil {
			return -1, fmt.Errorf("instance variable %s requires a Blueprint graph instance", n.node.VariableName)
		}
		if !n.graph.instance.setVariable(n.node.InstanceVariableKey, value) {
			return -1, fmt.Errorf("instance variable %s not found", n.node.VariableName)
		}
	} else {
		if index >= len(n.graph.variables) {
			return -1, fmt.Errorf("variable %s not found", n.node.VariableName)
		}
		n.graph.variables[index] = value
	}
	out := n.GetOutPort(1)
	if out != nil {
		out.SetValue(value)
	}
	return 0, nil
}
