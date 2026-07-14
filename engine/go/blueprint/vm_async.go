package blueprint

import (
	"errors"
	"fmt"
	"sync/atomic"
)

var (
	ErrYieldResumed = errors.New("golang blueprint yield already resumed")
	ErrYieldInvalid = errors.New("golang blueprint yield is invalid")
)

type vmYieldState struct {
	token    uint64
	node     *ExecNode
	ctx      *ExecContext
	graph    *Graph
	nextPort int
}

// YieldHandle 是一次性恢复句柄。Resume 始终通过启动 Execution 时捕获的 Dispatcher 投递。
type YieldHandle struct {
	execution *Execution
	machine   *vmMachine
	token     uint64
	node      *ExecNode
	nextPort  int
	used      atomic.Bool
}

// Yield 暂停当前 Native 节点；节点必须将 ErrExecutionSuspended 返回给 VM。
func (n *BaseExecNode) Yield(nextPort int) (*YieldHandle, error) {
	if n == nil || n.graph == nil || n.node == nil || n.ctx == nil || n.graph.vm == nil {
		return nil, ErrYieldInvalid
	}
	return n.graph.vm.yield(n.graph, n.node, n.ctx, nextPort)
}

func (m *vmMachine) yield(graph *Graph, node *ExecNode, ctx *ExecContext, nextPort int) (*YieldHandle, error) {
	if m == nil || graph == nil || node == nil || ctx == nil || graph.execution == nil || m.graph != graph || m.pc != PC(node.Index) || m.pendingYield != nil {
		return nil, ErrYieldInvalid
	}
	if !node.isOutPortExec(nextPort) {
		return nil, fmt.Errorf("%w: node %s output %d is not exec", ErrYieldInvalid, node.ID, nextPort)
	}
	m.yieldSeq++
	state := &vmYieldState{token: m.yieldSeq, graph: graph, node: node, ctx: ctx, nextPort: nextPort}
	m.pendingYield = state
	return &YieldHandle{execution: graph.execution, machine: m, token: state.token, node: node, nextPort: nextPort}, nil
}

func (h *YieldHandle) Resume(outputs ...any) error {
	if h == nil || h.machine == nil {
		return ErrYieldInvalid
	}
	if h.execution != nil {
		if err := h.execution.cancellationError(); err != nil {
			return err
		}
	}
	return h.ResumeTo(h.nextPort, outputs...)
}

func (h *YieldHandle) ResumeTo(nextPort int, outputs ...any) error {
	if h == nil || h.machine == nil || h.execution == nil {
		return ErrYieldInvalid
	}
	if h.node == nil || !h.node.isOutPortExec(nextPort) {
		return fmt.Errorf("%w: output %d is not exec", ErrYieldInvalid, nextPort)
	}
	if !h.used.CompareAndSwap(false, true) {
		return ErrYieldResumed
	}
	if err := h.execution.cancellationError(); err != nil {
		return err
	}
	values := append([]any(nil), outputs...)
	if err := h.execution.submit(func() {
		if !h.execution.beginRun() {
			return
		}
		err := h.machine.resumeYield(h.token, nextPort, values...)
		if err == nil {
			err = h.machine.run()
		}
		h.execution.finishRun(h.execution.graph.resultSnapshot(), err)
	}); err != nil {
		h.used.CompareAndSwap(true, false)
		return err
	}
	return nil
}

func (m *vmMachine) resumeYield(token uint64, nextPort int, outputs ...any) error {
	state := m.pendingYield
	if state == nil || state.token != token {
		return ErrYieldResumed
	}
	if err := state.node.applyOutputArgs(state.ctx, outputs...); err != nil {
		return fmt.Errorf("resume node %s outputs: %w", state.node.ID, err)
	}
	state.graph.releaseContext(state.node, state.ctx)
	m.pendingYield = nil
	m.pc = PC(state.node.Index)
	m.inputPortID = state.ctx.ExecInputPortID
	m.outputArgs = nil
	m.advanceFromPort(nextPort)
	return m.err
}
