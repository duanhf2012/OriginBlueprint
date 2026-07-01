package golang

import "testing"

func BenchmarkBlueprintDoSharedCompiledGraph(b *testing.B) {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entrance_IntParam", func() IExecNode { return &EntranceIntParam{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("TestRecorder", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "record", Class: "TestRecorder", PortDefault: map[int]any{1: 7}},
		},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "record", DesPortID: 0}},
	})
	if err != nil {
		b.Fatal(err)
	}

	var bp Blueprint
	bp.AddCompiledGraph("bench", compiled)
	graphIDs := make([]int64, 1024)
	for index := range graphIDs {
		graphIDs[index] = bp.Create("bench")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := bp.Do(graphIDs[index%len(graphIDs)], 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFunctionCall(b *testing.B) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
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
		b.Fatal(err)
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
		b.Fatal(err)
	}
	graph := NewGraph(mainGraph)

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := graph.Do(1); err != nil {
			b.Fatal(err)
		}
	}
}
