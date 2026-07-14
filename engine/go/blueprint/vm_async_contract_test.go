package blueprint

import (
	"context"
	"errors"
	"testing"
)

type vmSuspendWithoutYieldNode struct{ BaseExecNode }

func (*vmSuspendWithoutYieldNode) GetName() string { return "VMSuspendWithoutYield" }
func (*vmSuspendWithoutYieldNode) Exec() (int, error) {
	return -1, ErrExecutionSuspended
}

type vmYieldWithoutSuspendNode struct {
	BaseExecNode
	handle **YieldHandle
}

func (*vmYieldWithoutSuspendNode) GetName() string { return "VMYieldWithoutSuspend" }
func (n *vmYieldWithoutSuspendNode) Exec() (int, error) {
	handle, err := n.Yield(0)
	if err != nil {
		return -1, err
	}
	*n.handle = handle
	return 0, nil
}

func compileVMAsyncContractGraph(t *testing.T, class string, factory func() IExecNode) *CompiledGraph {
	t.Helper()
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition(class, factory, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "async", Class: class}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "async", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	return compiled
}

func runVMAsyncContractGraph(t *testing.T, compiled *CompiledGraph) (*Execution, *manualExecutionDispatcher) {
	t.Helper()
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("async-contract", compiled)
	execution, err := blueprint.Start(context.Background(), blueprint.Create("async-contract"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	return execution, dispatcher
}

func TestVMRejectsSuspensionWithoutYieldHandle(t *testing.T) {
	compiled := compileVMAsyncContractGraph(t, "VMSuspendWithoutYield", func() IExecNode {
		return &vmSuspendWithoutYieldNode{}
	})
	execution, _ := runVMAsyncContractGraph(t, compiled)
	if execution.State() != ExecutionFailed {
		t.Fatalf("state = %v, want ExecutionFailed", execution.State())
	}
	if _, err := execution.Result(); !errors.Is(err, ErrYieldInvalid) {
		t.Fatalf("Result error = %v, want ErrYieldInvalid", err)
	}
}

func TestVMRejectsYieldHandleWithoutSuspension(t *testing.T) {
	var handle *YieldHandle
	compiled := compileVMAsyncContractGraph(t, "VMYieldWithoutSuspend", func() IExecNode {
		return &vmYieldWithoutSuspendNode{handle: &handle}
	})
	execution, _ := runVMAsyncContractGraph(t, compiled)
	if execution.State() != ExecutionFailed {
		t.Fatalf("state = %v, want ExecutionFailed", execution.State())
	}
	if _, err := execution.Result(); !errors.Is(err, ErrYieldInvalid) {
		t.Fatalf("Result error = %v, want ErrYieldInvalid", err)
	}
	if handle == nil {
		t.Fatal("Yield did not return a handle")
	}
}

func TestVMResumeCanRetryAfterDispatcherRejectsSubmission(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "yield", Class: "VMYieldOnce", PortDefault: map[int]any{1: 1}},
		},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	execution, dispatcher := runVMAsyncContractGraph(t, compiled)
	dispatcher.mu.Lock()
	dispatcher.rejected = true
	dispatcher.mu.Unlock()
	if err := handle.Resume(); !errors.Is(err, ErrExecutionRejected) {
		t.Fatalf("first Resume error = %v, want ErrExecutionRejected", err)
	}
	if execution.State() != ExecutionSuspended {
		t.Fatalf("state after rejected Resume = %v, want ExecutionSuspended", execution.State())
	}
	dispatcher.mu.Lock()
	dispatcher.rejected = false
	dispatcher.mu.Unlock()
	if err := handle.Resume(); err != nil {
		t.Fatalf("retry Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	if _, err := execution.Result(); err != nil {
		t.Fatalf("Result failed: %v", err)
	}
}

func TestGraphDoRejectsYieldWithoutExecutionLifecycle(t *testing.T) {
	var handle *YieldHandle
	compiled := compileVMAsyncContractGraph(t, "VMYieldWithoutSuspend", func() IExecNode {
		return &vmYieldWithoutSuspendNode{handle: &handle}
	})
	graph := NewGraph(compiled)
	if _, err := graph.Do(1); !errors.Is(err, ErrYieldInvalid) {
		t.Fatalf("Do error = %v, want ErrYieldInvalid", err)
	}
	if handle != nil {
		t.Fatalf("Graph.Do unexpectedly created resumable handle: %v", handle)
	}
	if graph.vm != nil {
		t.Fatal("Graph.Do retained VM after rejected Yield")
	}
}
