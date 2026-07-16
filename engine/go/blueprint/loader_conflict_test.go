package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGraphDirRejectsDuplicateGraphNamesWithBothSources(t *testing.T) {
	root := t.TempDir()
	writeFunctionDocument(t, filepath.Join(root, "a.obpf"), "Shared", "functions/a")
	writeFunctionDocument(t, filepath.Join(root, "b.obpf"), "Shared", "functions/b")

	_, err := loadGraphDir(NewRegistry(), root)
	assertLoaderConflict(t, err, `graph name "Shared"`, "a.obpf", "b.obpf")
}

func TestLoadGraphDirRejectsDuplicateFunctionIDsWithBothSources(t *testing.T) {
	root := t.TempDir()
	writeFunctionDocument(t, filepath.Join(root, "a.obpf"), "FunctionA", "functions/shared")
	writeFunctionDocument(t, filepath.Join(root, "b.obpf"), "FunctionB", "functions/shared")

	_, err := loadGraphDir(NewRegistry(), root)
	assertLoaderConflict(t, err, `function key "functions/shared"`, "a.obpf", "b.obpf")
}

func TestLoadGraphDirRejectsFunctionNameAndPathAliasCollision(t *testing.T) {
	root := t.TempDir()
	functionDir := filepath.Join(root, "functions")
	writeFunctionDocument(t, filepath.Join(root, "a.obpf"), "functions/b.obpf", "functions/a")
	writeFunctionDocument(t, filepath.Join(functionDir, "b.obpf"), "FunctionB", "functions/b")

	_, err := loadGraphDir(NewRegistry(), root)
	assertLoaderConflict(t, err, `function key "functions/b.obpf"`, "a.obpf", filepath.Join("functions", "b.obpf"))
}

func TestLoadGraphDirAllowsAliasesOwnedBySameFunctionFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Same.obpf")
	writeFunctionDocument(t, path, "Same.obpf", "Same.obpf")

	graphs, err := loadGraphDir(NewRegistry(), root)
	if err != nil {
		t.Fatalf("loadGraphDir failed: %v", err)
	}
	if graphs["Same.obpf"] == nil {
		t.Fatal("function graph was not loaded")
	}
}

func writeFunctionDocument(t *testing.T, path string, graphName string, functionID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	writeTestFile(t, path, `{
		"schemaVersion":1,
		"graphName":`+strconvQuote(graphName)+`,
		"functionId":`+strconvQuote(functionID)+`,
		"nodes":[
			{"id":"entry","typeId":"origin.function.entry","values":{},"properties":{"functionSignature":{"inputs":[],"outputs":[]}}},
			{"id":"return","typeId":"origin.function.return","values":{},"properties":{"functionSignature":{"inputs":[],"outputs":[]}}}
		],
		"connections":[{"source":"entry","sourceOutput":"exec","target":"return","targetInput":"exec"}],
		"variables":[],
		"functionSignature":{"inputs":[],"outputs":[]}
	}`)
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func assertLoaderConflict(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("loadGraphDir unexpectedly accepted conflicting graph keys")
	}
	message := filepath.ToSlash(err.Error())
	for _, part := range parts {
		part = filepath.ToSlash(part)
		if !strings.Contains(message, part) {
			t.Fatalf("error %q does not contain %q", message, part)
		}
	}
}
