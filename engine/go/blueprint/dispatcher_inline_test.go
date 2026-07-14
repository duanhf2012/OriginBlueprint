package blueprint

import (
	"errors"
	"testing"
)

func TestInlineExecutionDispatcherRunsBeforeSubmitReturns(t *testing.T) {
	dispatcher := NewInlineExecutionDispatcher()
	ran := false
	if err := dispatcher.Submit(func() { ran = true }); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if !ran {
		t.Fatal("task did not run before Submit returned")
	}
}

func TestInlineExecutionDispatcherRejectsNilTask(t *testing.T) {
	if err := NewInlineExecutionDispatcher().Submit(nil); !errors.Is(err, ErrExecutionRejected) {
		t.Fatalf("Submit(nil) error = %v, want ErrExecutionRejected", err)
	}
}

type inlineNestedDoNode struct {
	BaseExecNode
	blueprint *Blueprint
	graphID   int64
}

func (n *inlineNestedDoNode) GetName() string { return "InlineNestedDo" }

func (n *inlineNestedDoNode) Exec() (int, error) {
	_, err := n.blueprint.Do(n.graphID, 1)
	return 0, err
}

type inlineRunCounterNode struct {
	BaseExecNode
	runs *int
}

func (n *inlineRunCounterNode) GetName() string { return "InlineRunCounter" }

func (n *inlineRunCounterNode) Exec() (int, error) {
	(*n.runs)++
	return 0, nil
}

func TestInlineDispatcherNestedDoCompletes(t *testing.T) {
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(NewInlineExecutionDispatcher())

	runs := 0
	innerEntrance := NewExecNode("inner-entrance", NewNodeDefinition("InnerEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	innerCounter := NewExecNode("inner-counter", NewNodeDefinition("InlineRunCounter", func() IExecNode {
		return &inlineRunCounterNode{runs: &runs}
	}, []IPort{NewPortExec()}, nil))
	innerEntrance.Next = []*ExecNode{innerCounter}
	innerCounter.BeConnect = true
	bp.AddCompiledGraph("inline-inner", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: innerEntrance},
		NodeCount: 2,
	})
	innerGraphID := bp.Create("inline-inner")

	outerEntrance := NewExecNode("outer-entrance", NewNodeDefinition("OuterEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	outerNested := NewExecNode("outer-nested", NewNodeDefinition("InlineNestedDo", func() IExecNode {
		return &inlineNestedDoNode{blueprint: bp, graphID: innerGraphID}
	}, []IPort{NewPortExec()}, nil))
	outerEntrance.Next = []*ExecNode{outerNested}
	outerNested.BeConnect = true
	bp.AddCompiledGraph("inline-outer", &CompiledGraph{
		Entrances: map[int64]*ExecNode{1: outerEntrance},
		NodeCount: 2,
	})

	if _, err := bp.Do(bp.Create("inline-outer"), 1); err != nil {
		t.Fatalf("outer Do failed: %v", err)
	}
	if runs != 1 {
		t.Fatalf("inner graph runs = %d, want 1", runs)
	}
}
