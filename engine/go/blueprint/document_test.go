package blueprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"
)

const passthroughFunctionDocument = `{
  "schemaVersion": 1,
  "graphName": "Passthrough",
  "functionId": "functions/Passthrough.obpf",
  "nodes": [
    {
      "id": "entry",
      "typeId": "origin.function.entry",
      "values": {},
      "properties": {
        "functionName": "Passthrough",
        "functionSignature": {
          "inputs": [{"id": "input", "name": "Input", "type": "integer"}],
          "outputs": [{"id": "result", "name": "Result", "type": "integer"}]
        }
      }
    },
    {
      "id": "return",
      "typeId": "origin.function.return",
      "values": {},
      "properties": {
        "functionName": "Passthrough",
        "functionSignature": {
          "inputs": [{"id": "input", "name": "Input", "type": "integer"}],
          "outputs": [{"id": "result", "name": "Result", "type": "integer"}]
        }
      }
    }
  ],
  "connections": [
    {"source": "entry", "sourceOutput": "exec", "target": "return", "targetInput": "exec"},
    {"source": "entry", "sourceOutput": "input_input", "target": "return", "targetInput": "output_result"}
  ],
  "variables": [],
  "variableGroups": [],
  "view": {"x": 0, "y": 0, "zoom": 1},
  "functionSignature": {
    "inputs": [{"id": "input", "name": "Input", "type": "integer"}],
    "outputs": [{"id": "result", "name": "Result", "type": "integer"}]
  }
}`

func TestParseGraphDocumentFunctionCanBeCalled(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})

	functionConfig, err := ParseGraphConfigJSON([]byte(passthroughFunctionDocument))
	if err != nil {
		t.Fatalf("ParseGraphConfigJSON failed: %v", err)
	}
	functionGraph, err := CompileGraph(registry, functionConfig)
	if err != nil {
		t.Fatalf("CompileGraph function failed: %v", err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"Passthrough": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "Passthrough", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 31}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph main failed: %v", err)
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 31 {
		t.Fatalf("recorder values = %#v, want [31]", recorder)
	}
}

func TestLoadGraphDirLoadsOBPFFunctionsForGraphCalls(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})

	root := t.TempDir()
	functionDir := filepath.Join(root, "functions")
	if err := os.MkdirAll(functionDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(functionDir, "Passthrough.obpf"), []byte(passthroughFunctionDocument), 0644); err != nil {
		t.Fatalf("WriteFile function failed: %v", err)
	}
	main := `{
		"nodes": [
			{"id":"entry","class":"Entrance_IntParam_1"},
			{"id":"call","class":"FunctionCall","functionName":"Passthrough","functionInputTypes":["Integer"],"functionOutputTypes":["Integer"],"port_defaultv":{"1":52}},
			{"id":"record","class":"TestRecorder"}
		],
		"edges": [
			{"source_node_id":"entry","des_node_id":"call","source_port_id":0,"des_port_id":0},
			{"source_node_id":"call","des_node_id":"record","source_port_id":0,"des_port_id":0},
			{"source_node_id":"call","des_node_id":"record","source_port_id":1,"des_port_id":1}
		]
	}`
	if err := os.WriteFile(filepath.Join(root, "main.vgf"), []byte(main), 0644); err != nil {
		t.Fatalf("WriteFile main failed: %v", err)
	}

	graphs, err := loadGraphDir(registry, root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	graph := NewGraph(graphs["main"])
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 52 {
		t.Fatalf("recorder values = %#v, want [52]", recorder)
	}
}

func TestLoadGraphDirResolvesFunctionCallsByStableFunctionID(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})

	root := t.TempDir()
	functionDir := filepath.Join(root, "functions")
	if err := os.MkdirAll(functionDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(functionDir, "Passthrough.obpf"), []byte(passthroughFunctionDocument), 0644); err != nil {
		t.Fatalf("WriteFile function failed: %v", err)
	}
	main := `{
	  "schemaVersion": 1,
	  "graphName": "Main",
	  "nodes": [
	    {"id":"entry","typeId":"origin.event.entry-two-integers","values":{}},
	    {
	      "id":"call",
	      "typeId":"origin.function.call",
	      "values":{"input_input":61},
	      "properties":{
	        "functionId":"functions/Passthrough.obpf",
	        "functionName":"DisplayNameMayChange",
	        "functionSignature":{
	          "inputs":[{"id":"input","name":"Input","type":"integer"}],
	          "outputs":[{"id":"result","name":"Result","type":"integer"}]
	        }
	      }
	    },
	    {"id":"record","typeId":"origin.legacy.placeholder","properties":{
	      "legacyClass":"TestRecorder",
	      "legacyInputs":[{"key":"exec","type":"exec"},{"key":"value","type":"integer"}],
	      "legacyOutputs":[]
	    }}
	  ],
	  "connections": [
	    {"source":"entry","sourceOutput":"exec","target":"call","targetInput":"exec"},
	    {"source":"call","sourceOutput":"exec","target":"record","targetInput":"exec"},
	    {"source":"call","sourceOutput":"output_result","target":"record","targetInput":"value"}
	  ],
	  "variables": [],
	  "variableGroups": [],
	  "view": {"x":0,"y":0,"zoom":1}
	}`
	if err := os.WriteFile(filepath.Join(root, "main.obp"), []byte(main), 0644); err != nil {
		t.Fatalf("WriteFile main failed: %v", err)
	}

	graphs, err := loadGraphDir(registry, root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	graph := NewGraph(graphs["Main"])
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 61 {
		t.Fatalf("recorder values = %#v, want [61]", recorder)
	}
}

func TestGraphDocumentToConfigMapsDynamicBranchCasesFromOne(t *testing.T) {
	config, _, err := graphDocumentToConfig(graphDocument{
		Nodes: []graphDocumentNode{
			{
				ID:     "switch",
				TypeID: "origin.flow.equal-switch-new",
				Values: map[string]any{"cases": []any{1, 2, 3, 4, 5, 6}},
			},
			{
				ID:     "record",
				TypeID: "origin.result.append-integer",
			},
		},
		Connections: []graphDocumentConnection{
			{Source: "switch", SourceOutput: "case1", Target: "record", TargetInput: "exec"},
			{Source: "switch", SourceOutput: "case6", Target: "record", TargetInput: "exec"},
		},
	})
	if err != nil {
		t.Fatalf("graphDocumentToConfig failed: %v", err)
	}
	if len(config.Edges) != 2 {
		t.Fatalf("edges = %#v", config.Edges)
	}
	if config.Edges[0].SourcePortID != 2 || config.Edges[1].SourcePortID != 7 {
		t.Fatalf("dynamic branch source ports = %#v", config.Edges)
	}
	if _, ok := documentNodeSpecs["origin.flow.equal-switch-new"].outputs["case0"]; ok {
		t.Fatal("dynamic branch specs should not expose hidden case0")
	}
}

func TestGraphDocumentDynamicSequenceOutputCompilesAndRuns(t *testing.T) {
	config, _, err := graphDocumentToConfig(graphDocument{
		Nodes: []graphDocumentNode{
			{
				ID:     "entry",
				TypeID: "origin.event.entry-two-integers",
			},
			{
				ID:     "sequence",
				TypeID: "origin.flow.sequence",
				Properties: graphDocumentProperties{
					DynamicOutputCount: 5,
				},
			},
			{
				ID:     "record",
				TypeID: "origin.result.append-string",
				Values: map[string]any{"value": "then4"},
			},
		},
		Connections: []graphDocumentConnection{
			{Source: "entry", SourceOutput: "exec", Target: "sequence", TargetInput: "exec"},
			{Source: "sequence", SourceOutput: "then4", Target: "record", TargetInput: "exec"},
		},
	})
	if err != nil {
		t.Fatalf("graphDocumentToConfig failed: %v", err)
	}

	registry := NewRegistry()
	if err := loadDefinitionDir(registry, filepath.Join("..", "..", "..", "nodes"), BuiltinExecNodeFactories()); err != nil {
		t.Fatalf("loadDefinitionDir failed: %v", err)
	}
	graph, err := CompileGraph(registry, config)
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(graph).Do(1)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if len(returns) != 1 || returns[0].StrVal != "then4" {
		t.Fatalf("returns = %#v, want then4", returns)
	}
}

func TestAllNativeDocumentNodeSpecsCompile(t *testing.T) {
	registry := NewRegistry()
	if err := loadDefinitionDir(registry, filepath.Join("..", "..", "..", "nodes"), BuiltinExecNodeFactories()); err != nil {
		t.Fatalf("loadDefinitionDir failed: %v", err)
	}
	for typeID, spec := range documentNodeSpecs {
		t.Run(typeID, func(t *testing.T) {
			_, err := CompileGraph(registry, GraphConfig{
				Nodes: []NodeConfig{{ID: "node", Class: spec.class}},
			})
			if err != nil {
				t.Fatalf("CompileGraph failed for %s/%s: %v", typeID, spec.class, err)
			}
		})
	}
}

func TestTopLevelJSONNodeSchemasMatchDocumentExecutionContracts(t *testing.T) {
	registry := NewRegistry()
	if err := loadDefinitionDir(registry, filepath.Join("..", "..", "..", "nodes"), BuiltinExecNodeFactories()); err != nil {
		t.Fatalf("loadDefinitionDir failed: %v", err)
	}
	for _, schema := range loadTopLevelNodeSchemas(t) {
		t.Run(schema.ID, func(t *testing.T) {
			assertDynamicBranchContract(t, schema)
			_, spec, err := documentNodeToConfig(graphDocumentNode{
				ID:     "node",
				TypeID: schema.ID,
				Values: map[string]any{},
			}, nil)
			if err != nil {
				t.Fatalf("documentNodeToConfig failed: %v", err)
			}
			if got, want := sortedStringKeys(spec.inputs), schemaInputKeys(schema); !reflect.DeepEqual(got, want) {
				t.Fatalf("input contract keys = %v, want %v", got, want)
			}
			if got, want := sortedIntMapKeys(spec.outputs), schemaOutputKeys(schema); !reflect.DeepEqual(got, want) {
				t.Fatalf("output contract keys = %v, want %v", got, want)
			}
			if _, err := CompileGraph(registry, GraphConfig{Nodes: []NodeConfig{{ID: "node", Class: spec.class}}}); err != nil {
				t.Fatalf("CompileGraph failed for %s/%s: %v", schema.ID, spec.class, err)
			}
		})
	}
}

func TestRemovedFileTableDictionaryDocumentNodesAreUnsupported(t *testing.T) {
	removedTypeIDs := []string{
		"origin.io.file-path",
		"origin.io.read-text",
		"origin.io.save-text",
		"origin.table.read-csv",
		"origin.table.preview",
		"origin.dictionary.set",
		"origin.flow.foreach-table-row",
	}
	for _, typeID := range removedTypeIDs {
		if _, ok := documentNodeSpecs[typeID]; ok {
			t.Fatalf("documentNodeSpecs still contains removed node %s", typeID)
		}
	}

	registry := NewRegistry()
	for _, className := range []string{"FilePath", "ReadText", "TablePreview", "DictionarySet", "ForeachTableRow"} {
		t.Run(className, func(t *testing.T) {
			_, err := CompileGraph(registry, GraphConfig{
				Nodes: []NodeConfig{{ID: "node", Class: className}},
			})
			if err == nil {
				t.Fatalf("CompileGraph unexpectedly accepted removed class %s", className)
			}
		})
	}
}

type jsonNodeSchemaContract struct {
	ID            string                     `json:"id"`
	Inputs        []jsonPortSchemaContract   `json:"inputs"`
	Outputs       []jsonPortSchemaContract   `json:"outputs"`
	DynamicBranch *jsonDynamicBranchContract `json:"dynamicBranch"`
}

type jsonPortSchemaContract struct {
	Key string `json:"key"`
}

type jsonDynamicBranchContract struct {
	ControlInput     string `json:"controlInput"`
	DefaultOutput    string `json:"defaultOutput"`
	OutputPrefix     string `json:"outputPrefix"`
	OutputStartIndex int    `json:"outputStartIndex"`
	MaxBranches      int    `json:"maxBranches"`
}

func assertDynamicBranchContract(t *testing.T, schema jsonNodeSchemaContract) {
	t.Helper()
	branch := schema.DynamicBranch
	if branch == nil {
		return
	}
	if branch.ControlInput == "" || !stringSliceContains(schemaInputKeys(schema), branch.ControlInput) {
		t.Fatalf("dynamicBranch controlInput %q is not declared as an input", branch.ControlInput)
	}
	fixedOutputs := make([]string, 0, len(schema.Outputs))
	for _, port := range schema.Outputs {
		fixedOutputs = append(fixedOutputs, port.Key)
	}
	if branch.DefaultOutput == "" || !stringSliceContains(fixedOutputs, branch.DefaultOutput) {
		t.Fatalf("dynamicBranch defaultOutput %q is not declared as a fixed output", branch.DefaultOutput)
	}
	if branch.OutputPrefix == "" || branch.OutputStartIndex < 0 || branch.MaxBranches <= 0 {
		t.Fatalf("invalid dynamicBranch output range: %#v", branch)
	}
}

func loadTopLevelNodeSchemas(t *testing.T) []jsonNodeSchemaContract {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "nodes", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	result := make([]jsonNodeSchemaContract, 0)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", file, err)
		}
		var schemas []jsonNodeSchemaContract
		if err := json.Unmarshal(data, &schemas); err != nil {
			t.Fatalf("Unmarshal %s failed: %v", file, err)
		}
		for _, schema := range schemas {
			if schema.ID != "" {
				result = append(result, schema)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func schemaInputKeys(schema jsonNodeSchemaContract) []string {
	keys := make([]string, 0, len(schema.Inputs))
	for _, port := range schema.Inputs {
		if port.Key != "" {
			keys = append(keys, port.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func schemaOutputKeys(schema jsonNodeSchemaContract) []string {
	keys := make([]string, 0, len(schema.Outputs))
	for _, port := range schema.Outputs {
		if port.Key != "" {
			keys = append(keys, port.Key)
		}
	}
	if branch := schema.DynamicBranch; branch != nil {
		for index := 0; index < branch.MaxBranches; index++ {
			keys = append(keys, branch.OutputPrefix+strconv.Itoa(branch.OutputStartIndex+index))
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntMapKeys(values map[string]int) []string {
	return sortedStringKeys(values)
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
