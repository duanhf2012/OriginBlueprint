package blueprint

import (
	"errors"
	"fmt"
)

var ErrUnsupportedAsyncNode = errors.New("blueprint Delay/Timer nodes are not provided by the VM core")

// TimerHandle 仅作为旧蓝图文件的数据类型占位；VM Core 不实现 Timer 生命周期。
type TimerHandle struct {
	BlueprintID uint64
	GraphID     int64
	TimerID     uint64
	Generation  uint64
}

func (h TimerHandle) Valid() bool {
	return h.BlueprintID != 0 && h.GraphID != 0 && h.TimerID != 0 && h.Generation != 0
}

const DelayNodeName = "Delay"
const SleepNodeName = "Sleep"

type DelayNode struct{ BaseExecNode }
type SleepNode = DelayNode

func (n *DelayNode) GetName() string { return DelayNodeName }
func (n *DelayNode) Exec() (int, error) {
	return -1, fmt.Errorf("%w: %s", ErrUnsupportedAsyncNode, n.GetName())
}

func NewDelayNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(DelayNodeName, func() IExecNode { return &DelayNode{} }, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}

func NewSleepNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(SleepNodeName, func() IExecNode { return &DelayNode{} }, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}

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
func (n *ClearTimerNode) GetName() string         { return "ClearTimer" }
func (n *PauseTimerNode) GetName() string         { return "PauseTimer" }
func (n *UnpauseTimerNode) GetName() string       { return "UnpauseTimer" }
func (n *IsTimerActiveNode) GetName() string      { return "IsTimerActive" }
func (n *IsTimerPausedNode) GetName() string      { return "IsTimerPaused" }
func (n *IsTimerValidNode) GetName() string       { return "IsTimerValid" }
func (n *GetTimerRemainingNode) GetName() string  { return "GetTimerRemaining" }
func (n *GetTimerElapsedNode) GetName() string    { return "GetTimerElapsed" }

func unsupportedTimerExec(name string) (int, error) {
	return -1, fmt.Errorf("%w: %s", ErrUnsupportedAsyncNode, name)
}

func (n *SetTimerByFunctionNode) Exec() (int, error) { return unsupportedTimerExec(n.GetName()) }
func (n *ClearTimerNode) Exec() (int, error)         { return unsupportedTimerExec(n.GetName()) }
func (n *PauseTimerNode) Exec() (int, error)         { return unsupportedTimerExec(n.GetName()) }
func (n *UnpauseTimerNode) Exec() (int, error)       { return unsupportedTimerExec(n.GetName()) }
func (n *IsTimerActiveNode) Exec() (int, error)      { return unsupportedTimerExec(n.GetName()) }
func (n *IsTimerPausedNode) Exec() (int, error)      { return unsupportedTimerExec(n.GetName()) }
func (n *IsTimerValidNode) Exec() (int, error)       { return unsupportedTimerExec(n.GetName()) }
func (n *GetTimerRemainingNode) Exec() (int, error)  { return unsupportedTimerExec(n.GetName()) }
func (n *GetTimerElapsedNode) Exec() (int, error)    { return unsupportedTimerExec(n.GetName()) }

func setTimerByFunctionDefinition(inputTypes []string) (*NodeDefinition, error) {
	inputs := []IPort{NewPortExec(), NewPortInt(), NewPortBool(), NewPortInt()}
	for _, inputType := range inputTypes {
		port, err := newPortFromDataType(inputType)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, port)
	}
	return NewNodeDefinition("SetTimerByFunction", func() IExecNode { return &SetTimerByFunctionNode{} }, inputs, []IPort{NewPortExec(), NewPortTimerHandle()}), nil
}
