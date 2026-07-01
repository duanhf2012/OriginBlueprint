package golang

import (
	"os"
	"path/filepath"
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

func TestAllNativeDocumentNodeSpecsCompile(t *testing.T) {
	registry := NewRegistry()
	if err := loadDefinitionDir(registry, filepath.Join("..", "..", "nodes"), BuiltinExecNodeFactories()); err != nil {
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
