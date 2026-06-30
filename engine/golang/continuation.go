package golang

import (
	"errors"
	"fmt"
	"sync"
)

var ErrContinuationResumed = errors.New("golang blueprint continuation already resumed")

type Continuation struct {
	graph     *Graph
	node      *ExecNode
	ctx       *ExecContext
	nextIndex int

	mu      sync.Mutex
	resumed bool
}

func (n *BaseExecNode) Suspend(nextIndex int) (*Continuation, error) {
	if n == nil || n.graph == nil || n.node == nil || n.ctx == nil {
		return nil, fmt.Errorf("node is not executing")
	}
	if nextIndex != -1 && (nextIndex < 0 || nextIndex >= len(n.node.Next)) {
		return nil, fmt.Errorf("next index %d not found", nextIndex)
	}
	return &Continuation{
		graph:     n.graph,
		node:      n.node,
		ctx:       n.ctx,
		nextIndex: nextIndex,
	}, nil
}

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
