package blueprint

import (
	"context"
	"testing"
)

type testReadVariable struct {
	BaseExecNode
	values []PortInt
}

func (n *testReadVariable) GetName() string { return "TestReadVariable" }
func (n *testReadVariable) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if ok {
		n.values = append(n.values, value)
	}
	return -1, nil
}

func TestCompilerSupportsLegacyGetSetVariableNodes(t *testing.T) {
	var reader *testReadVariable
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entrance_IntParam", func() IExecNode {
		return &EntranceIntParam{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("TestReadVariable", func() IExecNode {
		reader = &testReadVariable{}
		return reader
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "Count", Type: "Integer"}},
		Nodes: []NodeConfig{
			{ID: "entrance", Class: "Entrance_IntParam_1"},
			{ID: "set", Class: "Set_Count", PortDefault: map[int]any{1: 33}},
			{ID: "get", Class: "Get_Count"},
			{ID: "read", Class: "TestReadVariable"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entrance", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "set", SourcePortID: 0, DesNodeID: "read", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "read", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	var bp Blueprint
	bp.AddCompiledGraph("test", compiled)
	graphID := bp.Create("test")
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if reader == nil || len(reader.values) != 1 || reader.values[0] != 33 {
		t.Fatalf("reader values = %#v, want [33]", reader)
	}
}

func TestBlueprintVariablesResetAcrossExecutions(t *testing.T) {
	var reader *testReadVariable
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entrance_IntParam", func() IExecNode {
		return &EntranceIntParam{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("TestReadVariable", func() IExecNode {
		reader = &testReadVariable{}
		return reader
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "Count", Type: "Integer"}},
		Nodes: []NodeConfig{
			{ID: "setEntry", Class: "Entrance_IntParam_1"},
			{ID: "readEntry", Class: "Entrance_IntParam_2"},
			{ID: "set", Class: "Set_Count", PortDefault: map[int]any{1: 44}},
			{ID: "get", Class: "Get_Count"},
			{ID: "read", Class: "TestReadVariable"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "setEntry", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "readEntry", SourcePortID: 0, DesNodeID: "read", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "read", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	var bp Blueprint
	bp.AddCompiledGraph("test", compiled)
	graphID := bp.Create("test")
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("set Do failed: %v", err)
	}
	if _, err := bp.Do(graphID, 2); err != nil {
		t.Fatalf("read Do failed: %v", err)
	}
	if reader == nil || len(reader.values) != 1 || reader.values[0] != 0 {
		t.Fatalf("reader values = %#v, want [0]", reader)
	}
}

func TestBlueprintConcurrentStartsUseIndependentVariableSlots(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Variables: []VariableConfig{{Name: "Count", Type: "Integer"}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "set", Class: "Set_Count"},
			{ID: "get", Class: "Get_Count"},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "set", DesPortID: 1},
			{SourceNodeID: "set", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("variables", compiled)
	graphID := bp.Create("variables")
	first, err := bp.Start(context.Background(), graphID, 1, 11)
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	second, err := bp.Start(context.Background(), graphID, 1, 22)
	if err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	firstGraph := first.graph
	secondGraph := second.graph
	dispatcher.runNext(t)
	dispatcher.runNext(t)
	firstResult, firstErr := first.Result()
	secondResult, secondErr := second.Result()
	if firstErr != nil || secondErr != nil {
		t.Fatalf("Result errors = %v, %v", firstErr, secondErr)
	}
	assertVMIntReturns(t, firstResult, 11)
	assertVMIntReturns(t, secondResult, 22)
	if firstGraph.variables[0] == secondGraph.variables[0] {
		t.Fatal("executions share the same variable slot")
	}
}
