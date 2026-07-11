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

type dynamicContinuationNode struct {
	BaseExecNode
	target **Continuation
}

type dynamicResumeBeforeSuspendReturns struct {
	BaseExecNode
	nextIndex int
	value     PortInt
}

func (n *dynamicResumeBeforeSuspendReturns) GetName() string {
	return "DynamicResumeBeforeSuspendReturns"
}
func (n *dynamicResumeBeforeSuspendReturns) Exec() (int, error) {
	continuation, err := n.SuspendForResume()
	if err != nil {
		return -1, err
	}
	if err := continuation.ResumeTo(n.nextIndex, n.value); err != nil {
		return -1, err
	}
	return -1, ErrExecutionSuspended
}

func (n *dynamicContinuationNode) GetName() string { return "DynamicContinuation" }
func (n *dynamicContinuationNode) Exec() (int, error) {
	continuation, err := n.SuspendForResume()
	if err != nil {
		return -1, err
	}
	*n.target = continuation
	return -1, ErrExecutionSuspended
}

func newDynamicContinuationExecution(t *testing.T) (*manualExecutionDispatcher, *Execution, **Continuation, *[]PortInt, *[]PortInt) {
	t.Helper()
	dispatcher := &manualExecutionDispatcher{}
	var continuation *Continuation
	var successValues []PortInt
	var failureValues []PortInt
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("DynamicContinuation", func() IExecNode {
		return &dynamicContinuationNode{target: &continuation}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortExec(), NewPortInt()}))
	success := NewExecNode("success", NewNodeDefinition("SuccessRecorder", func() IExecNode { return &testAppendRecorder{values: &successValues} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	failure := NewExecNode("failure", NewNodeDefinition("FailureRecorder", func() IExecNode { return &testAppendRecorder{values: &failureValues} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{success, failure}
	wait.BeConnect = true
	success.BeConnect = true
	failure.BeConnect = true
	success.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 2}
	failure.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 2}

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("dynamic-continuation", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 4})
	execution, err := bp.Start(context.Background(), bp.Create("dynamic-continuation"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if continuation == nil || execution.State() != ExecutionSuspended {
		t.Fatalf("continuation=%v state=%v", continuation, execution.State())
	}
	return dispatcher, execution, &continuation, &successValues, &failureValues
}

func TestContinuationResumeToSuccessBranch(t *testing.T) {
	dispatcher, execution, continuation, successValues, failureValues := newDynamicContinuationExecution(t)
	if err := (*continuation).ResumeTo(0, 41); err != nil {
		t.Fatal(err)
	}
	if len(*successValues) != 0 || dispatcher.len() != 1 {
		t.Fatalf("resume ran inline: success=%v queued=%d", *successValues, dispatcher.len())
	}
	dispatcher.runNext(t)
	if len(*successValues) != 1 || (*successValues)[0] != 41 || len(*failureValues) != 0 {
		t.Fatalf("success=%v failure=%v", *successValues, *failureValues)
	}
	if execution.State() != ExecutionCompleted {
		t.Fatalf("state=%v, want completed", execution.State())
	}
}

func TestContinuationResumeToFailureBranch(t *testing.T) {
	dispatcher, execution, continuation, successValues, failureValues := newDynamicContinuationExecution(t)
	if err := (*continuation).ResumeTo(1, 42); err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if len(*failureValues) != 1 || (*failureValues)[0] != 42 || len(*successValues) != 0 {
		t.Fatalf("success=%v failure=%v", *successValues, *failureValues)
	}
	if execution.State() != ExecutionCompleted {
		t.Fatalf("state=%v, want completed", execution.State())
	}
}

func TestContinuationResumeToEarlyCallbackKeepsSelectedBranch(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	var successValues []PortInt
	var failureValues []PortInt
	entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("DynamicResumeBeforeSuspendReturns", func() IExecNode {
		return &dynamicResumeBeforeSuspendReturns{nextIndex: 1, value: 73}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortExec(), NewPortInt()}))
	success := NewExecNode("success", NewNodeDefinition("SuccessRecorder", func() IExecNode { return &testAppendRecorder{values: &successValues} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	failure := NewExecNode("failure", NewNodeDefinition("FailureRecorder", func() IExecNode { return &testAppendRecorder{values: &failureValues} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{success, failure}
	wait.BeConnect = true
	success.BeConnect = true
	failure.BeConnect = true
	success.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 2}
	failure.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 2}

	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("dynamic-early", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 4})
	execution, err := bp.Start(context.Background(), bp.Create("dynamic-early"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if len(successValues) != 0 || len(failureValues) != 0 || dispatcher.len() != 1 {
		t.Fatalf("early callback ran inline: success=%v failure=%v queued=%d", successValues, failureValues, dispatcher.len())
	}
	dispatcher.runNext(t)
	if len(successValues) != 0 || len(failureValues) != 1 || failureValues[0] != 73 {
		t.Fatalf("success=%v failure=%v", successValues, failureValues)
	}
	if execution.State() != ExecutionCompleted {
		t.Fatalf("state=%v", execution.State())
	}
}

func TestContinuationDynamicAPIValidation(t *testing.T) {
	t.Run("invalid target does not consume continuation", func(t *testing.T) {
		dispatcher, _, continuation, successValues, _ := newDynamicContinuationExecution(t)
		if err := (*continuation).ResumeTo(2, 10); err == nil {
			t.Fatal("data output was accepted as an exec target")
		}
		if err := (*continuation).ResumeTo(0, 11); err != nil {
			t.Fatalf("valid retry failed: %v", err)
		}
		dispatcher.runNext(t)
		if len(*successValues) != 1 || (*successValues)[0] != 11 {
			t.Fatalf("success values=%v", *successValues)
		}
	})

	t.Run("dynamic continuation requires ResumeTo", func(t *testing.T) {
		dispatcher, _, continuation, successValues, _ := newDynamicContinuationExecution(t)
		if err := (*continuation).Resume(12); !errors.Is(err, ErrContinuationTargetRequired) {
			t.Fatalf("Resume error=%v", err)
		}
		if err := (*continuation).ResumeAsync(12); !errors.Is(err, ErrContinuationTargetRequired) {
			t.Fatalf("ResumeAsync error=%v", err)
		}
		if err := (*continuation).ResumeTo(0, 12); err != nil {
			t.Fatal(err)
		}
		dispatcher.runNext(t)
		if len(*successValues) != 1 || (*successValues)[0] != 12 {
			t.Fatalf("success values=%v", *successValues)
		}
	})

	t.Run("fixed continuation rejects ResumeTo", func(t *testing.T) {
		dispatcher := &manualExecutionDispatcher{}
		var continuation *Continuation
		entrance := NewExecNode("entrance", NewNodeDefinition("ExecutionEntrance", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
		wait := NewExecNode("wait", NewNodeDefinition("ExecutionWait", func() IExecNode { return &captureSingleContinuation{target: &continuation} }, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
		entrance.Next = []*ExecNode{wait}
		wait.BeConnect = true
		bp := &Blueprint{}
		bp.SetExecutionDispatcher(dispatcher)
		bp.AddCompiledGraph("fixed-continuation", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 2})
		if _, err := bp.Start(context.Background(), bp.Create("fixed-continuation"), 1); err != nil {
			t.Fatal(err)
		}
		dispatcher.runNext(t)
		if err := continuation.ResumeTo(0); !errors.Is(err, ErrContinuationTargetFixed) {
			t.Fatalf("ResumeTo error=%v", err)
		}
		if err := continuation.Resume(); err != nil {
			t.Fatalf("fixed Resume failed after rejected ResumeTo: %v", err)
		}
	})

	t.Run("duplicate response is rejected", func(t *testing.T) {
		dispatcher, _, continuation, _, _ := newDynamicContinuationExecution(t)
		if err := (*continuation).ResumeTo(0, 13); err != nil {
			t.Fatal(err)
		}
		if err := (*continuation).ResumeTo(1, 14); !errors.Is(err, ErrContinuationResumed) {
			t.Fatalf("second response error=%v", err)
		}
		if dispatcher.len() != 1 {
			t.Fatalf("queued tasks=%d, want 1", dispatcher.len())
		}
	})

	t.Run("canceled execution rejects late response", func(t *testing.T) {
		dispatcher, execution, continuation, _, _ := newDynamicContinuationExecution(t)
		if !execution.Cancel() {
			t.Fatal("Cancel returned false")
		}
		if err := (*continuation).ResumeTo(0, 15); !errors.Is(err, ErrExecutionCanceled) {
			t.Fatalf("late response error=%v", err)
		}
		if dispatcher.len() != 0 {
			t.Fatalf("late response queued %d task(s)", dispatcher.len())
		}
	})
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
