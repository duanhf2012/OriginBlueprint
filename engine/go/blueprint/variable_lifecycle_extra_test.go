package blueprint

import (
	"context"
	"testing"
)

type variableYieldOnceNode struct {
	BaseExecNode
	handle  **YieldHandle
	yielded *bool
}

func (n *variableYieldOnceNode) GetName() string { return "VMVariableYield" }

func (n *variableYieldOnceNode) Exec() (int, error) {
	if *n.yielded {
		return 0, nil
	}
	*n.yielded = true
	handle, err := n.Yield(0)
	if err != nil {
		return -1, err
	}
	*n.handle = handle
	return -1, ErrExecutionSuspended
}

type variableArrayFirstReturnNode struct{ BaseExecNode }

func (n *variableArrayFirstReturnNode) GetName() string { return "VMArrayFirstReturn" }

func (n *variableArrayFirstReturnNode) Exec() (int, error) {
	items, ok := n.GetInPortArray(1)
	if !ok || len(items) == 0 {
		return -1, nil
	}
	n.GetAndCreateReturnPort().AppendArrayValInt(items[0].IntVal)
	return -1, nil
}

type variableStringReturnNode struct{ BaseExecNode }

func (n *variableStringReturnNode) GetName() string { return "VMStringReturn" }

func (n *variableStringReturnNode) Exec() (int, error) {
	value, ok := n.GetInPortStr(1)
	if !ok {
		return -1, nil
	}
	n.GetAndCreateReturnPort().AppendArrayValStr(value)
	return -1, nil
}

func TestVMYieldResumeKeepsArrayVariableSnapshot(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMArrayEntry", func() IExecNode { return &testEntrance{} }, nil, []IPort{NewPortExec(), NewPortArray()}))
	registry.Register(NewNodeDefinition("VMVariableYield", func() IExecNode {
		return &variableYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("VMArrayFirstReturn", func() IExecNode { return &variableArrayFirstReturnNode{} }, []IPort{NewPortExec(), NewPortArray()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "Items", Type: "Array", Value: PortArray{{IntVal: 7}}}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMArrayEntry_1"},
			{ID: "set", Class: "Set_Items"},
			{ID: "yield", Class: "VMVariableYield"},
			{ID: "get", Class: "Get_Items"},
			{ID: "result", Class: "VMArrayFirstReturn"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "set", DesPortID: 1},
			{SourceNodeID: "set", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0},
			{SourceNodeID: "yield", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("array-variable", compiled)
	graphID := bp.Create("array-variable")
	source := PortArray{{IntVal: 10}}
	execution, err := bp.Start(context.Background(), graphID, 1, source)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || handle == nil {
		t.Fatalf("state/handle = %v/%v, want suspended/non-nil", execution.State(), handle)
	}

	source[0].IntVal = 99
	source = append(source, ArrayData{IntVal: 100})
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	returns, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	assertVMIntReturns(t, returns, 10)

	nextExecution, err := bp.Start(context.Background(), graphID, 1, PortArray{{IntVal: 42}})
	if err != nil {
		t.Fatalf("next Start failed: %v", err)
	}
	dispatcher.runNext(t)
	nextReturns, err := nextExecution.Result()
	if err != nil {
		t.Fatalf("next Result failed: %v", err)
	}
	assertVMIntReturns(t, nextReturns, 42)
}

func TestVMHotReloadSeparatesOldAndNewVariableSchemas(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMVariableYield", func() IExecNode {
		return &variableYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec()}, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("VMStringReturn", func() IExecNode { return &variableStringReturnNode{} }, []IPort{NewPortExec(), NewPortStr()}, nil))

	oldGraph, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "Count", Type: "Integer", Value: 5}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "set", Class: "Set_Count", PortDefault: map[int]any{1: 41}},
			{ID: "yield", Class: "VMVariableYield"},
			{ID: "get", Class: "Get_Count"},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "set", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0},
			{SourceNodeID: "yield", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile old graph failed: %v", err)
	}
	newGraph, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{Name: "Count", Type: "String", Value: "new-default"}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "get", Class: "Get_Count"},
			{ID: "result", Class: "VMStringReturn"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile new graph failed: %v", err)
	}

	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("hot-variable", oldGraph)
	graphID := bp.Create("hot-variable")
	oldExecution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("old Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if oldExecution.State() != ExecutionSuspended || handle == nil {
		t.Fatalf("old state/handle = %v/%v, want suspended/non-nil", oldExecution.State(), handle)
	}

	(&hotReloadPlan{blueprint: bp, graphs: map[string]*CompiledGraph{"hot-variable": newGraph}}).apply()
	if err := handle.Resume(); err != nil {
		t.Fatalf("old Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	oldReturns, err := oldExecution.Result()
	if err != nil {
		t.Fatalf("old Result failed: %v", err)
	}
	assertVMIntReturns(t, oldReturns, 41)

	newExecution, err := bp.Start(context.Background(), graphID, 1)
	if err != nil {
		t.Fatalf("new Start failed: %v", err)
	}
	dispatcher.runNext(t)
	newReturns, err := newExecution.Result()
	if err != nil {
		t.Fatalf("new Result failed: %v", err)
	}
	assertVerificationReturns(t, newReturns, PortArray{{StrVal: "new-default"}})
}
