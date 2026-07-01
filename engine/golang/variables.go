package golang

import "fmt"

// GetVariableNode reads a per-instance variable into its output port.
type GetVariableNode struct {
	BaseExecNode
}

// SetVariableNode writes a per-instance variable and continues exec flow.
type SetVariableNode struct {
	BaseExecNode
}

func (n *GetVariableNode) GetName() string {
	return "GetVariable"
}

func (n *GetVariableNode) Exec() (int, error) {
	if n.graph.variableMu != nil {
		n.graph.variableMu.RLock()
	}
	port := n.graph.variables[n.node.VariableName]
	if port != nil {
		// Return a clone so later SetVariable calls do not mutate this node's
		// already captured output context.
		port = port.Clone()
	}
	if n.graph.variableMu != nil {
		n.graph.variableMu.RUnlock()
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
	if n.graph.variableMu != nil {
		n.graph.variableMu.Lock()
	}
	n.graph.variables[n.node.VariableName] = value
	if n.graph.variableMu != nil {
		n.graph.variableMu.Unlock()
	}
	out := n.GetOutPort(1)
	if out != nil {
		out.SetValue(value)
	}
	return 0, nil
}
