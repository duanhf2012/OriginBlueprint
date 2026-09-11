package blueprint

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// TimerHandle 只用于打开历史文件；新 Timer API 使用静态 Timer Key，不再公开句柄。
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
	duration, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("Delay duration is missing")
	}
	delay, err := checkedMilliseconds(duration)
	if err != nil {
		return -1, fmt.Errorf("%w: %dms", ErrTimerDurationInvalid, duration)
	}
	handle, err := n.Yield(0)
	if err != nil {
		return -1, err
	}
	if n.graph == nil || n.graph.execution == nil || n.graph.execution.blueprint == nil {
		handle.abandon()
		return -1, fmt.Errorf("Delay requires Blueprint.Start or Blueprint.DoContext")
	}
	execution := n.graph.execution
	var cancelHook atomic.Uint64
	scheduled, err := execution.blueprint.scheduler().Schedule(delay, func() {
		execution.removeCancelHook(cancelHook.Load())
		if resumeErr := handle.Resume(); errors.Is(resumeErr, ErrExecutionRejected) {
			execution.finishSubmissionError(resumeErr)
		}
	})
	if err != nil {
		handle.abandon()
		return -1, err
	}
	cancelHook.Store(execution.addCancelHook(func() { scheduled.Cancel() }))
	if err := execution.cancellationError(); err != nil {
		scheduled.Cancel()
	}
	return -1, ErrExecutionSuspended
}

func NewDelayNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(DelayNodeName, func() IExecNode { return &DelayNode{} }, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}
func NewSleepNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(SleepNodeName, func() IExecNode { return &DelayNode{} }, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}

// CreateTimerNode 的输出 0 是同步 Created，输出 1 是调度器触发的 callback。
type CreateTimerNode struct{ BaseExecNode }

func (n *CreateTimerNode) GetName() string { return "CreateTimer" }
func (n *CreateTimerNode) Exec() (int, error) {
	duration, durationOK := n.GetInPortInt(1)
	looping, loopingOK := n.GetInPortBool(2)
	if !durationOK || !loopingOK {
		return -1, fmt.Errorf("CreateTimer inputs are invalid")
	}
	if n.graph == nil || n.graph.execution == nil || n.graph.execution.blueprint == nil {
		return -1, fmt.Errorf("CreateTimer requires Blueprint.Start or Blueprint.DoContext")
	}
	if err := n.graph.execution.blueprint.createTimer(n.graph, n.node, duration, looping); err != nil {
		return -1, err
	}
	return 0, nil
}

type ClearTimerByKeyNode struct{ BaseExecNode }

func (n *ClearTimerByKeyNode) GetName() string { return "ClearTimerByKey" }
func (n *ClearTimerByKeyNode) Exec() (int, error) {
	if n.graph == nil || n.graph.instance == nil || n.node == nil {
		return -1, fmt.Errorf("ClearTimer runtime is unavailable")
	}
	n.SetOutPortBool(1, n.graph.instance.clearTimer(n.node.TimerKey))
	return 0, nil
}
