package blueprint

import (
	"strings"
	"testing"
)

func validationRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("ExecTarget", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec()}, nil))
	registry.Register(NewNodeDefinition("IntSource", func() IExecNode { return &testRecorder{} }, nil, []IPort{NewPortInt()}))
	registry.Register(NewNodeDefinition("StringSource", func() IExecNode { return &testRecorder{} }, nil, []IPort{NewPortStr()}))
	registry.Register(NewNodeDefinition("IntTarget", func() IExecNode { return &testRecorder{} }, []IPort{NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("StringTarget", func() IExecNode { return &testRecorder{} }, []IPort{NewPortStr()}, nil))
	registry.Register(NewNodeDefinition("IntTransform", func() IExecNode { return &testRecorder{} }, []IPort{NewPortInt()}, []IPort{NewPortInt()}))
	registry.Register(NewNodeDefinition("ExecStep", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	nilSource := NewNodeDefinition("NilSource", func() IExecNode { return &testRecorder{} }, nil, []IPort{NewPortInt()})
	nilSource.OutPorts[0] = (*Port)(nil)
	registry.Register(nilSource)
	return registry
}

func TestCompileGraphRejectsDataDependencyCycleWithNodeIDs(t *testing.T) {
	_, err := CompileGraph(validationRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "alpha", Class: "IntTransform"}, {ID: "beta", Class: "IntTransform"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "alpha", SourcePortID: 0, DesNodeID: "beta", DesPortID: 0},
			{SourceNodeID: "beta", SourcePortID: 0, DesNodeID: "alpha", DesPortID: 0},
		},
	})
	if err == nil {
		t.Fatal("CompileGraph unexpectedly accepted a data dependency cycle")
	}
	if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Fatalf("cycle error = %q, want both node IDs", err)
	}
}

func TestCompileGraphRejectsUnstructuredExecCycleInNewFormat(t *testing.T) {
	_, err := CompileGraph(validationRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "alpha", Class: "ExecStep"}, {ID: "beta", Class: "ExecStep"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "alpha", SourcePortID: 0, DesNodeID: "beta", DesPortID: 0},
			{SourceNodeID: "beta", SourcePortID: 0, DesNodeID: "alpha", DesPortID: 0},
		},
	})
	if err == nil {
		t.Fatal("CompileGraph unexpectedly accepted an unstructured exec cycle")
	}
}

func TestCompileGraphAllowsForLoopBreakBodyBackEdge(t *testing.T) {
	_, err := CompileGraph(validationRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "Entry_1"}, {ID: "loop", Class: "ForLoopBreak"}, {ID: "body", Class: "ExecStep"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "body", SourcePortID: 0, DesNodeID: "loop", DesPortID: 3},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph rejected structured ForLoopBreak back edge: %v", err)
	}
}

func TestCompileGraphAllowsDirectForLoopBreakBodyToBreak(t *testing.T) {
	_, err := CompileGraph(validationRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "loop", Class: "ForLoopBreak"}},
		Edges: []EdgeConfig{{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "loop", DesPortID: 3}},
	})
	if err != nil {
		t.Fatalf("CompileGraph rejected direct ForLoopBreak body-to-break edge: %v", err)
	}
}

func TestCompileGraphRejectsBreakInputCycleOutsideLoopBody(t *testing.T) {
	_, err := CompileGraph(validationRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "loop", Class: "ForLoopBreak"}, {ID: "after", Class: "ExecStep"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "loop", SourcePortID: 2, DesNodeID: "after", DesPortID: 0},
			{SourceNodeID: "after", SourcePortID: 0, DesNodeID: "loop", DesPortID: 3},
		},
	})
	if err == nil {
		t.Fatal("CompileGraph accepted a break-input cycle outside the loop body")
	}
}

func TestCompileGraphAllowsLegacyExecCycle(t *testing.T) {
	_, err := CompileGraph(validationRegistry(), GraphConfig{
		Legacy: true,
		Nodes:  []NodeConfig{{ID: "alpha", Class: "ExecStep"}, {ID: "beta", Class: "ExecStep"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "alpha", SourcePortID: 0, DesNodeID: "beta", DesPortID: 0},
			{SourceNodeID: "beta", SourcePortID: 0, DesNodeID: "alpha", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph rejected legacy exec cycle: %v", err)
	}
}

func TestCompileGraphRejectsInvalidStructure(t *testing.T) {
	tests := []struct {
		name   string
		config GraphConfig
	}{
		{name: "empty node id", config: GraphConfig{Nodes: []NodeConfig{{Class: "IntSource"}}}},
		{name: "duplicate node id", config: GraphConfig{Nodes: []NodeConfig{{ID: "same", Class: "IntSource"}, {ID: "same", Class: "IntSource"}}}},
		{name: "empty variable name", config: GraphConfig{Variables: []VariableConfig{{Type: "integer", Value: 1}}}},
		{name: "duplicate variable name", config: GraphConfig{Variables: []VariableConfig{{Name: "score", Type: "integer", Value: 1}, {Name: "score", Type: "integer", Value: 2}}}},
		{name: "invalid variable type", config: GraphConfig{Variables: []VariableConfig{{Name: "score", Type: "mystery", Value: 1}}}},
		{name: "invalid variable default", config: GraphConfig{Variables: []VariableConfig{{Name: "score", Type: "integer", Value: "bad"}}}},
		{name: "duplicate entrance id", config: GraphConfig{Nodes: []NodeConfig{{ID: "one", Class: "Entry_1"}, {ID: "two", Class: "Entry_1"}}}},
		{name: "source port out of bounds", config: GraphConfig{Nodes: []NodeConfig{{ID: "source", Class: "IntSource"}, {ID: "target", Class: "IntTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "source", SourcePortID: 1, DesNodeID: "target", DesPortID: 0}}}},
		{name: "nil builtin port", config: GraphConfig{Nodes: []NodeConfig{{ID: "source", Class: "NilSource"}, {ID: "target", Class: "IntTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "source", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}}}},
		{name: "destination port out of bounds", config: GraphConfig{Nodes: []NodeConfig{{ID: "source", Class: "IntSource"}, {ID: "target", Class: "IntTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "source", SourcePortID: 0, DesNodeID: "target", DesPortID: 1}}}},
		{name: "exec to data", config: GraphConfig{Nodes: []NodeConfig{{ID: "source", Class: "Entry_1"}, {ID: "target", Class: "IntTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "source", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}}}},
		{name: "data to exec", config: GraphConfig{Nodes: []NodeConfig{{ID: "source", Class: "IntSource"}, {ID: "target", Class: "ExecTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "source", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}}}},
		{name: "duplicate data producer", config: GraphConfig{Nodes: []NodeConfig{{ID: "left", Class: "IntSource"}, {ID: "right", Class: "IntSource"}, {ID: "target", Class: "IntTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "left", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}, {SourceNodeID: "right", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}}}},
		{name: "concrete type mismatch", config: GraphConfig{Nodes: []NodeConfig{{ID: "source", Class: "StringSource"}, {ID: "target", Class: "IntTarget"}}, Edges: []EdgeConfig{{SourceNodeID: "source", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CompileGraph(validationRegistry(), test.config); err == nil {
				t.Fatal("CompileGraph unexpectedly accepted invalid structure")
			}
		})
	}
}

func TestCompileGraphAllowsLegacyExecFanout(t *testing.T) {
	config := GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "Entry_1"}, {ID: "left", Class: "ExecTarget"}, {ID: "right", Class: "ExecTarget"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "left", DesPortID: 0}, {SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "right", DesPortID: 0}},
	}
	if _, err := CompileGraph(validationRegistry(), config); err != nil {
		t.Fatalf("CompileGraph rejected legacy exec fan-out: %v", err)
	}
}

func TestCompileGraphAllowsPreservedLegacyPortDirectionMismatch(t *testing.T) {
	config := GraphConfig{
		Legacy: true,
		Nodes:  []NodeConfig{{ID: "entry", Class: "Entry_1"}, {ID: "target", Class: "IntTarget"}},
		Edges:  []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "target", DesPortID: 0}},
	}
	if _, err := CompileGraph(validationRegistry(), config); err != nil {
		t.Fatalf("CompileGraph rejected preserved legacy mismatch: %v", err)
	}
}

func TestAssignPortValuePreservesTargetKindAndClonesValues(t *testing.T) {
	integerTarget := NewPortInt().(*Port)
	anySource := NewPortAny().(*Port)
	anySource.SetAny(PortInt(9))
	if err := assignPortValue(integerTarget, anySource); err != nil {
		t.Fatal(err)
	}
	if value, ok := integerTarget.GetInt(); !ok || value != 9 || integerTarget.kind != portKindInt {
		t.Fatalf("integer target = %d,%v kind=%d", value, ok, integerTarget.kind)
	}
	anySource.SetAny("bad")
	if err := assignPortValue(integerTarget, anySource); err == nil || integerTarget.kind != portKindInt {
		t.Fatalf("invalid any assignment error=%v kind=%d", err, integerTarget.kind)
	}

	anyTarget := NewPortAny().(*Port)
	integerSource := NewPortInt().(*Port)
	integerSource.SetInt(7)
	if err := assignPortValue(anyTarget, integerSource); err != nil {
		t.Fatal(err)
	}
	if value := anyTarget.GetAny(); value != PortInt(7) || anyTarget.kind != portKindAny {
		t.Fatalf("any target = %#v kind=%d", value, anyTarget.kind)
	}

	arraySource := NewPortArray().(*Port)
	arraySource.AppendArrayValInt(1)
	arrayTarget := NewPortArray().(*Port)
	if err := assignPortValue(arrayTarget, arraySource); err != nil {
		t.Fatal(err)
	}
	arraySource.arrv[0].IntVal = 99
	if value, ok := arrayTarget.GetArrayValInt(0); !ok || value != 1 {
		t.Fatalf("array target = %d,%v, want cloned 1", value, ok)
	}
}

func TestCompilerBuildsGraphFromNodeAndEdgeConfig(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("TestRecorder", func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entrance", Class: "TestEntrance_1"},
			{ID: "record", Class: "TestRecorder", PortDefault: map[int]any{1: 7}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entrance", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
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
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 7 {
		t.Fatalf("recorder values = %#v, want [7]", recorder)
	}
}

func TestCompilerReportsUnknownNodeClass(t *testing.T) {
	registry := NewRegistry()
	_, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "missing", Class: "MissingNode"}},
	})
	if err == nil {
		t.Fatalf("CompileGraph succeeded for unknown node class")
	}
}

func TestCompilerPrecomputesNodeIndexesAndDefaultPorts(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("TestRecorder", func() IExecNode {
		return &testRecorder{}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entrance", Class: "TestEntrance_1"},
			{ID: "record", Class: "TestRecorder", PortDefault: map[int]any{1: 7}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entrance", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	if compiled.NodeCount != 2 {
		t.Fatalf("NodeCount = %d, want 2", compiled.NodeCount)
	}
	entrance := compiled.Entrances[1]
	if entrance == nil || entrance.Index != 0 {
		t.Fatalf("entrance index = %#v, want 0", entrance)
	}
	record := entrance.Next[0]
	if record == nil || record.Index != 1 {
		t.Fatalf("record index = %#v, want 1", record)
	}
	if len(record.DefaultInputs) != 2 || len(record.DefaultInputSet) != 2 || !record.DefaultInputSet[1] {
		t.Fatalf("default input metadata = %#v %#v", record.DefaultInputs, record.DefaultInputSet)
	}
	value, ok := record.DefaultInputs[1].GetInt()
	if !ok || value != 7 {
		t.Fatalf("default input value = %d,%v want 7,true", value, ok)
	}
}

func TestNodeDefinitionPrecomputesDataInputIndexes(t *testing.T) {
	definition := NewNodeDefinition("MixedInputs", func() IExecNode {
		return &testRecorder{}
	}, []IPort{NewPortExec(), NewPortInt(), nil, NewPortStr()}, nil)

	want := []int{1, 3}
	if len(definition.DataInPortIndexes) != len(want) {
		t.Fatalf("DataInPortIndexes = %#v, want %#v", definition.DataInPortIndexes, want)
	}
	for index := range want {
		if definition.DataInPortIndexes[index] != want[index] {
			t.Fatalf("DataInPortIndexes = %#v, want %#v", definition.DataInPortIndexes, want)
		}
	}
}

func TestCompilerPrecomputesInputBindings(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("AddInt", func() IExecNode {
		return &AddInt{}
	}, []IPort{NewPortInt(), NewPortInt()}, []IPort{NewPortInt()}))
	registry.Register(NewNodeDefinition("TestRecorder", func() IExecNode {
		return &testRecorder{}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TestEntrance_1"},
			{ID: "add", Class: "AddInt", PortDefault: map[int]any{0: 2, 1: 5}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "add", SourcePortID: 0, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	record := compiled.Entrances[1].Next[0]
	if len(record.InputBindings) != 1 {
		t.Fatalf("record InputBindings = %#v, want one producer binding", record.InputBindings)
	}
	binding := record.InputBindings[0]
	if binding.Kind != InputBindingProducer || binding.InputPortID != 1 || binding.Producer == nil || binding.Producer.ID != "add" || binding.ProducerOutPortID != 0 || !binding.RecomputeProducer {
		t.Fatalf("record binding = %#v", binding)
	}

	add := binding.Producer
	if len(add.InputBindings) != 2 {
		t.Fatalf("add InputBindings = %#v, want two default bindings", add.InputBindings)
	}
	for index, binding := range add.InputBindings {
		if binding.Kind != InputBindingDefault || binding.InputPortID != index || binding.DefaultPort == nil {
			t.Fatalf("add binding[%d] = %#v", index, binding)
		}
		value, ok := binding.DefaultPort.GetInt()
		if !ok || value != PortInt([]int{2, 5}[index]) {
			t.Fatalf("add binding[%d] default = %d,%v", index, value, ok)
		}
	}
}

func TestCompilerIgnoresInvalidDefaultOnConnectedInput(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("ArraySource", func() IExecNode {
		return &testRecorder{}
	}, nil, []IPort{NewPortArray()}))
	registry.Register(NewNodeDefinition("ArrayConsumer", func() IExecNode {
		return &testRecorder{}
	}, []IPort{NewPortExec(), NewPortArray()}, nil))

	_, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TestEntrance_1"},
			{ID: "array", Class: "ArraySource"},
			{ID: "consumer", Class: "ArrayConsumer", PortDefault: map[int]any{1: false}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "consumer", DesPortID: 0},
			{SourceNodeID: "array", SourcePortID: 0, DesNodeID: "consumer", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph returned error for connected bad default: %v", err)
	}
}
