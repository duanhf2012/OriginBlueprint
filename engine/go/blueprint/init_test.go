package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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
	if err := bp.Init(execDir, graphDir, nil); err != nil {
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

func TestBlueprintInitRejectsReinitializationWhileInstanceExists(t *testing.T) {
	var bp Blueprint
	bp.AddCompiledGraph("test", &CompiledGraph{})
	if graphID := bp.Create("test"); graphID == 0 {
		t.Fatal("Create returned 0")
	}

	err := bp.Init(filepath.Join(t.TempDir(), "missing-definitions"), filepath.Join(t.TempDir(), "missing-graphs"), nil)
	if err != ErrBlueprintInUse {
		t.Fatalf("Init error = %v, want %v", err, ErrBlueprintInUse)
	}
}

func TestBlueprintInitFailureDoesNotChangePublishedConfiguration(t *testing.T) {
	var bp Blueprint
	bp.execDefPath = "old-definitions"
	bp.graphPath = "old-graphs"
	oldGraph := &CompiledGraph{}
	bp.AddCompiledGraph("old", oldGraph)

	err := bp.Init(filepath.Join(t.TempDir(), "missing-definitions"), filepath.Join(t.TempDir(), "missing-graphs"), nil)
	if err == nil {
		t.Fatal("Init succeeded, want load error")
	}
	if bp.execDefPath != "old-definitions" || bp.graphPath != "old-graphs" {
		t.Fatalf("paths changed after failed Init: %q %q", bp.execDefPath, bp.graphPath)
	}
	if len(bp.graphs) != 1 || bp.graphs["old"] != oldGraph {
		t.Fatalf("graphs changed after failed Init: %#v", bp.graphs)
	}
}

func TestBlueprintRepeatedInitWithoutInstancesReplacesGraphPool(t *testing.T) {
	definitions := t.TempDir()
	graphs := t.TempDir()
	var bp Blueprint
	bp.AddCompiledGraph("old", &CompiledGraph{})

	if err := bp.Init(definitions, graphs, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if len(bp.graphs) != 0 {
		t.Fatalf("graphs = %#v, want complete replacement with empty pool", bp.graphs)
	}
}

func TestBlueprintInitAfterCloseReturnsStableError(t *testing.T) {
	var bp Blueprint
	if err := bp.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := bp.Init(t.TempDir(), t.TempDir(), nil); err != ErrBlueprintClosed {
		t.Fatalf("Init error = %v, want %v", err, ErrBlueprintClosed)
	}
}

func TestBlueprintHotReloadReplacesGraphsForExistingInstances(t *testing.T) {
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
	writeGraph := func(value int) {
		t.Helper()
		data := []byte(fmt.Sprintf(`{
			"nodes": [
				{"id":"entrance","class":"TestEntrance_1"},
				{"id":"record","class":"TestRecorder","port_defaultv":{"1":%d}}
			],
			"edges": [
				{"source_node_id":"entrance","des_node_id":"record","source_port_id":0,"des_port_id":0}
			]
		}`, value))
		if err := os.WriteFile(filepath.Join(graphDir, "test.vgf"), data, 0644); err != nil {
			t.Fatalf("WriteFile graph failed: %v", err)
		}
	}
	writeGraph(10)

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &testEntrance{} })
	bp.RegisterExecNode(func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphID := bp.Create("test")
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 10 {
		t.Fatalf("first recorder values = %#v, want [10]", recorder)
	}

	writeGraph(20)
	result, err := bp.HotReload()
	if err != nil {
		t.Fatalf("HotReload failed: %v", err)
	}
	if result == nil || result.GraphCount != 1 {
		t.Fatalf("HotReload result = %#v, want graph=1", result)
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("second Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 20 {
		t.Fatalf("second recorder values = %#v, want [20]", recorder)
	}
}

func TestHotReloadDoesNotMutateExistingInstance(t *testing.T) {
	oldGraph := &CompiledGraph{Entrances: map[int64]*ExecNode{}}
	var bp Blueprint
	bp.AddCompiledGraph("test", oldGraph)
	graphID := bp.Create("test")
	instance := bp.instances[graphID]
	newGraph := &CompiledGraph{Entrances: map[int64]*ExecNode{}}
	result := (&hotReloadPlan{blueprint: &bp, graphs: map[string]*CompiledGraph{"test": newGraph}, result: HotReloadResult{GraphCount: 1}}).apply()
	if result.GraphCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if bp.instances[graphID] != instance {
		t.Fatal("hot reload replaced GraphInstance identity")
	}
	if bp.graphs["test"] != newGraph {
		t.Fatal("hot reload did not replace compiled graph pool")
	}
}

func TestHotReloadRemovedGraphKeepsExistingInstanceOnly(t *testing.T) {
	compiled := &CompiledGraph{Entrances: map[int64]*ExecNode{}}
	var bp Blueprint
	bp.AddCompiledGraph("removed", compiled)
	graphID := bp.Create("removed")
	instance := bp.instances[graphID]
	result := (&hotReloadPlan{blueprint: &bp, graphs: map[string]*CompiledGraph{}, result: HotReloadResult{}}).apply()
	if result.GraphCount != 0 || bp.instances[graphID] != instance {
		t.Fatalf("result=%#v instance changed", result)
	}
	if _, err := bp.Start(nil, graphID, 1); err != ErrGraphNotFound {
		t.Fatalf("Start removed graph error = %v, want %v", err, ErrGraphNotFound)
	}
	if got := bp.Create("removed"); got != 0 {
		t.Fatalf("Create removed graph = %d, want 0", got)
	}
	(&hotReloadPlan{blueprint: &bp, graphs: map[string]*CompiledGraph{"removed": compiled}, result: HotReloadResult{GraphCount: 1}}).apply()
	if _, err := bp.Start(nil, graphID, 1); err != ErrEntranceNotFound {
		t.Fatalf("Start re-added graph error = %v, want %v", err, ErrEntranceNotFound)
	}
}

func TestBlueprintPrepareHotReloadAppliesOnlyWhenRequested(t *testing.T) {
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
	writeGraph := func(value int) {
		t.Helper()
		data := []byte(fmt.Sprintf(`{
			"nodes": [
				{"id":"entrance","class":"TestEntrance_1"},
				{"id":"record","class":"TestRecorder","port_defaultv":{"1":%d}}
			],
			"edges": [
				{"source_node_id":"entrance","des_node_id":"record","source_port_id":0,"des_port_id":0}
			]
		}`, value))
		if err := os.WriteFile(filepath.Join(graphDir, "test.vgf"), data, 0644); err != nil {
			t.Fatalf("WriteFile graph failed: %v", err)
		}
	}
	writeGraph(10)

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &testEntrance{} })
	bp.RegisterExecNode(func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphID := bp.Create("test")

	writeGraph(20)
	plan, err := bp.prepareHotReload()
	if err != nil {
		t.Fatalf("prepareHotReload failed: %v", err)
	}
	if plan == nil {
		t.Fatalf("prepareHotReload returned nil plan")
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do before apply failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 10 {
		t.Fatalf("before apply recorder values = %#v, want [10]", recorder)
	}

	result := plan.apply()
	if result.GraphCount != 1 {
		t.Fatalf("Apply result = %#v, want graph=1", result)
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do after apply failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 20 {
		t.Fatalf("after apply recorder values = %#v, want [20]", recorder)
	}
}

func TestBlueprintHotReloadFailureKeepsExistingGraphs(t *testing.T) {
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
	validGraph := []byte(`{
		"nodes": [
			{"id":"entrance","class":"TestEntrance_1"},
			{"id":"record","class":"TestRecorder","port_defaultv":{"1":10}}
		],
		"edges": [
			{"source_node_id":"entrance","des_node_id":"record","source_port_id":0,"des_port_id":0}
		]
	}`)
	if err := os.WriteFile(filepath.Join(graphDir, "test.vgf"), validGraph, 0644); err != nil {
		t.Fatalf("WriteFile valid graph failed: %v", err)
	}

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &testEntrance{} })
	bp.RegisterExecNode(func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphID := bp.Create("test")

	invalidGraph := []byte(`{
		"nodes": [
			{"id":"entrance","class":"TestEntrance_1"},
			{"id":"bad","class":"MissingNode"}
		],
		"edges": [
			{"source_node_id":"entrance","des_node_id":"bad","source_port_id":0,"des_port_id":0}
		]
	}`)
	if err := os.WriteFile(filepath.Join(graphDir, "test.vgf"), invalidGraph, 0644); err != nil {
		t.Fatalf("WriteFile invalid graph failed: %v", err)
	}
	if result, err := bp.HotReload(); err == nil || result != nil {
		t.Fatalf("HotReload = %#v,%v; want nil result and error", result, err)
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do after failed reload failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 10 {
		t.Fatalf("recorder values after failed reload = %#v, want [10]", recorder)
	}
}

func TestBlueprintHotReloadCanRunInGoroutineWithConcurrentDo(t *testing.T) {
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
	writeGraph := func(value int) {
		t.Helper()
		data := []byte(fmt.Sprintf(`{
			"nodes": [
				{"id":"entrance","class":"TestEntrance_1"},
				{"id":"record","class":"TestRecorder","port_defaultv":{"1":%d}}
			],
			"edges": [
				{"source_node_id":"entrance","des_node_id":"record","source_port_id":0,"des_port_id":0}
			]
		}`, value))
		if err := os.WriteFile(filepath.Join(graphDir, "test.vgf"), data, 0644); err != nil {
			t.Fatalf("WriteFile graph failed: %v", err)
		}
	}
	writeGraph(10)

	var bp Blueprint
	bp.RegisterExecNode(func() IExecNode { return &testEntrance{} })
	bp.RegisterExecNode(func() IExecNode { return &testRecorder{} })
	if err := bp.Init(execDir, graphDir, nil); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	graphIDs := make([]int64, 8)
	for index := range graphIDs {
		graphIDs[index] = bp.Create("test")
		if graphIDs[index] == 0 {
			t.Fatalf("Create %d returned 0", index)
		}
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, graphID := range graphIDs {
		graphID := graphID
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if _, err := bp.Do(graphID, 1); err != nil {
						t.Errorf("Do graph %d failed: %v", graphID, err)
						return
					}
				}
			}
		}()
	}

	writeGraph(20)
	done := make(chan struct {
		result *HotReloadResult
		err    error
	}, 1)
	go func() {
		result, err := bp.HotReload()
		done <- struct {
			result *HotReloadResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case hotReload := <-done:
		if hotReload.err != nil {
			t.Fatalf("HotReload failed: %v", hotReload.err)
		}
		if hotReload.result == nil || hotReload.result.GraphCount != 1 {
			t.Fatalf("HotReload result = %#v, want graph=1", hotReload.result)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("HotReload timed out")
	}
	close(stop)
	wg.Wait()
}
