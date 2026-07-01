package blueprint

import (
	"errors"
	"fmt"
	"sync"
)

// ErrContinuationResumed 表示同一个异步续点被重复恢复。
var ErrContinuationResumed = errors.New("golang blueprint continuation already resumed")

// Continuation 保存暂停节点继续执行所需的最小上下文。
type Continuation struct {
	graph     *Graph
	node      *ExecNode
	ctx       *ExecContext
	nextIndex int

	mu      sync.Mutex
	resumed bool
}

// Suspend 暂停当前节点，并返回可在异步回调中恢复的续点。
//
// nextIndex 表示恢复后继续执行的输出执行端口。
func (n *BaseExecNode) Suspend(nextIndex int) (*Continuation, error) {
	if n == nil || n.graph == nil || n.node == nil || n.ctx == nil {
		return nil, fmt.Errorf("node is not executing")
	}
	if nextIndex != -1 && nextIndex < 0 {
		return nil, fmt.Errorf("next index %d not found", nextIndex)
	}
	if nextIndex != -1 && nextIndex >= len(n.node.Next) && !n.node.isOutPortExec(nextIndex) {
		return nil, fmt.Errorf("next index %d not found", nextIndex)
	}
	return &Continuation{
		graph:     n.graph,
		node:      n.node,
		ctx:       n.ctx,
		nextIndex: nextIndex,
	}, nil
}

// Resume 恢复被暂停的节点，并把参数写入恢复节点的输出数据端口。
func (c *Continuation) Resume(outPortArgs ...any) error {
	if c == nil {
		return fmt.Errorf("continuation is nil")
	}

	c.mu.Lock()
	if c.resumed {
		c.mu.Unlock()
		return ErrContinuationResumed
	}
	if err := c.node.applyOutputArgs(c.ctx, outPortArgs...); err != nil {
		c.mu.Unlock()
		return err
	}
	c.resumed = true
	c.graph.setContext(c.node, c.ctx)
	nextIndex := c.nextIndex
	c.mu.Unlock()

	return c.node.doNext(c.graph, nextIndex)
}
