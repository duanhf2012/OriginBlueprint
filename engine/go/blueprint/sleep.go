package blueprint

import (
	"fmt"
	"math"
	"sync"
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
		scheduler = execution.timerScheduler()
	}

	var cancelHookMu sync.Mutex
	var cancelHookID uint64
	callbackFired := false
	handle, err := scheduler.Schedule(time.Duration(delay)*time.Millisecond, func() {
		cancelHookMu.Lock()
		callbackFired = true
		hookID := cancelHookID
		cancelHookID = 0
		cancelHookMu.Unlock()
		if execution != nil {
			execution.removeCancelHook(hookID)
			_ = continuation.ResumeAsync()
			return
		}
		_ = defaultExecutionDispatcher.Submit(func() { _ = continuation.Resume() })
	})
	if err != nil {
		return -1, err
	}
	if execution != nil {
		hookID := execution.addCancelHook(func() { scheduler.Cancel(handle) })
		cancelHookMu.Lock()
		if callbackFired {
			cancelHookMu.Unlock()
			execution.removeCancelHook(hookID)
		} else {
			cancelHookID = hookID
			cancelHookMu.Unlock()
		}
	}
	return -1, ErrExecutionSuspended
}
