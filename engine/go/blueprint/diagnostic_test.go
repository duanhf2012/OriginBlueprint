package blueprint

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type testDiagnosticSink struct {
	errors []BlueprintError
}

func (s *testDiagnosticSink) ReportBlueprintError(value BlueprintError) {
	s.errors = append(s.errors, value)
}

type vmDataPanicNode struct{ BaseExecNode }

func (n *vmDataPanicNode) GetName() string { return "VMDataPanic" }
func (n *vmDataPanicNode) Exec() (int, error) {
	panic("data boom")
}

func TestBlueprintDiagnosticSinkReceivesStructuredExecutionFailure(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "panic-node", Class: "VMPanic"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "panic-node", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	sink := &testDiagnosticSink{}
	blueprint := &Blueprint{}
	blueprint.SetDiagnosticSink(sink)
	blueprint.AddCompiledGraph("diagnostic", compiled)
	graphID := blueprint.Create("diagnostic")

	_, err = blueprint.Do(graphID, 1)
	if err == nil {
		t.Fatal("Do unexpectedly succeeded")
	}
	var structured *BlueprintError
	if !errors.As(err, &structured) {
		t.Fatalf("Do error = %T %v, want BlueprintError", err, err)
	}
	if structured.Stage != BlueprintStageExecute || structured.GraphName != "diagnostic" || structured.GraphID != graphID || structured.EntranceID != 1 || structured.ExecutionID == 0 || structured.NodeID != "panic-node" || structured.PC < 0 {
		t.Fatalf("structured error = %#v", structured)
	}
	if len(sink.errors) != 1 || sink.errors[0].ExecutionID != structured.ExecutionID {
		t.Fatalf("diagnostic sink errors = %#v", sink.errors)
	}
}

func TestGraphDoReportsMissingEntrance(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}}})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	if _, err := NewGraph(compiled).Do(999); !errors.Is(err, ErrEntranceNotFound) {
		t.Fatalf("Graph.Do error = %v, want ErrEntranceNotFound", err)
	}
}

func TestControlFlowFailureIncludesNodeAndPC(t *testing.T) {
	compiled, err := CompileGraph(vmLoopRegistry(nil), GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "break-loop", Class: "ForLoopBreak"}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "break-loop", DesPortID: 3}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	_, err = NewGraph(compiled).Do(1)
	var structured *BlueprintError
	if err == nil || !errors.As(err, &structured) {
		t.Fatalf("Graph.Do error = %T %v, want BlueprintError", err, err)
	}
	if structured.NodeID != "break-loop" || structured.PC != PC(compiled.Entrances[1].Next[0].Index) {
		t.Fatalf("structured error = %#v, want break-loop node and pc", structured)
	}
}

func TestBlueprintDoContextAllowsNilContext(t *testing.T) {
	logger := &testTraceLogger{}
	blueprint, graphID := newTraceTestBlueprint(t, logger)
	if _, err := blueprint.DoContext(nil, graphID, 1); err != nil {
		t.Fatalf("DoContext(nil) failed: %v", err)
	}
}

func TestDataProducerPanicIdentifiesProducerNode(t *testing.T) {
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMDataPanic", func() IExecNode { return &vmDataPanicNode{} }, nil, []IPort{NewPortInt()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "producer", Class: "VMDataPanic"}, {ID: "consumer", Class: "VMReturnPort"}},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "consumer", DesPortID: 0},
			{SourceNodeID: "producer", SourcePortID: 0, DesNodeID: "consumer", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}

	_, err = NewGraph(compiled).Do(1)
	if err == nil || !strings.Contains(err.Error(), "producer") || strings.Contains(err.Error(), "native node consumer panic") {
		t.Fatalf("Graph.Do error = %v, want producer panic attribution", err)
	}
	var structured *BlueprintError
	if !errors.As(err, &structured) || structured.NodeID != "producer" {
		t.Fatalf("Graph.Do structured error = %#v, want producer node", structured)
	}
}

func TestYieldResumePanicBecomesExecutionFailure(t *testing.T) {
	var handle *YieldHandle
	yielded := false
	registry := vmNativeRegistry()
	registry.Register(NewNodeDefinition("VMYieldOnce", func() IExecNode {
		return &vmYieldOnceNode{handle: &handle, yielded: &yielded}
	}, []IPort{NewPortExec(), NewPortInt()}, []IPort{NewPortExec()}))
	compiled, err := CompileGraph(registry, GraphConfig{
		Nodes: []NodeConfig{{ID: "entry", Class: "VMEntry_1"}, {ID: "yield", Class: "VMYieldOnce", PortDefault: map[int]any{1: 1}}},
		Edges: []EdgeConfig{{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "yield", DesPortID: 0}},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	dispatcher := &manualExecutionDispatcher{}
	blueprint := &Blueprint{}
	blueprint.SetExecutionDispatcher(dispatcher)
	blueprint.AddCompiledGraph("resume-panic", compiled)
	execution, err := blueprint.Start(context.Background(), blueprint.Create("resume-panic"), 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	dispatcher.runNext(t)
	if handle == nil || handle.machine == nil || handle.machine.pendingYield == nil {
		t.Fatal("yield handle was not captured")
	}
	handle.machine.pendingYield.ctx = nil
	if err := handle.Resume(); err != nil {
		t.Fatalf("Resume submission failed: %v", err)
	}
	dispatcher.runNext(t)
	<-execution.Done()
	_, err = execution.Result()
	if err == nil || !strings.Contains(err.Error(), "resume panic") {
		t.Fatalf("Execution.Result error = %v, want recovered resume panic", err)
	}
}
