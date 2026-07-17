package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestProjectSettingsWritesUseAtomicBoundary(t *testing.T) {
	root := t.TempDir()
	var writes []string
	app := NewApp()
	app.atomicWrite = func(path string, data []byte, mode fs.FileMode) error {
		writes = append(writes, fmt.Sprintf("%s:%s:%o", path, data, mode))
		return nil
	}
	loaded, err := app.LoadProjectSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != defaultProjectSettingsContent || len(writes) != 1 {
		t.Fatalf("loaded = %#v, writes = %#v", loaded, writes)
	}
	if _, err := app.SaveProjectSettings(root, `{"version":1}`); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 || !strings.Contains(writes[1], `{"version":1}`) {
		t.Fatalf("writes = %#v", writes)
	}
}

func TestWriteAppConfigReturnsAtomicWriteFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", path)
	if err := writeAppConfig(appConfig{RecentFiles: []string{"graph.obp"}}); err == nil {
		t.Fatal("writeAppConfig should return the atomic replacement failure")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("failed config write replaced the existing target: info=%#v err=%v", info, err)
	}
}

func TestForceSaveGraphCreatesBackupBeforeReplacingSource(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "recover.obpf")
	original := []byte(`{"schemaVersion":1,"graphName":"Original","nodes":[],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1}}`)
	updated := `{"schemaVersion":1,"graphName":"Updated","nodes":[],"connections":[],"groups":[],"variables":[],"variableGroups":[],"view":{"x":0,"y":0,"zoom":1}}`
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	saved, err := app.ForceSaveGraph(path, updated)
	if err != nil {
		t.Fatal(err)
	}
	if saved != path {
		t.Fatalf("saved path = %q, want %q", saved, path)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatalf("backup = %q, want original %q", backup, original)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != updated {
		t.Fatalf("current = %q, want updated %q", current, updated)
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

func TestGraphContentRejectsNativeTimerDocumentAtVGFPath(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Timer",
		Nodes: []GraphNode{{
			ID:         "timer",
			TypeID:     "origin.timer.set-by-function",
			Properties: GraphNodeProperties{FunctionID: "callback"},
		}},
	}
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graphContentForPath("timer.vgf", string(content)); err == nil {
		t.Fatal("native timer document was accepted at a legacy .vgf path")
	}
}

func TestSaveGraphAddsOBPExtensionForNativeTimerDocument(t *testing.T) {
	t.Setenv("ORIGIN_BLUEPRINT_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	app := NewApp()
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Timer",
		Nodes: []GraphNode{{
			ID:         "timer",
			TypeID:     "origin.timer.set-by-function",
			Properties: GraphNodeProperties{FunctionID: "callback"},
		}},
	}
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "timer")
	path, err := app.SaveGraph(base, string(content))
	if err != nil {
		t.Fatal(err)
	}
	if path != base+".obp" {
		t.Fatalf("saved path = %q, want %q", path, base+".obp")
	}
}

func TestLegacyVGFMigrationPreservesKnownAndUnknownContent(t *testing.T) {
	legacy := legacyGraph{
		GraphName: "Compat Sample",
		Time:      "2026-07-04T00:00:00Z",
		Nodes: []legacyNode{
			{ID: "begin", Class: "BeginNode", Module: "legacy", Position: []float64{10, 20}},
			{ID: "add", Class: "AddInt", Module: "legacy", Position: []float64{220, 20}, PortDefaults: map[string]interface{}{"0": float64(1), "1": float64(2)}},
			{ID: "unknown", Class: "CustomLegacyNode", Module: "legacy.custom", Position: []float64{430, 20}, PortDefaults: map[string]interface{}{"0": "keep me"}},
		},
		Edges: []legacyEdge{
			{EdgeID: "exec-edge", SourceNodeID: "begin", SourceIndex: 0, TargetNodeID: "unknown", TargetIndex: 0},
			{EdgeID: "data-edge", SourceNodeID: "add", SourceIndex: 0, TargetNodeID: "unknown", TargetIndex: 1},
		},
		Groups: []legacyGroup{{Title: "Legacy Group", Nodes: []string{"begin", "add", "unknown"}}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	document, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != GraphSchemaVersion || document.GraphName != "Compat Sample" {
		t.Fatalf("document header = %#v", document)
	}
	if len(document.Nodes) == 0 {
		t.Fatalf("known runtime nodes were not migrated: %#v", document)
	}
	if document.Legacy == nil || len(document.Legacy.HiddenNodes) != 1 {
		t.Fatalf("unknown legacy node was not preserved: %#v", document.Legacy)
	}
	if document.Legacy.HiddenNodes[0].Class != "CustomLegacyNode" {
		t.Fatalf("hidden node class = %q", document.Legacy.HiddenNodes[0].Class)
	}
	if len(document.Legacy.HiddenEdges) != 2 {
		t.Fatalf("legacy edges were not preserved for unknown endpoints: %#v", document.Legacy.HiddenEdges)
	}
	if len(document.Legacy.Groups) != 1 || document.Legacy.Groups[0].Title != "Legacy Group" {
		t.Fatalf("legacy group was not preserved: %#v", document.Legacy.Groups)
	}
}

func TestSampleProjectBlueprintDocumentsParse(t *testing.T) {
	paths := []string{
		filepath.Join("examples", "sample-project", "blueprints", "getting-started.obp"),
		filepath.Join("examples", "sample-project", "functions", "calculate-damage.obpf"),
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document GraphDocument
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			if document.SchemaVersion != GraphSchemaVersion || document.GraphName == "" {
				t.Fatalf("invalid sample document header: %#v", document)
			}
			if issues := validateGraph(document); len(issues) != 0 {
				t.Fatalf("sample document should validate cleanly: %#v", issues)
			}
		})
	}
}

func TestGraphContentForLegacyPathRejectsFunctionNodes(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Function Calls",
		Nodes: []GraphNode{{
			ID:       "call",
			TypeID:   "origin.function.call",
			Position: GraphPosition{X: 12, Y: 34},
			Properties: GraphNodeProperties{
				Label:        "CalculateDamage",
				FunctionRole: "call",
				FunctionID:   "fn_calculate",
				FunctionName: "CalculateDamage",
				FunctionSignature: GraphFunctionSignature{
					Inputs:  []GraphFunctionSignaturePort{{ID: "target", Name: "Target", Type: "integer"}},
					Outputs: []GraphFunctionSignaturePort{{ID: "damage", Name: "Damage", Type: "integer"}},
				},
			},
		}},
	}
	content, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graphContentForPath("choiceskill_dead.vgf", string(content)); err == nil {
		t.Fatal("function graph was accepted at a legacy .vgf path")
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

func TestProjectSettingsRoundTrip(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	content := `{"version":1,"appearance":{"locale":"zh-CN"},"layout":{"panels":{"files":240}}}`

	saved, err := app.SaveProjectSettings(dir, content)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "originblueprint.project")
	if saved != wantPath {
		t.Fatalf("project settings path = %q, want %q", saved, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("project settings content = %s, want %s", data, content)
	}
	loaded, err := app.LoadProjectSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Path != wantPath || loaded.Content != content {
		t.Fatalf("loaded project settings = %#v", loaded)
	}
}

func TestLoadProjectSettingsCreatesDefaultWhenMissing(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	loaded, err := app.LoadProjectSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Path != filepath.Join(dir, "originblueprint.project") {
		t.Fatalf("project settings path = %q", loaded.Path)
	}
	if !strings.Contains(loaded.Content, `"version": 1`) || !strings.Contains(loaded.Content, `"appearance"`) {
		t.Fatalf("default project settings content = %s", loaded.Content)
	}
	if _, err := os.Stat(loaded.Path); err != nil {
		t.Fatalf("default project settings was not created: %v", err)
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
	legacyInputs := make([]GraphLegacyPort, 15)
	for index := range legacyInputs {
		legacyInputs[index] = GraphLegacyPort{Key: fmt.Sprintf("in%d", index), Label: fmt.Sprintf("in%d", index), Type: "any"}
	}
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Runtime JSON",
		Nodes: []GraphNode{{
			ID:       "hit",
			TypeID:   "origin.custom.do-hit-effect",
			Position: GraphPosition{X: 11, Y: 22},
			Values:   map[string]interface{}{"in14": 99},
			Properties: GraphNodeProperties{
				LegacyClass:  "DoHitEffect",
				LegacyModule: "tools.json_node_loader",
				LegacyInputs: legacyInputs,
			},
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

func TestValidateGraphReportsTimerWithoutFunction(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "timer", TypeID: "origin.timer.set-by-function"},
		},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "timer.function-missing", "timer") {
		t.Fatalf("missing timer function issue not reported: %#v", issues)
	}
}

func TestValidateGraphReportsUnreachableFlowNodesFromEntries(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.entry-two-integers"},
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
			{ID: "add", TypeID: "origin.math.add-integer", Values: map[string]interface{}{"a": 1, "b": 2}},
		},
		Connections: []GraphConnection{
			{Source: "entry", SourceOutput: "exec", Target: "debug", TargetInput: "exec"},
			{Source: "add", SourceOutput: "result", Target: "debug", TargetInput: "integer"},
		},
	}
	issues := validateGraph(document)
	if hasIssue(issues, "flow.unreachable-node", "add") {
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

func TestValidateGraphLegacyPlaceholderDoesNotSuppressKnownFlowIssues(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.begin"},
			{ID: "legacy", TypeID: "origin.legacy.placeholder", Properties: GraphNodeProperties{LegacyClass: "FutureNode"}},
			{ID: "unreachable", TypeID: "origin.action.print"},
		},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "flow.unreachable-node", "unreachable") {
		t.Fatalf("issues = %#v, want known unreachable node even with a legacy placeholder", issues)
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

func requireValidationIssue(t *testing.T, issues []ValidationIssue, code string) ValidationIssue {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return issue
		}
	}
	t.Fatalf("issues = %#v, want code %q", issues, code)
	return ValidationIssue{}
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

func TestCoreIssueBlocksSaveUsesExplicitLanguageNeutralCodes(t *testing.T) {
	blocking := []string{
		"schema.unsupported",
		"node.missing-id",
		"node.duplicate-id",
		"connection.dangling",
		"connection.missing-port",
		"connection.type-mismatch",
		"connection.multiple-producers",
		"flow.exec-fanout",
		"flow.data-cycle",
		"flow.exec-cycle",
	}
	for _, code := range blocking {
		if !coreIssueBlocksSave(code) {
			t.Errorf("%s should block save", code)
		}
	}
	nonBlocking := []string{
		"flow.unreachable-node",
		"flow.missing-entry",
		"flow.possible-cycle",
		"node.legacy-placeholder",
		"engine.compile",
	}
	for _, code := range nonBlocking {
		if coreIssueBlocksSave(code) {
			t.Errorf("%s should not block save", code)
		}
	}
}

func TestValidateGraphMarksStructuralCoreErrorsAsSaveBlocking(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{
			{ID: "duplicate", TypeID: "origin.literal.string"},
			{ID: "duplicate", TypeID: "origin.literal.string"},
		},
	}
	issue := requireValidationIssue(t, validateGraph(document), "node.duplicate-id")
	if !issue.BlocksSave {
		t.Fatalf("issue = %#v, want BlocksSave", issue)
	}
}

func TestValidateGraphForWorkspaceUsesProductionCompilerRules(t *testing.T) {
	tests := []struct {
		name        string
		nodes       []GraphNode
		connections []GraphConnection
	}{
		{
			name: "duplicate data producer",
			nodes: []GraphNode{
				{ID: "left", TypeID: "origin.literal.string"},
				{ID: "right", TypeID: "origin.literal.string"},
				{ID: "target", TypeID: "origin.action.print"},
			},
			connections: []GraphConnection{
				{Source: "left", SourceOutput: "value", Target: "target", TargetInput: "value"},
				{Source: "right", SourceOutput: "value", Target: "target", TargetInput: "value"},
			},
		},
		{
			name: "native exec fanout",
			nodes: []GraphNode{
				{ID: "entry", TypeID: "origin.event.begin"},
				{ID: "left", TypeID: "origin.action.print"},
				{ID: "right", TypeID: "origin.action.print"},
			},
			connections: []GraphConnection{
				{Source: "entry", SourceOutput: "exec", Target: "left", TargetInput: "exec"},
				{Source: "entry", SourceOutput: "exec", Target: "right", TargetInput: "exec"},
			},
		},
		{
			name: "data dependency cycle",
			nodes: []GraphNode{
				{ID: "left", TypeID: "origin.math.add-integer"},
				{ID: "right", TypeID: "origin.math.add-integer"},
			},
			connections: []GraphConnection{
				{Source: "left", SourceOutput: "result", Target: "right", TargetInput: "a"},
				{Source: "right", SourceOutput: "result", Target: "left", TargetInput: "a"},
			},
		},
		{
			name: "duplicate entrance id",
			nodes: []GraphNode{
				{ID: "first", TypeID: "origin.event.entry-two-integers"},
				{ID: "second", TypeID: "origin.event.entry-two-integers"},
			},
		},
		{
			name:  "unknown executable node",
			nodes: []GraphNode{{ID: "future", TypeID: "origin.future.node"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := GraphDocument{
				SchemaVersion:  GraphSchemaVersion,
				GraphName:      "validation-test",
				Nodes:          test.nodes,
				Connections:    test.connections,
				Groups:         []GraphGroup{},
				Variables:      []GraphVariable{},
				VariableGroups: []GraphVariableGroup{{ID: "default", Name: "Default"}},
				View:           GraphView{Zoom: 1},
			}
			data, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			issues, err := NewApp().ValidateGraphForWorkspace(string(data), "", "")
			if err != nil {
				t.Fatal(err)
			}
			if !hasIssue(issues, "engine.compile", "") && !hasIssue(issues, "engine.parse", "") && !hasIssue(issues, "engine.definition", "") {
				t.Fatalf("issues = %#v, want a production engine error", issues)
			}
		})
	}
}

func TestValidationAbsolutePathKeepsEmptyInputEmpty(t *testing.T) {
	if got := validationAbsolutePath("  "); got != "" {
		t.Fatalf("validationAbsolutePath(empty) = %q, want empty", got)
	}
}

func TestValidateGraphForWorkspaceReturnsDecodeIssueForRecoverableSource(t *testing.T) {
	issues, err := NewApp().ValidateGraphForWorkspace(`{"schemaVersion":1,"nodes":"invalid"}`, "", "broken.obp")
	if err != nil {
		t.Fatalf("ValidateGraphForWorkspace returned transport error: %v", err)
	}
	if !hasIssue(issues, "document.decode", "") {
		t.Fatalf("issues = %#v, want document.decode", issues)
	}
}

func TestValidateGraphForWorkspaceUsesWorkspaceFunctionSignatures(t *testing.T) {
	workspace := t.TempDir()
	functionDir := filepath.Join(workspace, "functions")
	if err := os.MkdirAll(functionDir, 0755); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("examples", "sample-project", "functions", "calculate-damage.obpf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(functionDir, "calculate-damage.obpf"), fixture, 0644); err != nil {
		t.Fatal(err)
	}
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "function-caller",
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.entry-two-integers"},
			{ID: "call", TypeID: "origin.function.call", Properties: GraphNodeProperties{
				FunctionID:   "fn_calculate_damage",
				FunctionName: "CalculateDamage",
				FunctionSignature: GraphFunctionSignature{
					Inputs:  []GraphFunctionSignaturePort{{ID: "base", Name: "BaseDamage", Type: "string"}},
					Outputs: []GraphFunctionSignaturePort{{ID: "damage", Name: "Damage", Type: "integer"}},
				},
			}},
		},
		Connections:    []GraphConnection{{Source: "entry", SourceOutput: "exec", Target: "call", TargetInput: "exec"}},
		Groups:         []GraphGroup{},
		Variables:      []GraphVariable{},
		VariableGroups: []GraphVariableGroup{{ID: "default", Name: "Default"}},
		View:           GraphView{Zoom: 1},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := NewApp().ValidateGraphForWorkspace(string(data), workspace, filepath.Join(workspace, "main.obp"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(issues, "engine.compile", "call") {
		t.Fatalf("issues = %#v, want function signature compiler error on call", issues)
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

func TestValidateGraphRejectsUnsafeDynamicSequenceCounts(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
	}{
		{name: "negative", count: -1},
		{name: "above engine limit", count: 257},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := GraphDocument{
				SchemaVersion: GraphSchemaVersion,
				Nodes: []GraphNode{{
					ID:         "sequence",
					TypeID:     "origin.flow.sequence",
					Properties: GraphNodeProperties{DynamicOutputCount: test.count},
				}},
			}
			issues := validateGraph(document)
			if !hasIssue(issues, "node.dynamic-output-count", "sequence") {
				t.Fatalf("issues = %#v, want dynamic output count error", issues)
			}
		})
	}
}

func TestValidateGraphRejectsOversizedFunctionSignature(t *testing.T) {
	inputs := make([]GraphFunctionSignaturePort, 129)
	for index := range inputs {
		inputs[index] = GraphFunctionSignaturePort{ID: fmt.Sprintf("input-%d", index), Name: fmt.Sprintf("Input %d", index), Type: "integer"}
	}
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{{
			ID:     "call",
			TypeID: "origin.function.call",
			Properties: GraphNodeProperties{FunctionSignature: GraphFunctionSignature{
				Inputs: inputs,
			}},
		}},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "function.signature-limit", "call") {
		t.Fatalf("issues = %#v, want function signature limit error", issues)
	}
}

func TestValidateGraphRejectsOversizedLegacyPortList(t *testing.T) {
	inputs := make([]GraphLegacyPort, 4097)
	for index := range inputs {
		inputs[index] = GraphLegacyPort{Key: fmt.Sprintf("input-%d", index), Type: "any"}
	}
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		Nodes: []GraphNode{{
			ID:     "legacy",
			TypeID: "origin.legacy.placeholder",
			Properties: GraphNodeProperties{
				LegacyClass:  "HugeLegacyNode",
				LegacyInputs: inputs,
			},
		}},
	}
	issues := validateGraph(document)
	if !hasIssue(issues, "node.port-limit", "legacy") {
		t.Fatalf("issues = %#v, want legacy port limit error", issues)
	}
}

func TestValidateGraphHandlesDeepExecutionFlowIteratively(t *testing.T) {
	const depth = 20000
	nodes := make([]GraphNode, 0, depth+1)
	connections := make([]GraphConnection, 0, depth)
	nodes = append(nodes, GraphNode{ID: "entry", TypeID: "origin.event.begin"})
	previous := "entry"
	previousOutput := "exec"
	for index := 0; index < depth; index++ {
		id := fmt.Sprintf("sequence-%d", index)
		nodes = append(nodes, GraphNode{ID: id, TypeID: "origin.flow.sequence", Properties: GraphNodeProperties{DynamicOutputCount: 1}})
		connections = append(connections, GraphConnection{Source: previous, SourceOutput: previousOutput, Target: id, TargetInput: "exec"})
		previous = id
		previousOutput = "then0"
	}
	document := GraphDocument{SchemaVersion: GraphSchemaVersion, Nodes: nodes, Connections: connections}
	if issues := validateGraph(document); hasValidationErrors(issues) {
		t.Fatalf("deep acyclic flow should validate without errors, got first issues: %#v", issues[:min(5, len(issues))])
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

func TestMigrateLegacyHidesRemovedFileTableAndDictionaryNodes(t *testing.T) {
	content := `{"graph_name":"Data","nodes":[{"id":"file","class":"FileNode","port_defaultv":{"0":"data.csv"}},{"id":"read","class":"TableReader","port_defaultv":{"2":",","3":true}},{"id":"preview","class":"PreviewTable","port_defaultv":{}},{"id":"keys","class":"Keys (Dict)","port_defaultv":{}}],"edges":[{"source_node_id":"file","source_port_index":0,"des_node_id":"read","des_port_index":1},{"source_node_id":"read","source_port_index":1,"des_node_id":"preview","des_port_index":0}],"groups":[],"variables":[{"name":"Table","type":"DataFrame","value":{"columns":[],"rows":[]}},{"name":"Lookup","type":"Dict","value":{}}]}`
	document, err := migrateLegacyGraph([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 0 {
		t.Fatalf("removed nodes should be hidden, got %#v", document.Nodes)
	}
	if document.Legacy == nil || len(document.Legacy.HiddenNodes) != 4 || len(document.Legacy.HiddenEdges) != 2 {
		t.Fatalf("legacy state = %#v", document.Legacy)
	}
	if document.Variables[0].Type != "string" || document.Variables[1].Type != "string" {
		t.Fatalf("variables = %#v", document.Variables)
	}
	if issues := validateGraph(document); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestMigrateLegacyShowsRuntimeFallbackNodesButPreservesUnknownForRoundTrip(t *testing.T) {
	content := `{"graph_name":"Legacy","nodes":[{"id":"targets","class":"RuntimeOnlyMissingNode","module":"tools.json_node_loader","pos":[1,2],"port_defaultv":{"2":true}},{"id":"hidden","class":"UnknownSource","module":"old","pos":[5,6],"port_defaultv":{"0":"x"}},{"id":"loop","class":"ForeachIntArray","module":"old","pos":[9,10],"port_defaultv":{}}],"edges":[{"edge_id":"known","source_node_id":"targets","source_port_index":1,"des_node_id":"loop","des_port_index":1},{"edge_id":"hidden-edge","source_node_id":"hidden","source_port_index":2,"des_node_id":"loop","des_port_index":0}],"groups":[],"variables":[]}`
	document, err := migrateLegacyGraph([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 2 || len(document.Connections) != 1 {
		t.Fatalf("document = %#v", document)
	}
	foundFallback := false
	for _, node := range document.Nodes {
		if node.Properties.LegacyClass == "UnknownSource" {
			t.Fatalf("unknown node should be hidden, got %#v", node)
		}
		if node.Properties.LegacyClass == "RuntimeOnlyMissingNode" && node.TypeID == "origin.legacy.placeholder" {
			foundFallback = true
		}
	}
	if !foundFallback {
		t.Fatalf("runtime fallback node should be visible, nodes=%#v", document.Nodes)
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
		var native GraphDocument
		if err := json.Unmarshal(data, &native); err == nil && native.SchemaVersion == GraphSchemaVersion {
			for _, node := range native.Nodes {
				if strings.TrimSpace(node.TypeID) == "" {
					t.Fatalf("%s contains native node %q without a type id", path, node.ID)
				}
			}
			continue
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

func monsterChoiceRuntimeSpecs(t *testing.T) map[string]runtimeLegacySpec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "runtime_nodes", "monster_choices.json"))
	if err != nil {
		t.Fatal(err)
	}
	return runtimeLegacyNodeSpecsFromDocuments([]RuntimeNodeSchemaDocument{{Path: "testdata/runtime_nodes/monster_choices.json", Content: string(data)}})
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
			if issue.Severity == "error" && issue.Code != "flow.unreachable-node" {
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
	document, err := migrateLegacyGraphWithRuntimeSpecs(data, monsterChoiceRuntimeSpecs(t))
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
	document, err := migrateLegacyGraphWithRuntimeSpecs(data, monsterChoiceRuntimeSpecs(t))
	if err != nil {
		t.Fatal(err)
	}
	issues := validateGraph(document)
	if hasIssue(issues, "flow.missing-entry", "") {
		t.Fatalf("monster choice skill entry should be recognized: %#v", issues)
	}
	if !hasIssue(issues, "flow.unreachable-node", "") {
		t.Fatalf("detached legacy node should still be reported: %#v", issues)
	}
}

func TestChoiceskillEasyUsesRuntimeJsonTitlesInsteadOfFallbackNames(t *testing.T) {
	path := filepath.Join("build", "bin", "vgf", "monsterChoiceskill", "choiceskill_easy.vgf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("choiceskill_easy.vgf sample not available")
	}
	document, err := migrateLegacyGraphWithRuntimeSpecs(data, monsterChoiceRuntimeSpecs(t))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, node := range document.Nodes {
		switch node.Properties.LegacyClass {
		case "Entrance_MonsterChoiceSkill_40300", "GetObjectInfo", "GetSkillByType", "AppendAiChoiceSkillAndTarget":
			found[node.Properties.LegacyClass] = true
			if node.TypeID == "origin.legacy.placeholder" {
				t.Fatalf("%s should use restored runtime JSON schema, got placeholder", node.Properties.LegacyClass)
			}
			if node.Properties.Label == node.Properties.LegacyClass {
				t.Fatalf("%s should not persist fallback class name as display label", node.Properties.LegacyClass)
			}
		}
	}
	for _, class := range []string{"Entrance_MonsterChoiceSkill_40300", "GetObjectInfo", "GetSkillByType", "AppendAiChoiceSkillAndTarget"} {
		if !found[class] {
			t.Fatalf("expected %s in migrated choiceskill_easy graph", class)
		}
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
				Values:   map[string]interface{}{"cases": []interface{}{1, 2, 3, 4, 5, 6}},
			},
			{
				ID:       "target",
				TypeID:   "origin.result.append-integer",
				Position: GraphPosition{X: 200, Y: 20},
			},
		},
		Connections: []GraphConnection{{
			Source: "switch", SourceOutput: "case6", Target: "target", TargetInput: "exec",
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
	if len(legacy.Edges) != 1 || legacyPortIndex(legacy.Edges[0].SourcePortID, legacy.Edges[0].SourceIndex) != 7 {
		t.Fatalf("legacy edges = %#v", legacy.Edges)
	}
	roundTrip, err := migrateLegacyGraph(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(roundTrip.Nodes) != 2 || roundTrip.Nodes[0].TypeID != "origin.flow.equal-switch-new" {
		t.Fatalf("round-trip nodes = %#v", roundTrip.Nodes)
	}
	if len(roundTrip.Connections) != 1 || roundTrip.Connections[0].SourceOutput != "case6" {
		t.Fatalf("round-trip connections = %#v", roundTrip.Connections)
	}
}

func TestValidateDynamicBranchOutputsUseGeneratedCaseKeys(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "DynamicSwitch",
		Nodes: []GraphNode{
			{
				ID:     "entry",
				TypeID: "origin.event.begin",
			},
			{
				ID:     "switch",
				TypeID: "origin.flow.equal-switch-new",
				Values: map[string]interface{}{"cases": []interface{}{1, 2, 3, 4, 5, 6}},
			},
			{
				ID:     "target",
				TypeID: "origin.result.append-integer",
			},
		},
		Connections: []GraphConnection{
			{Source: "entry", SourceOutput: "exec", Target: "switch", TargetInput: "exec"},
			{Source: "switch", SourceOutput: "case6", Target: "target", TargetInput: "exec"},
		},
	}

	issues := validateGraph(document)
	if hasIssue(issues, "connection.missing-port", "target") {
		t.Fatalf("generated dynamic branch output case6 should validate: %#v", issues)
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

func TestExportLegacyGraphUsesEditedVisibleGroupTitle(t *testing.T) {
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "Groups",
		Nodes: []GraphNode{
			{ID: "visible", TypeID: "origin.math.add-integer", Position: GraphPosition{X: 10, Y: 20}},
		},
		Groups: []GraphGroup{{
			ID:      "group-1",
			Title:   "Edited Group",
			NodeIDs: []string{"visible"},
		}},
		Legacy: &GraphLegacyState{
			Format: "vgf",
			Groups: []legacyGroup{{
				Title: "Original Group",
				Nodes: []string{"visible"},
			}},
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
	if len(legacy.Groups) != 1 {
		t.Fatalf("groups = %#v", legacy.Groups)
	}
	if legacy.Groups[0].Title != "Edited Group" {
		t.Fatalf("group title = %q, want edited title", legacy.Groups[0].Title)
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

func TestFindNodeReferencesMatchesFunctionCalls(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "functions"), 0755); err != nil {
		t.Fatal(err)
	}
	writeDocument := func(path string, document GraphDocument) {
		t.Helper()
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	callProperties := GraphNodeProperties{FunctionID: "functions/Calc.obpf", FunctionName: "Calc"}
	writeDocument(filepath.Join(dir, "main.obp"), GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "main",
		Nodes: []GraphNode{
			{ID: "call1", TypeID: "origin.function.call", Properties: callProperties},
			{ID: "timer1", TypeID: "origin.timer.set-by-function", Properties: callProperties},
			{ID: "other", TypeID: "origin.function.call", Properties: GraphNodeProperties{FunctionID: "functions/Other.obpf", FunctionName: "Other"}},
		},
	})
	writeDocument(filepath.Join(dir, "functions", "worker.obpf"), GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "worker",
		Nodes: []GraphNode{
			{ID: "call2", TypeID: "origin.function.call", Properties: callProperties},
		},
	})

	results, err := app.FindNodeReferences(dir, "function:functions/Calc.obpf")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2: %#v", len(results), results)
	}
	if results[0].Name != "worker.obpf" || results[0].Count != 1 {
		t.Fatalf("first result = %#v", results[0])
	}
	if results[1].Name != "main.obp" || results[1].Count != 2 {
		t.Fatalf("second result = %#v", results[1])
	}

	results, err = app.FindNodeReferences(dir, "function:Calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("function name fallback len(results) = %d, want 2: %#v", len(results), results)
	}
}

func TestRevealInFolderRejectsMissingFile(t *testing.T) {
	app := NewApp()
	err := app.RevealInFolder(filepath.Join(t.TempDir(), "missing.vgf"))
	if err == nil {
		t.Fatal("expected missing file to return an error")
	}
}
