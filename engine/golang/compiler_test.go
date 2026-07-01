package golang

import "testing"

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
