package blueprint

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type manualScheduledTimer struct {
	mu       sync.Mutex
	canceled bool
	fired    bool
	delay    time.Duration
	callback func()
}

func (timer *manualScheduledTimer) Cancel() bool {
	timer.mu.Lock()
	defer timer.mu.Unlock()
	if timer.canceled || timer.fired {
		return false
	}
	timer.canceled = true
	return true
}

func (timer *manualScheduledTimer) fire() bool {
	timer.mu.Lock()
	if timer.canceled || timer.fired {
		timer.mu.Unlock()
		return false
	}
	timer.fired = true
	callback := timer.callback
	timer.mu.Unlock()
	callback()
	return true
}

type manualTimerScheduler struct {
	mu     sync.Mutex
	timers []*manualScheduledTimer
}

func (scheduler *manualTimerScheduler) Schedule(delay time.Duration, callback func()) (ScheduledTimer, error) {
	timer := &manualScheduledTimer{delay: delay, callback: callback}
	scheduler.mu.Lock()
	scheduler.timers = append(scheduler.timers, timer)
	scheduler.mu.Unlock()
	return timer, nil
}

func (scheduler *manualTimerScheduler) delays() []time.Duration {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	delays := make([]time.Duration, len(scheduler.timers))
	for index, timer := range scheduler.timers {
		delays[index] = timer.delay
	}
	return delays
}

func (scheduler *manualTimerScheduler) fireNext() bool {
	scheduler.mu.Lock()
	timers := append([]*manualScheduledTimer(nil), scheduler.timers...)
	scheduler.mu.Unlock()
	for _, timer := range timers {
		if timer.fire() {
			return true
		}
	}
	return false
}

type timerTestEntry struct{ BaseExecNode }

func (n *timerTestEntry) GetName() string    { return "TimerTestEntry" }
func (n *timerTestEntry) Exec() (int, error) { return 0, nil }

type timerTestCapture struct {
	BaseExecNode
	label string
	calls chan timerTestCall
}
type timerTestCall struct {
	label string
	value int64
}

func (n *timerTestCapture) GetName() string { return n.label }
func (n *timerTestCapture) Exec() (int, error) {
	value, _ := n.GetInPortInt(1)
	n.calls <- timerTestCall{label: n.label, value: value}
	return -1, nil
}

func timerTestRegistry(calls chan timerTestCall) *Registry {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("TimerTestEntry", func() IExecNode { return &timerTestEntry{} }, nil, []IPort{NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("CreateTimer", func() IExecNode { return &CreateTimerNode{} }, []IPort{NewPortExec(), NewPortInt(), NewPortBool(), NewPortInt(), NewPortStr()}, []IPort{NewPortExec(), NewPortCallback()}))
	registry.Register(NewNodeDefinition("ClearTimerByKey", func() IExecNode { return &ClearTimerByKeyNode{} }, []IPort{NewPortExec(), NewPortStr()}, []IPort{NewPortExec(), NewPortBool()}))
	registry.Register(NewNodeDefinition("Delay", func() IExecNode { return &DelayNode{} }, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("CaptureA", func() IExecNode { return &timerTestCapture{label: "A", calls: calls} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("CaptureB", func() IExecNode { return &timerTestCapture{label: "B", calls: calls} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("CreatedA", func() IExecNode { return &timerTestCapture{label: "created-a", calls: calls} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("CreatedB", func() IExecNode { return &timerTestCapture{label: "created-b", calls: calls} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("TimerPass", func() IExecNode { return &timerTestEntry{} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	return registry
}

func addTimerTestGraph(t *testing.T, compiled *CompiledGraph, scheduler TimerScheduler) (*Blueprint, int64) {
	t.Helper()
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(NewInlineExecutionDispatcher())
	blueprint.SetTimerScheduler(scheduler)
	blueprint.AddCompiledGraph("timer-test", compiled)
	graphID := blueprint.Create("timer-test")
	if graphID == 0 {
		t.Fatal("Create returned zero graph ID")
	}
	t.Cleanup(func() { _ = blueprint.Close() })
	return blueprint, graphID
}

func waitTimerTestExecution(t *testing.T, execution *Execution) error {
	t.Helper()
	select {
	case <-execution.Done():
		_, err := execution.Result()
		return err
	case <-time.After(time.Second):
		t.Fatal("execution did not finish")
		return nil
	}
}

func TestCreateTimerStartsIndependentCallbacksWithCapturedValues(t *testing.T) {
	calls := make(chan timerTestCall, 4)
	scheduler := &manualTimerScheduler{}
	compiled, err := CompileGraph(timerTestRegistry(calls), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry-a", Class: "TimerTestEntry_1"},
			{ID: "entry-b", Class: "TimerTestEntry_2"},
			{ID: "timer-a", Class: "CreateTimer", TimerKey: "spawn-60", PortDefault: map[int]any{1: int64(60000), 2: false, 3: int64(-1), 4: "spawn-60"}},
			{ID: "timer-b", Class: "CreateTimer", TimerKey: "spawn-120", PortDefault: map[int]any{1: int64(120000), 2: false, 3: int64(-1), 4: "spawn-120"}},
			{ID: "capture-a", Class: "CaptureA"},
			{ID: "capture-b", Class: "CaptureB"},
			{ID: "created-a", Class: "CreatedA"},
			{ID: "created-b", Class: "CreatedB"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry-a", SourcePortID: 0, DesNodeID: "timer-a", DesPortID: 0},
			{SourceNodeID: "timer-a", SourcePortID: 1, DesNodeID: "capture-a", DesPortID: 0},
			{SourceNodeID: "timer-a", SourcePortID: 0, DesNodeID: "created-a", DesPortID: 0},
			{SourceNodeID: "entry-a", SourcePortID: 1, DesNodeID: "capture-a", DesPortID: 1},
			{SourceNodeID: "entry-b", SourcePortID: 0, DesNodeID: "timer-b", DesPortID: 0},
			{SourceNodeID: "timer-b", SourcePortID: 1, DesNodeID: "capture-b", DesPortID: 0},
			{SourceNodeID: "timer-b", SourcePortID: 0, DesNodeID: "created-b", DesPortID: 0},
			{SourceNodeID: "entry-b", SourcePortID: 1, DesNodeID: "capture-b", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	blueprint, graphID := addTimerTestGraph(t, compiled, scheduler)
	first, err := blueprint.Start(t.Context(), graphID, 1, int64(11))
	if err != nil || waitTimerTestExecution(t, first) != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	second, err := blueprint.Start(t.Context(), graphID, 2, int64(22))
	if err != nil || waitTimerTestExecution(t, second) != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	if firstCreated, secondCreated := <-calls, <-calls; firstCreated.label != "created-a" || secondCreated.label != "created-b" {
		t.Fatalf("Created branches = %#v, %#v", firstCreated, secondCreated)
	}
	if !scheduler.fireNext() || !scheduler.fireNext() {
		t.Fatal("expected two scheduled callbacks")
	}
	got := map[string]int64{}
	for range 2 {
		select {
		case call := <-calls:
			got[call.label] = call.value
		case <-time.After(time.Second):
			t.Fatal("callback did not execute")
		}
	}
	if got["A"] != 11 || got["B"] != 22 {
		t.Fatalf("captured values = %#v, want A=11 B=22", got)
	}
}

func TestCreateTimerRejectsDuplicateRuntimeKeyAndClearCancels(t *testing.T) {
	calls := make(chan timerTestCall, 1)
	scheduler := &manualTimerScheduler{}
	compiled, err := CompileGraph(timerTestRegistry(calls), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "create-entry", Class: "TimerTestEntry_1"},
			{ID: "clear-entry", Class: "TimerTestEntry_2"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "activity", PortDefault: map[int]any{1: int64(1000), 2: false, 3: int64(-1), 4: "activity"}},
			{ID: "clear", Class: "ClearTimerByKey", TimerKey: "activity", PortDefault: map[int]any{1: "activity"}},
			{ID: "capture", Class: "CaptureA"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "create-entry", SourcePortID: 0, DesNodeID: "timer", DesPortID: 0},
			{SourceNodeID: "timer", SourcePortID: 1, DesNodeID: "capture", DesPortID: 0},
			{SourceNodeID: "clear-entry", SourcePortID: 0, DesNodeID: "clear", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	blueprint, graphID := addTimerTestGraph(t, compiled, scheduler)
	created, err := blueprint.Start(t.Context(), graphID, 1)
	if err != nil || waitTimerTestExecution(t, created) != nil {
		t.Fatalf("create failed: %v", err)
	}
	duplicate, err := blueprint.Start(t.Context(), graphID, 1)
	if err != nil {
		t.Fatalf("duplicate Start failed early: %v", err)
	}
	if err := waitTimerTestExecution(t, duplicate); !errors.Is(err, ErrTimerKeyAlreadyExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	cleared, err := blueprint.Start(t.Context(), graphID, 2)
	if err != nil || waitTimerTestExecution(t, cleared) != nil {
		t.Fatalf("clear failed: %v", err)
	}
	if scheduler.fireNext() {
		t.Fatal("cleared timer still fired")
	}
	select {
	case call := <-calls:
		t.Fatalf("unexpected callback: %#v", call)
	default:
	}
}

func TestLoopingTimerRearmsAfterCallbackAndReleaseCancelsIt(t *testing.T) {
	calls := make(chan timerTestCall, 3)
	scheduler := &manualTimerScheduler{}
	compiled, err := CompileGraph(timerTestRegistry(calls), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TimerTestEntry_1"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "loop", PortDefault: map[int]any{1: int64(1000), 2: true, 3: int64(250), 4: "loop"}},
			{ID: "capture", Class: "CaptureA"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "timer", DesPortID: 0},
			{SourceNodeID: "timer", SourcePortID: 1, DesNodeID: "capture", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	blueprint, graphID := addTimerTestGraph(t, compiled, scheduler)
	created, err := blueprint.Start(t.Context(), graphID, 1)
	if err != nil || waitTimerTestExecution(t, created) != nil {
		t.Fatalf("create failed: %v", err)
	}
	if delays := scheduler.delays(); len(delays) != 1 || delays[0] != 250*time.Millisecond {
		t.Fatalf("initial delays = %v, want [250ms]", delays)
	}
	if !scheduler.fireNext() {
		t.Fatal("expected first loop callback")
	}
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("first loop callback did not execute")
	}
	if delays := scheduler.delays(); len(delays) != 2 || delays[1] != time.Second {
		t.Fatalf("rearm delays = %v, want second delay 1s", delays)
	}
	if !scheduler.fireNext() {
		t.Fatal("expected second loop callback")
	}
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("second loop callback did not execute")
	}
	blueprint.ReleaseGraph(graphID)
	if scheduler.fireNext() {
		t.Fatal("released graph still fired a rearmed timer")
	}
}

func TestDelaySuspendsOnlyEachTriggeredExecution(t *testing.T) {
	calls := make(chan timerTestCall, 2)
	scheduler := &manualTimerScheduler{}
	compiled, err := CompileGraph(timerTestRegistry(calls), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry-a", Class: "TimerTestEntry_1"}, {ID: "entry-b", Class: "TimerTestEntry_2"},
			{ID: "delay", Class: "Delay", PortDefault: map[int]any{1: int64(1000)}}, {ID: "capture", Class: "CaptureA"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry-a", SourcePortID: 0, DesNodeID: "delay", DesPortID: 0},
			{SourceNodeID: "entry-b", SourcePortID: 0, DesNodeID: "delay", DesPortID: 0},
			{SourceNodeID: "delay", SourcePortID: 0, DesNodeID: "capture", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	blueprint, graphID := addTimerTestGraph(t, compiled, scheduler)
	first, err := blueprint.Start(t.Context(), graphID, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := blueprint.Start(t.Context(), graphID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.State() != ExecutionSuspended || second.State() != ExecutionSuspended {
		t.Fatalf("states = %v, %v", first.State(), second.State())
	}
	if !scheduler.fireNext() || !scheduler.fireNext() {
		t.Fatal("expected two delay tasks")
	}
	if err := waitTimerTestExecution(t, first); err != nil {
		t.Fatal(err)
	}
	if err := waitTimerTestExecution(t, second); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("callback count = %d, want 2", len(calls))
	}
}

func TestCompileTimerRequiresUniqueLiteralKeysAndCallback(t *testing.T) {
	calls := make(chan timerTestCall, 1)
	registry := timerTestRegistry(calls)
	_, err := CompileGraph(registry, GraphConfig{Nodes: []NodeConfig{
		{ID: "first", Class: "CreateTimer", TimerKey: "same", PortDefault: map[int]any{1: int64(1), 2: false, 3: int64(-1), 4: "same"}},
		{ID: "second", Class: "CreateTimer", TimerKey: "same", PortDefault: map[int]any{1: int64(1), 2: false, 3: int64(-1), 4: "same"}},
	}})
	if err == nil || !errors.Is(err, ErrTimerKeyAlreadyExists) && !containsErrorText(err, "duplicate timer key") {
		t.Fatalf("duplicate key err = %v", err)
	}

	_, err = CompileGraph(registry, GraphConfig{Nodes: []NodeConfig{
		{ID: "timer", Class: "CreateTimer", TimerKey: "missing-callback", PortDefault: map[int]any{1: int64(1), 2: false, 3: int64(-1), 4: "missing-callback"}},
	}})
	if err == nil || !containsErrorText(err, "callback is not connected") {
		t.Fatalf("missing callback err = %v", err)
	}
}

func TestCompileTimerCallbackIsAnAsyncCycleBoundary(t *testing.T) {
	registry := timerTestRegistry(make(chan timerTestCall, 1))
	_, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TimerTestEntry_1"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "repeat", PortDefault: map[int]any{1: int64(1), 2: false, 3: int64(-1), 4: "repeat"}},
			{ID: "pass", Class: "TimerPass"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "timer", DesPortID: 0},
			{SourceNodeID: "timer", SourcePortID: 1, DesNodeID: "pass", DesPortID: 0},
			{SourceNodeID: "pass", SourcePortID: 0, DesNodeID: "timer", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("callback boundary compile failed: %v", err)
	}
}

func containsErrorText(err error, text string) bool {
	return err != nil && strings.Contains(err.Error(), text)
}
