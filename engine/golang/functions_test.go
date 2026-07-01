package golang

import (
	"strings"
	"testing"
	"time"
)

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
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
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
