package blueprint

import (
	"sync/atomic"
	"testing"
)

func TestBenchmarkComplexFlowFixtureRuns(t *testing.T) {
	bp, graphIDs := newBenchmarkComplexBlueprint(t, 128)
	for index, graphID := range graphIDs {
		returns, err := bp.Do(graphID, 1, PortInt(100), PortInt(index%8), PortInt(7))
		if err != nil {
			t.Fatalf("Do graph %d failed: %v", graphID, err)
		}
		if len(returns) != 7 {
			t.Fatalf("returns len = %d, want 7; returns = %#v", len(returns), returns)
		}
	}
}

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

func BenchmarkBlueprintDoComplexSharedCompiledGraph(b *testing.B) {
	bp, graphIDs := newBenchmarkComplexBlueprint(b, 4096)

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := bp.Do(graphIDs[index%len(graphIDs)], 1, PortInt(100), PortInt(index%8), PortInt(7)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlueprintDoComplexSharedCompiledGraphParallel(b *testing.B) {
	bp, graphIDs := newBenchmarkComplexBlueprint(b, 65536)
	var next uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			index := atomic.AddUint64(&next, 1) - 1
			graphID := graphIDs[int(index)%len(graphIDs)]
			if _, err := bp.Do(graphID, 1, PortInt(100), PortInt(index%8), PortInt(7)); err != nil {
				b.Fatal(err)
			}
		}
	})
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

type benchmarkFataler interface {
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func newBenchmarkComplexBlueprint(t benchmarkFataler, instanceCount int) (*Blueprint, []int64) {
	registry := NewRegistry()
	for _, factory := range BuiltinExecNodeFactories() {
		exec := factory()
		if exec == nil {
			continue
		}
		name, _, _ := parseEntranceClass(exec.GetName())
		if registry.Get(name) == nil {
			registry.Register(NewNodeDefinition(name, factory, benchmarkPortsForNode(name), benchmarkOutPortsForNode(name)))
		}
	}
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_000001"},
			{ID: "sequence", Class: "Sequence"},
			{ID: "outer", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 3}},
			{ID: "array", Class: "CreateIntArray", PortDefault: map[int]any{0: []int{10, 20}}},
			{ID: "inner", Class: "ForeachIntArray"},
			{ID: "sum", Class: "AddInt"},
			{ID: "append_sum", Class: "AppendIntReturn"},
			{ID: "range", Class: "RangeCompare", PortDefault: map[int]any{1: 4, 2: []int{3, 6}}},
			{ID: "range_hit", Class: "AppendStringReturn", PortDefault: map[int]any{1: "range-hit"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "outer", DesPortID: 0},
			{SourceNodeID: "outer", SourcePortID: 0, DesNodeID: "inner", DesPortID: 0},
			{SourceNodeID: "array", SourcePortID: 0, DesNodeID: "inner", DesPortID: 1},
			{SourceNodeID: "inner", SourcePortID: 0, DesNodeID: "append_sum", DesPortID: 0},
			{SourceNodeID: "outer", SourcePortID: 2, DesNodeID: "sum", DesPortID: 0},
			{SourceNodeID: "inner", SourcePortID: 3, DesNodeID: "sum", DesPortID: 1},
			{SourceNodeID: "sum", SourcePortID: 0, DesNodeID: "append_sum", DesPortID: 1},
			{SourceNodeID: "sequence", SourcePortID: 1, DesNodeID: "range", DesPortID: 0},
			{SourceNodeID: "range", SourcePortID: 3, DesNodeID: "range_hit", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	var bp Blueprint
	bp.AddCompiledGraph("complex", compiled)
	graphIDs := make([]int64, instanceCount)
	for index := range graphIDs {
		graphIDs[index] = bp.Create("complex")
		if graphIDs[index] == 0 {
			t.Fatal("Create complex graph returned 0")
		}
	}
	return &bp, graphIDs
}

func benchmarkPortsForNode(name string) []IPort {
	switch name {
	case "Entrance_IntParam":
		return nil
	case "Sequence":
		return []IPort{NewPortExec()}
	case "Foreach":
		return []IPort{NewPortExec(), NewPortInt(), NewPortInt()}
	case "ForeachIntArray":
		return []IPort{NewPortExec(), NewPortArray()}
	case "CreateIntArray":
		return []IPort{NewPortArray()}
	case "AddInt":
		return []IPort{NewPortInt(), NewPortInt()}
	case "AppendIntReturn":
		return []IPort{NewPortExec(), NewPortInt()}
	case "AppendStringReturn":
		return []IPort{NewPortExec(), NewPortStr()}
	case "RangeCompare":
		return []IPort{NewPortExec(), NewPortInt(), NewPortArray()}
	default:
		return nil
	}
}

func benchmarkOutPortsForNode(name string) []IPort {
	switch name {
	case "Entrance_IntParam":
		return []IPort{NewPortExec(), NewPortInt(), NewPortInt(), NewPortInt()}
	case "Sequence":
		return []IPort{NewPortExec(), NewPortExec(), NewPortExec()}
	case "Foreach":
		return []IPort{NewPortExec(), NewPortExec(), NewPortInt()}
	case "ForeachIntArray":
		return []IPort{NewPortExec(), NewPortExec(), NewPortInt(), NewPortInt()}
	case "CreateIntArray":
		return []IPort{NewPortArray()}
	case "AddInt":
		return []IPort{NewPortInt()}
	case "AppendIntReturn", "AppendStringReturn":
		return []IPort{NewPortExec()}
	case "RangeCompare":
		return []IPort{NewPortExec(), NewPortExec(), NewPortExec(), NewPortExec(), NewPortExec(), NewPortExec()}
	default:
		return nil
	}
}
