package blueprint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGraphDocumentRejectsUnknownExecutionField(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"Typo",
		"nodes":[],
		"connetions":[],
		"variables":[]
	}`)

	_, err := ParseGraphConfigJSON(data)
	if err == nil || !strings.Contains(err.Error(), "connetions") {
		t.Fatalf("ParseGraphConfigJSON error = %v, want unknown field connetions", err)
	}
	var structured *BlueprintError
	if !errors.As(err, &structured) || structured.Stage != BlueprintStageParse {
		t.Fatalf("ParseGraphConfigJSON error = %#v, want parse BlueprintError", err)
	}
}

func TestCompileGraphReturnsStructuredCompileError(t *testing.T) {
	_, err := CompileGraph(testSystemRegistry(t), GraphConfig{Nodes: []NodeConfig{
		{ID: "duplicate", Class: "LiteralInt"},
		{ID: "duplicate", Class: "LiteralInt"},
	}})
	var structured *BlueprintError
	if err == nil || !errors.As(err, &structured) || structured.Stage != BlueprintStageCompile {
		t.Fatalf("CompileGraph error = %#v, want compile BlueprintError", err)
	}
}

func TestParseGraphDocumentAllowsKnownEditorMetadata(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"Metadata",
		"functionCategory":"Category",
		"nodes":[{
			"id":"literal",
			"typeId":"origin.literal.string",
			"position":{"x":1,"y":2},
			"values":{"value":"ok"},
			"properties":{"label":"Literal"}
		}],
		"connections":[],
		"groups":[],
		"variables":[],
		"variableGroups":[],
		"view":{"x":0,"y":0,"zoom":1}
	}`)

	if _, err := ParseGraphConfigJSON(data); err != nil {
		t.Fatalf("ParseGraphConfigJSON rejected editor metadata: %v", err)
	}
}

func TestParseGraphDocumentRejectsUnknownNodeValue(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"DefaultTypo",
		"nodes":[{
			"id":"literal",
			"typeId":"origin.literal.string",
			"values":{"vale":"wrong"}
		}],
		"connections":[],
		"variables":[]
	}`)

	_, err := ParseGraphConfigJSON(data)
	if err == nil || !strings.Contains(err.Error(), "literal") || !strings.Contains(err.Error(), "vale") {
		t.Fatalf("ParseGraphConfigJSON error = %v, want node and unknown value key", err)
	}
}

func TestParseGraphDocumentRejectsDuplicateVariableID(t *testing.T) {
	data := []byte(`{
		"schemaVersion":1,
		"graphName":"Variables",
		"nodes":[],
		"connections":[],
		"variables":[
			{"id":"shared","name":"A","type":"Integer","defaultValue":1},
			{"id":"shared","name":"B","type":"Integer","defaultValue":2}
		]
	}`)

	_, err := ParseGraphConfigJSON(data)
	if err == nil || !strings.Contains(err.Error(), "duplicate variable id") || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("ParseGraphConfigJSON error = %v, want duplicate variable id", err)
	}
}

func TestLoadGraphDirRejectsGraphWithoutEntranceWithSourcePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing-entrance.obp")
	writeTestFile(t, path, `{
		"schemaVersion":1,
		"graphName":"MissingEntrance",
		"nodes":[{"id":"literal","typeId":"origin.literal.string","values":{"value":"ok"}}],
		"connections":[],
		"variables":[]
	}`)

	_, err := loadGraphDir(testSystemRegistry(t), root)
	if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "missing-entrance.obp") || !strings.Contains(err.Error(), "entrance") {
		t.Fatalf("loadGraphDir error = %v, want source path and missing entrance", err)
	}
}

func TestLoadGraphDirAllowsEmptyLegacyPlaceholderGraph(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "empty-placeholder.vgf"), `{
		"graph_name":"",
		"time":"",
		"nodes":[],
		"edges":[],
		"groups":[],
		"variables":[]
	}`)

	graphs, err := loadGraphDir(testSystemRegistry(t), root)
	if err != nil {
		t.Fatalf("loadGraphDir rejected legacy placeholder: %v", err)
	}
	if graphs["empty-placeholder"] == nil {
		t.Fatal("empty legacy placeholder graph was not loaded")
	}
}

func TestLoadGraphDirRejectsFunctionWithoutReturnWithSourcePath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "missing-return.obpf")
	if err := os.WriteFile(path, []byte(`{
		"schemaVersion":1,
		"graphName":"MissingReturn",
		"functionId":"missing-return",
		"nodes":[{"id":"entry","typeId":"origin.function.entry","values":{},"properties":{"functionSignature":{"inputs":[],"outputs":[]}}}],
		"connections":[],
		"variables":[],
		"functionSignature":{"inputs":[],"outputs":[]}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadGraphDir(testSystemRegistry(t), root)
	if err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "missing-return.obpf") || !strings.Contains(err.Error(), "FunctionReturn") {
		t.Fatalf("loadGraphDir error = %v, want source path and missing FunctionReturn", err)
	}
	var structured *BlueprintError
	if !errors.As(err, &structured) || structured.Stage != BlueprintStageCompile || filepath.Clean(structured.SourcePath) != filepath.Clean(path) {
		t.Fatalf("loadGraphDir error = %#v, want compile BlueprintError with source path %s", err, path)
	}
}
