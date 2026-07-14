package blueprint

import (
	"context"
	"errors"
	"testing"
)

type vmYieldOnceNode struct {
	BaseExecNode
	handle  **YieldHandle
	yielded *bool
}

func (n *vmYieldOnceNode) GetName() string { return "VMYieldOnce" }
func (n *vmYieldOnceNode) Exec() (int, error) {
	value, _ := n.GetInPortInt(1)
	n.GetAndCreateReturnPort().AppendArrayValInt(value)
	if value == 1 && !*n.yielded {
		*n.yielded = true
		handle, err := n.Yield(0)
		if err != nil {
			return -1, err
		}
		*n.handle = handle
		return -1, ErrExecutionSuspended
	}
	return 0, nil
}

func TestVMYieldResumesLoopAtNextIterationOnCapturedDispatcher(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmLoopRegistry(nil)
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "loop", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 5}}, {ID: "body", Class: "VMYieldOnce"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 2, DesNodeID: "body", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("loop", compiled)
	graphID := blueprint.Create("loop")
	execution, err := blueprint.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || handle == nil {
		t.Fatalf("state/handle = %v/%v, want suspended/non-nil", execution.State(), handle)
	}
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if execution.State() == ExecutionCompleted {
		t.Fatal("Resume bypassed captured dispatcher")
	}
	dispatcher.runNext(t)
	<-execution.Done()
	returns, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	assertVMIntReturns(t, returns, 0, 1, 2, 3, 4)
	if err := handle.Resume(); !errors.Is(err, ErrYieldResumed) {
		t.Fatalf("second Resume error = %v, want ErrYieldResumed", err)
	}
}

func TestVMYieldInvalidResumePortDoesNotConsumeHandle(t *testing.T) {
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
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("invalid-resume", compiled)
	execution, err := blueprint.Start(context.Background(), blueprint.Create("invalid-resume"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if err := handle.ResumeTo(99); !errors.Is(err, ErrYieldInvalid) {
		t.Fatalf("ResumeTo invalid port error = %v, want ErrYieldInvalid", err)
	}
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume after invalid ResumeTo failed: %v", err)
	}
	dispatcher.runNext(t)
	if _, err := execution.Result(); err != nil {
		t.Fatalf("Result failed: %v", err)
	}
}

func TestVMYieldPreservesSequenceContinuation(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmFlowRegistry()
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"}, {ID: "sequence", Class: "Sequence"},
			{ID: "yield", Class: "VMYieldOnce", PortDefault: map[int]any{1: 1}},
			{ID: "second", Class: "VMReturnPort", PortDefault: map[int]any{1: 2}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 1, DesNodeID: "second", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("sequence", compiled)
	execution, err := blueprint.Start(context.Background(), blueprint.Create("sequence"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	returns, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	assertVMIntReturns(t, returns, 1, 2)
}

func TestVMYieldPreservesFunctionCallStack(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmFunctionRegistry()
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	function, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "yield", Class: "VMYieldOnce", PortDefault: map[int]any{1: 1}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0},
			{SourceNodeID: "yield", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	main, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"yielding": function},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "yielding", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "call", DesPortID: 1},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("function", main)
	execution, err := blueprint.Start(context.Background(), blueprint.Create("function"), 1, 44)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	returns, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	assertVMIntReturns(t, returns, 44)
}
