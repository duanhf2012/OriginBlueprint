package blueprint

import (
	"context"
	"testing"
	"time"
)

type testSignalRecorder struct {
	BaseExecNode
	done chan struct{}
}

func (n *testSignalRecorder) GetName() string { return "TestSignalRecorder" }
func (n *testSignalRecorder) Exec() (int, error) {
	close(n.done)
	return -1, nil
}

type immediateTimerScheduler struct {
	nextID ScheduledTaskHandle
}

func (s *immediateTimerScheduler) Schedule(_ time.Duration, callback func()) (ScheduledTaskHandle, error) {
	s.nextID++
	callback()
	return s.nextID, nil
}

func (s *immediateTimerScheduler) Cancel(ScheduledTaskHandle) bool {
	return true
}

func TestSleepNodeResumesAfterDelay(t *testing.T) {
	done := make(chan struct{})
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	sleep := NewExecNode("sleep", NewSleepNodeDefinition())
	record := NewExecNode("record", NewNodeDefinition("TestSignalRecorder", func() IExecNode {
		return &testSignalRecorder{done: done}
	}, []IPort{NewPortExec()}, nil))

	sleep.DefaultIn[1] = 5
	entrance.Next = []*ExecNode{sleep}
	sleep.Next = []*ExecNode{record}
	sleep.BeConnect = true
	record.BeConnect = true

	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}})
	if _, err := graph.Do(1); err != ErrExecutionSuspended {
		t.Fatalf("Do error = %v, want ErrExecutionSuspended", err)
	}
	select {
	case <-done:
		t.Fatalf("recorder ran before sleep resumed")
	default:
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("recorder did not signal completion")
	}
}

func TestReleaseGraphPreventsSleepContinuation(t *testing.T) {
	done := make(chan struct{})
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	sleep := NewExecNode("sleep", NewSleepNodeDefinition())
	record := NewExecNode("record", NewNodeDefinition("TestSignalRecorder", func() IExecNode { return &testSignalRecorder{done: done} }, []IPort{NewPortExec()}, nil))
	sleep.DefaultIn[1] = 40
	entrance.Next = []*ExecNode{sleep}
	sleep.Next = []*ExecNode{record}
	sleep.BeConnect = true
	record.BeConnect = true

	var bp Blueprint
	dispatcher := &manualExecutionDispatcher{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("sleep", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 3})
	graphID := bp.Create("sleep")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended {
		t.Fatalf("state = %v, want suspended", execution.State())
	}
	bp.ReleaseGraph(graphID)
	select {
	case <-done:
		t.Fatal("sleep continuation ran after graph release")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDelayNodeDoesNotRegisterCancelHookAfterImmediateCallback(t *testing.T) {
	done := make(chan struct{})
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	delay := NewExecNode("delay", NewDelayNodeDefinition())
	record := NewExecNode("record", NewNodeDefinition("TestSignalRecorder", func() IExecNode { return &testSignalRecorder{done: done} }, []IPort{NewPortExec()}, nil))
	delay.DefaultIn[1] = 1
	entrance.Next = []*ExecNode{delay}
	delay.Next = []*ExecNode{record}
	delay.BeConnect = true
	record.BeConnect = true

	dispatcher := &manualExecutionDispatcher{}
	scheduler := &immediateTimerScheduler{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("immediate-delay", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 3})
	execution, err := bp.Start(context.Background(), bp.Create("immediate-delay"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)

	execution.mu.Lock()
	hookCount := len(execution.cancelHooks)
	execution.mu.Unlock()
	if hookCount != 0 {
		t.Fatalf("cancel hooks = %d, want 0 after an immediate callback", hookCount)
	}

	dispatcher.runNext(t)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delay continuation did not complete")
	}
}

func TestContinuationResumeReturnsGraphReleased(t *testing.T) {
	instance := &GraphInstance{releasedCh: make(chan struct{})}
	instance.markReleased()
	continuation := &Continuation{graph: &Graph{instance: instance}}
	if err := continuation.Resume(); err != ErrGraphReleased {
		t.Fatalf("Resume error = %v, want ErrGraphReleased", err)
	}
}
