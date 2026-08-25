package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

// LoadNodeSchemaDocumentsForWorkspace 加载内建、全局扩展和当前工作区节点定义。
// 工作区节点最后加载，因此可以按相对路径覆盖低优先级来源。
func (a *App) LoadNodeSchemaDocumentsForWorkspace(workspaceRoot string) RuntimeNodeSchemaDocumentLoadResult {
	return loadRuntimeNodeSchemaDocumentsForWorkspace(workspaceRoot)
}

func runtimeNodeDirectories() []string {
	directories, _ := runtimeNodeDirectoriesForWorkspace("")
	return directories
}

func runtimeNodeDirectoriesForWorkspace(workspaceRoot string) ([]string, error) {
	var candidates []string
	add := func(base string) {
		if strings.TrimSpace(base) != "" {
			candidates = append(candidates, filepath.Join(base, "nodes"))
		}
	}
	if executable, err := os.Executable(); err == nil {
		add(filepath.Dir(executable))
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}

	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot != "" {
		absolute, err := filepath.Abs(workspaceRoot)
		if err != nil {
			return nil, fmt.Errorf("resolve workspace %q: %w", workspaceRoot, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("open workspace %q: %w", absolute, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("workspace %q is not a directory", absolute)
		}
		add(absolute)
	}
	return uniqueRuntimeNodeDirectories(candidates), nil
}

func uniqueRuntimeNodeDirectories(candidates []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		key := absolute
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, absolute)
	}
	return result
}

func loadRuntimeNodeSchemaDocumentsForWorkspace(workspaceRoot string) RuntimeNodeSchemaDocumentLoadResult {
	directories, err := runtimeNodeDirectoriesForWorkspace(workspaceRoot)
	if err != nil {
		return RuntimeNodeSchemaDocumentLoadResult{Errors: []RuntimeNodeLoadError{{
			Path:    strings.TrimSpace(workspaceRoot),
			Message: err.Error(),
		}}}
	}
	return loadRuntimeNodeSchemaDocumentsWithEmbedded(directories)
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
