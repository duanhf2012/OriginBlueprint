package blueprint

import (
	"context"
	"reflect"
	"testing"
)

type orderedExecRecorder struct {
	BaseExecNode
	label string
	calls *[]string
	next  int
	err   error
}

func (n *orderedExecRecorder) GetName() string { return n.label }

func (n *orderedExecRecorder) Exec() (int, error) {
	*n.calls = append(*n.calls, n.label)
	return n.next, n.err
}

func TestWhileRefreshesConnectedConditionAfterEachBodyIteration(t *testing.T) {
	registry := testSystemRegistry(t)
	compiled, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "counter", Type: "Integer", Value: 0}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "while", Class: "WhileNode"},
			{ID: "get", Class: "Get_counter"},
			{ID: "condition", Class: "CompareGreaterInteger", PortDefault: map[int]any{0: 3}},
			{ID: "increment", Class: "AddInt", PortDefault: map[int]any{1: 1}},
			{ID: "set", Class: "Set_counter"},
			{ID: "done", Class: "AppendIntReturn"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "while", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "condition", DesPortID: 1},
			{SourceNodeID: "condition", SourcePortID: 0, DesNodeID: "while", DesPortID: 1},
			{SourceNodeID: "while", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "increment", DesPortID: 0},
			{SourceNodeID: "increment", SourcePortID: 0, DesNodeID: "set", DesPortID: 1},
			{SourceNodeID: "while", SourcePortID: 1, DesNodeID: "done", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "done", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	returns, err := NewGraph(compiled).Do(1)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if len(returns) != 1 || returns[0].IntVal != 3 {
		t.Fatalf("returns = %#v, want [3]", returns)
	}
}

func TestLegacyExecFanoutRunsEveryBranchInEdgeOrder(t *testing.T) {
	var calls []string
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("LeftStart", func() IExecNode {
		return &orderedExecRecorder{label: "left-start", calls: &calls, next: 0}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("LeftEnd", func() IExecNode {
		return &orderedExecRecorder{label: "left-end", calls: &calls, next: -1}
	}, []IPort{NewPortExec()}, nil))
	registry.Register(NewNodeDefinition("Right", func() IExecNode {
		return &orderedExecRecorder{label: "right", calls: &calls, next: -1}
	}, []IPort{NewPortExec()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "left-start", Class: "LeftStart"},
			{ID: "left-end", Class: "LeftEnd"},
			{ID: "right", Class: "Right"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "left-start", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "right", DesPortID: 0},
			{SourceNodeID: "left-start", SourcePortID: 0, DesNodeID: "left-end", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	if _, err := NewGraph(compiled).Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	want := []string{"left-start", "left-end", "right"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestLegacyNestedExecFanoutUsesDepthFirstEdgeOrder(t *testing.T) {
	var calls []string
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	for _, label := range []string{"outer", "inner-a", "inner-b", "sibling"} {
		label := label
		outputs := []IPort(nil)
		next := -1
		if label == "outer" {
			outputs = []IPort{NewPortExec()}
			next = 0
		}
		registry.Register(NewNodeDefinition(label, func() IExecNode {
			return &orderedExecRecorder{label: label, calls: &calls, next: next}
		}, []IPort{NewPortExec()}, outputs))
	}

	compiled, err := CompileGraph(registry, GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "outer", Class: "outer"},
			{ID: "inner-a", Class: "inner-a"},
			{ID: "inner-b", Class: "inner-b"},
			{ID: "sibling", Class: "sibling"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "outer", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sibling", DesPortID: 0},
			{SourceNodeID: "outer", SourcePortID: 0, DesNodeID: "inner-a", DesPortID: 0},
			{SourceNodeID: "outer", SourcePortID: 0, DesNodeID: "inner-b", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	if _, err := NewGraph(compiled).Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if want := []string{"outer", "inner-a", "inner-b", "sibling"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestLegacyExecFanoutWaitsForEachSuspendedBranchInOrder(t *testing.T) {
	var calls []string
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewDelayNodeDefinition())
	registry.Register(NewNodeDefinition("FirstResult", func() IExecNode {
		return &orderedExecRecorder{label: "first", calls: &calls, next: -1}
	}, []IPort{NewPortExec()}, nil))
	registry.Register(NewNodeDefinition("SecondResult", func() IExecNode {
		return &orderedExecRecorder{label: "second", calls: &calls, next: -1}
	}, []IPort{NewPortExec()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "first-delay", Class: "Delay", PortDefault: map[int]any{1: 10}},
			{ID: "first-result", Class: "FirstResult"},
			{ID: "second-delay", Class: "Delay", PortDefault: map[int]any{1: 20}},
			{ID: "second-result", Class: "SecondResult"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "first-delay", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "second-delay", DesPortID: 0},
			{SourceNodeID: "first-delay", SourcePortID: 0, DesNodeID: "first-result", DesPortID: 0},
			{SourceNodeID: "second-delay", SourcePortID: 0, DesNodeID: "second-result", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("legacy-fanout-delay", compiled)
	execution, err := bp.Start(context.Background(), bp.Create("legacy-fanout-delay"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || len(calls) != 0 {
		t.Fatalf("initial state=%v calls=%v", execution.State(), calls)
	}

	scheduler.fire(t, scheduler.onlyHandle(t))
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || !reflect.DeepEqual(calls, []string{"first"}) {
		t.Fatalf("after first resume state=%v calls=%v", execution.State(), calls)
	}

	scheduler.fire(t, scheduler.onlyHandle(t))
	dispatcher.runNext(t)
	if execution.State() != ExecutionCompleted {
		t.Fatalf("final state=%v, want completed", execution.State())
	}
	if want := []string{"first", "second"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestCancelLegacyExecFanoutStopsSuspendedAndPendingBranches(t *testing.T) {
	var calls []string
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewDelayNodeDefinition())
	registry.Register(NewNodeDefinition("PendingSibling", func() IExecNode {
		return &orderedExecRecorder{label: "pending", calls: &calls, next: -1}
	}, []IPort{NewPortExec()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "delay", Class: "Delay", PortDefault: map[int]any{1: 10}},
			{ID: "pending", Class: "PendingSibling"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "delay", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "pending", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("cancel-legacy-fanout", compiled)
	execution, err := bp.Start(context.Background(), bp.Create("cancel-legacy-fanout"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	handle := scheduler.onlyHandle(t)
	if !execution.Cancel() {
		t.Fatal("Cancel returned false")
	}
	if !scheduler.canceled[handle] {
		t.Fatalf("scheduled delay %d was not canceled", handle)
	}
	if execution.State() != ExecutionCanceled || len(calls) != 0 {
		t.Fatalf("state=%v calls=%v, want canceled with no sibling run", execution.State(), calls)
	}
}
