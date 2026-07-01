package golang

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlueprintLegacyFacadeIntegrationPath(t *testing.T) {
	root := t.TempDir()
	execDir := filepath.Join(root, "nodes")
	graphDir := filepath.Join(root, "graphs")
	if err := os.Mkdir(execDir, 0755); err != nil {
		t.Fatalf("Mkdir execDir failed: %v", err)
	}
	if err := os.Mkdir(graphDir, 0755); err != nil {
		t.Fatalf("Mkdir graphDir failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(execDir, "compat.json"), []byte(`[
		{"name":"TestEntrance","inputs":[],"outputs":[{"type":"exec","port_id":0}]},
		{"name":"AddInt","inputs":[{"type":"data","data_type":"int","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[{"type":"data","data_type":"int","port_id":0}]},
		{"name":"TestRecorder","inputs":[{"type":"exec","port_id":0},{"type":"data","data_type":"int","port_id":1}],"outputs":[]}
	]`), 0644); err != nil {
		t.Fatalf("WriteFile definitions failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "server.vgf"), []byte(`{
		"nodes": [
			{"id":"entry","class":"TestEntrance_1"},
			{"id":"add","class":"AddInt","port_defaultv":{"0":11,"1":22}},
			{"id":"record","class":"TestRecorder"}
		],
		"edges": [
			{"source_node_id":"entry","des_node_id":"record","source_port_id":0,"des_port_id":0},
			{"source_node_id":"add","des_node_id":"record","source_port_id":0,"des_port_id":1}
		]
	}`), 0644); err != nil {
		t.Fatalf("WriteFile graph failed: %v", err)
	}

	logger := &testTraceLogger{}
	recorder := &testRecorder{}
	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &testEntrance{} })
	bp.RegisterExecNode(func() IExecNode { return &AddInt{} })
	bp.RegisterExecNode(func() IExecNode { return recorder })
	if err := bp.Init(execDir, graphDir, nil, nil, logger); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if bp.GetLogger() != logger {
		t.Fatalf("GetLogger did not return Init logger")
	}

	applyHotReload, err := bp.StartHotReload()
	if err != nil {
		t.Fatalf("StartHotReload failed: %v", err)
	}
	if applyHotReload == nil {
		t.Fatalf("StartHotReload returned nil apply function")
	}
	applyHotReload()

	graphID := bp.Create("server")
	if graphID == 0 {
		t.Fatalf("Create returned 0")
	}
	if graphName := bp.GetGraphName(graphID); graphName != "server" {
		t.Fatalf("GetGraphName = %q, want server", graphName)
	}

	bp.SetTraceEnabled(true)
	if err := bp.TriggerEvent(graphID, 1); err != nil {
		t.Fatalf("TriggerEvent failed: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 || values[0] != 33 {
		t.Fatalf("recorder values = %#v, want [33]", values)
	}
	if len(logger.events) == 0 {
		t.Fatalf("trace logger did not receive events")
	}

	bp.ReleaseGraph(graphID)
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do after ReleaseGraph returned error: %v", err)
	}
	if values := recorder.snapshot(); len(values) != 1 {
		t.Fatalf("recorder values after release = %#v, want unchanged", values)
	}
}

func TestBlueprintReleaseGraphCancelsInstanceTimersThroughLegacyCallback(t *testing.T) {
	var canceled []uint64
	var bp Blueprint
	bp.cancelTimer = func(timerID *uint64) bool {
		canceled = append(canceled, *timerID)
		return true
	}
	bp.AddCompiledGraph("compat", &CompiledGraph{Entrances: map[int64]*ExecNode{}})

	graphID := bp.Create("compat")
	if graphID == 0 {
		t.Fatalf("Create returned 0")
	}

	instance := bp.instances[graphID]
	instance.timerMu.Lock()
	instance.timers[77] = struct{}{}
	instance.timers[88] = struct{}{}
	instance.timerMu.Unlock()

	bp.ReleaseGraph(graphID)

	if len(canceled) != 2 {
		t.Fatalf("canceled timers = %#v, want two timer ids", canceled)
	}
	if !containsTimerID(canceled, 77) || !containsTimerID(canceled, 88) {
		t.Fatalf("canceled timers = %#v, want 77 and 88", canceled)
	}
}

func containsTimerID(values []uint64, want uint64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
