package golang

import "testing"

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
	values []PortInt
}

func (n *testRecorder) GetName() string { return "TestRecorder" }
func (n *testRecorder) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if ok {
		n.values = append(n.values, value)
	}
	return -1, nil
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
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
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
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if err := async.continuation.Resume(42); err != nil {
		t.Fatalf("first Resume failed: %v", err)
	}
	if err := async.continuation.Resume(43); err == nil {
		t.Fatalf("second Resume succeeded, want error")
	}
}
