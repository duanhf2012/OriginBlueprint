package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed nodes
var embeddedNodeFiles embed.FS

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
	return loadRuntimeNodeSchemaDocumentsWithEmbedded(runtimeNodeDirectories())
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
	if executable, err := os.Executable(); err == nil {
		add(filepath.Dir(executable))
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	return result
}

func loadRuntimeNodeSchemaDocuments(directories []string) RuntimeNodeSchemaDocumentLoadResult {
	return loadRuntimeNodeSchemaDocumentsFromSources(false, directories)
}

func loadRuntimeNodeSchemaDocumentsWithEmbedded(directories []string) RuntimeNodeSchemaDocumentLoadResult {
	return loadRuntimeNodeSchemaDocumentsFromSources(true, directories)
}

func loadRuntimeNodeSchemaDocumentsFromSources(includeEmbedded bool, directories []string) RuntimeNodeSchemaDocumentLoadResult {
	result := RuntimeNodeSchemaDocumentLoadResult{}
	byPath := map[string]RuntimeNodeSchemaDocument{}

	if includeEmbedded {
		loadEmbeddedNodeSchemaDocuments(&result, byPath)
	}
	for _, dir := range directories {
		loadDirectoryNodeSchemaDocuments(dir, &result, byPath)
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

func loadEmbeddedNodeSchemaDocuments(result *RuntimeNodeSchemaDocumentLoadResult, byPath map[string]RuntimeNodeSchemaDocument) {
	_ = fs.WalkDir(embeddedNodeFiles, "nodes", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, RuntimeNodeLoadError{Path: path, Message: err.Error()})
			return nil
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".json" {
			return nil
		}
		data, err := embeddedNodeFiles.ReadFile(path)
		if err != nil {
			result.Errors = append(result.Errors, RuntimeNodeLoadError{Path: path, Message: err.Error()})
			return nil
		}
		key := filepath.ToSlash(path)
		byPath[key] = RuntimeNodeSchemaDocument{Path: "embedded:" + key, Content: string(data)}
		return nil
	})
}

func loadDirectoryNodeSchemaDocuments(dir string, result *RuntimeNodeSchemaDocumentLoadResult, byPath map[string]RuntimeNodeSchemaDocument) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
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
		key, displayPath := runtimeNodeDocumentPath(dir, path)
		byPath[key] = RuntimeNodeSchemaDocument{Path: displayPath, Content: string(data)}
		return nil
	})
}

func runtimeNodeDocumentPath(root, path string) (string, string) {
	key, err := filepath.Rel(root, path)
	if err != nil {
		key = path
	}
	key = filepath.ToSlash(filepath.Join("nodes", key))
	return key, filepath.ToSlash(path)
}
