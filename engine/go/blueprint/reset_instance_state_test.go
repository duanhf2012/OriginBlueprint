package blueprint

import (
	"testing"
	"time"
)

// resetStateTestRegistry 组合定时器结点与 VM 入口/返回结点，覆盖"变量复位 + 定时器取消"两条复位路径
func resetStateTestRegistry(calls chan timerTestCall) *Registry {
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("CreateTimer", func() IExecNode { return &CreateTimerNode{} }, []IPort{NewPortExec(), NewPortInt(), NewPortBool(), nil, NewPortStr()}, []IPort{NewPortExec(), NewPortCallback()}))
	registry.Register(NewNodeDefinition("WaveCapture", func() IExecNode { return &timerTestCapture{label: "wave", calls: calls} }, []IPort{NewPortExec(), NewPortInt()}, nil))
	return registry
}

func addResetStateGraph(t *testing.T, scheduler TimerScheduler) (*Blueprint, int64, chan timerTestCall) {
	t.Helper()
	calls := make(chan timerTestCall, 4)
	compiled, err := CompileGraph(resetStateTestRegistry(calls), GraphConfig{
		// instance 级变量 Count，编译期默认值 7
		Variables: []VariableConfig{{ID: "cnt", Name: "Count", Type: "integer", Value: 7, Scope: VariableScopeInstance}},
		Nodes: []NodeConfig{
			{ID: "entry-set", Class: "VMEntry_1"},
			{ID: "entry-timer", Class: "VMEntry_2"},
			{ID: "entry-get", Class: "VMEntry_3"},
			{ID: "set", Class: "Set_Count"},
			{ID: "timer", Class: "CreateTimer", TimerKey: "wave", PortDefault: map[int]any{1: int64(1000), 2: false, 4: "wave"}},
			{ID: "capture", Class: "WaveCapture"},
			{ID: "get", Class: "Get_Count"},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry-set", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "entry-set", SourcePortID: 1, DesNodeID: "set", DesPortID: 1},
			{SourceNodeID: "entry-timer", SourcePortID: 0, DesNodeID: "timer", DesPortID: 0},
			{SourceNodeID: "timer", SourcePortID: 1, DesNodeID: "capture", DesPortID: 0},
			{SourceNodeID: "entry-get", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(NewInlineExecutionDispatcher())
	blueprint.SetTimerScheduler(scheduler)
	blueprint.AddCompiledGraph("reset-test", compiled)
	graphID := blueprint.Create("reset-test")
	if graphID == 0 {
		t.Fatal("Create returned zero graph ID")
	}
	t.Cleanup(func() { _ = blueprint.Close() })
	return blueprint, graphID, calls
}

// TestResetInstanceStateRestoresVariablesAndCancelsTimers 复位后：变量回默认值、定时器已取消不再触发、同 key 可重建
func TestResetInstanceStateRestoresVariablesAndCancelsTimers(t *testing.T) {
	scheduler := &manualTimerScheduler{}
	blueprint, graphID, calls := addResetStateGraph(t, scheduler)

	// 前置：变量写入 99 并读回确认
	if _, err := blueprint.Do(graphID, 1, int64(99)); err != nil {
		t.Fatalf("set Do failed: %v", err)
	}
	returns, err := blueprint.Do(graphID, 3, int64(0))
	if err != nil {
		t.Fatalf("get Do failed: %v", err)
	}
	assertVMIntReturns(t, returns, 99)

	// 前置：创建定时器并确认调度器有 1 个活跃回调
	if _, err := blueprint.Do(graphID, 2, int64(0)); err != nil {
		t.Fatalf("timer Do failed: %v", err)
	}
	if got := scheduler.pending(); got != 1 {
		t.Fatalf("pending timers = %d, want 1", got)
	}

	// 执行复位
	if err := blueprint.ResetInstanceState(graphID); err != nil {
		t.Fatalf("ResetInstanceState failed: %v", err)
	}

	// 断言1：变量恢复编译期默认值 7
	returns, err = blueprint.Do(graphID, 3, int64(0))
	if err != nil {
		t.Fatalf("get after reset failed: %v", err)
	}
	assertVMIntReturns(t, returns, 7)

	// 断言2：定时器已被取消——fire 同步返回 false 且回调不派发
	if scheduler.fire(0) {
		t.Fatal("timer should have been canceled by reset")
	}
	select {
	case call := <-calls:
		t.Fatalf("callback executed after reset: %#v", call)
	case <-time.After(50 * time.Millisecond):
	}

	// 断言3：实例未销毁，同 key 定时器可重建（旧定时器已取消不再计入活跃数）
	if _, err := blueprint.Do(graphID, 2, int64(0)); err != nil {
		t.Fatalf("re-create timer after reset failed: %v", err)
	}
	if got := scheduler.pending(); got != 1 {
		t.Fatalf("active timers after re-create = %d, want 1", got)
	}
}

// TestResetInstanceStateErrors 图不存在返回 ErrGraphNotFound；关闭后返回 ErrBlueprintClosed
func TestResetInstanceStateErrors(t *testing.T) {
	scheduler := &manualTimerScheduler{}
	blueprint, graphID, _ := addResetStateGraph(t, scheduler)

	if err := blueprint.ResetInstanceState(graphID + 999); err != ErrGraphNotFound {
		t.Fatalf("error = %v, want ErrGraphNotFound", err)
	}
	if err := blueprint.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := blueprint.ResetInstanceState(graphID); err != ErrBlueprintClosed {
		t.Fatalf("error = %v, want ErrBlueprintClosed", err)
	}
}
