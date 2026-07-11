package blueprint

import (
	"context"
	"sync"
	"testing"
	"time"
)

type manualTimerScheduler struct {
	mu       sync.Mutex
	nextID   uint64
	tasks    map[ScheduledTaskHandle]func()
	canceled map[ScheduledTaskHandle]bool
	delays   map[ScheduledTaskHandle]time.Duration
}

func newManualTimerScheduler() *manualTimerScheduler {
	return &manualTimerScheduler{
		tasks:    map[ScheduledTaskHandle]func(){},
		canceled: map[ScheduledTaskHandle]bool{},
		delays:   map[ScheduledTaskHandle]time.Duration{},
	}
}

func (s *manualTimerScheduler) Schedule(delay time.Duration, callback func()) (ScheduledTaskHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	handle := ScheduledTaskHandle(s.nextID)
	s.tasks[handle] = callback
	s.delays[handle] = delay
	return handle, nil
}

func (s *manualTimerScheduler) Cancel(handle ScheduledTaskHandle) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[handle] == nil {
		return false
	}
	delete(s.tasks, handle)
	delete(s.delays, handle)
	s.canceled[handle] = true
	return true
}

func (s *manualTimerScheduler) fire(t *testing.T, handle ScheduledTaskHandle) {
	t.Helper()
	s.mu.Lock()
	callback := s.tasks[handle]
	delete(s.tasks, handle)
	delete(s.delays, handle)
	s.mu.Unlock()
	if callback == nil {
		t.Fatalf("scheduled task %d not found", handle)
	}
	callback()
}

func (s *manualTimerScheduler) onlyHandle(t *testing.T) ScheduledTaskHandle {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) != 1 {
		t.Fatalf("scheduled tasks = %d, want 1", len(s.tasks))
	}
	for handle := range s.tasks {
		return handle
	}
	return 0
}

func TestDelaySuspendsAndResumesExecutionThroughScheduler(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	runs := 0
	entrance := NewExecNode("entrance", NewNodeDefinition("DelayEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	delay := NewExecNode("delay", NewDelayNodeDefinition())
	result := NewExecNode("result", NewNodeDefinition("DelayResult", func() IExecNode {
		return &executionResultNode{value: 73, runs: &runs}
	}, []IPort{NewPortExec()}, nil))
	delay.DefaultIn[1] = PortInt(25)
	entrance.Next = []*ExecNode{delay}
	delay.Next = []*ExecNode{result}
	delay.BeConnect = true
	result.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("delay", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 3})
	graphID := bp.Create("delay")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || runs != 0 {
		t.Fatalf("state=%v runs=%d, want suspended and zero runs", execution.State(), runs)
	}

	scheduler.fire(t, scheduler.onlyHandle(t))
	if dispatcher.len() != 1 || runs != 0 {
		t.Fatalf("delay resumed outside dispatcher: queued=%d runs=%d", dispatcher.len(), runs)
	}
	dispatcher.runNext(t)
	resultValues, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	if len(resultValues) != 1 || resultValues[0].IntVal != 73 {
		t.Fatalf("result = %#v, want 73", resultValues)
	}
}

func TestCancelExecutionRemovesScheduledDelay(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	entrance := NewExecNode("entrance", NewNodeDefinition("DelayEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	delay := NewExecNode("delay", NewDelayNodeDefinition())
	delay.DefaultIn[1] = PortInt(100)
	entrance.Next = []*ExecNode{delay}
	delay.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("delay", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 2})
	graphID := bp.Create("delay")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	handle := scheduler.onlyHandle(t)
	if !execution.Cancel() {
		t.Fatal("Cancel returned false")
	}
	if !scheduler.canceled[handle] {
		t.Fatalf("scheduled delay %d was not canceled", handle)
	}
}

func TestDelayRejectsNegativeAndOverflowMilliseconds(t *testing.T) {
	for _, value := range []PortInt{-1, PortInt(maxTimerMilliseconds + 1)} {
		t.Run(time.Duration(value).String(), func(t *testing.T) {
			node := &DelayNode{}
			ctx := bindNode(t, node, []IPort{NewPortExec(), intPort(value)}, []IPort{NewPortExec()})
			_ = ctx
			if _, err := node.Exec(); err == nil {
				t.Fatalf("Delay(%d) succeeded, want validation error", value)
			}
		})
	}
}
