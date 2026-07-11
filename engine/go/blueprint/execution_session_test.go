package blueprint

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type manualExecutionDispatcher struct {
	mu       sync.Mutex
	tasks    []func()
	rejected bool
}

func (d *manualExecutionDispatcher) Submit(task func()) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rejected {
		return ErrExecutionRejected
	}
	d.tasks = append(d.tasks, task)
	return nil
}

func (d *manualExecutionDispatcher) runNext(t *testing.T) {
	t.Helper()
	d.mu.Lock()
	if len(d.tasks) == 0 {
		d.mu.Unlock()
		t.Fatal("dispatcher has no queued task")
	}
	task := d.tasks[0]
	d.tasks = d.tasks[1:]
	d.mu.Unlock()
	task()
}

func (d *manualExecutionDispatcher) len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tasks)
}

type executionResultNode struct {
	BaseExecNode
	value PortInt
	runs  *int
}

type panicExecutionNode struct{ BaseExecNode }

func (n *panicExecutionNode) GetName() string { return "PanicExecution" }
func (n *panicExecutionNode) Exec() (int, error) {
	panic("test panic")
}

type blockingExecutionNode struct {
	BaseExecNode
	started chan struct{}
	release chan struct{}
}

func (n *blockingExecutionNode) GetName() string { return "BlockingExecution" }
func (n *blockingExecutionNode) Exec() (int, error) {
	close(n.started)
	<-n.release
	return 0, nil
}

func TestCancelWaitsForRunningNodeAndStopsFollowingFlow(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	started := make(chan struct{})
	release := make(chan struct{})
	downstreamRuns := 0
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	blocking := NewExecNode("blocking", NewNodeDefinition("BlockingExecution", func() IExecNode {
		return &blockingExecutionNode{started: started, release: release}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	downstream := NewExecNode("downstream", NewNodeDefinition("ExecutionResult", func() IExecNode {
		return &executionResultNode{value: 1, runs: &downstreamRuns}
	}, []IPort{NewPortExec()}, nil))
	entrance.Next = []*ExecNode{blocking}
	blocking.Next = []*ExecNode{downstream}
	blocking.BeConnect = true
	downstream.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("blocking", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 3})
	execution, err := bp.Start(context.Background(), bp.Create("blocking"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatchDone := make(chan struct{})
	go func() {
		dispatcher.runNext(t)
		close(dispatchDone)
	}()
	<-started
	if !execution.Cancel() {
		t.Fatal("Cancel returned false")
	}
	select {
	case <-execution.Done():
		t.Fatal("Done closed while a node was still running")
	default:
	}
	close(release)
	<-dispatchDone
	<-execution.Done()
	if downstreamRuns != 0 {
		t.Fatalf("downstream nodes ran %d times after cancellation", downstreamRuns)
	}
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("Result error = %v, want ErrExecutionCanceled", err)
	}
}

func TestExecutionConvertsNodePanicToFailure(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	panicNode := NewExecNode("panic", NewNodeDefinition("PanicExecution", func() IExecNode {
		return &panicExecutionNode{}
	}, []IPort{NewPortExec()}, nil))
	entrance.Next = []*ExecNode{panicNode}
	panicNode.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("panic", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 2})
	graphID := bp.Create("panic")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if _, err := execution.Result(); err == nil || !strings.Contains(err.Error(), "test panic") {
		t.Fatalf("panic result error = %v", err)
	}
	if execution.State() != ExecutionFailed || bp.activeExecutionCount() != 0 {
		t.Fatalf("panic state=%v active=%d", execution.State(), bp.activeExecutionCount())
	}
}

func (n *executionResultNode) GetName() string { return "ExecutionResult" }
func (n *executionResultNode) Exec() (int, error) {
	*n.runs++
	n.graph.appendReturn(ArrayData{IntVal: n.value})
	return -1, nil
}

func newExecutionTestBlueprint(t *testing.T, dispatcher ExecutionDispatcher) (*Blueprint, int64, *int) {
	t.Helper()
	runs := 0
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	result := NewExecNode("result", NewNodeDefinition("ExecutionResult", func() IExecNode {
		return &executionResultNode{value: 37, runs: &runs}
	}, []IPort{NewPortExec()}, nil))
	entrance.Next = []*ExecNode{result}
	result.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("execution", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: entrance},
		NodeCount: 2,
	})
	graphID := bp.Create("execution")
	if graphID == 0 {
		t.Fatal("Create returned zero graph ID")
	}
	return bp, graphID, &runs
}

func TestBlueprintStartDoesNotExecuteNodesOnCaller(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	bp, graphID, runs := newExecutionTestBlueprint(t, dispatcher)

	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if *runs != 0 {
		t.Fatalf("nodes ran before dispatcher admission: %d", *runs)
	}
	if execution.State() != ExecutionPending {
		t.Fatalf("state = %v, want pending", execution.State())
	}
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionPending) {
		t.Fatalf("pending Result error = %v, want ErrExecutionPending", err)
	}

	dispatcher.runNext(t)
	if *runs != 1 {
		t.Fatalf("nodes ran %d times, want 1", *runs)
	}
	result, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	if len(result) != 1 || result[0].IntVal != 37 {
		t.Fatalf("result = %#v, want integer 37", result)
	}
	if got := bp.activeExecutionCount(); got != 0 {
		t.Fatalf("active executions after completion = %d, want 0", got)
	}
}

func TestExecutionContextCancelCompletesSuspendedExecution(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	var continuation *Continuation
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("ExecutionWait", func() IExecNode {
		return &captureSingleContinuation{target: &continuation}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	entrance.Next = []*ExecNode{wait}
	wait.BeConnect = true

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("cancel", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 2})
	graphID := bp.Create("cancel")
	ctx, cancel := context.WithCancel(context.Background())
	execution, err := bp.Start(ctx, graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	cancel()
	<-execution.Done()
	if _, err := execution.Result(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Result error = %v, want context.Canceled", err)
	}
	if err := continuation.Resume(); !errors.Is(err, context.Canceled) && !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("Resume after cancel error = %v", err)
	}
}

func TestBlueprintCloseCancelsExecutionsAndRejectsNewWork(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	bp, graphID, runs := newExecutionTestBlueprint(t, dispatcher)
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := bp.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	<-execution.Done()
	if _, err := execution.Result(); !errors.Is(err, ErrBlueprintClosed) {
		t.Fatalf("Result error = %v, want ErrBlueprintClosed", err)
	}
	dispatcher.runNext(t)
	if *runs != 0 {
		t.Fatalf("closed blueprint executed %d nodes", *runs)
	}
	if _, err := bp.Start(context.Background(), graphID, 1); !errors.Is(err, ErrBlueprintClosed) {
		t.Fatalf("Start after close error = %v, want ErrBlueprintClosed", err)
	}
	if id := bp.Create("execution"); id != 0 {
		t.Fatalf("Create after close = %d, want 0", id)
	}
}

func TestBlueprintStartRejectsWithoutRetainingExecution(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{rejected: true}
	bp, graphID, _ := newExecutionTestBlueprint(t, dispatcher)

	if _, err := bp.Start(context.Background(), graphID, 1); !errors.Is(err, ErrExecutionRejected) {
		t.Fatalf("Start error = %v, want ErrExecutionRejected", err)
	}
	if got := bp.activeExecutionCount(); got != 0 {
		t.Fatalf("active executions = %d, want 0", got)
	}
}

func TestExecutionCancelBeforeDispatchPreventsNodeExecution(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	bp, graphID, runs := newExecutionTestBlueprint(t, dispatcher)

	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !execution.Cancel() {
		t.Fatal("Cancel returned false")
	}
	dispatcher.runNext(t)
	if *runs != 0 {
		t.Fatalf("canceled execution ran %d nodes", *runs)
	}
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("Result error = %v, want ErrExecutionCanceled", err)
	}
}

func TestContinuationResumeUsesExecutionDispatcher(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	var continuation *Continuation
	var values []PortInt
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("ExecutionWait", func() IExecNode {
		return &captureSingleContinuation{target: &continuation}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))
	record := NewExecNode("record", NewNodeDefinition("ExecutionRecord", func() IExecNode {
		return &testAppendRecorder{values: &values}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))
	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{record}
	wait.BeConnect = true
	record.BeConnect = true
	record.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 1}

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("suspend", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 3})
	graphID := bp.Create("suspend")
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || continuation == nil {
		t.Fatalf("state = %v continuation=%v, want suspended continuation", execution.State(), continuation)
	}
	if err := continuation.Resume(42); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if len(values) != 0 || dispatcher.len() != 1 {
		t.Fatalf("resume ran inline: values=%v queued=%d", values, dispatcher.len())
	}
	dispatcher.runNext(t)
	if len(values) != 1 || values[0] != 42 {
		t.Fatalf("values = %v, want [42]", values)
	}
	if execution.State() != ExecutionCompleted {
		t.Fatalf("state = %v, want completed", execution.State())
	}
}

type captureSingleContinuation struct {
	BaseExecNode
	target **Continuation
}

type resumeBeforeSuspendReturns struct{ BaseExecNode }

func (n *resumeBeforeSuspendReturns) GetName() string { return "ResumeBeforeSuspendReturns" }
func (n *resumeBeforeSuspendReturns) Exec() (int, error) {
	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}
	if err := continuation.Resume(64); err != nil {
		return -1, err
	}
	return -1, ErrExecutionSuspended
}

func TestContinuationResumeBeforeSuspendReturnsUsesDispatcher(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	var values []PortInt
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("ResumeBeforeSuspendReturns", func() IExecNode { return &resumeBeforeSuspendReturns{} }, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))
	record := NewExecNode("record", NewNodeDefinition("ExecutionRecord", func() IExecNode { return &testAppendRecorder{values: &values} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{record}
	wait.BeConnect = true
	record.BeConnect = true
	record.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 1}

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("early-resume", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 3})
	execution, err := bp.Start(context.Background(), bp.Create("early-resume"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if len(values) != 0 || dispatcher.len() != 1 {
		t.Fatalf("early resume ran inline: values=%v queued=%d", values, dispatcher.len())
	}
	dispatcher.runNext(t)
	if len(values) != 1 || values[0] != 64 || execution.State() != ExecutionCompleted {
		t.Fatalf("values=%v state=%v", values, execution.State())
	}
}

func (n *captureSingleContinuation) GetName() string { return "CaptureSingleContinuation" }
func (n *captureSingleContinuation) Exec() (int, error) {
	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}
	*n.target = continuation
	return -1, ErrExecutionSuspended
}
