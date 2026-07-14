package blueprint

import (
	"context"
	"testing"
)

type vmYieldAnyOnceNode struct {
	BaseExecNode
	handle  **YieldHandle
	yielded *bool
}

func (n *vmYieldAnyOnceNode) GetName() string { return "VMYieldAnyOnce" }
func (n *vmYieldAnyOnceNode) Exec() (int, error) {
	item, _ := n.GetInPort(1).GetAny().(ArrayData)
	n.GetAndCreateReturnPort().AppendArrayValInt(item.IntVal)
	if item.IntVal == 1 && !*n.yielded {
		*n.yielded = true
		handle, err := n.Yield(0)
		if err != nil {
			return -1, err
		}
		*n.handle = handle
		return -1, ErrExecutionSuspended
	}
	return 0, nil
}

func runYieldingLoopGraph(t *testing.T, registry *Registry, config GraphConfig, handle **YieldHandle) Port_Array {
	t.Helper()
	compiled, err := CompileGraph(registry, config)
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("yield-loop", compiled)
	execution, err := blueprint.Start(context.Background(), blueprint.Create("yield-loop"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if execution.State() != ExecutionSuspended || *handle == nil {
		t.Fatalf("state/handle = %v/%v, want suspended/non-nil", execution.State(), *handle)
	}
	if err := (*handle).Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	dispatcher.runNext(t)
	returns, err := execution.Result()
	if err != nil {
		t.Fatalf("Result failed: %v", err)
	}
	return returns
}

func TestVMYieldResumesEveryLoopKindWithoutRepeatingIteration(t *testing.T) {
	t.Run("ForeachIntArray", func(t *testing.T) {
		var handle *YieldHandle
		yielded := false
		registry := vmLoopRegistry(nil)
		registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
			return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
		}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
		returns := runYieldingLoopGraph(t, registry, GraphConfig{
			Nodes: []NodeConfig{
				{ID: "entry", Class: "VMEntry_1"},
				{ID: "loop", Class: "ForeachIntArray", PortDefault: map[int]any{1: PortArray{{IntVal: 0}, {IntVal: 1}, {IntVal: 2}}}},
				{ID: "body", Class: "VMYieldOnce"},
			},
			Edges: []EdgeConfig{
				{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
				{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
				{SourceNodeID: "loop", SourcePortID: 3, DesNodeID: "body", DesPortID: 1},
			},
		}, &handle)
		assertVMIntReturns(t, returns, 0, 1, 2)
	})

	t.Run("ForeachArray", func(t *testing.T) {
		var handle *YieldHandle
		yielded := false
		registry := vmLoopRegistry(nil)
		registry.Register(NewNodeDefinition("VMYieldAnyOnce", func() IExecNode {
			return &vmYieldAnyOnceNode{handle: &handle, yielded: &yielded}
		}, []IPort{NewPortExec(), NewPortAny()}, []IPort{NewPortExec()}))
		returns := runYieldingLoopGraph(t, registry, GraphConfig{
			Nodes: []NodeConfig{
				{ID: "entry", Class: "VMEntry_1"},
				{ID: "loop", Class: "ForeachArray", PortDefault: map[int]any{1: PortArray{{IntVal: 0}, {IntVal: 1}, {IntVal: 2}}}},
				{ID: "body", Class: "VMYieldAnyOnce"},
			},
			Edges: []EdgeConfig{
				{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
				{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
				{SourceNodeID: "loop", SourcePortID: 2, DesNodeID: "body", DesPortID: 1},
			},
		}, &handle)
		assertVMIntReturns(t, returns, 0, 1, 2)
	})

	t.Run("WhileNode", func(t *testing.T) {
		var handle *YieldHandle
		yielded := false
		state := &vmWhileState{limit: 3}
		registry := vmLoopRegistry(state)
		registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
			return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
		}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
		returns := runYieldingLoopGraph(t, registry, GraphConfig{
			Nodes: []NodeConfig{
				{ID: "entry", Class: "VMEntry_1"}, {ID: "condition", Class: "VMWhileCondition"},
				{ID: "loop", Class: "WhileNode"}, {ID: "body", Class: "VMYieldOnce", PortDefault: map[int]any{1: 1}},
			},
			Edges: []EdgeConfig{
				{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
				{SourceNodeID: "condition", SourcePortID: 0, DesNodeID: "loop", DesPortID: 1},
				{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			},
		}, &handle)
		assertVMIntReturns(t, returns, 1, 1, 1)
		if state.conditionReads != 4 {
			t.Fatalf("condition reads = %d, want 4", state.conditionReads)
		}
	})

	t.Run("ForLoopBreak", func(t *testing.T) {
		var handle *YieldHandle
		yielded := false
		registry := vmLoopRegistry(nil)
		registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
			return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
		}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
		returns := runYieldingLoopGraph(t, registry, GraphConfig{
			Nodes: []NodeConfig{
				{ID: "entry", Class: "VMEntry_1"},
				{ID: "loop", Class: "ForLoopBreak", PortDefault: map[int]any{1: 0, 2: 3}},
				{ID: "body", Class: "VMYieldOnce"},
			},
			Edges: []EdgeConfig{
				{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
				{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
				{SourceNodeID: "loop", SourcePortID: 1, DesNodeID: "body", DesPortID: 1},
			},
		}, &handle)
		assertVMIntReturns(t, returns, 0, 1, 2)
	})
}

func TestVMYieldInsideNestedLoopResumesInnermostIteration(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmLoopRegistry(nil)
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	returns := runYieldingLoopGraph(t, registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "outer", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 2}},
			{ID: "inner", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 2}},
			{ID: "body", Class: "VMYieldOnce"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "outer", DesPortID: 0},
			{SourceNodeID: "outer", SourcePortID: 0, DesNodeID: "inner", DesPortID: 0},
			{SourceNodeID: "inner", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "inner", SourcePortID: 2, DesNodeID: "body", DesPortID: 1},
		},
	}, &handle)
	assertVMIntReturns(t, returns, 0, 1, 0, 1)
}
