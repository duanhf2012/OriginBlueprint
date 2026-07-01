package golang

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinFactoriesCoverAllTopLevelNodeDefinitions(t *testing.T) {
	registry := NewRegistry()
	files, err := filepath.Glob(filepath.Join("..", "..", "nodes", "*.json"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no top-level node definition files found")
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", file, err)
		}
		if err := registry.LoadDefinitionsJSON(data, BuiltinExecNodeFactories()); err != nil {
			t.Fatalf("LoadDefinitionsJSON %s failed: %v", filepath.Base(file), err)
		}
	}

	for _, name := range []string{
		"GetArrayInt", "GetArrayString", "GetArrayLen", "CreateIntArray", "CreateStringArray",
		"AppendStringToArray", "AppendIntegerToArray", "AppendIntReturn", "AppendStringReturn",
		"Entrance_ArrayParam", "Entrance_IntParam", "Entrance_Timer",
		"CreateTimer", "CloseTimer",
		"AddInt", "SubInt", "MulInt", "DivInt", "ModInt", "RandNumber",
		"Sequence", "Foreach", "ForeachIntArray", "BoolIf", "GreaterThanInteger",
		"LessThanInteger", "EqualInteger", "RangeCompare", "EqualSwitch", "Probability",
		"DebugOutput",
	} {
		if registry.Get(name) == nil {
			t.Fatalf("builtin definition %s was not registered", name)
		}
	}
}

func TestDefinitionLoaderSkipsNodesJSONSubdirectory(t *testing.T) {
	root := t.TempDir()
	nodesDir := filepath.Join(root, "nodes")
	jsonDir := filepath.Join(nodesDir, "json")
	if err := os.MkdirAll(jsonDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nodesDir, "ok.json"), []byte(`[{"name":"TestEntrance","inputs":[],"outputs":[{"type":"exec","port_id":0}]}]`), 0644); err != nil {
		t.Fatalf("WriteFile ok failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jsonDir, "ignored.json"), []byte(`[{"name":"MissingIgnored","inputs":[],"outputs":[]}]`), 0644); err != nil {
		t.Fatalf("WriteFile ignored failed: %v", err)
	}

	registry := NewRegistry()
	if err := loadDefinitionDir(registry, nodesDir, []func() IExecNode{func() IExecNode { return &testEntrance{} }}); err != nil {
		t.Fatalf("loadDefinitionDir failed: %v", err)
	}
	if registry.Get("TestEntrance") == nil {
		t.Fatalf("TestEntrance was not loaded")
	}
}

func TestBuiltinMathNodes(t *testing.T) {
	assertPureIntNode(t, &AddInt{}, []IPort{intPort(2), intPort(3)}, 5)
	assertPureIntNode(t, &SubInt{}, []IPort{intPort(2), intPort(5), boolPort(true)}, 3)
	assertPureIntNode(t, &MulInt{}, []IPort{intPort(4), intPort(5)}, 20)
	assertPureIntNode(t, &DivInt{}, []IPort{intPort(5), intPort(2), boolPort(true)}, 3)
	assertPureIntNode(t, &ModInt{}, []IPort{intPort(5), intPort(2)}, 1)
}

func TestBuiltinArrayNodes(t *testing.T) {
	create := &CreateIntArray{}
	ctx := &ExecContext{InputPorts: []IPort{arrayPort(1, 2)}, OutputPorts: []IPort{NewPortArray()}}
	create.bind(NewGraph(&CompiledGraph{}), NewExecNode("create", NewNodeDefinition("CreateIntArray", func() IExecNode { return create }, []IPort{NewPortArray()}, []IPort{NewPortArray()})), ctx)
	if _, err := create.Exec(); err != nil {
		t.Fatalf("CreateIntArray failed: %v", err)
	}

	appendNode := &AppendIntegerToArray{}
	appendCtx := &ExecContext{InputPorts: []IPort{ctx.OutputPorts[0], intPort(3)}, OutputPorts: []IPort{NewPortArray()}}
	appendNode.bind(NewGraph(&CompiledGraph{}), NewExecNode("append", NewNodeDefinition("AppendIntegerToArray", func() IExecNode { return appendNode }, []IPort{NewPortArray(), NewPortInt()}, []IPort{NewPortArray()})), appendCtx)
	if _, err := appendNode.Exec(); err != nil {
		t.Fatalf("AppendIntegerToArray failed: %v", err)
	}

	get := &GetArrayInt{}
	getCtx := &ExecContext{InputPorts: []IPort{appendCtx.OutputPorts[0], intPort(2)}, OutputPorts: []IPort{NewPortInt()}}
	get.bind(NewGraph(&CompiledGraph{}), NewExecNode("get", NewNodeDefinition("GetArrayInt", func() IExecNode { return get }, []IPort{NewPortArray(), NewPortInt()}, []IPort{NewPortInt()})), getCtx)
	if _, err := get.Exec(); err != nil {
		t.Fatalf("GetArrayInt failed: %v", err)
	}
	if value, ok := getCtx.OutputPorts[0].GetInt(); !ok || value != 3 {
		t.Fatalf("GetArrayInt value = %d,%v want 3,true", value, ok)
	}
}

func TestBuiltinBranchNodes(t *testing.T) {
	assertNextIndex(t, &BoolIf{}, []IPort{NewPortExec(), boolPort(false)}, 0)
	assertNextIndex(t, &BoolIf{}, []IPort{NewPortExec(), boolPort(true)}, 1)
	assertNextIndex(t, &GreaterThanInteger{}, []IPort{NewPortExec(), boolPort(false), intPort(3), intPort(2)}, 1)
	assertNextIndex(t, &LessThanInteger{}, []IPort{NewPortExec(), boolPort(true), intPort(3), intPort(3)}, 1)
	assertNextIndex(t, &EqualInteger{}, []IPort{NewPortExec(), intPort(3), intPort(3)}, 1)
}

func assertPureIntNode(t *testing.T, node IExecNode, in []IPort, want PortInt) {
	t.Helper()
	base := node.(interface {
		bind(*Graph, *ExecNode, *ExecContext)
	})
	ctx := &ExecContext{InputPorts: in, OutputPorts: []IPort{NewPortInt()}}
	base.bind(NewGraph(&CompiledGraph{}), NewExecNode("n", NewNodeDefinition(node.GetName(), func() IExecNode { return node }, clonePorts(in), []IPort{NewPortInt()})), ctx)
	if _, err := node.Exec(); err != nil {
		t.Fatalf("%s Exec failed: %v", node.GetName(), err)
	}
	got, ok := ctx.OutputPorts[0].GetInt()
	if !ok || got != want {
		t.Fatalf("%s output = %d,%v want %d,true", node.GetName(), got, ok, want)
	}
}

func assertNextIndex(t *testing.T, node IExecNode, in []IPort, want int) {
	t.Helper()
	base := node.(interface {
		bind(*Graph, *ExecNode, *ExecContext)
	})
	ctx := &ExecContext{InputPorts: in, OutputPorts: []IPort{NewPortExec(), NewPortExec()}}
	base.bind(NewGraph(&CompiledGraph{}), NewExecNode("n", NewNodeDefinition(node.GetName(), func() IExecNode { return node }, clonePorts(in), []IPort{NewPortExec(), NewPortExec()})), ctx)
	got, err := node.Exec()
	if err != nil {
		t.Fatalf("%s Exec failed: %v", node.GetName(), err)
	}
	if got != want {
		t.Fatalf("%s next = %d, want %d", node.GetName(), got, want)
	}
}

func intPort(value PortInt) IPort {
	port := NewPortInt()
	_ = port.setAnyValue(value)
	return port
}

func boolPort(value bool) IPort {
	port := NewPortBool()
	_ = port.setAnyValue(value)
	return port
}

func arrayPort(values ...PortInt) IPort {
	port := NewPortArray()
	array := make(PortArray, 0, len(values))
	for _, value := range values {
		array = append(array, ArrayData{IntVal: value})
	}
	_ = port.setAnyValue(array)
	return port
}
