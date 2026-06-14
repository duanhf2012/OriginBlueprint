package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGraphFileRoundTrip(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.obp")
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
}

func TestListWorkspaceFiltersAndSorts(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Graphs"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.obp", "a.vgf", "ignored.txt"} {
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
