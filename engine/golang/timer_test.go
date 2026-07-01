package golang

import (
	"sync"
	"testing"
	"time"
)

type testBlueprintModule struct {
	mu               sync.Mutex
	triggeredGraphID int64
	triggeredEventID int64
}

func (m *testBlueprintModule) SafeAfterFunc(timerID *uint64, d time.Duration, data any, cb func(uint64, any)) {
	*timerID = 99
	time.AfterFunc(d, func() { cb(*timerID, data) })
}

func (m *testBlueprintModule) TriggerEvent(graphID int64, eventID int64, args ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triggeredGraphID = graphID
	m.triggeredEventID = eventID
	return nil
}

func (m *testBlueprintModule) CancelTimerId(graphID int64, timerID *uint64) bool {
	return true
}

func TestCreateTimerContinuesImmediatelyAndTriggersTimerEntrance(t *testing.T) {
	module := &testBlueprintModule{}
	create := &CreateTimer{}
	ctx := &ExecContext{
		InputPorts:  []IPort{NewPortExec(), intPort(1), arrayPort(7)},
		OutputPorts: []IPort{NewPortExec(), NewPortInt()},
	}
	graph := NewGraph(&CompiledGraph{})
	graph.graphID = 123
	graph.module = module
	create.bind(graph, NewExecNode("timer", NewNodeDefinition("CreateTimer", func() IExecNode { return create }, clonePorts(ctx.InputPorts), clonePorts(ctx.OutputPorts))), ctx)

	next, err := create.Exec()
	if err != nil {
		t.Fatalf("CreateTimer failed: %v", err)
	}
	if next != 0 {
		t.Fatalf("CreateTimer next = %d, want 0", next)
	}
	if timerID, ok := ctx.OutputPorts[1].GetInt(); !ok || timerID != 99 {
		t.Fatalf("timer id = %d,%v want 99,true", timerID, ok)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		graphID, eventID := module.triggered()
		if eventID != 0 {
			if graphID != 123 || eventID != EntranceIDTimer {
				t.Fatalf("triggered graph/event = %d/%d", graphID, eventID)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timer did not trigger event")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestBlueprintCancelTimerIdUsesLegacyCancelCallback(t *testing.T) {
	var canceled uint64
	var bp Blueprint
	bp.cancelTimer = func(timerID *uint64) bool {
		canceled = *timerID
		return true
	}
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{}})
	graphID := bp.Create("test")

	timerID := uint64(77)
	if !bp.CancelTimerId(graphID, &timerID) {
		t.Fatalf("CancelTimerId returned false")
	}
	if canceled != 77 {
		t.Fatalf("canceled timer = %d, want 77", canceled)
	}
}

func TestBlueprintConcurrentTimerCreateCancelAndRelease(t *testing.T) {
	module := &testBlueprintModule{}
	var bp Blueprint
	bp.module = module
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{}})
	graphID := bp.Create("test")
	instance := bp.instances[graphID]

	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := 0; index < 100; index++ {
				create := &CreateTimer{}
				ctx := &ExecContext{
					InputPorts:  []IPort{NewPortExec(), intPort(1), arrayPort(7)},
					OutputPorts: []IPort{NewPortExec(), NewPortInt()},
				}
				graph := NewGraph(&CompiledGraph{})
				graph.graphID = graphID
				graph.module = module
				graph.instance = instance
				create.bind(graph, NewExecNode("timer", NewNodeDefinition("CreateTimer", func() IExecNode { return create }, clonePorts(ctx.InputPorts), clonePorts(ctx.OutputPorts))), ctx)
				if _, err := create.Exec(); err != nil {
					t.Errorf("CreateTimer failed: %v", err)
					return
				}
				timerID := uint64(99)
				_ = bp.CancelTimerId(graphID, &timerID)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for index := 0; index < 100; index++ {
			bp.ReleaseGraph(graphID)
		}
	}()
	wg.Wait()
}

func (m *testBlueprintModule) triggered() (int64, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.triggeredGraphID, m.triggeredEventID
}
