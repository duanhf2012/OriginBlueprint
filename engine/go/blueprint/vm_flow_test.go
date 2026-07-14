package blueprint

import "testing"

func vmFlowRegistry() *Registry {
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("Sequence", func() IExecNode { return &Sequence{} }, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortExec()}))
	return registry
}

func TestVMSequenceExecutesBranchesInPortOrder(t *testing.T) {
	compiled, err := CompileGraph(vmFlowRegistry(), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "sequence", Class: "Sequence"},
			{ID: "first", Class: "VMReturnPort", PortDefault: map[int]any{1: 10}},
			{ID: "second", Class: "VMReturnPort", PortDefault: map[int]any{1: 20}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "first", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 1, DesNodeID: "second", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 10, 20)
}

func TestVMFanoutPreservesLegacyEdgeOrder(t *testing.T) {
	compiled, err := CompileGraph(vmFlowRegistry(), GraphConfig{
		Legacy: true,
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "first", Class: "VMReturnPort", PortDefault: map[int]any{1: 1}},
			{ID: "second", Class: "VMReturnPort", PortDefault: map[int]any{1: 2}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "first", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "second", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 1, 2)
}

func assertVMIntReturns(t *testing.T, returns PortArray, want ...PortInt) {
	t.Helper()
	if len(returns) != len(want) {
		t.Fatalf("returns = %#v, want %v", returns, want)
	}
	for index, value := range want {
		if returns[index].IntVal != value {
			t.Fatalf("returns[%d] = %#v, want %d; all returns %#v", index, returns[index], value, returns)
		}
	}
}
