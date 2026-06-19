package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RuntimeNodeSchemaDocumentLoadResult struct {
	Documents []RuntimeNodeSchemaDocument `json:"documents"`
	Errors    []RuntimeNodeLoadError      `json:"errors"`
}

type RuntimeNodeSchemaDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type RuntimeNodeLoadError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (a *App) LoadNodeSchemaDocuments() RuntimeNodeSchemaDocumentLoadResult {
	return loadRuntimeNodeSchemaDocuments(runtimeNodeDirectories())
}

func runtimeNodeDirectories() []string {
	seen := map[string]bool{}
	var result []string
	add := func(base string) {
		if base == "" {
			return
		}
		path, err := filepath.Abs(filepath.Join(base, "nodes"))
		if err != nil || seen[path] {
			return
		}
		seen[path] = true
		result = append(result, path)
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	if executable, err := os.Executable(); err == nil {
		add(filepath.Dir(executable))
	}
	return result
}

func loadRuntimeNodeSchemaDocuments(directories []string) RuntimeNodeSchemaDocumentLoadResult {
	result := RuntimeNodeSchemaDocumentLoadResult{}
	byPath := map[string]RuntimeNodeSchemaDocument{}

	for _, dir := range directories {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				result.Errors = append(result.Errors, RuntimeNodeLoadError{Path: path, Message: err.Error()})
				return nil
			}
			if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".json" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				result.Errors = append(result.Errors, RuntimeNodeLoadError{Path: path, Message: err.Error()})
				return nil
			}
			key := filepath.ToSlash(path)
			byPath[key] = RuntimeNodeSchemaDocument{Path: key, Content: string(data)}
			return nil
		})
	}

	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		result.Documents = append(result.Documents, byPath[path])
	}
	return result
}
