package blueprint

import (
	"testing"
)

// 宏实例执行测试：模拟"宏插入蓝图"产出的文档形态（实例内含入口节点 + 外部边界连线），
// 验证两件事：
//  1. 调用蓝图入口能正常触发到宏插入的入口节点并执行其逻辑（含边界连线路径）；
//  2. 宏内容更新（实例被同步替换为新节点/新连线）后，重新执行生效。

func macroInstanceGraph(addValue int, recorderFactory func() IExecNode) (*Registry, *GraphConfig) {
	registry := NewRegistry()
	registerFunctionTestNodes(registry, recorderFactory)
	// 文档形态（与编辑器宏插入/同步产物一致）：
	//   实例内：macro-entry（入口，entrance 2）exec → record exec；add(int) 输出 → record 参数（宏内部逻辑）
	//   边界：external-entry（入口，entrance 1）exec → record exec（外部连线进实例节点）
	config := &GraphConfig{
		Nodes: []NodeConfig{
			{ID: "external-entry", Class: "Entrance_IntParam_1"},
			{ID: "macro-entry", Class: "Entrance_IntParam_2"},
			{ID: "add", Class: "AddInt", PortDefault: map[int]any{0: addValue, 1: 1}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "macro-entry", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "external-entry", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "add", SourcePortID: 0, DesNodeID: "record", DesPortID: 1},
		},
	}
	return registry, config
}

func TestMacroInstanceEntranceTriggersInsertedLogic(t *testing.T) {
	var recorder *testRecorder
	registry, config := macroInstanceGraph(31, func() IExecNode { recorder = &testRecorder{}; return recorder })
	compiled, err := CompileGraph(registry, *config)
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	graph := NewGraph(compiled)

	// 宏插入的入口（entrance 2）被外部调用触发：宏内逻辑执行，记录 add 结果。
	if _, err := graph.Do(2); err != nil {
		t.Fatalf("Do(macro entrance) failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 32 {
		t.Fatalf("macro entrance run recorder values = %#v, want [32]", recorder)
	}

	// 外部入口（entrance 1）经边界连线驱动同一实例逻辑（新执行实例）。
	recorder = nil
	graphExternal := NewGraph(compiled)
	if _, err := graphExternal.Do(1); err != nil {
		t.Fatalf("Do(external entrance) failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 32 {
		t.Fatalf("external entrance run recorder values = %#v, want [32]", recorder)
	}
}

func TestMacroInstanceResyncTakesEffectOnExecution(t *testing.T) {
	var recorder *testRecorder
	registry, configV1 := macroInstanceGraph(31, func() IExecNode { recorder = &testRecorder{}; return recorder })
	graphV1, err := CompileGraph(registry, *configV1)
	if err != nil {
		t.Fatalf("CompileGraph v1 failed: %v", err)
	}
	g1 := NewGraph(graphV1)
	if _, err := g1.Do(2); err != nil {
		t.Fatalf("Do v1 failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 32 {
		t.Fatalf("v1 recorder values = %#v, want [32]", recorder)
	}

	// 宏内容修改：add 常量 31 → 77（同步产物=同结构、新常量的配置），重新编译执行。
	recorder = nil
	_, configV2 := macroInstanceGraph(77, func() IExecNode { recorder = &testRecorder{}; return recorder })
	graphV2, err := CompileGraph(registry, *configV2)
	if err != nil {
		t.Fatalf("CompileGraph v2 failed: %v", err)
	}
	g2 := NewGraph(graphV2)
	if _, err := g2.Do(2); err != nil {
		t.Fatalf("Do v2 failed: %v", err)
	}
	if recorder == nil || len(recorder.values) != 1 || recorder.values[0] != 78 {
		t.Fatalf("v2 recorder values = %#v, want [78]（宏修改后必须生效）", recorder)
	}
}
