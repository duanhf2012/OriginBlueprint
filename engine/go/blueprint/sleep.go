package blueprint

import (
	"fmt"
	"time"
)

// SleepNodeName 是异步等待节点的注册名称。
const SleepNodeName = "Sleep"

// SleepNode 会暂停当前执行流，并在 timer 回调中恢复后续节点。
type SleepNode struct {
	BaseExecNode
}

// NewSleepNodeDefinition 创建 Sleep 节点定义。
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
