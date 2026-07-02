package blueprint

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestBuiltinFactoriesCoverAllTopLevelNodeDefinitions(t *testing.T) {
	registry := NewRegistry()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "nodes", "*.json"))
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

func TestTopLevelSystemNodeBehaviorCoverage(t *testing.T) {
	covered := map[string]bool{
		"GetArrayInt":          true,
		"GetArrayString":       true,
		"GetArrayLen":          true,
		"CreateIntArray":       true,
		"CreateStringArray":    true,
		"AppendStringToArray":  true,
		"AppendIntegerToArray": true,
		"AppendIntReturn":      true,
		"AppendStringReturn":   true,
		"AddInt":               true,
		"SubInt":               true,
		"MulInt":               true,
		"DivInt":               true,
		"ModInt":               true,
		"RandNumber":           true,
		"Sequence":             true,
		"Foreach":              true,
		"ForeachIntArray":      true,
		"BoolIf":               true,
		"GreaterThanInteger":   true,
		"LessThanInteger":      true,
		"EqualInteger":         true,
		"RangeCompare":         true,
		"EqualSwitch":          true,
		"Probability":          true,
		"Entrance_ArrayParam":  true,
		"Entrance_IntParam":    true,
		"Entrance_Timer":       true,
		"CreateTimer":          true,
		"CloseTimer":           true,
		"DebugOutput":          true,
	}

	registry := NewRegistry()
	if err := loadDefinitionDir(registry, filepath.Join("..", "..", "..", "nodes"), BuiltinExecNodeFactories()); err != nil {
		t.Fatalf("loadDefinitionDir failed: %v", err)
	}
	missing := make([]string, 0)
	for name := range registry.definitions {
		if !covered[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("top-level node behavior coverage is missing: %v", missing)
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

	randNode := &RandNumber{}
	ctx := bindNode(t, randNode, []IPort{intPort(99), intPort(10), intPort(20)}, []IPort{NewPortInt()})
	if _, err := randNode.Exec(); err != nil {
		t.Fatalf("RandNumber Exec failed: %v", err)
	}
	value, ok := ctx.OutputPorts[0].GetInt()
	if !ok || value < 10 || value > 20 {
		t.Fatalf("RandNumber output = %d,%v, want 10..20,true", value, ok)
	}
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

	strArray := arrayPortStrings("alpha", "beta")
	getString := &GetArrayString{}
	stringCtx := bindNode(t, getString, []IPort{strArray, intPort(1)}, []IPort{NewPortStr()})
	if _, err := getString.Exec(); err != nil {
		t.Fatalf("GetArrayString failed: %v", err)
	}
	if value, ok := stringCtx.OutputPorts[0].GetStr(); !ok || value != "beta" {
		t.Fatalf("GetArrayString value = %q,%v want beta,true", value, ok)
	}

	lenNode := &GetArrayLen{}
	lenCtx := bindNode(t, lenNode, []IPort{arrayPort(1, 2, 3)}, []IPort{NewPortInt()})
	if _, err := lenNode.Exec(); err != nil {
		t.Fatalf("GetArrayLen failed: %v", err)
	}
	if value, ok := lenCtx.OutputPorts[0].GetInt(); !ok || value != 3 {
		t.Fatalf("GetArrayLen value = %d,%v want 3,true", value, ok)
	}

	createString := &CreateStringArray{}
	createStringCtx := bindNode(t, createString, []IPort{arrayPortStrings("go", "lua")}, []IPort{NewPortArray()})
	if _, err := createString.Exec(); err != nil {
		t.Fatalf("CreateStringArray failed: %v", err)
	}
	if value, ok := createStringCtx.OutputPorts[0].GetArrayValStr(1); !ok || value != "lua" {
		t.Fatalf("CreateStringArray value = %q,%v want lua,true", value, ok)
	}

	appendString := &AppendStringToArray{}
	appendStringCtx := bindNode(t, appendString, []IPort{arrayPortStrings("go"), strPort("csharp")}, []IPort{NewPortArray()})
	if _, err := appendString.Exec(); err != nil {
		t.Fatalf("AppendStringToArray failed: %v", err)
	}
	if value, ok := appendStringCtx.OutputPorts[0].GetArrayValStr(1); !ok || value != "csharp" {
		t.Fatalf("AppendStringToArray value = %q,%v want csharp,true", value, ok)
	}
}

func TestBuiltinBranchNodes(t *testing.T) {
	assertNextIndex(t, &BoolIf{}, []IPort{NewPortExec(), boolPort(false)}, 0)
	assertNextIndex(t, &BoolIf{}, []IPort{NewPortExec(), boolPort(true)}, 1)
	assertNextIndex(t, &GreaterThanInteger{}, []IPort{NewPortExec(), boolPort(false), intPort(3), intPort(2)}, 1)
	assertNextIndex(t, &LessThanInteger{}, []IPort{NewPortExec(), boolPort(true), intPort(3), intPort(3)}, 1)
	assertNextIndex(t, &EqualInteger{}, []IPort{NewPortExec(), intPort(3), intPort(3)}, 1)
	assertNextIndexWithOutputs(t, &RangeCompare{}, []IPort{NewPortExec(), intPort(4), arrayPort(2, 5, 8)}, execPorts(6), 3)
	assertNextIndexWithOutputs(t, &EqualSwitch{}, []IPort{NewPortExec(), intPort(8), arrayPort(2, 5, 8)}, execPorts(6), 4)
	assertNextIndex(t, &Probability{}, []IPort{NewPortExec(), intPort(10000)}, 1)
}

func TestBuiltinEntranceTimerAndDebugNodes(t *testing.T) {
	for _, node := range []IExecNode{&EntranceIntParam{}, &EntranceArrayParam{}, &EntranceTimer{}, &DebugOutput{}} {
		t.Run(node.GetName(), func(t *testing.T) {
			bindNode(t, node, nil, []IPort{NewPortExec()})
			if next, err := node.Exec(); err != nil || next != 0 {
				t.Fatalf("%s Exec = %d,%v want 0,nil", node.GetName(), next, err)
			}
		})
	}

	timer := &CreateTimer{}
	timerCtx := bindNode(t, timer, []IPort{NewPortExec(), intPort(1000), arrayPort(7)}, []IPort{NewPortExec(), NewPortInt()})
	if next, err := timer.Exec(); err != nil || next != 0 {
		t.Fatalf("CreateTimer Exec = %d,%v want 0,nil", next, err)
	}
	timerID, ok := timerCtx.OutputPorts[1].GetInt()
	if !ok || timerID == 0 {
		t.Fatalf("CreateTimer timer id = %d,%v want non-zero,true", timerID, ok)
	}

	closeTimer := &CloseTimer{}
	bindNodeWithGraph(t, closeTimer, timer.graph, []IPort{NewPortExec(), intPort(timerID)}, []IPort{NewPortExec()})
	if next, err := closeTimer.Exec(); err != nil || next != 0 {
		t.Fatalf("CloseTimer Exec = %d,%v want 0,nil", next, err)
	}
}

func TestBuiltinReturnNodesAppendGraphResults(t *testing.T) {
	graph := NewGraph(&CompiledGraph{})
	appendInt := &AppendIntReturn{}
	bindNodeWithGraph(t, appendInt, graph, []IPort{NewPortExec(), intPort(42)}, []IPort{NewPortExec()})
	if next, err := appendInt.Exec(); err != nil || next != 0 {
		t.Fatalf("AppendIntReturn Exec = %d,%v want 0,nil", next, err)
	}

	appendString := &AppendStringReturn{}
	bindNodeWithGraph(t, appendString, graph, []IPort{NewPortExec(), strPort("done")}, []IPort{NewPortExec()})
	if next, err := appendString.Exec(); err != nil || next != 0 {
		t.Fatalf("AppendStringReturn Exec = %d,%v want 0,nil", next, err)
	}
	want := PortArray{{IntVal: 42}, {StrVal: "done"}}
	if len(graph.returns) != len(want) || graph.returns[0] != want[0] || graph.returns[1] != want[1] {
		t.Fatalf("returns = %#v, want %#v", graph.returns, want)
	}
}

func TestBuiltinIntInArrayIsRegisteredAndFindsValue(t *testing.T) {
	registry := NewRegistry()
	definitions := []byte(`[
		{"name":"Entrance_IntParam","inputs":[{"type":"exec","port_id":0}],"outputs":[{"type":"exec","port_id":0}]},
		{"name":"IntInArray","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"Integer","port_id":1},{"type":"data","data_type":"array","port_id":2}],"outputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"Boolean","port_id":1}]},
		{"name":"BoolIf","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"Boolean","port_id":1}],"outputs":[{"type":"exec","port_id":0},{"type":"exec","port_id":1}]},
		{"name":"AppendIntReturn","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"Integer","port_id":1}],"outputs":[{"type":"exec","port_id":0}]}
	]`)
	if err := registry.LoadDefinitionsJSON(definitions, BuiltinExecNodeFactories()); err != nil {
		t.Fatalf("LoadDefinitionsJSON failed: %v", err)
	}

	graph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "contains", Class: "IntInArray", PortDefault: map[int]any{1: 3, 2: []int64{1, 3, 5}}},
			{ID: "branch", Class: "BoolIf"},
			{ID: "return", Class: "AppendIntReturn", PortDefault: map[int]any{1: 1}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "contains", DesPortID: 0},
			{SourceNodeID: "contains", SourcePortID: 0, DesNodeID: "branch", DesPortID: 0},
			{SourceNodeID: "contains", SourcePortID: 1, DesNodeID: "branch", DesPortID: 1},
			{SourceNodeID: "branch", SourcePortID: 1, DesNodeID: "return", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(graph).Do(1)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if len(returns) != 1 || returns[0].IntVal != 1 {
		t.Fatalf("returns = %#v, want [1]", returns)
	}
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
	assertNextIndexWithOutputs(t, node, in, []IPort{NewPortExec(), NewPortExec()}, want)
}

func assertNextIndexWithOutputs(t *testing.T, node IExecNode, in []IPort, out []IPort, want int) {
	t.Helper()
	bindNode(t, node, in, out)
	got, err := node.Exec()
	if err != nil {
		t.Fatalf("%s Exec failed: %v", node.GetName(), err)
	}
	if got != want {
		t.Fatalf("%s next = %d, want %d", node.GetName(), got, want)
	}
}

func bindNode(t *testing.T, node IExecNode, in []IPort, out []IPort) *ExecContext {
	t.Helper()
	return bindNodeWithGraph(t, node, NewGraph(&CompiledGraph{}), in, out)
}

func bindNodeWithGraph(t *testing.T, node IExecNode, graph *Graph, in []IPort, out []IPort) *ExecContext {
	t.Helper()
	base := node.(interface {
		bind(*Graph, *ExecNode, *ExecContext)
	})
	ctx := &ExecContext{InputPorts: in, OutputPorts: out}
	base.bind(graph, NewExecNode("n", NewNodeDefinition(node.GetName(), func() IExecNode { return node }, clonePorts(in), clonePorts(out))), ctx)
	return ctx
}

func execPorts(count int) []IPort {
	ports := make([]IPort, count)
	for index := range ports {
		ports[index] = NewPortExec()
	}
	return ports
}

func intPort(value PortInt) IPort {
	port := NewPortInt()
	_ = port.setAnyValue(value)
	return port
}

func strPort(value PortString) IPort {
	port := NewPortStr()
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

func arrayPortStrings(values ...PortString) IPort {
	port := NewPortArray()
	array := make(PortArray, 0, len(values))
	for _, value := range values {
		array = append(array, ArrayData{StrVal: value})
	}
	_ = port.setAnyValue(array)
	return port
}
