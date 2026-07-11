package blueprint

import (
	"fmt"
	"math"
	"time"
)

const DelayNodeName = "Delay"
const SleepNodeName = "Sleep"

const maxTimerMilliseconds = PortInt(math.MaxInt64 / int64(time.Millisecond))

type DelayNode struct {
	BaseExecNode
}

type SleepNode = DelayNode

func NewDelayNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(DelayNodeName, func() IExecNode {
		return &DelayNode{}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}

func NewSleepNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(SleepNodeName, func() IExecNode {
		return &DelayNode{}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}

func (n *DelayNode) GetName() string { return DelayNodeName }

func (n *DelayNode) Exec() (int, error) {
	delay, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("Delay duration input not found")
	}
	if delay < 0 || delay > maxTimerMilliseconds {
		return -1, fmt.Errorf("Delay duration %d is outside supported range", delay)
	}
	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}
	scheduler := defaultTimerScheduler
	var execution *Execution
	if n.graph != nil && n.graph.execution != nil {
		execution = n.graph.execution
		if execution.blueprint != nil {
			scheduler = execution.blueprint.timerScheduler()
		}
	}

	var cancelHookID uint64
	handle, err := scheduler.Schedule(time.Duration(delay)*time.Millisecond, func() {
		if execution != nil {
			execution.removeCancelHook(cancelHookID)
			_ = continuation.ResumeAsync()
			return
		}
		_ = defaultExecutionDispatcher.Submit(func() { _ = continuation.Resume() })
	})
	if err != nil {
		return -1, err
	}
	if execution != nil {
		cancelHookID = execution.addCancelHook(func() { scheduler.Cancel(handle) })
	}
	return -1, ErrExecutionSuspended
}
