package blueprint

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type vmReturnPortNode struct{ BaseExecNode }

func (n *vmReturnPortNode) GetName() string { return "VMReturnPort" }

func (n *vmReturnPortNode) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if !ok {
		return -1, nil
	}
	n.GetAndCreateReturnPort().AppendArrayValInt(value)
	return -1, nil
}

type vmPanicNode struct{ BaseExecNode }

func (n *vmPanicNode) GetName() string { return "VMPanic" }
func (n *vmPanicNode) Exec() (int, error) {
	panic("native boom")
}

type vmPassNode struct{ BaseExecNode }

func (n *vmPassNode) GetName() string    { return "VMPass" }
func (n *vmPassNode) Exec() (int, error) { return 0, nil }

func vmNativeRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("VMEntry", func() IExecNode { return &EntranceIntParam{} }, nil, []IPort{NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("VMReturnPort", func() IExecNode { return &vmReturnPortNode{} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("VMPanic", func() IExecNode { return &vmPanicNode{} }, []IPort{NewPortExec()}, nil))
	return registry
}

func TestVMNativePreservesEntranceArgsAndUpstreamData(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "result", Class: "VMReturnPort"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1, 37)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	if len(returns) != 1 || returns[0].IntVal != 37 {
		t.Fatalf("returns = %#v, want [37]", returns)
	}
}

func TestVMNativePreservesDefaultInputAndReturnPort(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "result", Class: "VMReturnPort", PortDefault: map[int]any{1: 91}}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "result", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	if len(returns) != 1 || returns[0].IntVal != 91 {
		t.Fatalf("returns = %#v, want [91]", returns)
	}
}

func TestVMNativePanicIncludesNodeAndPC(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "panic-node", Class: "VMPanic"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "panic-node", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	_, err = NewGraph(compiled).runVMEntrance(1)
	if err == nil || !strings.Contains(err.Error(), "panic-node") || !strings.Contains(err.Error(), "pc 1") {
		t.Fatalf("panic error = %v, want node and pc context", err)
	}
}

func TestVMExecutionBudgetStopsLongLinearFlow(t *testing.T) {
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMPass", func() IExecNode { return &vmPassNode{} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	nodes := []NodeConfig{{ID: "entry", Class: "VMEntry_1"}}
	edges := make([]EdgeConfig, 0, 10)
	previous := "entry"
	for index := 0; index < 10; index++ {
		id := fmt.Sprintf("step-%d", index)
		nodes = append(nodes, NodeConfig{ID: id, Class: "VMPass"})
		edges = append(edges, EdgeConfig{SourceNodeID: previous, SourcePortID: 0, DesNodeID: id, DesPortID: 0})
		previous = id
	}
	compiled, err := CompileGraph(registry, GraphConfig{Nodes: nodes, Edges: edges})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	graph := NewGraph(compiled)
	graph.stepLimit = 5
	if _, err := graph.Do(1); !errors.Is(err, ErrExecutionBudgetExceeded) {
		t.Fatalf("Do error = %v, want ErrExecutionBudgetExceeded", err)
	}
}
