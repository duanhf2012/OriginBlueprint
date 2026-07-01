package golang

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IBlueprintModule is the host integration needed by timer nodes.
//
// It matches the old server-facing blueprint module contract.
type IBlueprintModule interface {
	SafeAfterFunc(timerID *uint64, d time.Duration, additionData any, cb func(uint64, any))
	TriggerEvent(graphID int64, eventID int64, args ...any) error
	CancelTimerId(graphID int64, timerID *uint64) bool
}

// IBlueprintLogger is kept for Init signature compatibility with the old library.
type IBlueprintLogger interface{}

// RegisterExecNode registers a custom node factory before Init loads definitions.
func (b *Blueprint) RegisterExecNode(factory func() IExecNode) {
	if factory == nil {
		return
	}
	b.execFactories = append(b.execFactories, factory)
}

// Init loads node definitions and graph files from disk.
//
// Top-level node JSON files are loaded; a child directory named "json" is
// skipped to match the OriginBlueprint repository layout.
func (b *Blueprint) Init(execDefFilePath string, graphFilePath string, blueprintModule IBlueprintModule, cancelTimer func(*uint64) bool, logger ...IBlueprintLogger) error {
	b.ensure()
	b.module = blueprintModule
	b.cancelTimer = cancelTimer
	b.execDefPath = execDefFilePath
	b.graphPath = graphFilePath
	if len(logger) > 0 {
		b.logger = logger[0]
	}
	registry := NewRegistry()
	factories := append(BuiltinExecNodeFactories(), b.execFactories...)
	if err := loadDefinitionDir(registry, execDefFilePath, factories); err != nil {
		return err
	}
	graphs, err := loadGraphDir(registry, graphFilePath)
	if err != nil {
		return err
	}
	for name, graph := range graphs {
		b.AddCompiledGraph(name, graph)
	}
	return nil
}

func loadDefinitionDir(registry *Registry, dir string, factories []func() IExecNode) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir && entry.Name() == "json" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := registry.LoadDefinitionsJSON(data, factories); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	})
}

func loadGraphDir(registry *Registry, dir string) (map[string]*CompiledGraph, error) {
	type graphFile struct {
		name       string
		aliases    []string
		config     GraphConfig
		isFunction bool
	}
	files := make([]graphFile, 0)
	graphs := map[string]*CompiledGraph{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isGraphFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		config, isFunction, graphName, aliases, err := parseGraphFile(data, dir, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		files = append(files, graphFile{name: graphName, aliases: aliases, config: config, isFunction: isFunction})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Functions must be compiled first so regular graphs can link FunctionCall
	// nodes to the shared function table.
	functions := map[string]*CompiledGraph{}
	for _, file := range files {
		if !file.isFunction {
			continue
		}
		graph, err := CompileGraph(registry, file.config)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.name, err)
		}
		functions[file.name] = graph
		for _, alias := range file.aliases {
			functions[alias] = graph
		}
		graphs[file.name] = graph
	}
	for _, graph := range functions {
		graph.Functions = functions
	}
	for _, file := range files {
		if file.isFunction {
			continue
		}
		file.config.Functions = functions
		graph, err := CompileGraph(registry, file.config)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file.name, err)
		}
		graphs[file.name] = graph
	}
	return graphs, nil
}

func isGraphFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".vgf", ".obp", ".obpf":
		return true
	default:
		return false
	}
}

func parseGraphFile(data []byte, root string, path string) (GraphConfig, bool, string, []string, error) {
	var documentProbe struct {
		SchemaVersion     int                        `json:"schemaVersion"`
		GraphName         string                     `json:"graphName"`
		FunctionID        string                     `json:"functionId,omitempty"`
		FunctionSignature graphDocumentFuncSignature `json:"functionSignature,omitempty"`
	}
	if err := json.Unmarshal(data, &documentProbe); err == nil && documentProbe.SchemaVersion > 0 {
		var document graphDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return GraphConfig{}, false, "", nil, err
		}
		config, isFunction, err := graphDocumentToConfig(document)
		name := strings.TrimSpace(document.GraphName)
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		if strings.ToLower(filepath.Ext(path)) == ".obpf" {
			isFunction = true
		}
		aliases := graphFunctionAliases(document, root, path)
		return config, isFunction, name, aliases, err
	}

	config, err := ParseGraphConfigJSON(data)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return config, false, name, nil, err
}

func graphFunctionAliases(document graphDocument, root string, path string) []string {
	seen := map[string]bool{}
	aliases := make([]string, 0, 2)
	add := func(alias string) {
		alias = filepath.ToSlash(strings.TrimSpace(alias))
		if alias == "" || seen[alias] {
			return
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	add(document.FunctionID)
	if rel, err := filepath.Rel(root, path); err == nil {
		add(rel)
	}
	return aliases
}
