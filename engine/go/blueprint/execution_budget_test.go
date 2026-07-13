package blueprint

import (
	"context"
	"errors"
	"testing"
)

func TestGraphDoStopsLegacyExecCycleWithStableBudgetError(t *testing.T) {
	step := NewExecNode("step", NewNodeDefinition("ExecStep", func() IExecNode { return &testEntrance{} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	step.Next = []*ExecNode{step}
	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: step}, NodeCount: 1})
	graph.stepLimit = 3

	_, err := graph.Do(1)
	if !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Graph.Do error = %v, want ErrExecutionBudgetExceeded", err)
	}
}

func TestGraphDoCreatesIndependentBudgetForEachCall(t *testing.T) {
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, nil))
	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}, NodeCount: 1})
	graph.stepLimit = 1

	for call := 0; call < 2; call++ {
		if _, err := graph.Do(1); err != nil {
			t.Fatalf("Graph.Do call %d failed: %v", call+1, err)
		}
	}
}

func TestExecutionBudgetCountsDataProducerNodes(t *testing.T) {
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	producer := NewExecNode("producer", NewNodeDefinition("Producer", func() IExecNode { return &testRecorder{} }, nil, []IPort{NewPortInt()}))
	consumer := NewExecNode("consumer", NewNodeDefinition("Consumer", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	entry.Next = []*ExecNode{consumer}
	consumer.BeConnect = true
	consumer.PreInPort[1] = &PrePortNode{Node: producer, OutPortID: 0}
	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}, NodeCount: 3})
	graph.stepLimit = 2

	_, err := graph.Do(1)
	if !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Graph.Do error = %v, want data producer to consume the third step", err)
	}
}

func TestFunctionGraphSharesTopLevelExecutionBudget(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "FunctionEntry"}, {ID: "return", Class: "FunctionReturn"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "return", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"child": functionGraph},
		Nodes:     []NodeConfig{{ID: "entry", Class: "Entrance_IntParam_1"}, {ID: "call", Class: "FunctionCall", FunctionName: "child"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("budget", mainGraph)
	execution, err := bp.Start(context.Background(), bp.Create("budget"), 1)
	if err != nil {
		t.Fatal(err)
	}
	execution.scope.budget = newExecutionBudget(3)
	dispatcher.runNext(t)
	<-execution.Done()
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Execution.Result error = %v, want shared function budget exhaustion", err)
	}
}

func TestAsyncContinuationDoesNotResetExecutionBudget(t *testing.T) {
	var continuations []*Continuation
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("Wait", func() IExecNode { return &testCaptureAsync{continuations: &continuations} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	after := NewExecNode("after", NewNodeDefinition("After", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec()}, nil))
	entry.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{after}
	wait.BeConnect = true
	after.BeConnect = true

	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("async-budget", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}, NodeCount: 3})
	execution, err := bp.Start(context.Background(), bp.Create("async-budget"), 1)
	if err != nil {
		t.Fatal(err)
	}
	execution.scope.budget = newExecutionBudget(2)
	dispatcher.runNext(t)
	if len(continuations) != 1 || execution.State() != ExecutionSuspended {
		t.Fatalf("continuations=%d state=%v, want suspended execution", len(continuations), execution.State())
	}
	if err := continuations[0].Resume(); err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	<-execution.Done()
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Execution.Result error = %v, want budget exhaustion after resume", err)
	}
}

func TestStructuredForLoopConsumesExecutionBudget(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("Body", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec()}, nil))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "loop", Class: "ForLoopBreak", PortDefault: map[int]any{1: 0, 2: 3}},
			{ID: "body", Class: "Body"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(compiled)
	graph.stepLimit = 4
	if _, err := graph.Do(1); !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Graph.Do error = %v, want loop body executions to consume budget", err)
	}
}
