package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGraphFileRoundTrip(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")
	content := `{"schemaVersion":1,"nodes":[]}`

	saved, err := app.SaveGraph(path, content)
	if err != nil {
		t.Fatal(err)
	}
	if saved != path {
		t.Fatalf("saved path = %q, want %q", saved, path)
	}

	opened, err := app.OpenGraph(path)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Content != content {
		t.Fatalf("content = %q, want %q", opened.Content, content)
	}
	if got := app.lastGraphDirectory(); got != dir {
		t.Fatalf("last graph directory = %q, want %q", got, dir)
	}
}

func TestGraphContentForLegacyPathExportsVGF(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Legacy",
		Nodes: []GraphNode{{
			ID:       "add",
			TypeID:   "origin.math.add-integer",
			Position: GraphPosition{X: 12, Y: 34},
			Values:   map[string]interface{}{"a": 1, "b": 2},
			Properties: GraphNodeProperties{
				Label:        "AddInt",
				LegacyClass:  "AddInt",
				LegacyModule: "tools.json_node_loader",
			},
		}},
	}
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	data, err := graphContentForPath("test.vgf", string(content))
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.GraphName != "Legacy" || len(legacy.Nodes) != 1 || legacy.Nodes[0].Class != "AddInt" {
		t.Fatalf("legacy = %#v", legacy)
	}
	if strings.Contains(string(data), "schemaVersion") {
		t.Fatalf("vgf output should not contain schemaVersion: %s", data)
	}
}

func TestGraphContentForObpPathExportsLegacyVGFForExternalParser(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "External",
		Nodes: []GraphNode{{
			ID:       "add",
			TypeID:   "origin.math.add-integer",
			Position: GraphPosition{X: 1, Y: 2},
		}},
	}
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	data, err := graphContentForPath("external.obp", string(content))
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.GraphName != "External" || len(legacy.Nodes) != 1 || legacy.Nodes[0].Class != "AddInt" {
		t.Fatalf("legacy = %#v", legacy)
	}
	if strings.Contains(string(data), "schemaVersion") || strings.Contains(string(data), "typeId") {
		t.Fatalf("obp output should be legacy-compatible vgf JSON: %s", data)
	}
}

func TestFunctionBlueprintPathStaysNativeAndCreatesParentDirectory(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "functions", "CalculateDamage.obpf")
	content := `{"schemaVersion":1,"graphName":"CalculateDamage","nodes":[],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1}}`

	saved, err := app.SaveGraph(path, content)
	if err != nil {
		t.Fatal(err)
	}
	if saved != path {
		t.Fatalf("saved path = %q, want %q", saved, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("function blueprint content = %s, want native graph JSON", data)
	}
}

func TestFunctionBlueprintPreservesSignatureMetadata(t *testing.T) {
	content := `{"schemaVersion":1,"graphName":"CalculateDamage","nodes":[],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1},"functionSignature":{"inputs":[{"id":"target","name":"TargetId","type":"integer"}],"outputs":[{"id":"damage","name":"Damage","type":"integer"}]}}`
	data, err := graphContentForPath("CalculateDamage.obpf", content)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("function blueprint signature should stay in native JSON: %s", data)
	}
}

func TestValidateFunctionEntryAndReturnUsesSignaturePorts(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "CalculateDamage",
		Nodes: []GraphNode{
			{
				ID:     "entry",
				TypeID: "origin.function.entry",
				Properties: GraphNodeProperties{
					Label:        "CalculateDamage Entry",
					FunctionRole: "entry",
					FunctionName: "CalculateDamage",
					FunctionSignature: GraphFunctionSignature{
						Inputs:  []GraphFunctionSignaturePort{{ID: "target", Name: "TargetId", Type: "integer"}},
						Outputs: []GraphFunctionSignaturePort{{ID: "damage", Name: "Damage", Type: "integer"}},
					},
				},
			},
			{
				ID:     "return",
				TypeID: "origin.function.return",
				Properties: GraphNodeProperties{
					Label:        "CalculateDamage Return",
					FunctionRole: "return",
					FunctionName: "CalculateDamage",
					FunctionSignature: GraphFunctionSignature{
						Inputs:  []GraphFunctionSignaturePort{{ID: "target", Name: "TargetId", Type: "integer"}},
						Outputs: []GraphFunctionSignaturePort{{ID: "damage", Name: "Damage", Type: "integer"}},
					},
				},
			},
		},
		Connections: []GraphConnection{
			{Source: "entry", SourceOutput: "exec", Target: "return", TargetInput: "exec"},
			{Source: "entry", SourceOutput: "input_target", Target: "return", TargetInput: "output_damage"},
		},
	}
	issues := validateGraph(document)
	if len(issues) != 0 {
		t.Fatalf("validateGraph issues = %#v, want none", issues)
	}
}

func TestWorkspaceListsFunctionBlueprintFiles(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	functionDir := filepath.Join(dir, "functions")
	if err := os.MkdirAll(functionDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(functionDir, "CalculateDamage.obpf"), []byte(`{"schemaVersion":1}`), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := app.ListWorkspace(functionDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "CalculateDamage.obpf" {
		t.Fatalf("workspace entries = %#v, want function blueprint file", entries)
	}
}

func TestGraphFiltersIncludeFunctionBlueprintFiles(t *testing.T) {
	filters := graphFilters()
	for _, filter := range filters {
		if strings.Contains(filter.Pattern, "*.obpf") {
			return
		}
	}
	t.Fatalf("graph filters = %#v, want *.obpf support", filters)
}

func TestLegacyGraphRoundTripPreservesEntryConnectionVisibility(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Entry Visibility",
		Nodes: []GraphNode{
			{ID: "add1", TypeID: "origin.math.add-integer", Position: GraphPosition{X: 10, Y: 20}},
			{ID: "add2", TypeID: "origin.math.add-integer", Position: GraphPosition{X: 200, Y: 20}},
		},
		Connections: []GraphConnection{{
			Source:                 "add1",
			SourceOutput:           "result",
			Target:                 "add2",
			TargetInput:            "a",
			EntryConnectionVisible: true,
		}},
	}

	data, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Edges) != 1 || !legacy.Edges[0].EntryConnectionVisible {
		t.Fatalf("exported edge visibility was not preserved: %#v", legacy.Edges)
	}

	roundTrip, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(roundTrip.Connections) != 1 || !roundTrip.Connections[0].EntryConnectionVisible {
		t.Fatalf("migrated connection visibility was not preserved: %#v", roundTrip.Connections)
	}
}

func TestExportLegacyGraphSynthesizesNewVariableNodes(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Variables",
		Variables: []GraphVariable{{
			ID:           "score",
			Name:         "Score",
			Type:         "integer",
			DefaultValue: 0,
			GroupID:      "default",
		}},
		Nodes: []GraphNode{
			{ID: "get", TypeID: "origin.variable.get", Position: GraphPosition{X: 10, Y: 20}, Properties: GraphNodeProperties{VariableID: "score"}},
			{ID: "set", TypeID: "origin.variable.set", Position: GraphPosition{X: 30, Y: 40}, Properties: GraphNodeProperties{VariableID: "score"}},
		},
	}
	data, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	classes := map[string]bool{}
	for _, node := range legacy.Nodes {
		classes[node.Class] = true
	}
	if !classes["Get_Score"] || !classes["Set_Score"] {
		t.Fatalf("legacy variable nodes were not exported: %#v", legacy.Nodes)
	}
}

func TestExportLegacyGraphPreservesRuntimeJSONNodeClassAndPortIDs(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Runtime JSON",
		Nodes: []GraphNode{{
			ID:       "hit",
			TypeID:   "origin.custom.do-hit-effect",
			Position: GraphPosition{X: 11, Y: 22},
			Values:   map[string]interface{}{"in14": 99},
		}},
	}
	data, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Nodes) != 1 {
		t.Fatalf("legacy nodes = %#v", legacy.Nodes)
	}
	node := legacy.Nodes[0]
	if node.Class != "DoHitEffect" || node.Module != "tools.json_node_loader" {
		t.Fatalf("legacy node = %#v", node)
	}
	if node.PortDefaults["14"] != float64(99) {
		t.Fatalf("port defaults = %#v", node.PortDefaults)
	}
	if strings.Contains(string(data), "schemaVersion") || strings.Contains(string(data), "typeId") {
		t.Fatalf("runtime JSON node output should be legacy-compatible vgf JSON: %s", data)
	}
}

func TestValidateGraphAcceptsValidVariableGraph(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Variables",
		Variables:     []GraphVariable{{ID: "score", Name: "Score", Type: "integer", DefaultValue: 0}},
		Nodes: []GraphNode{
			{ID: "get", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "score"}},
			{ID: "cast", TypeID: "origin.cast.integer-string"},
		},
		Connections: []GraphConnection{{Source: "get", SourceOutput: "value", Target: "cast", TargetInput: "value"}},
	}
	issues := validateGraph(document)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateGraphReportsMissingVariableAndTypeMismatch(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "missing", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "not-found"}},
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "cast", TypeID: "origin.cast.integer-string"},
		},
		Connections: []GraphConnection{{Source: "begin", SourceOutput: "exec", Target: "cast", TargetInput: "value"}},
	}
	issues := validateGraph(document)
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2: %#v", len(issues), issues)
	}
	if issues[0].Code != "variable.missing" || issues[1].Code != "connection.type-mismatch" {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}

func TestValidateGraphReportsUnreachableFlowNodesFromEntries(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.begin"},
			{ID: "reachable", TypeID: "origin.debug.output"},
			{ID: "wild", TypeID: "origin.debug.output"},
		},
		Connections: []GraphConnection{{Source: "entry", SourceOutput: "exec", Target: "reachable", TargetInput: "exec"}},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "flow.unreachable-node", "wild") {
		t.Fatalf("issues = %#v, want unreachable wild node", issues)
	}
	if hasIssue(issues, "flow.unreachable-node", "reachable") {
		t.Fatalf("reachable node should not be reported: %#v", issues)
	}
}

func TestValidateGraphTreatsDataDependenciesAsEntryReachable(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.begin"},
			{ID: "debug", TypeID: "origin.debug.output"},
			{ID: "row-count", TypeID: "origin.table.row-count"},
		},
		Connections: []GraphConnection{
			{Source: "entry", SourceOutput: "exec", Target: "debug", TargetInput: "exec"},
			{Source: "row-count", SourceOutput: "count", Target: "debug", TargetInput: "integer"},
		},
	}
	issues := validateGraph(document)
	if hasIssue(issues, "flow.unreachable-node", "row-count") {
		t.Fatalf("data dependency should be attributed to the entry path: %#v", issues)
	}
}

func TestValidateGraphReportsPossibleExecCycles(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.begin"},
			{ID: "a", TypeID: "origin.debug.output"},
			{ID: "b", TypeID: "origin.debug.output"},
		},
		Connections: []GraphConnection{
			{Source: "entry", SourceOutput: "exec", Target: "a", TargetInput: "exec"},
			{Source: "a", SourceOutput: "exec", Target: "b", TargetInput: "exec"},
			{Source: "b", SourceOutput: "exec", Target: "a", TargetInput: "exec"},
		},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "flow.possible-cycle", "a") && !hasIssue(issues, "flow.possible-cycle", "b") {
		t.Fatalf("issues = %#v, want possible exec cycle", issues)
	}
}

func TestValidateGraphWarnsWhenExecutableGraphHasNoEntry(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "debug", TypeID: "origin.debug.output"},
			{ID: "other", TypeID: "origin.debug.output"},
		},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "flow.missing-entry", "") {
		t.Fatalf("issues = %#v, want missing entry warning", issues)
	}
	for _, issue := range issues {
		if issue.Code != "flow.missing-entry" {
			continue
		}
		if len(issue.NodeIDs) != 2 || issue.NodeIDs[0] != "debug" || issue.NodeIDs[1] != "other" {
			t.Fatalf("missing entry warning should include related node ids: %#v", issue)
		}
	}
}

func TestValidateGraphFollowsRuntimeJsonExecPorts(t *testing.T) {
	legacy := legacyGraph{
		GraphName: "runtime-json-exec",
		Nodes: []legacyNode{
			{ID: "entry", Class: "Entrance_AutoChoiceSkill_40301"},
			{ID: "skill", Class: "GetObjLeftMinHpPercent"},
			{ID: "switch", Class: "EqualSwitch"},
		},
		Edges: []legacyEdge{
			{SourceNodeID: "entry", SourceIndex: 0, SourcePortID: 0, TargetNodeID: "skill", TargetIndex: 0, TargetPortID: 0},
			{SourceNodeID: "skill", SourceIndex: 0, SourcePortID: 0, TargetNodeID: "switch", TargetIndex: 0, TargetPortID: 0},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	document, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	issues := validateGraph(document)
	if hasIssue(issues, "flow.missing-entry", "") {
		t.Fatalf("runtime JSON exec entry should be recognized: %#v", issues)
	}
	if hasIssue(issues, "flow.unreachable-node", "skill") || hasIssue(issues, "flow.unreachable-node", "switch") {
		t.Fatalf("runtime JSON exec chain should be reachable: %#v", issues)
	}
}

func TestValidateGraphDoesNotReportUnknownNodeType(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes:         []GraphNode{{ID: "unknown", TypeID: "origin.missing.node"}},
	}
	issues := validateGraph(document)
	if hasIssue(issues, "node.unknown-type", "unknown") {
		t.Fatalf("unknown node type should be ignored in test results: %#v", issues)
	}
}

func TestValidateGraphUsesChineseMessagesForExecutionIssues(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.begin"},
			{ID: "wild", TypeID: "origin.debug.output"},
		},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "flow.unreachable-node", "wild") {
		t.Fatalf("issues = %#v, want unreachable wild node", issues)
	}
	for _, issue := range issues {
		if issue.Code == "flow.unreachable-node" && !strings.Contains(issue.Message, "不可达") {
			t.Fatalf("unreachable message should be Chinese: %#v", issue)
		}
	}
}

func TestValidateGraphReportsCrossEntryDataConnections(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry-a", TypeID: "origin.event.entry-two-integers"},
			{ID: "entry-b", TypeID: "origin.event.entry-array"},
			{ID: "debug-a", TypeID: "origin.debug.output"},
			{ID: "debug-b", TypeID: "origin.debug.output"},
		},
		Connections: []GraphConnection{
			{Source: "entry-a", SourceOutput: "exec", Target: "debug-a", TargetInput: "exec"},
			{Source: "entry-b", SourceOutput: "exec", Target: "debug-b", TargetInput: "exec"},
			{Source: "entry-a", SourceOutput: "objectId", Target: "debug-b", TargetInput: "integer"},
		},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "flow.cross-entry-data", "debug-b") {
		t.Fatalf("issues = %#v, want cross-entry data issue", issues)
	}
}

func hasIssue(issues []ValidationIssue, code string, nodeID string) bool {
	for _, issue := range issues {
		if issue.Code == code && (nodeID == "" || issue.NodeID == nodeID) {
			return true
		}
	}
	return false
}

func hasValidationErrors(issues []ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

func TestValidateGraphServiceRejectsInvalidJSON(t *testing.T) {
	app := NewApp()
	if _, err := app.ValidateGraph("{"); err == nil {
		t.Fatal("ValidateGraph should reject invalid JSON")
	}
	document, _ := json.Marshal(GraphDocument{SchemaVersion: GraphSchemaVersion})
	if _, err := app.ValidateGraph(string(document)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateGraphAcceptsMigratedMathAndFlowNodes(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "add", TypeID: "origin.math.add-integer"},
			{ID: "compare", TypeID: "origin.flow.greater-integer"},
			{ID: "sequence", TypeID: "origin.flow.sequence"},
		},
		Connections: []GraphConnection{
			{Source: "add", SourceOutput: "result", Target: "compare", TargetInput: "a"},
			{Source: "compare", SourceOutput: "true", Target: "sequence", TargetInput: "exec"},
		},
	}
	if issues := validateGraph(document); hasValidationErrors(issues) {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateGraphAcceptsArrayAndDynamicSequence(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Variables:     []GraphVariable{{ID: "items", Name: "Items", Type: "array", DefaultValue: []interface{}{1, 2}}},
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.entry-array"},
			{ID: "length", TypeID: "origin.array.length"},
			{ID: "sequence", TypeID: "origin.flow.sequence", Properties: GraphNodeProperties{DynamicOutputCount: 5}},
			{ID: "debug", TypeID: "origin.debug.output"},
		},
		Connections: []GraphConnection{
			{Source: "entry", SourceOutput: "exec", Target: "sequence", TargetInput: "exec"},
			{Source: "entry", SourceOutput: "params", Target: "length", TargetInput: "array"},
			{Source: "sequence", SourceOutput: "then4", Target: "debug", TargetInput: "exec"},
		},
	}
	if issues := validateGraph(document); hasValidationErrors(issues) {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateGraphRejectsUnknownVariableType(t *testing.T) {
	document := GraphDocument{SchemaVersion: GraphSchemaVersion, Variables: []GraphVariable{{ID: "bad", Name: "Bad", Type: "mystery"}}}
	issues := validateGraph(document)
	if len(issues) != 1 || issues[0].Code != "variable.unknown-type" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateGraphAcceptsVariableGroups(t *testing.T) {
	document := GraphDocument{
		SchemaVersion:  GraphSchemaVersion,
		VariableGroups: []GraphVariableGroup{{ID: "default", Name: "Default"}, {ID: "combat", Name: "Combat"}},
		Variables:      []GraphVariable{{ID: "health", Name: "Health", Type: "integer", GroupID: "combat", DefaultValue: 100}},
	}
	if issues := validateGraph(document); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestValidateGraphRejectsInvalidVariableGroups(t *testing.T) {
	document := GraphDocument{
		SchemaVersion:  GraphSchemaVersion,
		VariableGroups: []GraphVariableGroup{{ID: "combat", Name: "Combat"}, {ID: "combat-2", Name: "Combat"}},
		Variables:      []GraphVariable{{ID: "health", Name: "Health", Type: "integer", GroupID: "missing"}},
	}
	issues := validateGraph(document)
	if len(issues) != 2 || issues[0].Code != "variable-group.duplicate-name" || issues[1].Code != "variable.missing-group" {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}

func TestExecuteGraphRunsLoopVariablesAndResults(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Variables:     []GraphVariable{{ID: "last", Name: "Last", Type: "integer", DefaultValue: 0}},
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "loop", TypeID: "origin.flow.for-loop", Values: map[string]interface{}{"start": 1, "end": 4}},
			{ID: "set", TypeID: "origin.variable.set", Properties: GraphNodeProperties{VariableID: "last"}},
			{ID: "get", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "last"}},
			{ID: "result", TypeID: "origin.result.append-integer"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "loop", TargetInput: "exec"},
			{Source: "loop", SourceOutput: "body", Target: "set", TargetInput: "exec"},
			{Source: "loop", SourceOutput: "index", Target: "set", TargetInput: "value"},
			{Source: "loop", SourceOutput: "completed", Target: "result", TargetInput: "exec"},
			{Source: "get", SourceOutput: "value", Target: "result", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Variables["Last"] != float64(3) || len(result.Results) != 1 || result.Results[0] != float64(3) {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteGraphReportsDivisionByZero(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "print", TypeID: "origin.action.print"},
			{ID: "divide", TypeID: "origin.math.divide-integer", Values: map[string]interface{}{"a": 5, "b": 0}},
			{ID: "cast", TypeID: "origin.cast.integer-string"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "print", TargetInput: "exec"},
			{Source: "divide", SourceOutput: "result", Target: "cast", TargetInput: "value"},
			{Source: "cast", SourceOutput: "result", Target: "print", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err == nil || !strings.Contains(err.Error(), "division by zero") || len(result.Logs) != 1 || result.Logs[0].Level != "error" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestExecuteGraphHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	document := GraphDocument{SchemaVersion: GraphSchemaVersion, Nodes: []GraphNode{{ID: "begin", TypeID: "origin.event.begin"}}}
	_, err := executeGraph(ctx, "test", document, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestExecuteGraphReadsAndPreviewsCSVTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "people.csv")
	if err := os.WriteFile(path, []byte("id,name\n1,Ada\n2,Grace\n"), 0644); err != nil {
		t.Fatal(err)
	}
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Variables:     []GraphVariable{{ID: "table", Name: "People", Type: "table", DefaultValue: RuntimeTable{Columns: []string{}, Rows: [][]interface{}{}}}},
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "file", TypeID: "origin.io.file-path", Values: map[string]interface{}{"path": path}},
			{ID: "read", TypeID: "origin.table.read-csv", Values: map[string]interface{}{"delimiter": ",", "header": true}},
			{ID: "set", TypeID: "origin.variable.set", Properties: GraphNodeProperties{VariableID: "table"}},
			{ID: "get", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "table"}},
			{ID: "preview", TypeID: "origin.table.preview"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "read", TargetInput: "exec"},
			{Source: "file", SourceOutput: "file", Target: "read", TargetInput: "file"},
			{Source: "read", SourceOutput: "exec", Target: "set", TargetInput: "exec"},
			{Source: "read", SourceOutput: "table", Target: "set", TargetInput: "value"},
			{Source: "get", SourceOutput: "value", Target: "preview", TargetInput: "table"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results = %#v", result.Results)
	}
	preview, ok := result.Results[0].(map[string]interface{})
	if !ok || preview["kind"] != "table" {
		t.Fatalf("preview = %#v", result.Results[0])
	}
	table, err := asRuntimeTable(preview["table"])
	if err != nil || len(table.Rows) != 2 || strings.Join(table.Columns, ",") != "id,name" {
		t.Fatalf("table = %#v, err = %v", table, err)
	}
}

func TestExecuteGraphUpdatesDictionary(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Variables:     []GraphVariable{{ID: "dict", Name: "Lookup", Type: "dictionary", DefaultValue: map[string]interface{}{}}},
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "get", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "dict"}},
			{ID: "dict-set", TypeID: "origin.dictionary.set", Values: map[string]interface{}{"key": "answer", "value": 42}},
			{ID: "set", TypeID: "origin.variable.set", Properties: GraphNodeProperties{VariableID: "dict"}},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "dict-set", TargetInput: "exec"},
			{Source: "get", SourceOutput: "value", Target: "dict-set", TargetInput: "dictionary"},
			{Source: "dict-set", SourceOutput: "exec", Target: "set", TargetInput: "exec"},
			{Source: "dict-set", SourceOutput: "dictionary", Target: "set", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	dictionary := asDictionary(result.Variables["Lookup"])
	if dictionary["answer"] != float64(42) {
		t.Fatalf("dictionary = %#v", dictionary)
	}
}

func TestAdvancedTableOperations(t *testing.T) {
	source := RuntimeTable{
		Columns: []string{"id", "name", "score", "unused"},
		Rows: [][]interface{}{
			{"2", "Grace", "91.5", ""},
			{"1", "Ada", "88", nil},
			{"3", "Linus", "88", "drop"},
		},
	}
	selected, err := selectTableColumns(source, []string{"id", "name", "score", "unused"})
	if err != nil {
		t.Fatal(err)
	}
	sorted, err := sortTable(selected, "id", true)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := filterTableEqual(sorted, "score", 88)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := renameTableColumn(filtered, "name", "author")
	if err != nil {
		t.Fatal(err)
	}
	dropped, err := dropTableColumns(renamed, []string{"unused"})
	if err != nil {
		t.Fatal(err)
	}
	filled, err := fillEmptyTableCells(dropped, "N/A")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(filled.Columns, ",") != "id,author,score" || len(filled.Rows) != 2 {
		t.Fatalf("table = %#v", filled)
	}
	if filled.Rows[0][0] != "1" || filled.Rows[1][1] != "Linus" {
		t.Fatalf("rows = %#v", filled.Rows)
	}
}

func TestExecuteGraphForEachArrayPreservesLegacyPortOrder(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "foreach", TypeID: "origin.flow.foreach-array", Values: map[string]interface{}{"array": []interface{}{"A", "B", "C"}}},
			{ID: "cast", TypeID: "origin.cast.any-string"},
			{ID: "result", TypeID: "origin.result.append-string"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "foreach", TargetInput: "exec"},
			{Source: "foreach", SourceOutput: "body", Target: "cast", TargetInput: "exec"},
			{Source: "foreach", SourceOutput: "value", Target: "cast", TargetInput: "value"},
			{Source: "cast", SourceOutput: "exec", Target: "result", TargetInput: "exec"},
			{Source: "cast", SourceOutput: "result", Target: "result", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 3 || result.Results[0] != "A" || result.Results[2] != "C" {
		t.Fatalf("results = %#v", result.Results)
	}
}

func TestExecuteGraphRunsWhileLoop(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Variables:     []GraphVariable{{ID: "count", Name: "Count", Type: "integer", DefaultValue: 0}},
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "while", TypeID: "origin.flow.while"},
			{ID: "compare", TypeID: "origin.compare.greater-integer", Values: map[string]interface{}{"a": 3}},
			{ID: "get", TypeID: "origin.variable.get", Properties: GraphNodeProperties{VariableID: "count"}},
			{ID: "add", TypeID: "origin.math.add-integer", Values: map[string]interface{}{"b": 1}},
			{ID: "set", TypeID: "origin.variable.set", Properties: GraphNodeProperties{VariableID: "count"}},
			{ID: "result", TypeID: "origin.result.append-integer"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "while", TargetInput: "exec"},
			{Source: "get", SourceOutput: "value", Target: "compare", TargetInput: "b"},
			{Source: "compare", SourceOutput: "result", Target: "while", TargetInput: "condition"},
			{Source: "while", SourceOutput: "body", Target: "set", TargetInput: "exec"},
			{Source: "get", SourceOutput: "value", Target: "add", TargetInput: "a"},
			{Source: "add", SourceOutput: "result", Target: "set", TargetInput: "value"},
			{Source: "while", SourceOutput: "completed", Target: "result", TargetInput: "exec"},
			{Source: "get", SourceOutput: "value", Target: "result", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Variables["Count"] != float64(3) || len(result.Results) != 1 || result.Results[0] != float64(3) {
		t.Fatalf("result = %#v", result)
	}
}

func TestExecuteGraphBreaksCurrentForLoop(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "loop", TypeID: "origin.flow.for-loop-break", Values: map[string]interface{}{"start": 0, "end": 10}},
			{ID: "compare", TypeID: "origin.compare.greater-integer", Values: map[string]interface{}{"b": 2}},
			{ID: "branch", TypeID: "origin.flow.branch"},
			{ID: "result", TypeID: "origin.result.append-integer"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "loop", TargetInput: "exec"},
			{Source: "loop", SourceOutput: "body", Target: "branch", TargetInput: "exec"},
			{Source: "loop", SourceOutput: "index", Target: "compare", TargetInput: "a"},
			{Source: "compare", SourceOutput: "result", Target: "branch", TargetInput: "condition"},
			{Source: "branch", SourceOutput: "true", Target: "loop", TargetInput: "break"},
			{Source: "branch", SourceOutput: "false", Target: "result", TargetInput: "exec"},
			{Source: "loop", SourceOutput: "index", Target: "result", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 3 || result.Results[0] != float64(0) || result.Results[2] != float64(2) {
		t.Fatalf("results = %#v", result.Results)
	}
}

func TestExecuteGraphRunsLegacyFloatArithmetic(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "begin", TypeID: "origin.event.begin"},
			{ID: "divide", TypeID: "origin.math.divide-float", Values: map[string]interface{}{"a": 5.0, "b": 2.0}},
			{ID: "cast", TypeID: "origin.cast.float-string"},
			{ID: "result", TypeID: "origin.result.append-string"},
		},
		Connections: []GraphConnection{
			{Source: "begin", SourceOutput: "exec", Target: "result", TargetInput: "exec"},
			{Source: "divide", SourceOutput: "result", Target: "cast", TargetInput: "value"},
			{Source: "cast", SourceOutput: "result", Target: "result", TargetInput: "value"},
		},
	}
	result, err := executeGraph(context.Background(), "test", document, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0] != "2.5" {
		t.Fatalf("results = %#v", result.Results)
	}
}

func TestTableRowDictionaryUsesColumnNames(t *testing.T) {
	row := tableRowDictionary([]string{"id", "name", "missing"}, []interface{}{7, "Ada"})
	if row["id"] != float64(7) || row["name"] != "Ada" || row["missing"] != nil {
		t.Fatalf("row = %#v", row)
	}
}

func TestMigrateLegacyTableAndDictionaryNodes(t *testing.T) {
	content := `{"graph_name":"Data","nodes":[{"id":"file","class":"FileNode","port_defaultv":{"0":"data.csv"}},{"id":"read","class":"TableReader","port_defaultv":{"2":",","3":true}},{"id":"preview","class":"PreviewTable","port_defaultv":{}},{"id":"keys","class":"Keys (Dict)","port_defaultv":{}}],"edges":[{"source_node_id":"file","source_port_index":0,"des_node_id":"read","des_port_index":1},{"source_node_id":"read","source_port_index":1,"des_node_id":"preview","des_port_index":0}],"groups":[],"variables":[{"name":"Table","type":"DataFrame","value":{"columns":[],"rows":[]}},{"name":"Lookup","type":"Dict","value":{}}]}`
	document, err := migrateLegacyGraph([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range document.Nodes {
		if node.TypeID == "origin.legacy.placeholder" {
			t.Fatalf("unexpected placeholder: %#v", node)
		}
	}
	if document.Variables[0].Type != "table" || document.Variables[1].Type != "dictionary" {
		t.Fatalf("variables = %#v", document.Variables)
	}
	if issues := validateGraph(document); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestMigrateLegacyHidesUnknownNodesButPreservesThemForRoundTrip(t *testing.T) {
	content := `{"graph_name":"Legacy","nodes":[{"id":"targets","class":"GetTargetsByCamp","module":"old","pos":[1,2],"port_defaultv":{"2":true}},{"id":"hidden","class":"UnknownSource","module":"old","pos":[5,6],"port_defaultv":{"0":"x"}},{"id":"loop","class":"ForeachIntArray","module":"old","pos":[9,10],"port_defaultv":{}}],"edges":[{"edge_id":"known","source_node_id":"targets","source_port_index":1,"des_node_id":"loop","des_port_index":1},{"edge_id":"hidden-edge","source_node_id":"hidden","source_port_index":2,"des_node_id":"loop","des_port_index":0}],"groups":[],"variables":[]}`
	document, err := migrateLegacyGraph([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 2 || len(document.Connections) != 1 {
		t.Fatalf("document = %#v", document)
	}
	for _, node := range document.Nodes {
		if node.Properties.LegacyClass == "UnknownSource" || node.TypeID == "origin.legacy.placeholder" {
			t.Fatalf("unknown node should be hidden, got %#v", node)
		}
	}
	if document.Legacy == nil || len(document.Legacy.HiddenNodes) != 1 || len(document.Legacy.HiddenEdges) != 1 {
		t.Fatalf("legacy state = %#v", document.Legacy)
	}
	roundTrip, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(roundTrip, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Nodes) != 3 || len(legacy.Edges) != 2 {
		t.Fatalf("legacy = %#v", legacy)
	}
	for _, issue := range validateGraph(document) {
		if issue.Severity == "error" {
			t.Fatalf("unexpected validation error: %#v", issue)
		}
	}
}

func TestMigrateLegacyHit20001ShowsDefinedNodes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("build", "bin", "vgf", "buffskill", "MonsterBuffSkill", "hit_20001.vgf"))
	if err != nil {
		t.Skip("hit_20001.vgf sample not available")
	}
	document, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	foundForeach := false
	for _, node := range document.Nodes {
		if node.Properties.LegacyClass == "ForeachIntArray" {
			foundForeach = true
			if node.Properties.Label != "" {
				t.Fatalf("legacy class should not override JSON title, node=%#v", node)
			}
		}
	}
	if !foundForeach {
		t.Fatalf("expected ForeachIntArray to be visible, nodes=%#v legacy=%#v", document.Nodes, document.Legacy)
	}
}

func TestMigrateBuildBinVGFFilesShowsAllDefinedNodes(t *testing.T) {
	root := filepath.Join("build", "bin", "vgf")
	if _, err := os.Stat(root); err != nil {
		t.Skip("build/bin/vgf samples not available")
	}
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".vgf") {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected build/bin/vgf to contain .vgf files")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var source legacyGraph
		if err := json.Unmarshal(data, &source); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		document, err := migrateLegacyGraph(data)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		hiddenNodes, hiddenEdges := 0, 0
		if document.Legacy != nil {
			hiddenNodes = len(document.Legacy.HiddenNodes)
			hiddenEdges = len(document.Legacy.HiddenEdges)
		}
		if hiddenNodes != 0 {
			t.Fatalf("%s has %d hidden node(s): %#v", path, hiddenNodes, document.Legacy.HiddenNodes)
		}
		if hiddenEdges != 0 {
			t.Fatalf("%s has %d hidden edge(s): %#v", path, hiddenEdges, document.Legacy.HiddenEdges)
		}
		if len(document.Nodes) != len(source.Nodes) || len(document.Connections) != len(source.Edges) {
			t.Fatalf("%s lost content: nodes %d/%d edges %d+%d/%d", path, len(document.Nodes), len(source.Nodes), len(document.Connections), hiddenEdges, len(source.Edges))
		}
		roundTrip, err := exportLegacyGraph(document)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var exported legacyGraph
		if err := json.Unmarshal(roundTrip, &exported); err != nil {
			t.Fatalf("%s exported invalid legacy JSON: %v", path, err)
		}
		if len(exported.Nodes) != len(source.Nodes) || len(exported.Edges) != len(source.Edges) {
			t.Fatalf("%s exported different content: nodes %d/%d edges %d/%d", path, len(exported.Nodes), len(source.Nodes), len(exported.Edges), len(source.Edges))
		}
		if strings.Contains(string(roundTrip), "schemaVersion") || strings.Contains(string(roundTrip), "typeId") {
			t.Fatalf("%s exported new-format fields in legacy output", path)
		}
	}
}

func TestMigrateLegacyRepositorySamples(t *testing.T) {
	root := filepath.Join("..", "OriginNodeEditor_old")
	if _, err := os.Stat(root); err != nil {
		t.Skip("legacy sample repository not available")
	}
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".vgf") {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var source legacyGraph
		if json.Unmarshal(data, &source) != nil {
			continue
		}
		document, err := migrateLegacyGraph(data)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		hiddenNodes, hiddenEdges := 0, 0
		if document.Legacy != nil {
			hiddenNodes = len(document.Legacy.HiddenNodes)
			hiddenEdges = len(document.Legacy.HiddenEdges)
		}
		if len(document.Nodes)+hiddenNodes != len(source.Nodes) || len(document.Connections)+hiddenEdges != len(source.Edges) {
			t.Fatalf("%s lost content: nodes %d+%d/%d edges %d+%d/%d", path, len(document.Nodes), hiddenNodes, len(source.Nodes), len(document.Connections), hiddenEdges, len(source.Edges))
		}
		for _, issue := range validateGraph(document) {
			if issue.Severity == "error" {
				t.Fatalf("%s: %#v", path, issue)
			}
		}
	}
}

func TestMigrateLegacyDoesNotCreatePlaceholderNodes(t *testing.T) {
	root := filepath.Join("..", "OriginNodeEditor_old")
	if _, err := os.Stat(root); err != nil {
		t.Skip("legacy sample repository not available")
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".vgf") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		document, err := migrateLegacyGraph(data)
		if err != nil {
			return err
		}
		for _, node := range document.Nodes {
			if node.TypeID == "origin.legacy.placeholder" {
				t.Fatalf("%s still contains COMPAT node %q", path, node.Properties.LegacyClass)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestChoiceskillEqualSwitchRoundTripKeepsLegacyBranchPorts(t *testing.T) {
	path := filepath.Join("build", "bin", "vgf", "monsterChoiceskill", "choiceskill_easy.vgf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("choiceskill_easy.vgf sample not available")
	}
	document, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]GraphNode{}
	for _, node := range document.Nodes {
		byID[node.ID] = node
	}
	first := byID["116323275342000809"]
	values, ok := first.Values["cases"].([]interface{})
	if !ok || len(values) != 4 {
		t.Fatalf("EqualSwitch cases = %#v", first.Values["cases"])
	}
	expectedOutputs := map[string]bool{"otherwise": false, "case1": false, "case2": false, "case3": false, "case4": false}
	for _, connection := range document.Connections {
		if connection.Source == first.ID {
			if _, exists := expectedOutputs[connection.SourceOutput]; exists {
				expectedOutputs[connection.SourceOutput] = true
			}
			if connection.SourceOutput == "case0" {
				t.Fatalf("hidden EqualSwitch placeholder output was connected: %#v", connection)
			}
		}
	}
	for key, seen := range expectedOutputs {
		if !seen {
			t.Fatalf("missing migrated EqualSwitch output %s", key)
		}
	}

	roundTrip, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(roundTrip, &legacy); err != nil {
		t.Fatal(err)
	}
	sourcePorts := map[int]bool{}
	for _, edge := range legacy.Edges {
		if edge.SourceNodeID == first.ID {
			sourcePorts[legacyPortIndex(edge.SourcePortID, edge.SourceIndex)] = true
		}
	}
	for _, port := range []int{0, 2, 3, 4, 5} {
		if !sourcePorts[port] {
			t.Fatalf("round-trip missing legacy source port %d; got %#v", port, sourcePorts)
		}
	}
	if sourcePorts[1] {
		t.Fatalf("round-trip unexpectedly connected hidden legacy source port 1")
	}
}

func TestValidateChoiceskillEasyRecognizesMonsterChoiceSkillEntry(t *testing.T) {
	path := filepath.Join("build", "bin", "vgf", "monsterChoiceskill", "choiceskill_easy.vgf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("choiceskill_easy.vgf sample not available")
	}
	document, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	issues := validateGraph(document)
	if hasIssue(issues, "flow.missing-entry", "") {
		t.Fatalf("monster choice skill entry should be recognized: %#v", issues)
	}
	if !hasIssue(issues, "flow.unreachable-node", "c18c0e9d88fd385f") {
		t.Fatalf("detached EqualSwitch should still be reported: %#v", issues)
	}
}

func TestNewEqualSwitchExportsAsLegacyEqualSwitch(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "NewSwitch",
		Nodes: []GraphNode{
			{
				ID:       "switch",
				TypeID:   "origin.flow.equal-switch-new",
				Position: GraphPosition{X: 10, Y: 20},
				Values:   map[string]interface{}{"cases": []interface{}{1, 2}},
			},
			{
				ID:       "target",
				TypeID:   "origin.result.append-integer",
				Position: GraphPosition{X: 200, Y: 20},
			},
		},
		Connections: []GraphConnection{{
			Source: "switch", SourceOutput: "case1", Target: "target", TargetInput: "exec",
		}},
	}
	data, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Nodes) != 2 || legacy.Nodes[0].Class != "EqualSwitch" {
		t.Fatalf("legacy nodes = %#v", legacy.Nodes)
	}
	if got := legacy.Nodes[0].PortDefaults["2"]; got == nil {
		t.Fatalf("cases default missing from legacy port 2: %#v", legacy.Nodes[0].PortDefaults)
	}
	if len(legacy.Edges) != 1 || legacyPortIndex(legacy.Edges[0].SourcePortID, legacy.Edges[0].SourceIndex) != 2 {
		t.Fatalf("legacy edges = %#v", legacy.Edges)
	}
}

func TestNewCreateIntegerArrayExportsAsLegacyCreateIntArray(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "NewArray",
		Nodes: []GraphNode{{
			ID:       "array",
			TypeID:   "origin.array.create-integer-new",
			Position: GraphPosition{X: 10, Y: 20},
			Values:   map[string]interface{}{"items": []interface{}{1, 2}},
		}},
	}
	data, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.Nodes) != 1 || legacy.Nodes[0].Class != "CreateIntArray" {
		t.Fatalf("legacy nodes = %#v", legacy.Nodes)
	}
	if got := legacy.Nodes[0].PortDefaults["0"]; got == nil {
		t.Fatalf("items default missing from legacy port 0: %#v", legacy.Nodes[0].PortDefaults)
	}
}

func TestMigrateLegacyGraphServiceReturnsDocument(t *testing.T) {
	content := `{"graph_name":"Legacy","nodes":[],"edges":[],"groups":[],"variables":[]}`
	result, err := NewApp().MigrateLegacyGraph(content)
	if err != nil {
		t.Fatal(err)
	}
	var document GraphDocument
	if err := json.Unmarshal([]byte(result), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != GraphSchemaVersion || document.GraphName != "Legacy" {
		t.Fatalf("document = %#v", document)
	}
}

func TestMigrateLegacyGraphPreservesEmptyGraphName(t *testing.T) {
	content := `{"graph_name":"","nodes":[],"edges":[],"groups":[],"variables":[]}`
	document, err := migrateLegacyGraph([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if document.GraphName != "" {
		t.Fatalf("graph name = %q, want empty", document.GraphName)
	}
	data, err := exportLegacyGraph(document)
	if err != nil {
		t.Fatal(err)
	}
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.GraphName != "" {
		t.Fatalf("legacy graph_name = %q, want empty", legacy.GraphName)
	}
}

func TestListWorkspaceFiltersAndSorts(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Graphs"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", ".gocache", "node_modules"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"b.obp", "a.vgf", "ignored.txt", "schema.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	items, err := app.ListWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if !items[0].IsDir || items[0].Name != "Graphs" {
		t.Fatalf("first item = %#v", items[0])
	}
	if items[1].Name != "a.vgf" || items[2].Name != "b.obp" {
		t.Fatalf("unexpected order: %#v", items)
	}
}

func TestFindNodeReferencesScansVgfAndObpFiles(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	legacyDocument := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "legacy",
		Nodes: []GraphNode{
			{ID: "add1", TypeID: "origin.math.add-integer", Position: GraphPosition{X: 1, Y: 2}},
			{ID: "add2", TypeID: "origin.math.add-integer", Position: GraphPosition{X: 3, Y: 4}},
		},
	}
	legacyData, err := exportLegacyGraph(legacyDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy.vgf"), legacyData, 0644); err != nil {
		t.Fatal(err)
	}
	newDocument := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "new",
		Nodes: []GraphNode{
			{ID: "add3", TypeID: "origin.math.add-integer", Position: GraphPosition{X: 5, Y: 6}},
			{ID: "branch", TypeID: "origin.flow.branch", Position: GraphPosition{X: 7, Y: 8}},
		},
	}
	newData, err := json.Marshal(newDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "new.obp"), newData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.json"), newData, 0644); err != nil {
		t.Fatal(err)
	}

	results, err := app.FindNodeReferences(dir, "origin.math.add-integer")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2: %#v", len(results), results)
	}
	if results[0].Name != "legacy.vgf" || results[0].Count != 2 {
		t.Fatalf("first result = %#v", results[0])
	}
	if results[1].Name != "new.obp" || results[1].Count != 1 {
		t.Fatalf("second result = %#v", results[1])
	}
}

func TestRevealInFolderRejectsMissingFile(t *testing.T) {
	app := NewApp()
	err := app.RevealInFolder(filepath.Join(t.TempDir(), "missing.vgf"))
	if err == nil {
		t.Fatal("expected missing file to return an error")
	}
}
