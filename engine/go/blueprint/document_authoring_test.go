package blueprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestAuthoredOBPFromNodeOperationsRunsInGoEngine(t *testing.T) {
	root := t.TempDir()

	graph := newTestDocumentAuthor("GeneratedMain")
	graph.AddNode("entry", "origin.event.entry-two-integers")
	graph.AddNode("sequence", "origin.flow.sequence").SetDynamicOutputCount("sequence", 3)
	graph.AddNode("add", "origin.math.add-integer")
	graph.AddNode("append_sum", "origin.result.append-integer")
	graph.AddNode("items", "origin.array.create-integer-new").SetValue("items", "items", []any{1, 2, 3})
	graph.AddNode("foreach", "origin.flow.foreach-integer-array")
	graph.AddNode("append_each", "origin.result.append-integer")
	graph.AddNode("range", "origin.flow.range-compare").SetValue("range", "value", 4).SetValue("range", "ranges", []any{3, 6, 10})
	graph.AddNode("range_hit", "origin.result.append-string").SetValue("range_hit", "value", "range-hit")
	graph.AddNode("switch", "origin.flow.equal-switch-new").SetValue("switch", "value", 7).SetValue("switch", "cases", []any{1, 7})
	graph.AddNode("switch_hit", "origin.result.append-string").SetValue("switch_hit", "value", "switch-hit")

	graph.Connect("entry", "exec", "sequence", "exec")
	graph.Connect("sequence", "then0", "append_sum", "exec")
	graph.Connect("entry", "param1", "add", "a")
	graph.Connect("entry", "param2", "add", "b")
	graph.Connect("add", "result", "append_sum", "value")
	graph.Connect("sequence", "then1", "foreach", "exec")
	graph.Connect("items", "array", "foreach", "array")
	graph.Connect("foreach", "body", "append_each", "exec")
	graph.Connect("foreach", "value", "append_each", "value")
	graph.Connect("sequence", "then2", "range", "exec")
	graph.Connect("range", "case2", "range_hit", "exec")
	graph.Connect("range_hit", "exec", "switch", "exec")
	graph.Connect("switch", "case2", "switch_hit", "exec")

	graph.SaveOBP(t, filepath.Join(root, "generated.obp"))

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	returns, err := NewGraph(graphs["GeneratedMain"]).Do(EntranceIDIntParam, PortInt(99), PortInt(5), PortInt(7))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	want := PortArray{
		{IntVal: 12},
		{IntVal: 1},
		{IntVal: 2},
		{IntVal: 3},
		{StrVal: "range-hit"},
		{StrVal: "switch-hit"},
	}
	if len(returns) != len(want) {
		t.Fatalf("returns = %#v, want %#v", returns, want)
	}
	for index := range want {
		if returns[index] != want[index] {
			t.Fatalf("returns[%d] = %#v, want %#v; all returns %#v", index, returns[index], want[index], returns)
		}
	}
}

func TestAuthoredFunctionCanCallAnotherAuthoredFunction(t *testing.T) {
	root := t.TempDir()
	functionDir := filepath.Join(root, "functions")
	if err := os.MkdirAll(functionDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	doubleSig := graphDocumentFuncSignature{
		Inputs:  []graphDocumentFuncPort{{ID: "value", Name: "Value", Type: "integer"}},
		Outputs: []graphDocumentFuncPort{{ID: "result", Name: "Result", Type: "integer"}},
	}
	double := newTestFunctionAuthor("Double", "functions/Double.obpf", doubleSig)
	double.AddFunctionEntry("entry", doubleSig)
	double.AddNode("add", "origin.math.add-integer")
	double.AddFunctionReturn("return", doubleSig)
	double.Connect("entry", "exec", "return", "exec")
	double.Connect("entry", "input_value", "add", "a")
	double.Connect("entry", "input_value", "add", "b")
	double.Connect("add", "result", "return", "output_result")
	double.SaveOBPF(t, filepath.Join(functionDir, "Double.obpf"))

	formatSig := graphDocumentFuncSignature{
		Inputs:  []graphDocumentFuncPort{{ID: "value", Name: "Value", Type: "integer"}},
		Outputs: []graphDocumentFuncPort{{ID: "text", Name: "Text", Type: "string"}},
	}
	format := newTestFunctionAuthor("FormatDouble", "functions/FormatDouble.obpf", formatSig)
	format.AddFunctionEntry("entry", formatSig)
	format.AddFunctionCall("double", "functions/Double.obpf", "Double", doubleSig)
	format.AddNode("cast", "origin.cast.integer-string")
	format.AddFunctionReturn("return", formatSig)
	format.Connect("entry", "exec", "double", "exec")
	format.Connect("entry", "input_value", "double", "input_value")
	format.Connect("double", "output_result", "cast", "value")
	format.Connect("double", "exec", "return", "exec")
	format.Connect("cast", "result", "return", "output_text")
	format.SaveOBPF(t, filepath.Join(functionDir, "FormatDouble.obpf"))

	main := newTestDocumentAuthor("Main")
	main.AddNode("entry", "origin.event.entry-two-integers")
	main.AddFunctionCall("format", "functions/FormatDouble.obpf", "FormatDouble", formatSig).SetValue("format", "input_value", 21)
	main.AddNode("append", "origin.result.append-string")
	main.Connect("entry", "exec", "format", "exec")
	main.Connect("format", "exec", "append", "exec")
	main.Connect("format", "output_text", "append", "value")
	main.SaveOBP(t, filepath.Join(root, "main.obp"))

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	returns, err := NewGraph(graphs["Main"]).Do(EntranceIDIntParam)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if len(returns) != 1 || returns[0].StrVal != "42" {
		t.Fatalf("returns = %#v, want [42]", returns)
	}
}

func TestNodeJSONDefinitionsHaveCompatibilityCoverage(t *testing.T) {
	topLevelSchemas := loadTopLevelNodeSchemas(t)
	if len(topLevelSchemas) == 0 {
		t.Fatalf("no top-level nodes/*.json schemas found")
	}
	missing := make([]string, 0)
	for _, schema := range topLevelSchemas {
		if _, ok := documentNodeSpecs[schema.ID]; !ok {
			missing = append(missing, schema.ID)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("nodes/*.json schemas missing document execution contracts: %v", missing)
	}

	businessRoot := filepath.Join("..", "..", "..", "nodes", "json")
	businessFiles := make([]string, 0)
	err := filepath.WalkDir(businessRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		businessFiles = append(businessFiles, path)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("WalkDir business schemas failed: %v", err)
	}
	if len(businessFiles) == 0 {
		t.Log("nodes/json/**/*.json is absent in this workspace; business schema execution coverage is skipped")
		return
	}
	sort.Strings(businessFiles)
	for _, file := range businessFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", file, err)
		}
		var schemas []jsonNodeSchemaContract
		if err := json.Unmarshal(data, &schemas); err != nil {
			t.Fatalf("Unmarshal %s failed: %v", file, err)
		}
		for _, schema := range schemas {
			if schema.ID == "" && schema.Name == "" {
				t.Fatalf("%s contains a schema without id or name", file)
			}
		}
	}
}

type testDocumentAuthor struct {
	document graphDocument
	nodes    map[string]int
}

func newTestDocumentAuthor(name string) *testDocumentAuthor {
	return &testDocumentAuthor{
		document: graphDocument{SchemaVersion: 1, GraphName: name},
		nodes:    map[string]int{},
	}
}

func newTestFunctionAuthor(name string, functionID string, signature graphDocumentFuncSignature) *testDocumentAuthor {
	author := newTestDocumentAuthor(name)
	author.document.FunctionID = functionID
	author.document.FunctionSignature = signature
	return author
}

func (a *testDocumentAuthor) AddNode(id string, typeID string) *testDocumentAuthor {
	a.nodes[id] = len(a.document.Nodes)
	a.document.Nodes = append(a.document.Nodes, graphDocumentNode{ID: id, TypeID: typeID, Values: map[string]any{}})
	return a
}

func (a *testDocumentAuthor) AddFunctionEntry(id string, signature graphDocumentFuncSignature) *testDocumentAuthor {
	a.AddNode(id, "origin.function.entry")
	a.document.Nodes[a.nodes[id]].Properties.FunctionSignature = signature
	return a
}

func (a *testDocumentAuthor) AddFunctionReturn(id string, signature graphDocumentFuncSignature) *testDocumentAuthor {
	a.AddNode(id, "origin.function.return")
	a.document.Nodes[a.nodes[id]].Properties.FunctionSignature = signature
	return a
}

func (a *testDocumentAuthor) AddFunctionCall(id string, functionID string, functionName string, signature graphDocumentFuncSignature) *testDocumentAuthor {
	a.AddNode(id, "origin.function.call")
	node := &a.document.Nodes[a.nodes[id]]
	node.Properties.FunctionID = functionID
	node.Properties.FunctionName = functionName
	node.Properties.FunctionSignature = signature
	return a
}

func (a *testDocumentAuthor) SetValue(nodeID string, key string, value any) *testDocumentAuthor {
	node := &a.document.Nodes[a.nodes[nodeID]]
	if node.Values == nil {
		node.Values = map[string]any{}
	}
	node.Values[key] = value
	return a
}

func (a *testDocumentAuthor) SetDynamicOutputCount(nodeID string, count int) *testDocumentAuthor {
	a.document.Nodes[a.nodes[nodeID]].Properties.DynamicOutputCount = count
	return a
}

func (a *testDocumentAuthor) Connect(source string, sourceOutput string, target string, targetInput string) *testDocumentAuthor {
	a.document.Connections = append(a.document.Connections, graphDocumentConnection{
		Source:       source,
		SourceOutput: sourceOutput,
		Target:       target,
		TargetInput:  targetInput,
	})
	return a
}

func (a *testDocumentAuthor) SaveOBP(t *testing.T, path string) {
	t.Helper()
	a.save(t, path)
}

func (a *testDocumentAuthor) SaveOBPF(t *testing.T, path string) {
	t.Helper()
	a.save(t, path)
}

func (a *testDocumentAuthor) save(t *testing.T, path string) {
	t.Helper()
	data, err := json.MarshalIndent(a.document, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile %s failed: %v", path, err)
	}
}
