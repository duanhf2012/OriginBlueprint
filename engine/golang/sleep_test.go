package golang

import (
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
	if _, err := graph.Do(1); err != nil {
		t.Fatalf("Do failed: %v", err)
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
