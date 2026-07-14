package blueprint

import (
	"strings"
	"testing"
)

type vmFunctionLocalProbe struct {
	BaseExecNode
	values *[]PortInt
}

func (n *vmFunctionLocalProbe) GetName() string { return "VMFunctionLocalProbe" }
func (n *vmFunctionLocalProbe) Exec() (int, error) {
	value, _ := n.graph.variables["counter"].GetInt()
	*n.values = append(*n.values, value)
	n.graph.variables["counter"].SetInt(value + 1)
	return 0, nil
}

func vmFunctionRegistry() *Registry {
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("AddInt", func() IExecNode { return &AddInt{} }, []IPort{NewPortInt(), NewPortInt()}, []IPort{NewPortInt()}))
	return registry
}

func registerFunctionTestNodes(registry *Registry, recorderFactory func() IExecNode) {
	registry.Register(NewNodeDefinition("Entrance_IntParam", func() IExecNode { return &EntranceIntParam{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("AddInt", func() IExecNode { return &AddInt{} }, []IPort{NewPortInt(), NewPortInt()}, []IPort{NewPortInt()}))
	registry.Register(NewSleepNodeDefinition())
	registry.Register(NewNodeDefinition("TestRecorder", recorderFactory, []IPort{NewPortExec(), NewPortInt()}, nil))
}

func TestVMFunctionCallMapsArgumentsAndReturnValues(t *testing.T) {
	registry := vmFunctionRegistry()
	function, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "add", Class: "AddInt", PortDefault: map[int]any{1: 5}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "add", DesPortID: 0},
			{SourceNodeID: "add", SourcePortID: 0, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	main, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"plus-five": function},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "plus-five", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}},
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
	returns, err := NewGraph(main).runVMEntrance(1, 7)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 12)
}

func TestVMFunctionRequiresFunctionReturn(t *testing.T) {
	registry := vmFunctionRegistry()
	function, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "FunctionEntry"}},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	main, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"invalid": function},
		Nodes:     []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "call", Class: "FunctionCall", FunctionName: "invalid"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}
	if _, err := NewGraph(main).runVMEntrance(1); err == nil {
		t.Fatal("function completed without FunctionReturn")
	}
}

func TestVMFunctionVariablesAreFreshPerInvocation(t *testing.T) {
	var values []PortInt
	registry := vmFlowRegistry()
	registry.Register(NewNodeDefinition("VMFunctionLocalProbe", func() IExecNode { return &vmFunctionLocalProbe{values: &values} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	function, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "counter", Type: "integer", Value: 0}},
		Nodes:     []NodeConfig{{ID: "entry", Class: "FunctionEntry"}, {ID: "probe", Class: "VMFunctionLocalProbe"}, {ID: "return", Class: "FunctionReturn"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "probe", DesPortID: 0}, {SourceNodeID: "probe", SourcePortID: 0, DesNodeID: "return", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	main, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"local": function},
		Nodes:     []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "sequence", Class: "Sequence"}, {ID: "first", Class: "FunctionCall", FunctionName: "local"}, {ID: "second", Class: "FunctionCall", FunctionName: "local"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "first", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 1, DesNodeID: "second", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}
	if _, err := NewGraph(main).runVMEntrance(1); err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	if len(values) != 2 || values[0] != 0 || values[1] != 0 {
		t.Fatalf("function local values = %v, want [0 0]", values)
	}
}

func TestVMFunctionDepthLimitStopsRecursion(t *testing.T) {
	registry := vmFunctionRegistry()
	function, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "FunctionEntry"}, {ID: "recursive", Class: "FunctionCall", FunctionName: "recursive"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "recursive", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	function.Functions = map[string]*CompiledGraph{"recursive": function}
	main, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"recursive": function},
		Nodes:     []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "call", Class: "FunctionCall", FunctionName: "recursive"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}
	_, err = NewGraph(main).runVMEntrance(1)
	if err == nil || !strings.Contains(err.Error(), "maximum function call depth") {
		t.Fatalf("recursion error = %v, want depth limit", err)
	}
}
