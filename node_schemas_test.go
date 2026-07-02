package main

import (
	"encoding/json"
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

func TestRangeCompareUsesDynamicBranchSchema(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("nodes", "SysFlowControl.json"))
	if err != nil {
		t.Fatal(err)
	}
	var definitions []map[string]interface{}
	if err := json.Unmarshal(data, &definitions); err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition["id"] != "origin.flow.range-compare" {
			continue
		}
		branch, ok := definition["dynamicBranch"].(map[string]interface{})
		if !ok {
			t.Fatal("range compare should use dynamicBranch")
		}
		if branch["controlInput"] != "ranges" || branch["defaultOutput"] != "otherwise" || branch["outputPrefix"] != "case" {
			t.Fatalf("unexpected dynamicBranch: %#v", branch)
		}
		if branch["outputStartIndex"] != float64(1) || branch["maxBranches"] != float64(4) {
			t.Fatalf("unexpected branch indexes: %#v", branch)
		}
		if _, ok := branch["hiddenOutputKeys"]; ok {
			t.Fatalf("dynamicBranch should not require hidden output keys: %#v", branch)
		}
		template, ok := branch["outputTemplate"].(map[string]interface{})
		if !ok || template["type"] != "exec" {
			t.Fatalf("dynamicBranch should declare an exec outputTemplate: %#v", branch)
		}
		outputs, _ := definition["outputs"].([]interface{})
		for _, value := range outputs {
			output, _ := value.(map[string]interface{})
			key, _ := output["key"].(string)
			if strings.HasPrefix(key, "case") {
				t.Fatalf("dynamic branch output %s should be generated, not declared", key)
			}
		}
		return
	}
	t.Fatal("range compare schema not found")
}

func TestNewNodeSchemasOmitOptionalKindAndCustom(t *testing.T) {
	files := []string{
		filepath.Join("nodes", "Base.json"),
		filepath.Join("nodes", "SysFlowControl.json"),
	}
	newNodeIDs := map[string]bool{
		"origin.array.create-integer-new": true,
		"origin.array.create-string-new":  true,
		"origin.flow.equal-switch-new":    true,
		"origin.flow.range-compare":       true,
	}
	found := map[string]bool{}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var definitions []map[string]interface{}
		if err := json.Unmarshal(data, &definitions); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, definition := range definitions {
			id, _ := definition["id"].(string)
			if !newNodeIDs[id] {
				continue
			}
			found[id] = true
			if _, ok := definition["kind"]; ok {
				t.Fatalf("%s should omit optional kind", id)
			}
			if _, ok := definition["custom"]; ok {
				t.Fatalf("%s should omit optional custom", id)
			}
		}
	}

	for id := range newNodeIDs {
		if !found[id] {
			t.Fatalf("new node schema not found: %s", id)
		}
	}
}

func TestNewNodeSchemasUseLegacyPortTypeShape(t *testing.T) {
	files := []string{
		filepath.Join("nodes", "Base.json"),
		filepath.Join("nodes", "SysFlowControl.json"),
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var definitions []map[string]interface{}
		if err := json.Unmarshal(data, &definitions); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for _, definition := range definitions {
			id, _ := definition["id"].(string)
			if id == "" {
				continue
			}
			for _, section := range []string{"inputs", "outputs"} {
				ports, _ := definition[section].([]interface{})
				for _, value := range ports {
					port, _ := value.(map[string]interface{})
					portType, _ := port["type"].(string)
					if portType != "exec" && portType != "data" {
						t.Fatalf("%s %s port %q should use type=data/exec, got %q", id, section, port["key"], portType)
					}
					if portType == "data" {
						if dataType, _ := port["data_type"].(string); dataType == "" {
							t.Fatalf("%s %s data port %q should declare data_type", id, section, port["key"])
						}
					}
				}
			}
		}
	}
}
