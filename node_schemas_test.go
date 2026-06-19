package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadNodeSchemaDocumentsReturnsRawJSONFiles(t *testing.T) {
	dir := t.TempDir()
	nodesDir := filepath.Join(dir, "nodes")
	nestedDir := filepath.Join(nodesDir, "custom")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}

	rootContent := `[{"name":"AddInt","title":"Add"}]`
	nestedContent := `{"nodes":[{"name":"CustomFoo","title":"Custom Foo"}]}`
	if err := os.WriteFile(filepath.Join(nodesDir, "math.json"), []byte(rootContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "custom.json"), []byte(nestedContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodesDir, "ignore.txt"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	result := loadRuntimeNodeSchemaDocuments([]string{nodesDir})

	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	if len(result.Documents) != 2 {
		t.Fatalf("expected 2 JSON documents, got %d", len(result.Documents))
	}
	if !strings.HasSuffix(result.Documents[0].Path, "custom/custom.json") || result.Documents[0].Content != nestedContent {
		t.Fatalf("unexpected first document: %#v", result.Documents[0])
	}
	if !strings.HasSuffix(result.Documents[1].Path, "math.json") || result.Documents[1].Content != rootContent {
		t.Fatalf("unexpected second document: %#v", result.Documents[1])
	}
}

func TestLoadNodeSchemaDocumentsReturnsEmptyWhenNodesDirectoryIsEmpty(t *testing.T) {
	dir := t.TempDir()
	nodesDir := filepath.Join(dir, "nodes")
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		t.Fatal(err)
	}

	result := loadRuntimeNodeSchemaDocuments([]string{nodesDir})

	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	if len(result.Documents) != 0 {
		t.Fatalf("expected no configured node documents, got %#v", result.Documents)
	}
}

func TestDefaultNodeDirectoryDocumentsLoad(t *testing.T) {
	result := loadRuntimeNodeSchemaDocuments([]string{"nodes"})

	if len(result.Errors) != 0 {
		t.Fatalf("default nodes contain errors: %#v", result.Errors)
	}
	if len(result.Documents) == 0 {
		t.Fatal("default nodes directory should provide node JSON documents")
	}
	foundJSON := false
	for _, document := range result.Documents {
		if strings.HasSuffix(strings.ToLower(document.Path), ".json") && strings.Contains(document.Content, `"name"`) {
			foundJSON = true
			break
		}
	}
	if !foundJSON {
		t.Fatal("default nodes should include JSON node definitions")
	}
}
