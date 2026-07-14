package blueprint

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVMCancelSuspendedExecutionInvalidatesYield(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmLoopRegistry(nil)
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "loop", Class: "Foreach", PortDefault: map[int]any{1: 1, 2: 2}}, {ID: "body", Class: "VMYieldOnce"}},
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
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("cancel", compiled)
	execution, err := bp.Start(context.Background(), bp.Create("cancel"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if !execution.Cancel() {
		t.Fatal("Cancel returned false")
	}
	<-execution.Done()
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("Result error = %v, want ErrExecutionCanceled", err)
	}
	if err := handle.Resume(); !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("Resume after cancel = %v, want ErrExecutionCanceled", err)
	}
}

func TestVMExecutionConvertsNativePanicToFailure(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "panic-node", Class: "VMPanic"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "panic-node", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("panic", compiled)
	execution, err := bp.Start(context.Background(), bp.Create("panic"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	_, err = execution.Result()
	if execution.State() != ExecutionFailed || err == nil || !strings.Contains(err.Error(), "panic-node") {
		t.Fatalf("state/error = %v/%v, want failed error with node", execution.State(), err)
	}
}

func TestVMHotReloadDoesNotChangeSuspendedProgram(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	oldGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "yield", Class: "VMYieldOnce", PortDefault: map[int]any{1: 1}},
			{ID: "old-result", Class: "VMReturnPort", PortDefault: map[int]any{1: 10}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0},
			{SourceNodeID: "yield", SourcePortID: 0, DesNodeID: "old-result", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("compile old graph failed: %v", err)
	}
	newGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "new-result", Class: "VMReturnPort", PortDefault: map[int]any{1: 20}}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "new-result", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("compile new graph failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("hot", oldGraph)
	graphID := bp.Create("hot")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	(&hotReloadPlan{blueprint: bp, graphs: map[string]*CompiledGraph{"hot": newGraph}}).apply()
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	oldReturns, err := execution.Result()
	if err != nil {
		t.Fatalf("old Result failed: %v", err)
	}
	assertVMIntReturns(t, oldReturns, 1, 10)

	newExecution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("new Start failed: %v", err)
	}
	dispatcher.runNext(t)
	newReturns, err := newExecution.Result()
	if err != nil {
		t.Fatalf("new Result failed: %v", err)
	}
	assertVMIntReturns(t, newReturns, 20)
}
