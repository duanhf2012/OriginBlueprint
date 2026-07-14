package blueprint

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestGraphInstanceReleaseDoesNotWaitForInFlightLease(t *testing.T) {
	instance := &GraphInstance{releasedCh: make(chan struct{})}
	if !instance.tryAcquireLease() {
		t.Fatal("initial lease was rejected")
	}
	done := make(chan struct{})
	go func() {
		instance.markReleased()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("release waited for in-flight lease")
	}
	if instance.tryAcquireLease() {
		t.Fatal("new lease acquired after release")
	}
	instance.releaseLease()
}

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
	if _, err := bp.Do(graphID, 1); !errors.Is(err, ErrGraphNotFound) {
		t.Fatalf("Do after release error = %v, want ErrGraphNotFound", err)
	}
}

func TestBlueprintCreateMissingGraphReturnsZero(t *testing.T) {
	var bp Blueprint
	if graphID := bp.Create("missing"); graphID != 0 {
		t.Fatalf("Create missing graph = %d, want 0", graphID)
	}
}

func TestBlueprintFacadeMethodsRemainAvailable(t *testing.T) {
	var bp Blueprint

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
