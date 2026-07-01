package blueprint

import (
	"sync"
	"testing"
)

func TestBlueprintCreateAndDoUsesCompiledGraph(t *testing.T) {
	var recorder *testRecorder
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	record := NewExecNode("record", NewNodeDefinition("TestRecorder", func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	}, []IPort{NewPortExec(), NewPortInt()}, nil))
	entrance.Next = []*ExecNode{record}
	record.BeConnect = true

	var bp Blueprint
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}})

	graphID := bp.Create("test")
	if graphID == 0 {
		t.Fatalf("Create returned 0")
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil {
		t.Fatalf("recorder did not execute")
	}
}

func TestBlueprintReleaseGraphRemovesInstance(t *testing.T) {
	var bp Blueprint
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{1: NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))}})

	graphID := bp.Create("test")
	if graphID == 0 {
		t.Fatalf("Create returned 0")
	}

	bp.ReleaseGraph(graphID)
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do after release returned error: %v", err)
	}
}

func TestBlueprintCreateMissingGraphReturnsZero(t *testing.T) {
	var bp Blueprint
	if graphID := bp.Create("missing"); graphID != 0 {
		t.Fatalf("Create missing graph = %d, want 0", graphID)
	}
}

func TestBlueprintLegacyFacadeMethodsRemainAvailable(t *testing.T) {
	var bp Blueprint
	stop, err := bp.StartHotReload()
	if err != nil {
		t.Fatalf("StartHotReload failed: %v", err)
	}
	if stop == nil {
		t.Fatalf("StartHotReload returned nil stop function")
	}
	stop()

	logger := struct{}{}
	bp.logger = logger
	if got := bp.GetLogger(); got != logger {
		t.Fatalf("GetLogger = %#v, want logger", got)
	}
}

func TestBlueprintConcurrentCreateDoReleaseAndLookup(t *testing.T) {
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))

	var bp Blueprint
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 1})

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := 0; index < 200; index++ {
				graphID := bp.Create("test")
				if graphID == 0 {
					t.Errorf("Create returned 0")
					return
				}
				if _, err := bp.Do(graphID, 1); err != nil {
					t.Errorf("Do failed: %v", err)
					return
				}
				_ = bp.GetGraphName(graphID)
				bp.ReleaseGraph(graphID)
				_, _ = bp.Do(graphID, 1)
			}
		}()
	}
	wg.Wait()
}
