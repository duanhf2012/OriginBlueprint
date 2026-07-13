package blueprint

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type functionLocalProbe struct {
	BaseExecNode
	values *[]PortInt
	locks  *[]*sync.RWMutex
}

type functionExecutionResult struct {
	BaseExecNode
	runs *int
}

func (n *functionExecutionResult) GetName() string { return "FunctionExecutionResult" }
func (n *functionExecutionResult) Exec() (int, error) {
	value, _ := n.GetInPortInt(1)
	*n.runs++
	n.graph.appendReturn(ArrayData{IntVal: value})
	return -1, nil
}

type immediateFunctionResume struct {
	BaseExecNode
	value    PortInt
	returned *bool
}

type inlineFunctionDispatcher struct{}

func (inlineFunctionDispatcher) Submit(task func()) error {
	task()
	return nil
}

func (n *immediateFunctionResume) GetName() string { return "ImmediateFunctionResume" }
func (n *immediateFunctionResume) Exec() (int, error) {
	continuation, err := n.SuspendForResume()
	if err != nil {
		return -1, err
	}
	if err := continuation.ResumeTo(0, n.value); err != nil {
		return -1, err
	}
	if n.returned != nil {
		*n.returned = true
	}
	return -1, ErrExecutionSuspended
}

type functionReentryProbe struct {
	BaseExecNode
	resumeReturned *bool
	reentered      *bool
}

func (n *functionReentryProbe) GetName() string { return "FunctionReentryProbe" }
func (n *functionReentryProbe) Exec() (int, error) {
	if n.resumeReturned != nil && !*n.resumeReturned {
		*n.reentered = true
	}
	value, _ := n.GetInPortInt(1)
	n.SetOutPortInt(1, value)
	return 0, nil
}

func (n *functionLocalProbe) GetName() string { return "FunctionLocalProbe" }
func (n *functionLocalProbe) Exec() (int, error) {
	port := n.graph.variables["counter"]
	value, _ := port.GetInt()
	*n.values = append(*n.values, value)
	*n.locks = append(*n.locks, n.graph.variableMu)
	port.SetInt(value + 1)
	return 0, nil
}

func TestFunctionVariablesAndLocksAreFreshPerInvocation(t *testing.T) {
	var values []PortInt
	var locks []*sync.RWMutex
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewNodeDefinition("FunctionLocalProbe", func() IExecNode {
		return &functionLocalProbe{values: &values, locks: &locks}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	functionGraph, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "counter", Type: "integer", Value: 0}},
		Nodes:     []NodeConfig{{ID: "entry", Class: "FunctionEntry"}, {ID: "probe", Class: "FunctionLocalProbe"}, {ID: "return", Class: "FunctionReturn"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "probe", DesPortID: 0}, {SourceNodeID: "probe", SourcePortID: 0, DesNodeID: "return", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"local": functionGraph},
		Nodes:     []NodeConfig{{ID: "entry", Class: "Entrance_IntParam_1"}, {ID: "call", Class: "FunctionCall", FunctionName: "local"}},
		Edges:     []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Do(1); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != 0 || values[1] != 0 {
		t.Fatalf("function local values = %v, want [0 0]", values)
	}
	if len(locks) != 2 || locks[0] == locks[1] || locks[0] == graph.variableMu || locks[1] == graph.variableMu {
		t.Fatal("function invocations shared a variable lock")
	}
}

func TestFunctionCallReturnsValuesToCaller(t *testing.T) {
	var recorder *testRecorder
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	})

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer", "Integer"}},
			{ID: "add", Class: "AddInt"},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "add", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 2, DesNodeID: "add", DesPortID: 1},
			{SourceNodeID: "add", SourcePortID: 0, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}

	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"sum": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "sum", FunctionInputTypes: []string{"Integer", "Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 2, 2: 5}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}
	callNode := mainGraph.Entrances[1].Next[0]
	if callNode.FunctionGraph != functionGraph {
		t.Fatalf("FunctionCall was not pre-resolved at compile time")
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 7 {
		t.Fatalf("recorder values = %#v, want [7]", recorder)
	}
}

func TestFunctionCallContinuesAfterAsyncFunctionReturn(t *testing.T) {
	recorder := &testRecorder{}
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode {
		return recorder
	})

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "sleep", Class: "Sleep", PortDefault: map[int]any{1: 5}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sleep", DesPortID: 0},
			{SourceNodeID: "sleep", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile async function failed: %v", err)
	}

	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"delayed": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "delayed", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 9}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err != ErrExecutionSuspended {
		t.Fatalf("Do error = %v, want ErrExecutionSuspended", err)
	}
	if len(recorder.snapshot()) != 0 {
		t.Fatalf("recorder ran before async function returned")
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		values := recorder.snapshot()
		if len(values) == 1 && values[0] == 9 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("recorder values = %#v, want [9]", recorder.snapshot())
}

func TestFunctionDelayUsesExecutionSchedulerAndDispatcher(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	bp, graphID, runs := newFunctionDelayExecutionBlueprint(t, dispatcher, scheduler)

	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || *runs != 0 {
		t.Fatalf("state=%v runs=%d, want suspended and zero runs", execution.State(), *runs)
	}

	scheduler.fire(t, scheduler.onlyHandle(t))
	if dispatcher.len() != 1 || *runs != 0 {
		t.Fatalf("function Delay resumed outside dispatcher: queued=%d runs=%d", dispatcher.len(), *runs)
	}
	dispatcher.runNext(t)
	if dispatcher.len() != 1 || *runs != 0 {
		t.Fatalf("FunctionReturn resumed caller outside dispatcher: queued=%d runs=%d", dispatcher.len(), *runs)
	}
	dispatcher.runNext(t)

	result, err := execution.Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].IntVal != 9 || *runs != 1 {
		t.Fatalf("result=%#v runs=%d, want [9] and one run", result, *runs)
	}
}

func TestFunctionDelayCancellationStopsNestedContinuation(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	bp, graphID, runs := newFunctionDelayExecutionBlueprint(t, dispatcher, scheduler)
	execution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	handle := scheduler.onlyHandle(t)
	lateCallback := scheduler.tasks[handle]

	if !execution.Cancel() {
		t.Fatal("Cancel returned false")
	}
	<-execution.Done()
	if _, err := execution.Result(); !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("Result error=%v, want ErrExecutionCanceled", err)
	}
	if !scheduler.canceled[handle] || len(scheduler.tasks) != 0 {
		t.Fatalf("scheduler canceled=%v tasks=%d, want canceled and empty", scheduler.canceled[handle], len(scheduler.tasks))
	}
	lateCallback()
	if dispatcher.len() != 0 || *runs != 0 {
		t.Fatalf("late callback queued=%d runs=%d after cancellation", dispatcher.len(), *runs)
	}
}

func TestFunctionDelayReleaseAndCloseCancelNestedSchedule(t *testing.T) {
	tests := []struct {
		name    string
		stop    func(*Blueprint, int64)
		wantErr error
	}{
		{name: "ReleaseGraph", stop: func(bp *Blueprint, graphID int64) { bp.ReleaseGraph(graphID) }, wantErr: ErrGraphReleased},
		{name: "Close", stop: func(bp *Blueprint, _ int64) { _ = bp.Close() }, wantErr: ErrBlueprintClosed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatcher := &manualExecutionDispatcher{}
			scheduler := newManualTimerScheduler()
			bp, graphID, runs := newFunctionDelayExecutionBlueprint(t, dispatcher, scheduler)
			execution, err := bp.Start(context.Background(), graphID, 1)
			if err != nil {
				t.Fatal(err)
			}
			dispatcher.runNext(t)
			handle := scheduler.onlyHandle(t)
			lateCallback := scheduler.tasks[handle]

			test.stop(bp, graphID)
			<-execution.Done()
			if _, err := execution.Result(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Result error=%v, want %v", err, test.wantErr)
			}
			if !scheduler.canceled[handle] || len(scheduler.tasks) != 0 {
				t.Fatalf("scheduler canceled=%v tasks=%d, want canceled and empty", scheduler.canceled[handle], len(scheduler.tasks))
			}
			lateCallback()
			if dispatcher.len() != 0 || *runs != 0 {
				t.Fatalf("late callback queued=%d runs=%d after %s", dispatcher.len(), *runs, test.name)
			}
		})
	}
}

func TestFunctionImmediateContinuationUsesExecutionDispatcher(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	runs := 0
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewNodeDefinition("ImmediateFunctionResume", func() IExecNode {
		return &immediateFunctionResume{value: 41}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("FunctionExecutionResult", func() IExecNode {
		return &functionExecutionResult{runs: &runs}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry"},
			{ID: "resume", Class: "ImmediateFunctionResume"},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "resume", DesPortID: 0},
			{SourceNodeID: "resume", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "resume", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"immediate": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "immediate", FunctionOutputTypes: []string{"Integer"}},
			{ID: "result", Class: "FunctionExecutionResult"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("immediate-function", mainGraph)
	execution, err := bp.Start(context.Background(), bp.Create("immediate-function"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if dispatcher.len() != 1 || runs != 0 || execution.IsDone() {
		t.Fatalf("queued=%d runs=%d done=%v, want continuation queued", dispatcher.len(), runs, execution.IsDone())
	}
	dispatcher.runNext(t)
	if dispatcher.len() != 1 || runs != 0 {
		t.Fatalf("queued=%d runs=%d, want caller continuation queued", dispatcher.len(), runs)
	}
	dispatcher.runNext(t)
	result, err := execution.Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].IntVal != 41 || runs != 1 {
		t.Fatalf("result=%#v runs=%d, want [41] and one run", result, runs)
	}
}

func TestFunctionImmediateContinuationDoesNotReenterRunningRoot(t *testing.T) {
	runs := 0
	resumeReturned := false
	reentered := false
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewNodeDefinition("ImmediateFunctionResume", func() IExecNode {
		return &immediateFunctionResume{value: 41, returned: &resumeReturned}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("FunctionReentryProbe", func() IExecNode {
		return &functionReentryProbe{resumeReturned: &resumeReturned, reentered: &reentered}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("FunctionExecutionResult", func() IExecNode {
		return &functionExecutionResult{runs: &runs}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry"},
			{ID: "resume", Class: "ImmediateFunctionResume"},
			{ID: "probe", Class: "FunctionReentryProbe"},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "resume", DesPortID: 0},
			{SourceNodeID: "resume", SourcePortID: 0, DesNodeID: "probe", DesPortID: 0},
			{SourceNodeID: "resume", SourcePortID: 1, DesNodeID: "probe", DesPortID: 1},
			{SourceNodeID: "probe", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "probe", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"immediate": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "immediate", FunctionOutputTypes: []string{"Integer"}},
			{ID: "result", Class: "FunctionExecutionResult"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(inlineFunctionDispatcher{})
	bp.AddCompiledGraph("inline-function", mainGraph)
	execution, err := bp.Start(context.Background(), bp.Create("inline-function"), 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := execution.Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].IntVal != 41 || runs != 1 {
		t.Fatalf("result=%#v runs=%d, want [41] and one run", result, runs)
	}
	if reentered {
		t.Fatal("function continuation resumed before ResumeTo returned")
	}
}

func TestNestedFunctionDelayResumesEachFunctionFrameInOrder(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	scheduler := newManualTimerScheduler()
	runs := 0
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewDelayNodeDefinition())
	registry.Register(NewNodeDefinition("FunctionExecutionResult", func() IExecNode {
		return &functionExecutionResult{runs: &runs}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	inner, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "delay", Class: "Delay", PortDefault: map[int]any{1: 25}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "delay", DesPortID: 0},
			{SourceNodeID: "delay", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	outer, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"inner": inner},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "call", Class: "FunctionCall", FunctionName: "inner", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "call", DesPortID: 1},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"outer": outer},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "outer", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 9}},
			{ID: "result", Class: "FunctionExecutionResult"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("nested-function", mainGraph)
	execution, err := bp.Start(context.Background(), bp.Create("nested-function"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || runs != 0 {
		t.Fatalf("state=%v runs=%d before timer", execution.State(), runs)
	}
	scheduler.fire(t, scheduler.onlyHandle(t))
	for step := 0; step < 3; step++ {
		if dispatcher.len() != 1 {
			t.Fatalf("step %d queued=%d, want one serialized continuation", step, dispatcher.len())
		}
		dispatcher.runNext(t)
	}
	result, err := execution.Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].IntVal != 9 || runs != 1 {
		t.Fatalf("result=%#v runs=%d, want [9] and one run", result, runs)
	}
}

func TestFunctionContinuationAdmissionRacesWithCancellation(t *testing.T) {
	dispatcher := &manualExecutionDispatcher{}
	var continuation *Continuation
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewNodeDefinition("DynamicContinuation", func() IExecNode {
		return &dynamicContinuationNode{target: &continuation}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortExec(), NewPortInt()}))

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry"},
			{ID: "wait", Class: "DynamicContinuation"},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "wait", DesPortID: 0},
			{SourceNodeID: "wait", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "wait", SourcePortID: 2, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"wait": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "wait", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("cancel-admission", mainGraph)
	execution, err := bp.Start(context.Background(), bp.Create("cancel-admission"), 1)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if continuation == nil || execution.State() != ExecutionSuspended {
		t.Fatalf("continuation=%v state=%v", continuation, execution.State())
	}

	execution.scope.mu.Lock()
	resumeErr := make(chan error, 1)
	go func() { resumeErr <- continuation.ResumeTo(0, PortInt(9)) }()
	deadline := time.Now().Add(time.Second)
	for {
		continuation.mu.Lock()
		reserved := continuation.resumed
		continuation.mu.Unlock()
		if reserved {
			break
		}
		if time.Now().After(deadline) {
			execution.scope.mu.Unlock()
			t.Fatal("continuation did not reach admission gate")
		}
		time.Sleep(time.Millisecond)
	}
	cancelDone := make(chan bool, 1)
	go func() { cancelDone <- execution.Cancel() }()
	cancelObserved := false
	select {
	case <-cancelDone:
		cancelObserved = true
	case <-time.After(20 * time.Millisecond):
	}
	execution.scope.mu.Unlock()

	if err := <-resumeErr; !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("ResumeTo error=%v, want ErrExecutionCanceled", err)
	}
	if !cancelObserved {
		<-cancelDone
	}
}

func TestFunctionCallDepthLimitStopsRecursion(t *testing.T) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry"},
			{ID: "call", Class: "FunctionCall", FunctionName: "recurse"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("compile function failed: %v", err)
	}
	functionGraph.Functions = map[string]*CompiledGraph{"recurse": functionGraph}

	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"recurse": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "recurse"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("compile main failed: %v", err)
	}

	graph := NewGraph(mainGraph)
	if _, err := graph.Do(1); err == nil || !strings.Contains(err.Error(), "maximum function call depth") {
		t.Fatalf("Do error = %v, want maximum function call depth", err)
	}
}

func registerFunctionTestNodes(registry *Registry, recorderFactory func() IExecNode) {
	registry.Register(NewNodeDefinition("Entrance_IntParam", func() IExecNode { return &EntranceIntParam{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("AddInt", func() IExecNode { return &AddInt{} }, []IPort{NewPortInt(), NewPortInt()}, []IPort{NewPortInt()}))
	registry.Register(NewSleepNodeDefinition())
	registry.Register(NewNodeDefinition("TestRecorder", recorderFactory, []IPort{NewPortExec(), NewPortInt()}, nil))
}

func newFunctionDelayExecutionBlueprint(t *testing.T, dispatcher ExecutionDispatcher, scheduler TimerScheduler) (*Blueprint, int64, *int) {
	t.Helper()
	runs := 0
	registry := NewRegistry()
	registerFunctionTestNodes(registry, func() IExecNode { return &testRecorder{} })
	registry.Register(NewDelayNodeDefinition())
	registry.Register(NewNodeDefinition("FunctionExecutionResult", func() IExecNode {
		return &functionExecutionResult{runs: &runs}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	functionGraph, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "FunctionEntry", FunctionInputTypes: []string{"Integer"}},
			{ID: "delay", Class: "Delay", PortDefault: map[int]any{1: 25}},
			{ID: "return", Class: "FunctionReturn", FunctionOutputTypes: []string{"Integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "delay", DesPortID: 0},
			{SourceNodeID: "delay", SourcePortID: 0, DesNodeID: "return", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mainGraph, err := CompileGraph(registry, GraphConfig{
		Functions: map[string]*CompiledGraph{"delayed": functionGraph},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entrance_IntParam_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "delayed", FunctionInputTypes: []string{"Integer"}, FunctionOutputTypes: []string{"Integer"}, PortDefault: map[int]any{1: 9}},
			{ID: "result", Class: "FunctionExecutionResult"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.SetTimerScheduler(scheduler)
	bp.AddCompiledGraph("function-delay", mainGraph)
	return bp, bp.Create("function-delay"), &runs
}
