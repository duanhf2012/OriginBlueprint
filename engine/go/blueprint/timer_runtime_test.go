package blueprint

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type blockingTimerCallbackDispatcher struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	tasks   []func()
}

type blockingRegistrationScheduler struct {
	started  chan struct{}
	release  chan struct{}
	canceled chan ScheduledTaskHandle
}

func (s *blockingRegistrationScheduler) Schedule(_ time.Duration, _ func()) (ScheduledTaskHandle, error) {
	close(s.started)
	<-s.release
	return 1, nil
}

func (s *blockingRegistrationScheduler) Cancel(handle ScheduledTaskHandle) bool {
	s.canceled <- handle
	return true
}

func (d *blockingTimerCallbackDispatcher) Submit(task func()) error {
	d.once.Do(func() { close(d.started) })
	<-d.release
	d.mu.Lock()
	d.tasks = append(d.tasks, task)
	d.mu.Unlock()
	return nil
}

func TestTimerHandlePortRejectsIntegerAssignment(t *testing.T) {
	handlePort := NewPortTimerHandle()
	integerPort := NewPortInt()
	integerPort.SetInt(17)
	if err := assignPortValue(handlePort, integerPort); err == nil {
		t.Fatal("integer assigned to TimerHandle port")
	}
	handle := TimerHandle{BlueprintID: 2, GraphID: 3, TimerID: 4, Generation: 5}
	if !handlePort.SetTimerHandle(handle) {
		t.Fatal("SetTimerHandle returned false")
	}
	clone := handlePort.Clone()
	got, ok := clone.GetTimerHandle()
	if !ok || got != handle {
		t.Fatalf("cloned handle = %#v,%v want %#v,true", got, ok, handle)
	}
}

type timerCallbackRecorder struct {
	BaseExecNode
	values *[]PortInt
}

func timerTestFunctionGraph(t *testing.T) *CompiledGraph {
	t.Helper()
	definition, err := functionEntryDefinition(nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := NewExecNode("function-entry", definition)
	entry.IsEntrance = true
	return &CompiledGraph{Entrances: map[int64]*ExecNode{FunctionEntranceID: entry}, NodeCount: 1}
}

func TestSetTimerByFunctionRejectsUnknownFunction(t *testing.T) {
	scheduler := newManualTimerScheduler()
	bp := &Blueprint{}
	bp.SetTimerScheduler(scheduler)
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp.AddCompiledGraph("timer", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}})
	graphID := bp.Create("timer")

	if _, err := bp.setTimerByFunction(bp.instances[graphID], "missing", "missing", nil, time.Second, time.Second, false); err == nil {
		t.Fatal("unknown timer function was accepted")
	}
	if len(scheduler.tasks) != 0 || len(bp.instances[graphID].runtimeTimers) != 0 {
		t.Fatal("unknown timer function created runtime state")
	}
}

func (n *timerCallbackRecorder) GetName() string { return "TimerCallbackRecorder" }
func (n *timerCallbackRecorder) Exec() (int, error) {
	value, _ := n.GetInPortInt(1)
	*n.values = append(*n.values, value)
	return -1, nil
}

func TestSetTimerByFunctionContinuesImmediatelyAndInvokesFunction(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	var callbackValues []PortInt

	functionEntryDefinition, err := functionEntryDefinition([]string{"Integer"})
	if err != nil {
		t.Fatal(err)
	}
	functionEntry := NewExecNode("function-entry", functionEntryDefinition)
	functionEntry.IsEntrance = true
	callback := NewExecNode("callback", NewNodeDefinition("TimerCallbackRecorder", func() IExecNode {
		return &timerCallbackRecorder{values: &callbackValues}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))
	functionEntry.Next = []*ExecNode{callback}
	callback.BeConnect = true
	callback.PreInPort[1] = &PrePortNode{Node: functionEntry, OutPortID: 1}
	functionGraph := &CompiledGraph{Entrances: map[int64]*ExecNode{FunctionEntranceID: functionEntry}, NodeCount: 2}

	entrance := NewExecNode("entrance", NewNodeDefinition("TimerEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	setDefinition, err := setTimerByFunctionDefinition([]string{"Integer"})
	if err != nil {
		t.Fatal(err)
	}
	setTimer := NewExecNode("set-timer", setDefinition)
	setTimer.FunctionID = "timer-callback"
	setTimer.FunctionName = "TimerCallback"
	setTimer.FunctionGraph = functionGraph
	setTimer.DefaultIn[1] = PortInt(20)
	setTimer.DefaultIn[2] = PortBool(false)
	setTimer.DefaultIn[3] = PortInt(-1)
	setTimer.DefaultIn[4] = PortInt(91)
	entrance.Next = []*ExecNode{setTimer}
	setTimer.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: entrance},
		Functions: map[string]*CompiledGraph{"timer-callback": functionGraph},
		NodeCount: 2,
	})
	graphID := bp.Create("timer")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	var handle TimerHandle
	execution.addCompletionHook(func(done *Execution) {
		setContext, ok := done.graph.getContext(setTimer)
		if !ok {
			t.Error("set timer context not found during completion")
			return
		}
		handle, ok = setContext.OutputPorts[1].GetTimerHandle()
		if !ok || !handle.Valid() {
			t.Errorf("timer handle = %#v,%v", handle, ok)
		}
	})
	dispatcher.runNext(t)
	if !execution.IsDone() || len(callbackValues) != 0 {
		t.Fatalf("timer creation did not finish independently: state=%v callbacks=%v", execution.State(), callbackValues)
	}
	if !handle.Valid() {
		t.Fatal("completion hook did not capture a valid timer handle")
	}

	scheduler.fire(t, scheduler.onlyHandle(t))
	if dispatcher.len() != 1 {
		t.Fatalf("callback tasks = %d, want 1", dispatcher.len())
	}
	dispatcher.runNext(t)
	if len(callbackValues) != 1 || callbackValues[0] != 91 {
		t.Fatalf("callback values = %v, want [91]", callbackValues)
	}
	if bp.isTimerValid(handle) {
		t.Fatal("one-shot timer remained valid after callback")
	}
}

func TestTimerPauseResumeAndClearLifecycle(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))},
		Functions: map[string]*CompiledGraph{"unused": timerTestFunctionGraph(t)},
	})
	graphID := bp.Create("timer")
	instance := bp.instances[graphID]
	handle, err := bp.setTimerByFunction(instance, "unused", "unused", nil, 100, 100, true)
	if err != nil {
		t.Fatalf("setTimerByFunction failed: %v", err)
	}
	firstScheduled := scheduler.onlyHandle(t)
	if !bp.isTimerActive(handle) || bp.isTimerPaused(handle) {
		t.Fatalf("new timer active=%v paused=%v", bp.isTimerActive(handle), bp.isTimerPaused(handle))
	}
	if !bp.pauseTimer(handle) {
		t.Fatal("pauseTimer returned false")
	}
	if !scheduler.canceled[firstScheduled] || !bp.isTimerPaused(handle) || bp.isTimerActive(handle) {
		t.Fatalf("paused timer state active=%v paused=%v canceled=%v", bp.isTimerActive(handle), bp.isTimerPaused(handle), scheduler.canceled[firstScheduled])
	}
	remaining := bp.timerRemaining(handle)
	if remaining < 0 || remaining > 100*time.Millisecond {
		t.Fatalf("remaining = %s, want [0,100ms]", remaining)
	}
	if !bp.resumeTimer(handle) {
		t.Fatal("resumeTimer returned false")
	}
	resumedScheduled := scheduler.onlyHandle(t)
	if resumedScheduled == firstScheduled || !bp.isTimerActive(handle) || bp.isTimerPaused(handle) {
		t.Fatalf("resumed timer handle=%d active=%v paused=%v", resumedScheduled, bp.isTimerActive(handle), bp.isTimerPaused(handle))
	}
	if !bp.clearTimer(handle, false) || bp.isTimerValid(handle) {
		t.Fatal("clearTimer did not invalidate timer")
	}
	if !scheduler.canceled[resumedScheduled] {
		t.Fatalf("resumed task %d was not canceled", resumedScheduled)
	}
}

func TestClearTimerCancelsCallbackWhileItIsBeingSubmitted(t *testing.T) {
	dispatcher := &blockingTimerCallbackDispatcher{started: make(chan struct{}), release: make(chan struct{})}
	scheduler := newManualTimerScheduler()
	functionGraph := timerTestFunctionGraph(t)
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: entry},
		Functions: map[string]*CompiledGraph{"callback": functionGraph},
	})
	graphID := bp.Create("timer")
	handle, err := bp.setTimerByFunction(bp.instances[graphID], "callback", "callback", nil, time.Second, time.Millisecond, true)
	if err != nil {
		t.Fatal(err)
	}
	fired := make(chan struct{})
	go func() {
		scheduler.fire(t, scheduler.onlyHandle(t))
		close(fired)
	}()
	<-dispatcher.started
	if !bp.clearTimer(handle, true) {
		t.Fatal("clearTimer returned false")
	}
	close(dispatcher.release)
	<-fired
	if bp.activeExecutionCount() != 0 {
		t.Fatalf("callback submission escaped cancellation: active=%d", bp.activeExecutionCount())
	}
}

func TestTimerKeepsSchedulerOwnershipAfterBlueprintSchedulerChanges(t *testing.T) {
	first := newManualTimerScheduler()
	second := newManualTimerScheduler()
	bp := &Blueprint{}
	bp.SetTimerScheduler(first)
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: entry},
		Functions: map[string]*CompiledGraph{"callback": timerTestFunctionGraph(t)},
	})
	graphID := bp.Create("timer")
	handle, err := bp.setTimerByFunction(bp.instances[graphID], "callback", "callback", nil, time.Second, time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	scheduled := first.onlyHandle(t)
	bp.SetTimerScheduler(second)
	if !bp.clearTimer(handle, false) {
		t.Fatal("clearTimer returned false")
	}
	if !first.canceled[scheduled] || len(second.canceled) != 0 {
		t.Fatalf("timer canceled on wrong scheduler: first=%v second=%v", first.canceled, second.canceled)
	}
}

func TestTimerRegistrationRacingGraphReleaseCancelsScheduledTask(t *testing.T) {
	scheduler := &blockingRegistrationScheduler{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		canceled: make(chan ScheduledTaskHandle, 1),
	}
	bp := &Blueprint{}
	bp.SetTimerScheduler(scheduler)
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: entry},
		Functions: map[string]*CompiledGraph{"callback": timerTestFunctionGraph(t)},
	})
	graphID := bp.Create("timer")
	instance := bp.instances[graphID]
	result := make(chan error, 1)
	go func() {
		_, err := bp.setTimerByFunction(instance, "callback", "callback", nil, time.Second, time.Second, true)
		result <- err
	}()
	<-scheduler.started
	bp.ReleaseGraph(graphID)
	close(scheduler.release)
	if err := <-result; !errors.Is(err, ErrSchedulerClosed) {
		t.Fatalf("registration error = %v, want ErrSchedulerClosed", err)
	}
	select {
	case handle := <-scheduler.canceled:
		if handle != 1 {
			t.Fatalf("canceled handle = %d, want 1", handle)
		}
	default:
		t.Fatal("orphaned scheduled task was not canceled")
	}
	if len(instance.runtimeTimers) != 0 {
		t.Fatalf("released instance kept %d timer(s)", len(instance.runtimeTimers))
	}
}

func TestClearTimerCanCancelRunningDelayedCallback(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()

	functionEntryDefinition, _ := functionEntryDefinition(nil)
	functionEntry := NewExecNode("function-entry", functionEntryDefinition)
	functionEntry.IsEntrance = true
	delay := NewExecNode("delay", NewDelayNodeDefinition())
	delay.DefaultIn[1] = PortInt(100)
	functionEntry.Next = []*ExecNode{delay}
	delay.BeConnect = true
	functionGraph := &CompiledGraph{Entrances: map[int64]*ExecNode{FunctionEntranceID: functionEntry}, NodeCount: 2}

	mainEntry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: mainEntry},
		Functions: map[string]*CompiledGraph{"callback": functionGraph},
	})
	graphID := bp.Create("timer")
	handle, err := bp.setTimerByFunction(bp.instances[graphID], "callback", "callback", nil, 20*time.Millisecond, 20*time.Millisecond, true)
	if err != nil {
		t.Fatalf("setTimerByFunction failed: %v", err)
	}
	scheduler.fire(t, scheduler.onlyHandle(t))
	dispatcher.runNext(t)
	if bp.instances[graphID].runtimeTimers[handle.TimerID].callback.State() != ExecutionSuspended {
		t.Fatal("timer callback did not suspend on Delay")
	}
	if !bp.clearTimer(handle, true) {
		t.Fatal("clearTimer returned false")
	}
	if bp.activeExecutionCount() != 0 {
		t.Fatalf("active executions = %d, want 0", bp.activeExecutionCount())
	}
	if len(scheduler.tasks) != 0 {
		t.Fatalf("scheduled tasks remain after clear: %d", len(scheduler.tasks))
	}
}

func TestLoopingTimerDoesNotReenterRunningCallback(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()

	functionEntryDefinition, _ := functionEntryDefinition(nil)
	functionEntry := NewExecNode("function-entry", functionEntryDefinition)
	functionEntry.IsEntrance = true
	delay := NewExecNode("delay", NewDelayNodeDefinition())
	delay.DefaultIn[1] = PortInt(100)
	functionEntry.Next = []*ExecNode{delay}
	delay.BeConnect = true
	functionGraph := &CompiledGraph{Entrances: map[int64]*ExecNode{FunctionEntranceID: functionEntry}, NodeCount: 2}

	mainEntry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: mainEntry},
		Functions: map[string]*CompiledGraph{"callback": functionGraph},
	})
	graphID := bp.Create("timer")
	handle, err := bp.setTimerByFunction(bp.instances[graphID], "callback", "callback", nil, 20*time.Millisecond, 20*time.Millisecond, true)
	if err != nil {
		t.Fatalf("setTimerByFunction failed: %v", err)
	}

	scheduler.fire(t, scheduler.onlyHandle(t))
	dispatcher.runNext(t)
	if bp.activeExecutionCount() != 1 {
		t.Fatalf("active callbacks = %d, want 1", bp.activeExecutionCount())
	}

	var nextTimer ScheduledTaskHandle
	scheduler.mu.Lock()
	for scheduled, scheduledDelay := range scheduler.delays {
		if scheduledDelay < 50*time.Millisecond {
			nextTimer = scheduled
			break
		}
	}
	scheduler.mu.Unlock()
	if nextTimer == 0 {
		t.Fatal("next looping timer task not found")
	}
	scheduler.fire(t, nextTimer)
	if dispatcher.len() != 0 || bp.activeExecutionCount() != 1 {
		t.Fatalf("running callback was reentered: queued=%d active=%d", dispatcher.len(), bp.activeExecutionCount())
	}

	if !bp.clearTimer(handle, true) {
		t.Fatal("clearTimer returned false")
	}
	if bp.activeExecutionCount() != 0 || len(scheduler.tasks) != 0 {
		t.Fatalf("timer cleanup left active=%d scheduled=%d", bp.activeExecutionCount(), len(scheduler.tasks))
	}
}

func TestReleaseGraphCancelsRuntimeTimers(t *testing.T) {
	scheduler := newManualTimerScheduler()
	bp := &Blueprint{}
	bp.SetTimerScheduler(scheduler)
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	bp.AddCompiledGraph("timer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: entry},
		Functions: map[string]*CompiledGraph{"unused": timerTestFunctionGraph(t)},
	})
	graphID := bp.Create("timer")
	handle, err := bp.setTimerByFunction(bp.instances[graphID], "unused", "unused", nil, time.Second, time.Second, true)
	if err != nil {
		t.Fatalf("setTimerByFunction failed: %v", err)
	}
	scheduled := scheduler.onlyHandle(t)
	bp.ReleaseGraph(graphID)
	if !scheduler.canceled[scheduled] {
		t.Fatalf("scheduled task %d was not canceled", scheduled)
	}
	if bp.isTimerValid(handle) {
		t.Fatal("timer remained valid after graph release")
	}
}
