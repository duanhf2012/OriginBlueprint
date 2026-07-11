package blueprint

import (
	"fmt"
	"time"
)

type SetTimerByFunctionNode struct{ BaseExecNode }

type ClearTimerNode struct{ BaseExecNode }
type PauseTimerNode struct{ BaseExecNode }
type UnpauseTimerNode struct{ BaseExecNode }
type IsTimerActiveNode struct{ BaseExecNode }
type IsTimerPausedNode struct{ BaseExecNode }
type IsTimerValidNode struct{ BaseExecNode }
type GetTimerRemainingNode struct{ BaseExecNode }
type GetTimerElapsedNode struct{ BaseExecNode }

func (n *SetTimerByFunctionNode) GetName() string { return "SetTimerByFunction" }

func (n *SetTimerByFunctionNode) Exec() (int, error) {
	if n.graph == nil || n.graph.execution == nil || n.node == nil {
		return -1, fmt.Errorf("SetTimerByFunction requires Blueprint execution")
	}
	intervalMilliseconds, intervalOK := n.GetInPortInt(1)
	looping, loopingOK := n.GetInPortBool(2)
	firstDelayMilliseconds, firstDelayOK := n.GetInPortInt(3)
	if !intervalOK || !loopingOK || !firstDelayOK {
		return -1, fmt.Errorf("SetTimerByFunction inputs are invalid")
	}
	if intervalMilliseconds < 0 || intervalMilliseconds > maxTimerMilliseconds {
		return -1, fmt.Errorf("timer interval %d is outside supported range", intervalMilliseconds)
	}
	if firstDelayMilliseconds == -1 {
		firstDelayMilliseconds = intervalMilliseconds
	}
	if firstDelayMilliseconds < 0 || firstDelayMilliseconds > maxTimerMilliseconds || (looping && intervalMilliseconds == 0) {
		return -1, fmt.Errorf("timer first delay %d or interval %d is invalid", firstDelayMilliseconds, intervalMilliseconds)
	}
	args := make([]any, 0, len(n.ctx.InputPorts)-4)
	for index := 4; index < len(n.ctx.InputPorts); index++ {
		args = append(args, portAnyValue(n.ctx.InputPorts[index]))
	}
	handle, err := n.graph.execution.blueprint.setTimerByFunction(
		n.graph.instance,
		n.node.FunctionID,
		n.node.FunctionName,
		args,
		time.Duration(intervalMilliseconds)*time.Millisecond,
		time.Duration(firstDelayMilliseconds)*time.Millisecond,
		looping,
	)
	if err != nil {
		return -1, err
	}
	n.SetOutPortTimerHandle(1, handle)
	return 0, nil
}

func setTimerByFunctionDefinition(inputTypes []string) (*NodeDefinition, error) {
	inputs := []IPort{NewPortExec(), NewPortInt(), NewPortBool(), NewPortInt()}
	for _, inputType := range inputTypes {
		port, err := newPortFromDataType(inputType)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, port)
	}
	return NewNodeDefinition("SetTimerByFunction", func() IExecNode {
		return &SetTimerByFunctionNode{}
	}, inputs, []IPort{NewPortExec(), NewPortTimerHandle()}), nil
}

func timerBlueprint(n *BaseExecNode) *Blueprint {
	if n == nil || n.graph == nil || n.graph.execution == nil {
		return nil
	}
	return n.graph.execution.blueprint
}

func (n *ClearTimerNode) GetName() string { return "ClearTimer" }
func (n *ClearTimerNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(1)
	cancelRunning, _ := n.GetInPortBool(2)
	bp := timerBlueprint(&n.BaseExecNode)
	success := bp != nil && bp.clearTimer(handle, cancelRunning)
	n.SetOutPortBool(1, success)
	return 0, nil
}

func (n *PauseTimerNode) GetName() string { return "PauseTimer" }
func (n *PauseTimerNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(1)
	bp := timerBlueprint(&n.BaseExecNode)
	success := bp != nil && bp.pauseTimer(handle)
	n.SetOutPortBool(1, success)
	return 0, nil
}

func (n *UnpauseTimerNode) GetName() string { return "UnpauseTimer" }
func (n *UnpauseTimerNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(1)
	bp := timerBlueprint(&n.BaseExecNode)
	success := bp != nil && bp.resumeTimer(handle)
	n.SetOutPortBool(1, success)
	return 0, nil
}

func (n *IsTimerActiveNode) GetName() string { return "IsTimerActive" }
func (n *IsTimerActiveNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(0)
	bp := timerBlueprint(&n.BaseExecNode)
	n.SetOutPortBool(0, bp != nil && bp.isTimerActive(handle))
	return -1, nil
}

func (n *IsTimerPausedNode) GetName() string { return "IsTimerPaused" }
func (n *IsTimerPausedNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(0)
	bp := timerBlueprint(&n.BaseExecNode)
	n.SetOutPortBool(0, bp != nil && bp.isTimerPaused(handle))
	return -1, nil
}

func (n *IsTimerValidNode) GetName() string { return "IsTimerValid" }
func (n *IsTimerValidNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(0)
	bp := timerBlueprint(&n.BaseExecNode)
	n.SetOutPortBool(0, bp != nil && bp.isTimerValid(handle))
	return -1, nil
}

func (n *GetTimerRemainingNode) GetName() string { return "GetTimerRemaining" }
func (n *GetTimerRemainingNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(0)
	bp := timerBlueprint(&n.BaseExecNode)
	remaining := time.Duration(-1)
	if bp != nil {
		remaining = bp.timerRemaining(handle)
	}
	n.SetOutPortInt(0, PortInt(remaining/time.Millisecond))
	return -1, nil
}

func (n *GetTimerElapsedNode) GetName() string { return "GetTimerElapsed" }
func (n *GetTimerElapsedNode) Exec() (int, error) {
	handle, _ := n.GetInPortTimerHandle(0)
	bp := timerBlueprint(&n.BaseExecNode)
	elapsed := time.Duration(-1)
	if bp != nil {
		elapsed = bp.timerElapsed(handle)
	}
	n.SetOutPortInt(0, PortInt(elapsed/time.Millisecond))
	return -1, nil
}

func NewClearTimerDefinition() *NodeDefinition {
	return NewNodeDefinition("ClearTimer", func() IExecNode { return &ClearTimerNode{} }, []IPort{NewPortExec(), NewPortTimerHandle(), NewPortBool()}, []IPort{NewPortExec(), NewPortBool()})
}

func NewPauseTimerDefinition() *NodeDefinition {
	return NewNodeDefinition("PauseTimer", func() IExecNode { return &PauseTimerNode{} }, []IPort{NewPortExec(), NewPortTimerHandle()}, []IPort{NewPortExec(), NewPortBool()})
}

func NewUnpauseTimerDefinition() *NodeDefinition {
	return NewNodeDefinition("UnpauseTimer", func() IExecNode { return &UnpauseTimerNode{} }, []IPort{NewPortExec(), NewPortTimerHandle()}, []IPort{NewPortExec(), NewPortBool()})
}

func NewIsTimerActiveDefinition() *NodeDefinition {
	return NewNodeDefinition("IsTimerActive", func() IExecNode { return &IsTimerActiveNode{} }, []IPort{NewPortTimerHandle()}, []IPort{NewPortBool()})
}

func NewIsTimerPausedDefinition() *NodeDefinition {
	return NewNodeDefinition("IsTimerPaused", func() IExecNode { return &IsTimerPausedNode{} }, []IPort{NewPortTimerHandle()}, []IPort{NewPortBool()})
}

func NewIsTimerValidDefinition() *NodeDefinition {
	return NewNodeDefinition("IsTimerValid", func() IExecNode { return &IsTimerValidNode{} }, []IPort{NewPortTimerHandle()}, []IPort{NewPortBool()})
}

func NewGetTimerRemainingDefinition() *NodeDefinition {
	return NewNodeDefinition("GetTimerRemaining", func() IExecNode { return &GetTimerRemainingNode{} }, []IPort{NewPortTimerHandle()}, []IPort{NewPortInt()})
}

func NewGetTimerElapsedDefinition() *NodeDefinition {
	return NewNodeDefinition("GetTimerElapsed", func() IExecNode { return &GetTimerElapsedNode{} }, []IPort{NewPortTimerHandle()}, []IPort{NewPortInt()})
}
