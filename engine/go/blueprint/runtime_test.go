package blueprint

import (
	"errors"
	"sync"
	"testing"
)

type testEntrance struct {
	BaseExecNode
}

func (n *testEntrance) GetName() string { return "TestEntrance" }
func (n *testEntrance) Exec() (int, error) {
	return 0, nil
}

type testAsync struct {
	BaseExecNode
	continuation *Continuation
}

func (n *testAsync) GetName() string { return "TestAsync" }
func (n *testAsync) Exec() (int, error) {
	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}
	n.continuation = continuation
	return -1, ErrExecutionSuspended
}

type testRecorder struct {
	BaseExecNode
	mu     sync.Mutex
	values []PortInt
}

func (n *testRecorder) GetName() string { return "TestRecorder" }
func (n *testRecorder) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if ok {
		n.mu.Lock()
		n.values = append(n.values, value)
		n.mu.Unlock()
	}
	return -1, nil
}

func (n *testRecorder) snapshot() []PortInt {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]PortInt(nil), n.values...)
}

type testArrayMutator struct {
	BaseExecNode
	mu      sync.Mutex
	lengths []PortInt
}

func (n *testArrayMutator) GetName() string { return "TestArrayMutator" }
func (n *testArrayMutator) Exec() (int, error) {
	length := n.GetInPort(1).GetArrayLen()
	_ = n.AppendInPortArrayValInt(1, 99)
	n.mu.Lock()
	n.lengths = append(n.lengths, length)
	n.mu.Unlock()
	return -1, nil
}

func (n *testArrayMutator) snapshot() []PortInt {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]PortInt(nil), n.lengths...)
}

type testGraphIDRecordSink struct {
	mu     sync.Mutex
	values map[int64][]PortInt
}

type testGraphIDRecorder struct {
	BaseExecNode
	sink *testGraphIDRecordSink
}

func (n *testGraphIDRecorder) GetName() string { return "TestGraphIDRecorder" }
func (n *testGraphIDRecorder) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if ok && n.sink != nil {
		n.sink.mu.Lock()
		if n.sink.values == nil {
			n.sink.values = map[int64][]PortInt{}
		}
		n.sink.values[n.graph.graphID] = append(n.sink.values[n.graph.graphID], value)
		n.sink.mu.Unlock()
	}
	return -1, nil
}

func (s *testGraphIDRecordSink) snapshot() map[int64][]PortInt {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make(map[int64][]PortInt, len(s.values))
	for graphID, graphValues := range s.values {
		values[graphID] = append([]PortInt(nil), graphValues...)
	}
	return values
}

func TestLegacyStyleSyncExecution(t *testing.T) {
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

	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}})
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if recorder == nil {
		t.Fatalf("recorder did not execute")
	}
}

func TestGraphRunEntranceReusesResultBuffers(t *testing.T) {
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 1})
	graph.returns = make(PortArray, 0, 8)
	graph.functionResults = make([]any, 0, 4)

	if _, err := graph.runEntrance(1); err != nil {
		t.Fatalf("runEntrance failed: %v", err)
	}
	if cap(graph.returns) != 8 {
		t.Fatalf("returns capacity = %d, want 8", cap(graph.returns))
	}
	if cap(graph.functionResults) != 4 {
		t.Fatalf("functionResults capacity = %d, want 4", cap(graph.functionResults))
	}
}

func TestGraphRepeatedDoDoesNotReuseDirtyInputPorts(t *testing.T) {
	mutator := &testArrayMutator{}
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	mutate := NewExecNode("mutate", NewNodeDefinition("TestArrayMutator", func() IExecNode {
		return mutator
	}, []IPort{NewPortExec(), NewPortArray()}, nil))
	defaultArray := NewPortArray()
	_ = defaultArray.setAnyValue([]int{1})
	mutate.DefaultInputs = []IPort{nil, defaultArray}
	mutate.DefaultInputSet = []bool{false, true}
	entrance.Next = []*ExecNode{mutate}
	mutate.BeConnect = true

	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 2})
	for index := 0; index < 2; index++ {
		if _, err := graph.Do(1); err != nil {
			t.Fatalf("Do %d failed: %v", index, err)
		}
	}
	if got := mutator.snapshot(); len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Fatalf("array input lengths = %#v, want [1 1]", got)
	}
}

func TestBlueprintConcurrentInstancesDoNotSharePortValues(t *testing.T) {
	sink := &testGraphIDRecordSink{}
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec(), NewPortInt()}))
	entrance.IsEntrance = true
	record := NewExecNode("record", NewNodeDefinition("TestGraphIDRecorder", func() IExecNode {
		return &testGraphIDRecorder{sink: sink}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))
	entrance.Next = []*ExecNode{record}
	record.BeConnect = true
	record.PreInPort[1] = &PrePortNode{Node: entrance, OutPortID: 1}

	var bp Blueprint
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}, NodeCount: 2})
	graphIDs := make([]int64, 8)
	for index := range graphIDs {
		graphIDs[index] = bp.Create("test")
		if graphIDs[index] == 0 {
			t.Fatalf("Create %d returned 0", index)
		}
	}

	var wg sync.WaitGroup
	for index, graphID := range graphIDs {
		graphID := graphID
		value := PortInt(index + 100)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repeat := 0; repeat < 50; repeat++ {
				if _, err := bp.Do(graphID, 1, value); err != nil {
					t.Errorf("Do graph %d failed: %v", graphID, err)
					return
				}
			}
		}()
	}
	wg.Wait()

	values := sink.snapshot()
	for index, graphID := range graphIDs {
		want := PortInt(index + 100)
		got := values[graphID]
		if len(got) != 50 {
			t.Fatalf("graph %d recorded %d values, want 50: %#v", graphID, len(got), got)
		}
		for valueIndex, value := range got {
			if value != want {
				t.Fatalf("graph %d value %d = %d, want %d; all values %#v", graphID, valueIndex, value, want, got)
			}
		}
	}
}

func TestAsyncContinuationResumeContinuesFromSuspendedNode(t *testing.T) {
	var async *testAsync
	var recorder *testRecorder
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("TestAsync", func() IExecNode {
		async = &testAsync{}
		return async
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))
	record := NewExecNode("record", NewNodeDefinition("TestRecorder", func() IExecNode {
		recorder = &testRecorder{}
		return recorder
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{record}
	wait.BeConnect = true
	record.BeConnect = true
	record.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 1}

	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}})
	if _, err := graph.Do(1); err != ErrExecutionSuspended {
		t.Fatalf("Do error = %v, want ErrExecutionSuspended", err)
	}
	if async == nil || async.continuation == nil {
		t.Fatalf("async node did not suspend")
	}
	if recorder != nil {
		t.Fatalf("recorder ran before resume")
	}

	if err := async.continuation.Resume(42); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 42 {
		t.Fatalf("recorder values = %#v, want [42]", recorder)
	}
}

func TestContinuationResumeOnlyOnce(t *testing.T) {
	var async *testAsync
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("TestAsync", func() IExecNode {
		async = &testAsync{}
		return async
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))

	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{nil}
	wait.BeConnect = true

	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}})
	if _, err := graph.Do(1); err != ErrExecutionSuspended {
		t.Fatalf("Do error = %v, want ErrExecutionSuspended", err)
	}
	if err := async.continuation.Resume(42); err != nil {
		t.Fatalf("first Resume failed: %v", err)
	}
	if err := async.continuation.Resume(43); err == nil {
		t.Fatalf("second Resume succeeded, want error")
	}
}

type contextArrayProducer struct{ BaseExecNode }

func (n *contextArrayProducer) GetName() string { return "ContextArrayProducer" }

func (n *contextArrayProducer) Exec() (int, error) {
	values := make(PortArray, 1024)
	n.GetOutPort(0).setAnyValue(values)
	return -1, nil
}

type contextSuspendNode struct {
	BaseExecNode
	target **Continuation
}

func (n *contextSuspendNode) GetName() string { return "ContextSuspend" }

func (n *contextSuspendNode) Exec() (int, error) {
	continuation, err := n.Suspend(-1)
	if err != nil {
		return -1, err
	}
	*n.target = continuation
	return -1, ErrExecutionSuspended
}

type reentrantContextNode struct {
	BaseExecNode
	t              *testing.T
	outerContextOK *bool
}

func (n *reentrantContextNode) GetName() string { return "ReentrantContext" }

func (n *reentrantContextNode) Exec() (int, error) {
	if n.ctx.ExecInputPortID == 1 {
		n.SetOutPortInt(1, 22)
		return -1, nil
	}
	n.SetOutPortInt(1, 11)
	if err := n.DoNext(0); err != nil {
		return -1, err
	}
	value, ok := n.GetOutPortInt(1)
	*n.outerContextOK = ok && value == 11
	if !*n.outerContextOK {
		n.t.Errorf("outer context changed during reentry: value=%d ok=%v", value, ok)
	}
	return -1, nil
}

func TestGraphReusesNodeContextFramesAcrossRuns(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("Record", func() IExecNode { return &testRecorder{} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "Entry_1"},
			{ID: "record", Class: "Record", PortDefault: map[int]any{1: 7}},
		},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "record", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	graph := NewGraph(compiled)
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	record := compiled.Entrances[1].Next[0]
	first, ok := graph.getContext(record)
	if !ok {
		t.Fatal("first context not found")
	}
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("second Do failed: %v", err)
	}
	second, ok := graph.getContext(record)
	if !ok {
		t.Fatal("second context not found")
	}
	if first != second {
		t.Fatalf("context frames were not reused: first=%p second=%p", first, second)
	}
}

func TestGraphContextFramesKeepReentrantExecutionsIsolated(t *testing.T) {
	outerContextOK := false
	definition := NewNodeDefinition(
		"ReentrantContext",
		func() IExecNode { return &reentrantContextNode{t: t, outerContextOK: &outerContextOK} },
		[]IPort{NewPortExec(), NewPortExec()},
		[]IPort{NewPortExec(), NewPortInt()},
	)
	node := NewExecNode("reentrant", definition)
	node.Index = 0
	node.Next = []*ExecNode{node}
	node.NextInPort = []int{1}
	graph := NewGraph(&CompiledGraph{NodeCount: 1})
	if err := node.doWithInput(graph, 0); err != nil {
		t.Fatalf("reentrant execution failed: %v", err)
	}
	if !outerContextOK {
		t.Fatal("outer execution context was not isolated")
	}
}

func TestSuspendedContextFrameIsReleasedAfterResume(t *testing.T) {
	var continuation *Continuation
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition(
		"ContextSuspend",
		func() IExecNode { return &contextSuspendNode{target: &continuation} },
		[]IPort{NewPortExec()},
		nil,
	))
	entry.Index = 0
	wait.Index = 1
	entry.Next = []*ExecNode{wait}
	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}, NodeCount: 2})
	if _, err := graph.Do(1); !errors.Is(err, ErrExecutionSuspended) {
		t.Fatalf("Do error = %v, want ErrExecutionSuspended", err)
	}
	ctx, ok := graph.getContext(wait)
	if !ok {
		t.Fatal("suspended context not found")
	}
	if ctx.state&execContextActiveBit == 0 {
		t.Fatalf("suspended context state = %x, want active", ctx.state)
	}
	if continuation == nil {
		t.Fatal("continuation was not captured")
	}
	if err := continuation.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	ctx, ok = graph.getContext(wait)
	if !ok {
		t.Fatal("resumed context not found")
	}
	if ctx.state&execContextActiveBit != 0 {
		t.Fatalf("resumed context state = %x, want inactive", ctx.state)
	}
}

func TestGraphCompletionReleasesCachedDynamicPortValues(t *testing.T) {
	producer := NewExecNode("producer", NewNodeDefinition(
		"ContextArrayProducer",
		func() IExecNode { return &contextArrayProducer{} },
		[]IPort{NewPortExec()},
		[]IPort{NewPortArray()},
	))
	producer.Index = 1
	entry := NewExecNode("entry", NewNodeDefinition("Entry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec()}))
	entry.Index = 0
	entry.Next = []*ExecNode{producer}
	graph := NewGraph(&CompiledGraph{Entrances: map[int64]*ExecNode{1: entry}, NodeCount: 2})
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	ctx, ok := graph.getContext(producer)
	if !ok {
		t.Fatal("producer context not found")
	}
	values, ok := ctx.OutputPorts[0].GetArray()
	if !ok || values != nil {
		t.Fatalf("cached array value retained after completion: len=%d ok=%v", len(values), ok)
	}
}
