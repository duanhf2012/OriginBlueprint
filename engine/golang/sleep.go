package golang

import (
	"fmt"
	"time"
)

// SleepNodeName is the runtime class name for the async delay node.
const SleepNodeName = "Sleep"

// SleepNode suspends execution and resumes from its exec output after a delay.
type SleepNode struct {
	BaseExecNode
}

// NewSleepNodeDefinition builds the dynamic definition for Sleep.
func NewSleepNodeDefinition() *NodeDefinition {
	return NewNodeDefinition(SleepNodeName, func() IExecNode {
		return &SleepNode{}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()})
}

func (n *SleepNode) GetName() string {
	return SleepNodeName
}

func (n *SleepNode) Exec() (int, error) {
	delay, ok := n.GetInPortInt(1)
	if !ok {
		return -1, fmt.Errorf("Sleep delay input not found")
	}
	if delay < 0 {
		return -1, fmt.Errorf("Sleep delay %d is negative", delay)
	}

	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}
	time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		_ = continuation.Resume()
	})
	return -1, ErrExecutionSuspended
}
