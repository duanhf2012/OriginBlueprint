package blueprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyBlueprintFileRunsComplexBranchAndNestedLoopFlow(t *testing.T) {
	root := t.TempDir()
	graphFile := filepath.Join(root, "complex.vgf")
	writeTestFile(t, graphFile, `{
		"nodes": [
			{"id":"entry","class":"Entrance_IntParam_000001"},
			{"id":"sequence","class":"Sequence"},
			{"id":"equal","class":"EqualInteger"},
			{"id":"not_equal","class":"AppendStringReturn","port_defaultv":{"1":"not-equal"}},
			{"id":"outer","class":"Foreach","port_defaultv":{"1":0,"2":3}},
			{"id":"array","class":"CreateIntArray","port_defaultv":{"0":[10,20]}},
			{"id":"inner","class":"ForeachIntArray"},
			{"id":"sum","class":"AddInt"},
			{"id":"append_sum","class":"AppendIntReturn"},
			{"id":"range","class":"RangeCompare","port_defaultv":{"1":4,"2":[3,6]}},
			{"id":"range_hit","class":"AppendStringReturn","port_defaultv":{"1":"range-hit"}},
			{"id":"switch","class":"EqualSwitch","port_defaultv":{"1":7,"2":[1,7]}},
			{"id":"switch_hit","class":"AppendStringReturn","port_defaultv":{"1":"switch-hit"}}
		],
		"edges": [
			{"source_node_id":"entry","source_port_id":0,"des_node_id":"sequence","des_port_id":0},
			{"source_node_id":"sequence","source_port_id":0,"des_node_id":"equal","des_port_id":0},
			{"source_node_id":"entry","source_port_id":2,"des_node_id":"equal","des_port_id":1},
			{"source_node_id":"entry","source_port_id":3,"des_node_id":"equal","des_port_id":2},
			{"source_node_id":"equal","source_port_id":0,"des_node_id":"not_equal","des_port_id":0},
			{"source_node_id":"sequence","source_port_id":1,"des_node_id":"outer","des_port_id":0},
			{"source_node_id":"outer","source_port_id":0,"des_node_id":"inner","des_port_id":0},
			{"source_node_id":"array","source_port_id":0,"des_node_id":"inner","des_port_id":1},
			{"source_node_id":"inner","source_port_id":0,"des_node_id":"append_sum","des_port_id":0},
			{"source_node_id":"outer","source_port_id":2,"des_node_id":"sum","des_port_id":0},
			{"source_node_id":"inner","source_port_id":3,"des_node_id":"sum","des_port_id":1},
			{"source_node_id":"sum","source_port_id":0,"des_node_id":"append_sum","des_port_id":1},
			{"source_node_id":"sequence","source_port_id":2,"des_node_id":"range","des_port_id":0},
			{"source_node_id":"range","source_port_id":3,"des_node_id":"range_hit","des_port_id":0},
			{"source_node_id":"range_hit","source_port_id":0,"des_node_id":"switch","des_port_id":0},
			{"source_node_id":"switch","source_port_id":3,"des_node_id":"switch_hit","des_port_id":0}
		]
	}`)

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	returns, err := NewGraph(graphs["complex"]).Do(1, PortInt(100), PortInt(5), PortInt(7))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	want := []ArrayData{
		{StrVal: "not-equal"},
		{IntVal: 10}, {IntVal: 20},
		{IntVal: 11}, {IntVal: 21},
		{IntVal: 12}, {IntVal: 22},
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

func TestNativeBlueprintFileCallsFunctionAndContinuesFlow(t *testing.T) {
	root := t.TempDir()
	functionDir := filepath.Join(root, "functions")
	if err := os.MkdirAll(functionDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	writeTestFile(t, filepath.Join(functionDir, "Calc.obpf"), `{
		"schemaVersion": 1,
		"graphName": "Calc",
		"functionId": "functions/Calc.obpf",
		"nodes": [
			{"id":"entry","typeId":"origin.function.entry","properties":{"functionSignature":{"inputs":[{"id":"a","name":"A","type":"float"},{"id":"b","name":"B","type":"float"}],"outputs":[{"id":"text","name":"Text","type":"string"}]}}},
			{"id":"add","typeId":"origin.math.add-float"},
			{"id":"cast","typeId":"origin.cast.float-string"},
			{"id":"return","typeId":"origin.function.return","properties":{"functionSignature":{"inputs":[{"id":"a","name":"A","type":"float"},{"id":"b","name":"B","type":"float"}],"outputs":[{"id":"text","name":"Text","type":"string"}]}}}
		],
		"connections": [
			{"source":"entry","sourceOutput":"exec","target":"return","targetInput":"exec"},
			{"source":"entry","sourceOutput":"input_a","target":"add","targetInput":"a"},
			{"source":"entry","sourceOutput":"input_b","target":"add","targetInput":"b"},
			{"source":"add","sourceOutput":"result","target":"cast","targetInput":"value"},
			{"source":"cast","sourceOutput":"result","target":"return","targetInput":"output_text"}
		],
		"variables": [],
		"variableGroups": [],
		"view": {"x":0,"y":0,"zoom":1},
		"functionSignature": {"inputs":[{"id":"a","name":"A","type":"float"},{"id":"b","name":"B","type":"float"}],"outputs":[{"id":"text","name":"Text","type":"string"}]}
	}`)
	writeTestFile(t, filepath.Join(root, "main.obp"), `{
		"schemaVersion": 1,
		"graphName": "Main",
		"nodes": [
			{"id":"entry","typeId":"origin.event.entry-two-integers","values":{}},
			{"id":"call","typeId":"origin.function.call","values":{"input_a":2.5,"input_b":3.75},"properties":{"functionId":"functions/Calc.obpf","functionName":"Calc","functionSignature":{"inputs":[{"id":"a","name":"A","type":"float"},{"id":"b","name":"B","type":"float"}],"outputs":[{"id":"text","name":"Text","type":"string"}]}}},
			{"id":"append","typeId":"origin.result.append-string"}
		],
		"connections": [
			{"source":"entry","sourceOutput":"exec","target":"call","targetInput":"exec"},
			{"source":"call","sourceOutput":"exec","target":"append","targetInput":"exec"},
			{"source":"call","sourceOutput":"output_text","target":"append","targetInput":"value"}
		],
		"variables": [],
		"variableGroups": [],
		"view": {"x":0,"y":0,"zoom":1}
	}`)

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	returns, err := NewGraph(graphs["Main"]).Do(1)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if len(returns) != 1 || returns[0].StrVal != "6.25" {
		t.Fatalf("returns = %#v, want [6.25]", returns)
	}
}

func TestLegacyBlueprintFileBreaksForLoopAndContinuesCompletedFlow(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "break.vgf"), `{
		"nodes": [
			{"id":"entry","class":"Entrance_IntParam_000001"},
			{"id":"loop","class":"ForLoopBreak","port_defaultv":{"1":0,"2":10}},
			{"id":"gt","class":"CompareGreaterInteger","port_defaultv":{"1":2}},
			{"id":"branch","class":"BoolIf"},
			{"id":"append_index","class":"AppendIntReturn"},
			{"id":"done","class":"AppendStringReturn","port_defaultv":{"1":"done"}}
		],
		"edges": [
			{"source_node_id":"entry","source_port_id":0,"des_node_id":"loop","des_port_id":0},
			{"source_node_id":"loop","source_port_id":0,"des_node_id":"branch","des_port_id":0},
			{"source_node_id":"loop","source_port_id":1,"des_node_id":"gt","des_port_id":0},
			{"source_node_id":"gt","source_port_id":0,"des_node_id":"branch","des_port_id":1},
			{"source_node_id":"branch","source_port_id":0,"des_node_id":"append_index","des_port_id":0},
			{"source_node_id":"loop","source_port_id":1,"des_node_id":"append_index","des_port_id":1},
			{"source_node_id":"branch","source_port_id":1,"des_node_id":"loop","des_port_id":3},
			{"source_node_id":"loop","source_port_id":2,"des_node_id":"done","des_port_id":0}
		]
	}`)

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	returns, err := NewGraph(graphs["break"]).Do(1)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	want := []ArrayData{{IntVal: 0}, {IntVal: 1}, {IntVal: 2}, {StrVal: "done"}}
	if len(returns) != len(want) {
		t.Fatalf("returns = %#v, want %#v", returns, want)
	}
	for index := range want {
		if returns[index] != want[index] {
			t.Fatalf("returns[%d] = %#v, want %#v; all returns %#v", index, returns[index], want[index], returns)
		}
	}
}

func testSystemRegistry(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := loadDefinitionDir(registry, filepath.Join("..", "..", "..", "nodes"), BuiltinExecNodeFactories()); err != nil {
		t.Fatalf("loadDefinitionDir failed: %v", err)
	}
	return registry
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s failed: %v", path, err)
	}
}
