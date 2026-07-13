package blueprint

import (
	"errors"
	"fmt"
	"sync"
)

// ErrContinuationResumed 表示同一个异步续点被重复恢复。
var ErrContinuationResumed = errors.New("golang blueprint continuation already resumed")

// ErrContinuationTargetRequired 表示动态续点必须在恢复时指定执行输出端口。
var ErrContinuationTargetRequired = errors.New("golang blueprint dynamic continuation requires ResumeTo")

// ErrContinuationTargetFixed 表示固定出口续点不能在恢复时改选执行输出端口。
var ErrContinuationTargetFixed = errors.New("golang blueprint fixed continuation cannot use ResumeTo")

var ErrGraphReleased = errors.New("golang blueprint graph released")

// Continuation 保存暂停节点继续执行所需的最小上下文。
type Continuation struct {
	graph     *Graph
	node      *ExecNode
	ctx       *ExecContext
	nextIndex int
	dynamic   bool

	mu      sync.Mutex
	resumed bool
}

// SuspendForResume 暂停当前节点，并允许异步回调在恢复时选择执行输出端口。
func (n *BaseExecNode) SuspendForResume() (*Continuation, error) {
	if n == nil || n.graph == nil || n.node == nil || n.ctx == nil {
		return nil, fmt.Errorf("node is not executing")
	}
	return &Continuation{
		graph:     n.graph,
		node:      n.node,
		ctx:       n.ctx,
		nextIndex: -1,
		dynamic:   true,
	}, nil
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
	if c.dynamic {
		return ErrContinuationTargetRequired
	}
	if c.graph != nil && c.graph.functionFrame != nil {
		return c.graph.functionFrame.schedule(c, c.nextIndex, outPortArgs...)
	}
	if c.graph != nil && c.graph.execution != nil {
		return c.graph.execution.scheduleContinuation(c, outPortArgs...)
	}
	if err := c.reserve(); err != nil {
		return err
	}
	return c.resumeReserved(outPortArgs...)
}

// ResumeAsync 恢复外部异步节点；若当前执行仍在退出调用栈，则先登记并在挂起状态落定后投递。
func (c *Continuation) ResumeAsync(outPortArgs ...any) error {
	if c == nil {
		return fmt.Errorf("continuation is nil")
	}
	if c.dynamic {
		return ErrContinuationTargetRequired
	}
	if c.graph != nil && c.graph.functionFrame != nil {
		return c.graph.functionFrame.schedule(c, c.nextIndex, outPortArgs...)
	}
	if c.graph != nil && c.graph.execution != nil {
		return c.graph.execution.scheduleContinuation(c, outPortArgs...)
	}
	if err := c.reserve(); err != nil {
		return err
	}
	return defaultExecutionDispatcher.Submit(func() { _ = c.resumeReserved(outPortArgs...) })
}

// ResumeTo 恢复动态 continuation，并从指定执行输出端口继续。
func (c *Continuation) ResumeTo(nextIndex int, outPortArgs ...any) error {
	if c == nil {
		return fmt.Errorf("continuation is nil")
	}
	if !c.dynamic {
		return ErrContinuationTargetFixed
	}
	if c.node == nil || !c.node.isOutPortExec(nextIndex) {
		return fmt.Errorf("next index %d is not an exec output", nextIndex)
	}
	if c.graph != nil && c.graph.functionFrame != nil {
		return c.graph.functionFrame.schedule(c, nextIndex, outPortArgs...)
	}
	if c.graph != nil && c.graph.execution != nil {
		return c.graph.execution.scheduleContinuationAt(c, nextIndex, outPortArgs...)
	}
	if err := c.reserve(); err != nil {
		return err
	}
	return c.resumeReservedAt(nextIndex, outPortArgs...)
}

func (c *Continuation) reserve() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resumed {
		return ErrContinuationResumed
	}
	c.resumed = true
	return nil
}

func (c *Continuation) resumeReserved(outPortArgs ...any) error {
	return c.resumeReservedAt(c.nextIndex, outPortArgs...)
}

func (c *Continuation) resumeReservedAt(nextIndex int, outPortArgs ...any) error {
	if c.graph != nil && c.graph.instance != nil {
		if !c.graph.instance.tryAcquireLease() {
			return ErrGraphReleased
		}
		defer c.graph.instance.releaseLease()
	}

	if err := c.node.applyOutputArgs(c.ctx, outPortArgs...); err != nil {
		return err
	}
	c.graph.setContext(c.node, c.ctx)
	return c.node.doNext(c.graph, nextIndex)
}
