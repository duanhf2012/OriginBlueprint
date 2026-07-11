package blueprint

import (
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
