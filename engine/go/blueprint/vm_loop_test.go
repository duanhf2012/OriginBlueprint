package blueprint

import (
	"fmt"
	"testing"
)

type vmBreakAtNode struct{ BaseExecNode }

func (n *vmBreakAtNode) GetName() string { return "VMBreakAt" }
func (n *vmBreakAtNode) Exec() (int, error) {
	index, _ := n.GetInPortInt(1)
	n.GetAndCreateReturnPort().AppendArrayValInt(index)
	if index >= 2 {
		return 1, nil
	}
	return 0, nil
}

type vmAnyReturnNode struct{ BaseExecNode }

func (n *vmAnyReturnNode) GetName() string { return "VMAnyReturn" }
func (n *vmAnyReturnNode) Exec() (int, error) {
	value := n.GetInPort(1).GetAny()
	item, _ := value.(ArrayData)
	n.GetAndCreateReturnPort().AppendArrayValInt(item.IntVal)
	return -1, nil
}

type vmWhileState struct {
	conditionReads int
	bodyRuns       int
	limit          int
}

type vmWhileCondition struct {
	BaseExecNode
	state *vmWhileState
}

func (n *vmWhileCondition) GetName() string { return "VMWhileCondition" }
func (n *vmWhileCondition) Exec() (int, error) {
	n.state.conditionReads++
	n.SetOutPortBool(0, n.state.conditionReads <= n.state.limit)
	return -1, nil
}

type vmWhileBody struct {
	BaseExecNode
	state *vmWhileState
}

func (n *vmWhileBody) GetName() string { return "VMWhileBody" }
func (n *vmWhileBody) Exec() (int, error) {
	n.state.bodyRuns++
	n.GetAndCreateReturnPort().AppendArrayValInt(PortInt(n.state.bodyRuns))
	return -1, nil
}

func vmLoopRegistry(state *vmWhileState) *Registry {
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("Foreach", func() IExecNode { return &Foreach{} }, []IPort{NewPortExec(), NewPortInt(), NewPortInt()}, []IPort{NewPortExec(), NewPortExec(), NewPortInt()}))
	registry.Register(NewNodeDefinition("ForeachIntArray", func() IExecNode { return &ForeachIntArray{} }, []IPort{NewPortExec(), NewPortArray()}, []IPort{NewPortExec(), NewPortExec(), NewPortInt(), NewPortInt()}))
	registry.Register(NewNodeDefinition("ForeachArray", func() IExecNode { return &ForeachArray{} }, []IPort{NewPortExec(), NewPortArray()}, []IPort{NewPortExec(), NewPortExec(), NewPortAny(), NewPortInt()}))
	registry.Register(NewNodeDefinition("WhileNode", func() IExecNode { return &WhileNode{} }, []IPort{NewPortExec(), NewPortBool()}, []IPort{NewPortExec(), NewPortExec()}))
	registry.Register(NewNodeDefinition("ForLoopBreak", func() IExecNode { return &ForLoopBreak{} }, []IPort{NewPortExec(), NewPortInt(), NewPortInt(), NewPortExec()}, []IPort{NewPortExec(), NewPortInt(), NewPortExec()}))
	registry.Register(NewNodeDefinition("VMBreakAt", func() IExecNode { return &vmBreakAtNode{} }, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec(), NewPortExec()}))
	registry.Register(NewNodeDefinition("VMAnyReturn", func() IExecNode { return &vmAnyReturnNode{} }, []IPort{NewPortExec(), NewPortAny()}, nil))
	if state != nil {
		registry.Register(NewNodeDefinition("VMWhileCondition", func() IExecNode { return &vmWhileCondition{state: state} }, nil, []IPort{NewPortBool()}))
		registry.Register(NewNodeDefinition("VMWhileBody", func() IExecNode { return &vmWhileBody{state: state} }, []IPort{NewPortExec()}, nil))
	}
	return registry
}

func TestVMRangeLoopUsesHalfOpenBoundsAndCompletedBranch(t *testing.T) {
	compiled, err := CompileGraph(vmLoopRegistry(nil), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "loop", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 3}},
			{ID: "body", Class: "VMReturnPort"},
			{ID: "completed", Class: "VMReturnPort", PortDefault: map[int]any{1: 99}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 2, DesNodeID: "body", DesPortID: 1},
			{SourceNodeID: "loop", SourcePortID: 1, DesNodeID: "completed", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 0, 1, 2, 99)
}

func TestVMIntArrayLoopSnapshotsArrayAndVisitsEachItemOnce(t *testing.T) {
	array := PortArray{{IntVal: 4}, {IntVal: 5}, {IntVal: 6}}
	compiled, err := CompileGraph(vmLoopRegistry(nil), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "loop", Class: "ForeachIntArray", PortDefault: map[int]any{1: array}},
			{ID: "body", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 3, DesNodeID: "body", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 4, 5, 6)
}

func TestVMAnyArrayLoopPreservesItemValues(t *testing.T) {
	array := PortArray{{IntVal: 7}, {IntVal: 8}}
	compiled, err := CompileGraph(vmLoopRegistry(nil), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "loop", Class: "ForeachArray", PortDefault: map[int]any{1: array}}, {ID: "body", Class: "VMAnyReturn"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 2, DesNodeID: "body", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 7, 8)
}

func TestVMWhileLoopRecomputesConditionOncePerIteration(t *testing.T) {
	state := &vmWhileState{limit: 3}
	compiled, err := CompileGraph(vmLoopRegistry(state), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "condition", Class: "VMWhileCondition"}, {ID: "loop", Class: "WhileNode"}, {ID: "body", Class: "VMWhileBody"}, {ID: "completed", Class: "VMReturnPort", PortDefault: map[int]any{1: 99}}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "condition", SourcePortID: 0, DesNodeID: "loop", DesPortID: 1},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 1, DesNodeID: "completed", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 1, 2, 3, 99)
	if state.conditionReads != 4 || state.bodyRuns != 3 {
		t.Fatalf("condition reads/body runs = %d/%d, want 4/3", state.conditionReads, state.bodyRuns)
	}
}

func TestVMBreakableLoopExecutesCurrentBodyBeforeBreak(t *testing.T) {
	compiled, err := CompileGraph(vmLoopRegistry(nil), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "loop", Class: "ForLoopBreak", PortDefault: map[int]any{1: 0, 2: 5}}, {ID: "body", Class: "VMBreakAt"}, {ID: "completed", Class: "VMReturnPort", PortDefault: map[int]any{1: 99}}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "loop", SourcePortID: 1, DesNodeID: "body", DesPortID: 1},
			{SourceNodeID: "body", SourcePortID: 1, DesNodeID: "loop", DesPortID: 3},
			{SourceNodeID: "loop", SourcePortID: 2, DesNodeID: "completed", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 0, 1, 2, 99)
}

func TestVMNestedLoopsResumeOuterLoopAfterInnerCompleted(t *testing.T) {
	compiled, err := CompileGraph(vmLoopRegistry(nil), GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "outer", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 2}},
			{ID: "inner", Class: "Foreach", PortDefault: map[int]any{1: 0, 2: 2}},
			{ID: "body", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "outer", DesPortID: 0},
			{SourceNodeID: "outer", SourcePortID: 0, DesNodeID: "inner", DesPortID: 0},
			{SourceNodeID: "inner", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
			{SourceNodeID: "inner", SourcePortID: 2, DesNodeID: "body", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	returns, err := NewGraph(compiled).runVMEntrance(1)
	if err != nil {
		t.Fatalf("runVMEntrance failed: %v", err)
	}
	assertVMIntReturns(t, returns, 0, 1, 0, 1)
}

func TestVMLoopZeroAndSingleIterationBoundaries(t *testing.T) {
	run := func(t *testing.T, registry *Registry, config GraphConfig, want ...int) {
		t.Helper()
		compiled, err := CompileGraph(registry, config)
		if err != nil {
			t.Fatalf("CompileGraph failed: %v", err)
		}
		returns, err := NewGraph(compiled).runVMEntrance(1)
		if err != nil {
			t.Fatalf("runVMEntrance failed: %v", err)
		}
		converted := make([]PortInt, len(want))
		for index, value := range want {
			converted[index] = PortInt(value)
		}
		assertVMIntReturns(t, returns, converted...)
	}

	for _, test := range []struct {
		name  string
		class string
		start int
		end   int
		want  []int
	}{
		{name: "ForeachZero", class: "Foreach", start: 3, end: 3},
		{name: "ForeachOne", class: "Foreach", start: 3, end: 4, want: []int{3}},
		{name: "ForLoopBreakZero", class: "ForLoopBreak", start: 3, end: 3},
		{name: "ForLoopBreakOne", class: "ForLoopBreak", start: 3, end: 4, want: []int{3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			indexPort := 2
			if test.class == "ForLoopBreak" {
				indexPort = 1
			}
			run(t, vmLoopRegistry(nil), GraphConfig{
				Nodes: []NodeConfig{
					{ID: "entry", Class: "VMEntry_1"},
					{ID: "loop", Class: test.class, PortDefault: map[int]any{1: test.start, 2: test.end}},
					{ID: "body", Class: "VMReturnPort"},
				},
				Edges: []EdgeConfig{
					{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
					{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
					{SourceNodeID: "loop", SourcePortID: indexPort, DesNodeID: "body", DesPortID: 1},
				},
			}, test.want...)
		})
	}

	for _, test := range []struct {
		name      string
		class     string
		array     PortArray
		valuePort int
		bodyClass string
		want      []int
	}{
		{name: "ForeachIntArrayZero", class: "ForeachIntArray", valuePort: 3, bodyClass: "VMReturnPort"},
		{name: "ForeachIntArrayOne", class: "ForeachIntArray", array: PortArray{{IntVal: 7}}, valuePort: 3, bodyClass: "VMReturnPort", want: []int{7}},
		{name: "ForeachArrayZero", class: "ForeachArray", valuePort: 2, bodyClass: "VMAnyReturn"},
		{name: "ForeachArrayOne", class: "ForeachArray", array: PortArray{{IntVal: 7}}, valuePort: 2, bodyClass: "VMAnyReturn", want: []int{7}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run(t, vmLoopRegistry(nil), GraphConfig{
				Nodes: []NodeConfig{
					{ID: "entry", Class: "VMEntry_1"},
					{ID: "loop", Class: test.class, PortDefault: map[int]any{1: test.array}},
					{ID: "body", Class: test.bodyClass},
				},
				Edges: []EdgeConfig{
					{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
					{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
					{SourceNodeID: "loop", SourcePortID: test.valuePort, DesNodeID: "body", DesPortID: 1},
				},
			}, test.want...)
		})
	}

	for _, limit := range []int{0, 1} {
		t.Run(fmt.Sprintf("While%d", limit), func(t *testing.T) {
			state := &vmWhileState{limit: limit}
			run(t, vmLoopRegistry(state), GraphConfig{
				Nodes: []NodeConfig{
					{ID: "entry", Class: "VMEntry_1"}, {ID: "condition", Class: "VMWhileCondition"},
					{ID: "loop", Class: "WhileNode"}, {ID: "body", Class: "VMWhileBody"},
				},
				Edges: []EdgeConfig{
					{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "loop", DesPortID: 0},
					{SourceNodeID: "condition", SourcePortID: 0, DesNodeID: "loop", DesPortID: 1},
					{SourceNodeID: "loop", SourcePortID: 0, DesNodeID: "body", DesPortID: 0},
				},
			}, makeRange(1, limit+1)...)
			if state.bodyRuns != limit || state.conditionReads != limit+1 {
				t.Fatalf("body/condition = %d/%d, want %d/%d", state.bodyRuns, state.conditionReads, limit, limit+1)
			}
		})
	}
}

func makeRange(start, end int) []int {
	values := make([]int, 0, end-start)
	for value := start; value < end; value++ {
		values = append(values, value)
	}
	return values
}
