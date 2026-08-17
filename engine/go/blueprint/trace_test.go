package blueprint

import "testing"

type testTraceLogger struct {
	events []BlueprintTraceEvent
}

func (l *testTraceLogger) TraceBlueprintNode(event BlueprintTraceEvent) {
	l.events = append(l.events, event)
}

type legacyNodeNameCollector struct {
	names []string
}

func (l *legacyNodeNameCollector) LogNodeExec(nodeName string, nodeID string, inPorts []IPort, outPorts []IPort, execResult int, err error) {
	l.names = append(l.names, nodeName)
}

func TestLegacyLoggerCoversFunctionNodesAndControlNodes(t *testing.T) {
	logger := &legacyNodeNameCollector{}
	registry := vmFlowRegistry()

	functionCompiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "fn-entry", Class: "FunctionEntry", FunctionName: "Calc", FunctionInputTypes: []string{"integer"}},
			{ID: "fn-return", Class: "FunctionReturn", FunctionName: "Calc", FunctionOutputTypes: []string{"integer"}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "fn-entry", SourcePortID: 0, DesNodeID: "fn-return", DesPortID: 0},
			{SourceNodeID: "fn-entry", SourcePortID: 1, DesNodeID: "fn-return", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph function failed: %v", err)
	}

	mainCompiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "call", Class: "FunctionCall", FunctionName: "Calc", FunctionInputTypes: []string{"integer"}, FunctionOutputTypes: []string{"integer"}},
			{ID: "sequence", Class: "Sequence"},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "call", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "call", DesPortID: 1},
			{SourceNodeID: "call", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "call", SourcePortID: 1, DesNodeID: "result", DesPortID: 1},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
		},
		Functions: map[string]*CompiledGraph{"Calc": functionCompiled},
	})
	if err != nil {
		t.Fatalf("CompileGraph main failed: %v", err)
	}

	graph := NewGraph(mainCompiled)
	graph.logger = logger
	returns, err := graph.Do(1, PortInt(11))
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	assertVMIntReturns(t, returns, 11)

	want := []string{"VMEntry", "Calc", "Calc Entry", "Calc Return", "Sequence", "VMReturnPort"}
	if len(logger.names) != len(want) {
		t.Fatalf("legacy node names = %#v, want %#v", logger.names, want)
	}
	for index, name := range want {
		if logger.names[index] != name {
			t.Fatalf("legacy node names = %#v, want %#v", logger.names, want)
		}
	}
}

func TestBlueprintTraceDisabledByDefault(t *testing.T) {
	logger := &testTraceLogger{}
	bp, graphID := newTraceTestBlueprint(t, logger)

	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	if len(logger.events) != 0 {
		t.Fatalf("trace events = %#v, want none when trace is disabled", logger.events)
	}
}

func TestBlueprintTraceLogsNodeStepsInputsAndOutputsWhenEnabled(t *testing.T) {
	logger := &testTraceLogger{}
	bp, graphID := newTraceTestBlueprint(t, logger)
	bp.SetTraceEnabled(true)

	if _, err := bp.Do(graphID, 1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	if got, want := len(logger.events), 3; got != want {
		t.Fatalf("trace event count = %d, want %d: %#v", got, want, logger.events)
	}
	if logger.events[0].NodeID != "entry" || logger.events[0].NodeName != "TestEntrance" {
		t.Fatalf("entry event = %#v", logger.events[0])
	}
	if logger.events[0].ExecutionID == 0 || logger.events[0].EntranceID != 1 || logger.events[0].PC < 0 || logger.events[0].Stage == "" {
		t.Fatalf("entry correlation fields = %#v", logger.events[0])
	}
	if logger.events[1].NodeID != "add" || logger.events[1].NodeName != "AddInt" {
		t.Fatalf("add event = %#v", logger.events[1])
	}
	if logger.events[2].NodeID != "record" || logger.events[2].NodeName != "TestRecorder" {
		t.Fatalf("record event = %#v", logger.events[2])
	}

	addInputs := dataTraceValues(logger.events[1].Inputs)
	if len(addInputs) != 2 || addInputs[0].Value != PortInt(2) || addInputs[1].Value != PortInt(5) {
		t.Fatalf("add inputs = %#v, want 2 and 5", addInputs)
	}
	if addInputs[0].Type != "\u6574\u6570" || addInputs[1].Type != "\u6574\u6570" {
		t.Fatalf("add input types = %q/%q, want \u6574\u6570/\u6574\u6570", addInputs[0].Type, addInputs[1].Type)
	}
	addOutputs := dataTraceValues(logger.events[1].Outputs)
	if len(addOutputs) != 1 || addOutputs[0].Value != PortInt(7) {
		t.Fatalf("add outputs = %#v, want 7", addOutputs)
	}
	if addOutputs[0].Type != "\u6574\u6570" {
		t.Fatalf("add output type = %q, want \u6574\u6570", addOutputs[0].Type)
	}
	recordInputs := dataTraceValues(logger.events[2].Inputs)
	if len(recordInputs) != 1 || recordInputs[0].Value != PortInt(7) {
		t.Fatalf("record inputs = %#v, want 7", recordInputs)
	}
}

func TestBlueprintTraceIncludesVMControlNodes(t *testing.T) {
	logger := &testTraceLogger{}
	registry := vmFlowRegistry()
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "sequence", Class: "Sequence"},
			{ID: "result", Class: "VMReturnPort", PortDefault: map[int]any{1: 1}},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "sequence", DesPortID: 0},
			{SourceNodeID: "sequence", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	blueprint := &Blueprint{}
	blueprint.AddCompiledGraph("control-trace", compiled)
	blueprint.SetTraceLogger(logger)
	blueprint.SetTraceEnabled(true)
	if _, err := blueprint.Do(blueprint.Create("control-trace"), 1); err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	for _, event := range logger.events {
		if event.NodeID == "sequence" && event.Stage == "control" {
			return
		}
	}
	t.Fatalf("trace events = %#v, want sequence control event", logger.events)
}

func newTraceTestBlueprint(t *testing.T, logger *testTraceLogger) (*Blueprint, int64) {
	t.Helper()
	registry := NewRegistry()
	registry.Register(NewNodeDefinition("TestEntrance", func() IExecNode {
		return &testEntrance{}
	}, nil, []IPort{NewPortExec()}))
	registry.Register(NewNodeDefinition("AddInt", func() IExecNode {
		return &AddInt{}
	}, []IPort{NewPortInt(), NewPortInt()}, []IPort{NewPortInt()}))
	registry.Register(NewNodeDefinition("TestRecorder", func() IExecNode {
		return &testRecorder{}
	}, []IPort{NewPortExec(), NewPortInt()}, nil))

	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{
			{ID: "entry", Class: "TestEntrance_1"},
			{ID: "add", Class: "AddInt", PortDefault: map[int]any{0: 2, 1: 5}},
			{ID: "record", Class: "TestRecorder"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "record", DesPortID: 0},
			{SourceNodeID: "add", SourcePortID: 0, DesNodeID: "record", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	var bp Blueprint
	bp.AddCompiledGraph("trace", compiled)
	bp.SetTraceLogger(logger)
	graphID := bp.Create("trace")
	if graphID == 0 {
		t.Fatalf("Create returned 0")
	}
	return &bp, graphID
}

func dataTraceValues(values []BlueprintTracePortValue) []BlueprintTracePortValue {
	filtered := make([]BlueprintTracePortValue, 0, len(values))
	for _, value := range values {
		if !value.IsExec {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
