package golang

import (
	"errors"
	"fmt"
	"sync"
)

// ErrContinuationResumed protects async callbacks from resuming one suspension twice.
var ErrContinuationResumed = errors.New("golang blueprint continuation already resumed")

// Continuation captures the exact graph, node, and port context needed to resume
// execution after an async operation returns.
type Continuation struct {
	graph     *Graph
	node      *ExecNode
	ctx       *ExecContext
	nextIndex int

	mu      sync.Mutex
	resumed bool
}

// Suspend captures the current node context and the exec output to continue from.
//
// Async nodes return ErrExecutionSuspended after calling Suspend. Their callback
// later calls Resume with optional output values.
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

// Resume fills the suspended node's data outputs and continues execution once.
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
	c.graph.context[c.node.ID] = c.ctx
	nextIndex := c.nextIndex
	c.mu.Unlock()

	return c.node.doNext(c.graph, nextIndex)
}
