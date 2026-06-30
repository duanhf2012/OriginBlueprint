package golang

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlueprintInitLoadsDefinitionsAndGraphsFromDirectories(t *testing.T) {
	var recorder *testRecorder
	root := t.TempDir()
	execDir := filepath.Join(root, "json")
	graphDir := filepath.Join(root, "vgf")
	if err := os.Mkdir(execDir, 0755); err != nil {
		t.Fatalf("Mkdir execDir failed: %v", err)
	}
	if err := os.Mkdir(graphDir, 0755); err != nil {
		t.Fatalf("Mkdir graphDir failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(execDir, "nodes.json"), []byte(`[
		{"name":"TestEntrance","inputs":[],"outputs":[{"type":"exec","port_id":0}]},
		{"name":"TestRecorder","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[]}
	]`), 0644); err != nil {
		t.Fatalf("WriteFile nodes failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "test.vgf"), []byte(`{
		"nodes": [
			{"id":"entrance","class":"TestEntrance_1"},
			{"id":"record","class":"TestRecorder","port_defaultv":{"1":13}}
		],
		"edges": [
			{"source_node_id":"entrance","des_node_id":"record","source_port_id":0,"des_port_id":0}
		]
	}`), 0644); err != nil {
		t.Fatalf("WriteFile graph failed: %v", err)
	}

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &testEntrance{} })
	bp.RegisterExecNode(func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})
	if err := bp.Init(execDir, graphDir, nil, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	graphID := bp.Create("test")
	if graphID == 0 {
		t.Fatalf("Create returned 0")
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 13 {
		t.Fatalf("recorder values = %#v, want [13]", recorder)
	}
}
