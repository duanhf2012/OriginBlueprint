package main

import (
	"fmt"
	"testing"
)

func analyzerDocument(nodes []GraphNode, connections []GraphConnection) GraphDocument {
	return GraphDocument{
		SchemaVersion:  GraphSchemaVersion,
		GraphName:      "core-analyzer-test",
		Nodes:          nodes,
		Connections:    connections,
		Groups:         []GraphGroup{},
		Variables:      []GraphVariable{},
		VariableGroups: []GraphVariableGroup{{ID: "default", Name: "Default"}},
		View:           GraphView{Zoom: 1},
	}
}

func issuesWithCode(issues []ValidationIssue, code string) []ValidationIssue {
	result := make([]ValidationIssue, 0)
	for _, issue := range issues {
		if issue.Code == code {
			result = append(result, issue)
		}
	}
	return result
}

func TestCoreAnalyzerReportsReachabilityAndLiveness(t *testing.T) {
	document := analyzerDocument([]GraphNode{
		{ID: "entry", TypeID: "origin.event.begin"},
		{ID: "reachable", TypeID: "origin.action.print"},
		{ID: "unreachable", TypeID: "origin.action.print"},
		{ID: "unused", TypeID: "origin.literal.string"},
	}, []GraphConnection{
		{Source: "entry", SourceOutput: "exec", Target: "reachable", TargetInput: "exec"},
	})
	issues := validateGraph(document)
	unreachable := requireValidationIssue(t, issues, "flow.unreachable-node")
	if unreachable.NodeID != "unreachable" || unreachable.BlocksSave {
		t.Fatalf("unreachable issue = %#v", unreachable)
	}
	unused := requireValidationIssue(t, issues, "flow.unused-data-node")
	if unused.NodeID != "unused" || unused.Severity != "warning" || unused.BlocksSave {
		t.Fatalf("unused issue = %#v", unused)
	}
}

func TestCoreAnalyzerReportsPureOnlyDataIsland(t *testing.T) {
	document := analyzerDocument([]GraphNode{
		{ID: "left", TypeID: "origin.literal.string"},
		{ID: "right", TypeID: "origin.literal.string"},
	}, []GraphConnection{
		{Source: "left", SourceOutput: "value", Target: "right", TargetInput: "value"},
	})
	issues := issuesWithCode(validateGraph(document), "flow.unused-data-node")
	if len(issues) != 2 {
		t.Fatalf("unused pure nodes = %#v, want both nodes reported", issues)
	}
}

func TestCoreAnalyzerBlocksMultipleProducersAndExecFanout(t *testing.T) {
	document := analyzerDocument([]GraphNode{
		{ID: "entry", TypeID: "origin.event.begin"},
		{ID: "left", TypeID: "origin.literal.string"},
		{ID: "right", TypeID: "origin.literal.string"},
		{ID: "first", TypeID: "origin.action.print"},
		{ID: "second", TypeID: "origin.action.print"},
	}, []GraphConnection{
		{Source: "entry", SourceOutput: "exec", Target: "first", TargetInput: "exec"},
		{Source: "entry", SourceOutput: "exec", Target: "second", TargetInput: "exec"},
		{Source: "left", SourceOutput: "value", Target: "first", TargetInput: "value"},
		{Source: "right", SourceOutput: "value", Target: "first", TargetInput: "value"},
	})
	issues := validateGraph(document)
	for _, code := range []string{"connection.multiple-producers", "flow.exec-fanout"} {
		issue := requireValidationIssue(t, issues, code)
		if !issue.BlocksSave {
			t.Fatalf("%s issue = %#v, want BlocksSave", code, issue)
		}
	}
}

func TestCoreAnalyzerReturnsEveryConfirmedCycle(t *testing.T) {
	nodes := []GraphNode{
		{ID: "entry", TypeID: "origin.event.begin"},
		{ID: "sequence", TypeID: "origin.flow.sequence", Properties: GraphNodeProperties{DynamicOutputCount: 2}},
		{ID: "exec-a", TypeID: "origin.action.print"},
		{ID: "exec-b", TypeID: "origin.action.print"},
		{ID: "exec-c", TypeID: "origin.action.print"},
		{ID: "exec-d", TypeID: "origin.action.print"},
		{ID: "data-a", TypeID: "origin.math.add-integer"},
		{ID: "data-b", TypeID: "origin.math.add-integer"},
		{ID: "data-c", TypeID: "origin.math.add-integer"},
		{ID: "data-d", TypeID: "origin.math.add-integer"},
	}
	connections := []GraphConnection{
		{Source: "entry", SourceOutput: "exec", Target: "sequence", TargetInput: "exec"},
		{Source: "sequence", SourceOutput: "then0", Target: "exec-a", TargetInput: "exec"},
		{Source: "exec-a", SourceOutput: "exec", Target: "exec-b", TargetInput: "exec"},
		{Source: "exec-b", SourceOutput: "exec", Target: "exec-a", TargetInput: "exec"},
		{Source: "sequence", SourceOutput: "then1", Target: "exec-c", TargetInput: "exec"},
		{Source: "exec-c", SourceOutput: "exec", Target: "exec-d", TargetInput: "exec"},
		{Source: "exec-d", SourceOutput: "exec", Target: "exec-c", TargetInput: "exec"},
		{Source: "data-a", SourceOutput: "result", Target: "data-b", TargetInput: "a"},
		{Source: "data-b", SourceOutput: "result", Target: "data-a", TargetInput: "a"},
		{Source: "data-c", SourceOutput: "result", Target: "data-d", TargetInput: "a"},
		{Source: "data-d", SourceOutput: "result", Target: "data-c", TargetInput: "a"},
	}
	issues := validateGraph(analyzerDocument(nodes, connections))
	if got := len(issuesWithCode(issues, "flow.exec-cycle")); got != 2 {
		t.Fatalf("exec cycle issues = %d, want 2: %#v", got, issues)
	}
	if got := len(issuesWithCode(issues, "flow.data-cycle")); got != 2 {
		t.Fatalf("data cycle issues = %d, want 2: %#v", got, issues)
	}
	for _, code := range []string{"flow.exec-cycle", "flow.data-cycle"} {
		for _, issue := range issuesWithCode(issues, code) {
			if !issue.BlocksSave || len(issue.NodeIDs) != 2 {
				t.Fatalf("cycle issue = %#v", issue)
			}
		}
	}
}

func TestCoreAnalyzerAllowsStructuredLoopBreak(t *testing.T) {
	document := analyzerDocument([]GraphNode{
		{ID: "entry", TypeID: "origin.event.begin"},
		{ID: "loop", TypeID: "origin.flow.for-loop-break"},
		{ID: "body", TypeID: "origin.action.print"},
	}, []GraphConnection{
		{Source: "entry", SourceOutput: "exec", Target: "loop", TargetInput: "exec"},
		{Source: "loop", SourceOutput: "body", Target: "body", TargetInput: "exec"},
		{Source: "body", SourceOutput: "exec", Target: "loop", TargetInput: "break"},
	})
	if issues := issuesWithCode(validateGraph(document), "flow.exec-cycle"); len(issues) != 0 {
		t.Fatalf("structured break reported as cycle: %#v", issues)
	}
}

func TestCoreAnalyzerRejectsBreakFromOutsideLoopBody(t *testing.T) {
	document := analyzerDocument([]GraphNode{
		{ID: "entry", TypeID: "origin.event.begin"},
		{ID: "loop", TypeID: "origin.flow.for-loop-break"},
		{ID: "outside", TypeID: "origin.action.print"},
	}, []GraphConnection{
		{Source: "entry", SourceOutput: "exec", Target: "loop", TargetInput: "exec"},
		{Source: "loop", SourceOutput: "completed", Target: "outside", TargetInput: "exec"},
		{Source: "outside", SourceOutput: "exec", Target: "loop", TargetInput: "break"},
	})
	issue := requireValidationIssue(t, validateGraph(document), "flow.exec-cycle")
	if !issue.BlocksSave {
		t.Fatalf("issue = %#v, want BlocksSave", issue)
	}
}

func TestCoreAnalyzerDoesNotBlockOpaqueLegacyCycles(t *testing.T) {
	legacyPorts := GraphNodeProperties{
		LegacyInputs:  []GraphLegacyPort{{Key: "exec", Type: "exec"}},
		LegacyOutputs: []GraphLegacyPort{{Key: "exec", Type: "exec"}},
	}
	document := analyzerDocument([]GraphNode{
		{ID: "entry", TypeID: "origin.event.begin"},
		{ID: "legacy-a", TypeID: "origin.legacy.placeholder", Properties: legacyPorts},
		{ID: "legacy-b", TypeID: "origin.legacy.placeholder", Properties: legacyPorts},
	}, []GraphConnection{
		{Source: "entry", SourceOutput: "exec", Target: "legacy-a", TargetInput: "exec"},
		{Source: "legacy-a", SourceOutput: "exec", Target: "legacy-b", TargetInput: "exec"},
		{Source: "legacy-b", SourceOutput: "exec", Target: "legacy-a", TargetInput: "exec"},
	})
	issue := requireValidationIssue(t, validateGraph(document), "flow.possible-cycle")
	if issue.Severity != "warning" || issue.BlocksSave || issue.BlocksRun {
		t.Fatalf("opaque cycle issue = %#v, want non-blocking warning", issue)
	}
	if len(issue.NodeIDs) != 2 || issue.NodeIDs[0] != "legacy-a" || issue.NodeIDs[1] != "legacy-b" {
		t.Fatalf("opaque cycle nodes = %#v", issue.NodeIDs)
	}
}

func TestCoreAnalyzerHandlesDeepGraphIteratively(t *testing.T) {
	const count = 20000
	nodes := make([]GraphNode, 0, count+1)
	connections := make([]GraphConnection, 0, count)
	nodes = append(nodes, GraphNode{ID: "entry", TypeID: "origin.event.begin"})
	previous := "entry"
	previousOutput := "exec"
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("node-%05d", index)
		nodes = append(nodes, GraphNode{ID: id, TypeID: "origin.action.print"})
		connections = append(connections, GraphConnection{Source: previous, SourceOutput: previousOutput, Target: id, TargetInput: "exec"})
		previous = id
		previousOutput = "exec"
	}
	issues := validateGraph(analyzerDocument(nodes, connections))
	if cycles := issuesWithCode(issues, "flow.exec-cycle"); len(cycles) != 0 {
		t.Fatalf("deep acyclic graph has cycles: %#v", cycles)
	}
}
