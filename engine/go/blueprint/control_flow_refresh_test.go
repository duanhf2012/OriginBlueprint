package blueprint

import (
	"errors"
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

func TestLegacyExecFanoutStartsRemainingBranchesWhenOneSuspends(t *testing.T) {
	var calls []string
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("Suspends", func() IExecNode {
		return &orderedExecRecorder{label: "suspends", calls: &calls, next: -1, err: ErrExecutionSuspended}
	}, []IPort{NewPortExec()}, nil))
	registry.Register(NewNodeDefinition("Continues", func() IExecNode {
		return &orderedExecRecorder{label: "continues", calls: &calls, next: -1}
	}, []IPort{NewPortExec()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "suspends", Class: "Suspends"},
			{ID: "continues", Class: "Continues"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "suspends", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "continues", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	_, err = NewGraph(compiled).Do(1)
	if !errors.Is(err, ErrExecutionSuspended) {
		t.Fatalf("Do err = %v, want ErrExecutionSuspended", err)
	}
	if want := []string{"suspends", "continues"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}
