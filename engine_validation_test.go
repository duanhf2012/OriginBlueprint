package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func elementNames(t *testing.T, content string) []string {
	t.Helper()
	var elements []json.RawMessage
	if err := json.Unmarshal([]byte(content), &elements); err != nil {
		t.Fatalf("unmarshal %q: %v", content, err)
	}
	names := make([]string, 0, len(elements))
	for _, element := range elements {
		names = append(names, validationLegacyElementName(element))
	}
	return names
}

func TestDedupeValidationNodeDocumentsKeepsLastOccurrence(t *testing.T) {
	documents := []RuntimeNodeSchemaDocument{
		{Path: "builtin", Content: `[{"name":"GetArrayInt","inputs":[]},{"id":"origin.array.create-integer-new"}]`},
		{Path: "workspace", Content: `[{"name":"GetArrayInt","inputs":[]},{"name":"WorkspaceOnlyNode","inputs":[]}]`},
	}
	result := dedupeValidationNodeDocuments(documents)
	if len(result) != 2 {
		t.Fatalf("result length = %d, want 2", len(result))
	}
	if names := elementNames(t, result[0].Content); len(names) != 1 || names[0] != "CreateIntArray" {
		t.Fatalf("builtin names = %v, want only bridged CreateIntArray", names)
	}
	if names := elementNames(t, result[1].Content); len(names) != 2 || names[0] != "GetArrayInt" || names[1] != "WorkspaceOnlyNode" {
		t.Fatalf("workspace names = %v, want untouched", names)
	}
}

func TestDedupeValidationNodeDocumentsPrefersWorkspaceOverride(t *testing.T) {
	documents := []RuntimeNodeSchemaDocument{
		{Path: "builtin", Content: `[{"name":"GetArrayInt","inputs":[{"port_id":0}]},{"name":"BuiltinOnlyNode","inputs":[]}]`},
		{Path: "workspace", Content: `[{"name":"GetArrayInt","inputs":[{"port_id":9}]}]`},
	}
	result := dedupeValidationNodeDocuments(documents)
	if len(result) != 2 {
		t.Fatalf("result length = %d, want builtin remainder kept", len(result))
	}
	if names := elementNames(t, result[0].Content); len(names) != 1 || names[0] != "BuiltinOnlyNode" {
		t.Fatalf("builtin names = %v, want only BuiltinOnlyNode remainder", names)
	}
	if !strings.Contains(result[1].Content, `"port_id":9`) {
		t.Fatalf("workspace override lost: %s", result[1].Content)
	}
}

func TestDedupeValidationNodeDocumentsDropsBridgedSchemaIDBeforeExplicitName(t *testing.T) {
	documents := []RuntimeNodeSchemaDocument{
		{Path: "builtin", Content: `[{"id":"origin.flow.range-compare"}]`},
		{Path: "workspace", Content: `[{"name":"RangeCompare","inputs":[]}]`},
	}
	result := dedupeValidationNodeDocuments(documents)
	if len(result) != 1 {
		t.Fatalf("result length = %d, want bridged builtin dropped", len(result))
	}
	if names := elementNames(t, result[0].Content); len(names) != 1 || names[0] != "RangeCompare" {
		t.Fatalf("workspace names = %v, want explicit RangeCompare only", names)
	}
}

func TestDedupeValidationNodeDocumentsKeepsIntraDocumentDuplicates(t *testing.T) {
	documents := []RuntimeNodeSchemaDocument{
		{Path: "builtin", Content: `[{"name":"GetArrayInt"},{"name":"GetArrayInt"}]`},
	}
	result := dedupeValidationNodeDocuments(documents)
	if len(result) != 1 || result[0].Content != documents[0].Content {
		t.Fatalf("intra-document duplicates must stay for the engine to report, got %d docs %q", len(result), result[0].Content)
	}
}

func TestDedupeValidationNodeDocumentsPassesNonArrayDocumentsThrough(t *testing.T) {
	documents := []RuntimeNodeSchemaDocument{
		{Path: "wrapped", Content: `{"nodes":[{"name":"GetArrayInt"}]}`},
		{Path: "workspace", Content: `[{"name":"GetArrayInt"}]`},
	}
	result := dedupeValidationNodeDocuments(documents)
	if len(result) != 2 || result[0].Content != documents[0].Content {
		t.Fatalf("wrapped document must stay unchanged, got %#v", result)
	}
}

func TestValidateGraphForWorkspaceToleratesWorkspaceCopyOfBuiltinNodes(t *testing.T) {
	builtin, err := os.ReadFile(filepath.Join("nodes", "Base.json"))
	if err != nil {
		t.Skipf("builtin Base.json unavailable: %v", err)
	}
	workspace := t.TempDir()
	nodesDir := filepath.Join(workspace, "nodes", "common")
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Workspaces ship the built-in node library under nodes/common; validation
	// must not report the duplicate registrations as engine.compile errors.
	if err := os.WriteFile(filepath.Join(nodesDir, "Base.json"), builtin, 0644); err != nil {
		t.Fatal(err)
	}
	document := GraphDocument{
		SchemaVersion:  GraphSchemaVersion,
		GraphName:      "builtin-copy",
		Nodes:          []GraphNode{{ID: "entry", TypeID: "origin.event.entry-two-integers"}},
		Connections:    []GraphConnection{},
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
		t.Fatalf("ValidateGraphForWorkspace returned transport error: %v", err)
	}
	for _, issue := range issues {
		if strings.HasPrefix(issue.Code, "engine.") && issue.Severity == "error" {
			t.Fatalf("workspace copy of builtin nodes should compile: %#v", issues)
		}
	}
}

func TestValidateGraphForWorkspaceSkipsFallbackForBridgedSchemaIDs(t *testing.T) {
	workspace := t.TempDir()
	nodesDir := filepath.Join(workspace, "nodes")
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		t.Fatal(err)
	}
	schema := `[{"id":"origin.flow.range-compare","title":"区间比较 [新]"}]`
	if err := os.WriteFile(filepath.Join(nodesDir, "flow.json"), []byte(schema), 0644); err != nil {
		t.Fatal(err)
	}
	// The legacy RangeCompare class is only provided through the id-only
	// schema bridge; a graph fallback definition for it would collide with the
	// bridged registration.
	document := GraphDocument{
		SchemaVersion: GraphSchemaVersion,
		GraphName:     "bridged-range-compare",
		Nodes: []GraphNode{
			{ID: "entry", TypeID: "origin.event.entry-two-integers"},
			{ID: "compare", TypeID: "origin.custom.range-compare", Properties: GraphNodeProperties{
				LegacyClass:   "RangeCompare",
				LegacyInputs:  []GraphLegacyPort{{Key: "in0", Type: "exec"}},
				LegacyOutputs: []GraphLegacyPort{{Key: "out0", Type: "exec"}},
			}},
		},
		Connections:    []GraphConnection{{Source: "entry", SourceOutput: "exec", Target: "compare", TargetInput: "in0"}},
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
		t.Fatalf("ValidateGraphForWorkspace returned transport error: %v", err)
	}
	for _, issue := range issues {
		if strings.HasPrefix(issue.Code, "engine.") && issue.Severity == "error" {
			t.Fatalf("bridged schema id must not receive a colliding fallback: %#v", issues)
		}
	}
}
