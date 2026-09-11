package blueprint

import (
	"errors"
	"path/filepath"
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

type failingTimerScheduler struct{ err error }

func (scheduler failingTimerScheduler) Schedule(time.Duration, func()) (ScheduledTimer, error) {
	return nil, scheduler.err
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

func (scheduler *manualTimerScheduler) fire(index int) bool {
	scheduler.mu.Lock()
	if index < 0 || index >= len(scheduler.timers) {
		scheduler.mu.Unlock()
		return false
	}
	timer := scheduler.timers[index]
	scheduler.mu.Unlock()
	return timer.fire()
}

func (scheduler *manualTimerScheduler) pending() int {
	scheduler.mu.Lock()
	timers := append([]*manualScheduledTimer(nil), scheduler.timers...)
	scheduler.mu.Unlock()
	pending := 0
	for _, timer := range timers {
		timer.mu.Lock()
		active := !timer.canceled && !timer.fired
		timer.mu.Unlock()
		if active {
			pending++
		}
	}
	return pending
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
	registry.Register(NewNodeDefinition("CreateTimer", func() IExecNode { return &CreateTimerNode{} }, []IPort{NewPortExec(), NewPortInt(), NewPortBool(), nil, NewPortStr()}, []IPort{NewPortExec(), NewPortCallback()}))
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
			{ID: "timer-a", Class: "CreateTimer", TimerKey: "spawn-60", PortDefault: map[int]any{1: int64(60000), 2: false, 4: "spawn-60"}},
			{ID: "timer-b", Class: "CreateTimer", TimerKey: "spawn-120", PortDefault: map[int]any{1: int64(120000), 2: false, 4: "spawn-120"}},
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
			{ID: "timer", Class: "CreateTimer", TimerKey: "activity", PortDefault: map[int]any{1: int64(1000), 2: false, 4: "activity"}},
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

func TestCreateTimerCleansUpAfterScheduleFailure(t *testing.T) {
	calls := make(chan timerTestCall, 1)
	registry := timerTestRegistry(calls)
	valid, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TimerTestEntry_1"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "schedule-retry", PortDefault: map[int]any{1: int64(1000), 2: false, 4: "schedule-retry"}},
			{ID: "capture", Class: "CaptureA"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "timer", DesPortID: 0},
			{SourceNodeID: "timer", SourcePortID: 1, DesNodeID: "capture", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph schedule-retry fixture: %v", err)
	}
	failure := errors.New("scheduler unavailable")
	blueprint, graphID := addTimerTestGraph(t, valid, failingTimerScheduler{err: failure})
	execution, err := blueprint.Start(t.Context(), graphID, 1)
	if err != nil {
		t.Fatalf("failed-scheduler Start: %v", err)
	}
	if err := waitTimerTestExecution(t, execution); !errors.Is(err, failure) {
		t.Fatalf("failed scheduler error = %v, want %v", err, failure)
	}

	// 调度失败后同一个 Key 不应残留；更换调度器后可立即重新创建。
	scheduler := &manualTimerScheduler{}
	blueprint.SetTimerScheduler(scheduler)
	execution, err = blueprint.Start(t.Context(), graphID, 1)
	if err != nil || waitTimerTestExecution(t, execution) != nil {
		t.Fatalf("retry after scheduler failure: %v", err)
	}
	if !scheduler.fireNext() {
		t.Fatal("retry timer was not scheduled")
	}
}

func TestBlueprintCloseCancelsPendingTimer(t *testing.T) {
	calls := make(chan timerTestCall, 1)
	scheduler := &manualTimerScheduler{}
	compiled, err := CompileGraph(timerTestRegistry(calls), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TimerTestEntry_1"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "close-pending", PortDefault: map[int]any{1: int64(1000), 2: false, 4: "close-pending"}},
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
	execution, err := blueprint.Start(t.Context(), graphID, 1)
	if err != nil || waitTimerTestExecution(t, execution) != nil {
		t.Fatalf("create pending timer: %v", err)
	}
	if err := blueprint.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if scheduler.fireNext() {
		t.Fatal("closed blueprint still ran pending timer")
	}
}

func TestLoopingTimerRearmsAfterCallbackAndReleaseCancelsIt(t *testing.T) {
	calls := make(chan timerTestCall, 3)
	scheduler := &manualTimerScheduler{}
	compiled, err := CompileGraph(timerTestRegistry(calls), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TimerTestEntry_1"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "loop", PortDefault: map[int]any{1: int64(1000), 2: true, 4: "loop"}},
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
	if delays := scheduler.delays(); len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("initial delays = %v, want [1s]", delays)
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
		{ID: "first", Class: "CreateTimer", TimerKey: "same", PortDefault: map[int]any{1: int64(1), 2: false, 4: "same"}},
		{ID: "second", Class: "CreateTimer", TimerKey: "same", PortDefault: map[int]any{1: int64(1), 2: false, 4: "same"}},
	}})
	if err == nil || !errors.Is(err, ErrTimerKeyAlreadyExists) && !containsErrorText(err, "duplicate timer key") {
		t.Fatalf("duplicate key err = %v", err)
	}

	_, err = CompileGraph(registry, GraphConfig{Nodes: []NodeConfig{
		{ID: "timer", Class: "CreateTimer", TimerKey: "missing-callback", PortDefault: map[int]any{1: int64(1), 2: false, 4: "missing-callback"}},
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
			{ID: "timer", Class: "CreateTimer", TimerKey: "repeat", PortDefault: map[int]any{1: int64(1), 2: false, 4: "repeat"}},
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

// timerFixtureEntry/timerFixtureCapture 是生成 .obp 回归蓝图专用节点。
// 它们通过文档内嵌 legacy ports 模拟宿主业务入口和“创建 NPC”业务节点，
// 以覆盖真实文档加载、编译与回调捕获，不加入正式节点库。
type timerFixtureEntry struct{ BaseExecNode }

func (n *timerFixtureEntry) GetName() string    { return "TimerFixtureEntry" }
func (n *timerFixtureEntry) Exec() (int, error) { return 0, nil }

type timerFixtureCall struct {
	nodeID  string
	value   int64
	success bool
}

type timerFixtureCapture struct {
	BaseExecNode
	calls chan timerFixtureCall
}

func (n *timerFixtureCapture) GetName() string { return "TimerFixtureCapture" }
func (n *timerFixtureCapture) Exec() (int, error) {
	value, _ := n.GetInPortInt(1)
	n.calls <- timerFixtureCall{nodeID: n.node.ID, value: value}
	return -1, nil
}

type timerFixtureClearCapture struct {
	BaseExecNode
	calls chan timerFixtureCall
}

func (n *timerFixtureClearCapture) GetName() string { return "TimerFixtureClearCapture" }
func (n *timerFixtureClearCapture) Exec() (int, error) {
	success, _ := n.GetInPortBool(1)
	n.calls <- timerFixtureCall{nodeID: n.node.ID, success: bool(success)}
	return -1, nil
}

func timerFixtureRegistry(t *testing.T, calls chan timerFixtureCall) *Registry {
	t.Helper()
	registry := testSystemRegistry(t)
	registry.Register(NewNodeDefinition("TimerFixtureEntry", func() IExecNode { return &timerFixtureEntry{} }, nil, []IPort{NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("TimerFixtureCapture", func() IExecNode { return &timerFixtureCapture{calls: calls} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	registry.Register(NewNodeDefinition("TimerFixtureClearCapture", func() IExecNode { return &timerFixtureClearCapture{calls: calls} }, []IPort{NewPortExec(), NewPortBool()}, nil))
	return registry
}

func addTimerFixtureLegacyNode(author *testDocumentAuthor, id, class string, inputs, outputs []graphDocumentLegacyPort) {
	author.AddNode(id, "origin.test.timer-fixture."+id)
	node := &author.document.Nodes[author.nodes[id]]
	node.Properties.LegacyClass = class
	node.Properties.LegacyModule = "timer-runtime-test"
	node.Properties.LegacyInputs = inputs
	node.Properties.LegacyOutputs = outputs
}

func timerFixtureEntryPorts() []graphDocumentLegacyPort {
	return []graphDocumentLegacyPort{{Key: "exec", Type: "exec"}, {Key: "value", Type: "integer"}}
}

func timerFixtureCaptureInputs() []graphDocumentLegacyPort {
	return []graphDocumentLegacyPort{{Key: "exec", Type: "exec"}, {Key: "value", Type: "integer"}}
}

func timerFixtureClearInputs() []graphDocumentLegacyPort {
	return []graphDocumentLegacyPort{{Key: "exec", Type: "exec"}, {Key: "success", Type: "boolean"}}
}

// TestGeneratedTimerCallbackBlueprintIsolationAndClearByKey 生成真实 .obp，
// 模拟活动多入口创建 NPC：每个回调必须读取创建时的输入快照，且 Key 清除可靠。
func TestGeneratedTimerCallbackBlueprintIsolationAndClearByKey(t *testing.T) {
	root := t.TempDir()
	author := newTestDocumentAuthor("timer-callback-isolation")
	addTimerFixtureLegacyNode(author, "entry_a", "TimerFixtureEntry_1", nil, timerFixtureEntryPorts())
	addTimerFixtureLegacyNode(author, "entry_b", "TimerFixtureEntry_2", nil, timerFixtureEntryPorts())
	addTimerFixtureLegacyNode(author, "entry_loop", "TimerFixtureEntry_3", nil, timerFixtureEntryPorts())
	addTimerFixtureLegacyNode(author, "clear_a_entry", "TimerFixtureEntry_4", nil, timerFixtureEntryPorts())
	addTimerFixtureLegacyNode(author, "clear_loop_entry", "TimerFixtureEntry_5", nil, timerFixtureEntryPorts())

	author.AddNode("create_a", "origin.timer.create").SetValue("create_a", "duration", 60000).SetValue("create_a", "looping", false).SetValue("create_a", "timerKey", "spawn-a")
	author.AddNode("create_b", "origin.timer.create").SetValue("create_b", "duration", 120000).SetValue("create_b", "looping", false).SetValue("create_b", "timerKey", "spawn-b")
	author.AddNode("create_loop", "origin.timer.create").SetValue("create_loop", "duration", 1000).SetValue("create_loop", "looping", true).SetValue("create_loop", "timerKey", "spawn-loop")
	author.AddNode("clear_a", "origin.timer.clear-by-key").SetValue("clear_a", "timerKey", "spawn-a")
	author.AddNode("clear_loop", "origin.timer.clear-by-key").SetValue("clear_loop", "timerKey", "spawn-loop")

	for _, id := range []string{"created_a", "callback_a", "created_b", "callback_b", "created_loop", "callback_loop"} {
		addTimerFixtureLegacyNode(author, id, "TimerFixtureCapture", timerFixtureCaptureInputs(), nil)
	}
	addTimerFixtureLegacyNode(author, "clear_a_result", "TimerFixtureClearCapture", timerFixtureClearInputs(), nil)
	addTimerFixtureLegacyNode(author, "clear_loop_result", "TimerFixtureClearCapture", timerFixtureClearInputs(), nil)

	for _, item := range []struct{ entry, timer, created, callback string }{
		{"entry_a", "create_a", "created_a", "callback_a"},
		{"entry_b", "create_b", "created_b", "callback_b"},
		{"entry_loop", "create_loop", "created_loop", "callback_loop"},
	} {
		author.Connect(item.entry, "exec", item.timer, "exec")
		author.Connect(item.timer, "created", item.created, "exec")
		author.Connect(item.timer, "triggered", item.callback, "exec")
		author.Connect(item.entry, "value", item.created, "value")
		author.Connect(item.entry, "value", item.callback, "value")
	}
	author.Connect("clear_a_entry", "exec", "clear_a", "exec")
	author.Connect("clear_a", "then", "clear_a_result", "exec")
	author.Connect("clear_a", "success", "clear_a_result", "success")
	author.Connect("clear_loop_entry", "exec", "clear_loop", "exec")
	author.Connect("clear_loop", "then", "clear_loop_result", "exec")
	author.Connect("clear_loop", "success", "clear_loop_result", "success")

	fixturePath := filepath.Join(root, "timer-callback-isolation.obp")
	author.SaveOBP(t, fixturePath)

	calls := make(chan timerFixtureCall, 32)
	graphs, err := loadGraphDir(timerFixtureRegistry(t, calls), root)
	if err != nil {
		t.Fatalf("load generated timer fixture: %v", err)
	}
	compiled := graphs["timer-callback-isolation"]
	if compiled == nil {
		t.Fatal("generated timer fixture was not loaded")
	}

	scheduler := &manualTimerScheduler{}
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetTimerScheduler(scheduler)
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("timer-callback-isolation", compiled)
	graphID := blueprint.Create("timer-callback-isolation")
	t.Cleanup(func() { _ = blueprint.Close() })

	runAll := func() {
		for dispatcher.len() > 0 {
			dispatcher.runNext(t)
		}
	}
	startAndRun := func(entranceID, value int64) *Execution {
		execution, err := blueprint.Start(t.Context(), graphID, entranceID, PortInt(value))
		if err != nil {
			t.Fatalf("Start entrance %d: %v", entranceID, err)
		}
		runAll()
		if err := waitTimerTestExecution(t, execution); err != nil {
			t.Fatalf("entrance %d execution: %v", entranceID, err)
		}
		return execution
	}
	readCalls := func(count int) map[string]timerFixtureCall {
		result := make(map[string]timerFixtureCall, count)
		for range count {
			select {
			case call := <-calls:
				result[call.nodeID] = call
			case <-time.After(time.Second):
				t.Fatalf("timed out waiting for %d fixture calls; got %#v", count, result)
			}
		}
		return result
	}

	// 三个入口先后创建三种 Timer。Created 必须同步完成，但 callback 尚未执行。
	startAndRun(1, 101)
	startAndRun(2, 202)
	startAndRun(3, 303)
	created := readCalls(3)
	for nodeID, want := range map[string]int64{"created_a": 101, "created_b": 202, "created_loop": 303} {
		if got := created[nodeID].value; got != want {
			t.Fatalf("%s value = %d, want %d", nodeID, got, want)
		}
	}
	if delays := scheduler.delays(); len(delays) != 3 || delays[0] != time.Minute || delays[1] != 2*time.Minute || delays[2] != time.Second {
		t.Fatalf("generated fixture delays = %v", delays)
	}
	// 同一 Key 尚在等待时，即使从同一活动入口再次进入，也必须被拒绝且不能覆盖旧快照。
	duplicate, err := blueprint.Start(t.Context(), graphID, 1, PortInt(999))
	if err != nil {
		t.Fatalf("Start duplicate key: %v", err)
	}
	runAll()
	if err := waitTimerTestExecution(t, duplicate); !errors.Is(err, ErrTimerKeyAlreadyExists) {
		t.Fatalf("duplicate timer error = %v, want ErrTimerKeyAlreadyExists", err)
	}
	if delays := scheduler.delays(); len(delays) != 3 {
		t.Fatalf("duplicate create scheduled %d timers, want 3", len(delays))
	}

	// 反向触发和反向执行 callback，证明不同入口/不同 Timer 的捕获帧没有交叉覆盖。
	if !scheduler.fire(1) || !scheduler.fire(0) {
		t.Fatal("expected spawn-b and spawn-a callbacks to fire")
	}
	dispatcher.runAt(t, 1) // callback_a 先执行，队列中 callback_b 仍待执行。
	dispatcher.runAt(t, 0)
	callbacks := readCalls(2)
	if callbacks["callback_a"].value != 101 || callbacks["callback_b"].value != 202 {
		t.Fatalf("callback snapshots = %#v, want a=101 b=202", callbacks)
	}

	// 同一节点在已触发后可再次创建；新的回调必须使用新一轮快照。
	startAndRun(1, 404)
	if got := readCalls(1)["created_a"].value; got != 404 {
		t.Fatalf("second Created A value = %d, want 404", got)
	}
	if !scheduler.fire(3) {
		t.Fatal("expected re-created spawn-a callback")
	}
	runAll()
	if got := readCalls(1)["callback_a"].value; got != 404 {
		t.Fatalf("second callback A value = %d, want 404", got)
	}

	// Loop callback 已被调度但尚未运行时清除 Key：本轮可以完成，但不能再次重调度。
	if !scheduler.fire(2) {
		t.Fatal("expected loop callback")
	}
	clearLoop, err := blueprint.Start(t.Context(), graphID, 5, PortInt(0))
	if err != nil {
		t.Fatalf("Start clear loop: %v", err)
	}
	dispatcher.runAt(t, 1) // callback_loop 在 0，先执行 clear_loop。
	if err := waitTimerTestExecution(t, clearLoop); err != nil {
		t.Fatalf("clear loop execution: %v", err)
	}
	if call := readCalls(1)["clear_loop_result"]; !call.success {
		t.Fatalf("clear loop result = %#v, want success", call)
	}
	dispatcher.runAt(t, 0)
	if got := readCalls(1)["callback_loop"].value; got != 303 {
		t.Fatalf("loop callback value = %d, want 303", got)
	}
	if scheduler.pending() != 0 {
		t.Fatalf("cleared loop rearmed unexpectedly; pending=%d", scheduler.pending())
	}

	// Clear By Key 取消等待中的一次性 Timer，重复清除返回 false，且不产生回调。
	startAndRun(1, 505)
	_ = readCalls(1) // Created A
	clearA, err := blueprint.Start(t.Context(), graphID, 4, PortInt(0))
	if err != nil {
		t.Fatalf("Start clear A: %v", err)
	}
	runAll()
	if err := waitTimerTestExecution(t, clearA); err != nil {
		t.Fatalf("clear A execution: %v", err)
	}
	if call := readCalls(1)["clear_a_result"]; !call.success {
		t.Fatalf("clear A result = %#v, want success", call)
	}
	if scheduler.fire(4) {
		t.Fatal("cleared spawn-a timer still fired")
	}
	clearAgain, err := blueprint.Start(t.Context(), graphID, 4, PortInt(0))
	if err != nil {
		t.Fatalf("Start second clear A: %v", err)
	}
	runAll()
	if err := waitTimerTestExecution(t, clearAgain); err != nil {
		t.Fatalf("second clear A execution: %v", err)
	}
	if call := readCalls(1)["clear_a_result"]; call.success {
		t.Fatalf("second clear A result = %#v, want false", call)
	}
}
