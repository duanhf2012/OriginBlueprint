package blueprint

import "testing"

type testCaptureAsync struct {
	BaseExecNode
	continuations *[]*Continuation
}

func (n *testCaptureAsync) GetName() string { return "TestCaptureAsync" }
func (n *testCaptureAsync) Exec() (int, error) {
	continuation, err := n.Suspend(0)
	if err != nil {
		return -1, err
	}
	*n.continuations = append(*n.continuations, continuation)
	return -1, ErrExecutionSuspended
}

type testAppendRecorder struct {
	BaseExecNode
	values *[]PortInt
}

func (n *testAppendRecorder) GetName() string { return "TestAppendRecorder" }
func (n *testAppendRecorder) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if ok {
		*n.values = append(*n.values, value)
	}
	return -1, nil
}

func TestBlueprintDoCreatesIndependentExecutionSessions(t *testing.T) {
	var continuations []*Continuation
	var values []PortInt
	entrance := NewExecNode("entrance", NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	wait := NewExecNode("wait", NewNodeDefinition("TestCaptureAsync", func() IExecNode {
		return &testCaptureAsync{continuations: &continuations}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec(), NewPortInt()}))
	record := NewExecNode("record", NewNodeDefinition("TestAppendRecorder", func() IExecNode {
		return &testAppendRecorder{values: &values}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	entrance.Next = []*ExecNode{wait}
	wait.Next = []*ExecNode{record}
	wait.BeConnect = true
	record.BeConnect = true
	record.PreInPort[1] = &PrePortNode{Node: wait, OutPortID: 1}

	var bp Blueprint
	bp.AddCompiledGraph("test", &CompiledGraph{Entrances: map[int64]*ExecNode{1: entrance}})
	graphID := bp.Create("test")

	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("first Do failed: %v", err)
	}
	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("second Do failed: %v", err)
	}
	if len(continuations) != 2 {
		t.Fatalf("continuations = %d, want 2", len(continuations))
	}
	if continuations[0].graph == continuations[1].graph {
		t.Fatalf("continuations share one execution session")
	}

	if err := continuations[0].Resume(11); err != nil {
		t.Fatalf("first Resume failed: %v", err)
	}
	if err := continuations[1].Resume(22); err != nil {
		t.Fatalf("second Resume failed: %v", err)
	}
	if len(values) != 2 || values[0] != 11 || values[1] != 22 {
		t.Fatalf("values = %v, want [11 22]", values)
	}
}
