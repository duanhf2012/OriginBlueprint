package blueprint

import (
	"strings"
	"sync"
	"testing"
	"time"
)

type functionLocalProbe struct {
	BaseExecNode
	values *[]PortInt
	locks  *[]*sync.RWMutex
}

func (n *functionLocalProbe) GetName() string { return "FunctionLocalProbe" }
func (n *functionLocalProbe) Exec() (int, error) {
	port := n.graph.variables["counter"]
	value, _ := port.GetInt()
	*n.values = append(*n.values, value)
	*n.locks = append(*n.locks, n.graph.variableMu)
	port.SetInt(value + 1)
	return 0, nil
}

func TestFunctionVariablesAndLocksAreFreshPerInvocation(t *testing.T) {
	var values []PortInt
	var locks []*sync.RWMutex
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewNodeDefinition("FunctionLocalProbe", func() IExecNode {
		return &functionLocalProbe{values: &values, locks: &locks}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	functionGraph, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "counter", Type: "integer", Value: 0}},
		Nodes:     []NodeConfig{{ID: "entry", Class: "FunctionEntry"}, {ID: "probe", Class: "FunctionLocalProbe"}, {ID: "return", Class: "FunctionReturn"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "probe", DesPortID: 0}, {SourceNodeID: "probe", SourcePortID: 0, DesNodeID: "return", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"local": functionGraph},
		Nodes:     []NodeConfig{{ID: "entry", Class: "Entrance_IntParam_1"}, {ID: "call", Class: "FunctionCall", FunctionName: "local"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Do(1); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != 0 || values[1] != 0 {
		t.Fatalf("function local values = %v, want [0 0]", values)
	}
	if len(locks) != 2 || locks[0] == locks[1] || locks[0] == graph.variableMu || locks[1] == graph.variableMu {
		t.Fatal("function invocations shared a variable lock")
	}
}

func TestFunctionCallReturnsValuesToCaller(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer", "Integer"}},
			{ID: "add", Class: "AddInt"},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "add", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 2, DesNodeID: "add", DesPortID: 1},
			{SourceNodeID: "add", SourcePortID: 0, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}

	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"sum": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "sum", FunctionInputTypes: []string{"Integer", "Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 2, 2: 5}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}
	callNode := mainGraph.Entrances[1].Next[0]
	if callNode.FunctionGraph != functionGraph {
		t.Fatalf("FunctionCall was not pre-resolved at compile time")
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 7 {
		t.Fatalf("recorder values = %#v, want [7]", recorder)
	}
}

func TestFunctionCallContinuesAfterAsyncFunctionReturn(t *testing.T) {
	recorder := &testRecorder{}
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		return recorder
	})

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "sleep", Class: "Sleep", PortDefault: map[int]any{1: 5}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sleep", DesPortID: 0},
			{SourceNodeID: "sleep", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile async function failed: %v", err)
	}

	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"delayed": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "delayed", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 9}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != ErrExecutionSuspended {
		t.Fatalf("Do error = %v, want ErrExecutionSuspended", err)
	}
	if len(recorder.snapshot()) != 0 {
		t.Fatalf("recorder ran before async function returned")
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		values := recorder.snapshot()
		if len(values) == 1 && values[0] == 9 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("recorder values = %#v, want [9]", recorder.snapshot())
}

func TestFunctionCallDepthLimitStopsRecursion(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry"},
			{ID: "call", Class: "FunctionCall", FunctionName: "recurse"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	functionGraph.Functions = map[string]*CompiledGraph{"recurse": functionGraph}

	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"recurse": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "recurse"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err == nil || !strings.Contains(err.Error(), "maximum function call depth") {
		t.Fatalf("Do error = %v, want maximum function call depth", err)
	}
}

func registerFunctionTestNodes(registry *Registry, recorderFactory func() IExecNode) {
	registry.Register(NewNodeDefinition("Entrance_IntParam", func() IExecNode { return &EntranceIntParam{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("AddInt", func() IExecNode { return &AddInt{} }, []IPort{NewPortInt(), NewPortInt()}, []IPort{NewPortInt()}))
	registry.Register(NewSleepNodeDefinition())
	registry.Register(NewNodeDefinition("TestRecorder", recorderFactory, []IPort{NewPortExec(), NewPortInt()}, nil))
}
