package blueprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IBlueprintModule 由业务层实现，用于接入服务器事件触发。
type IBlueprintModule interface {
	TriggerEvent(graphID int64, eventID int64, args ...any) error
}

// IBlueprintLogger 保留旧接口的日志对象类型。
type IBlueprintLogger interface{}

// RegisterExecNode 注册业务自定义执行节点工厂。
func (b *Blueprint) RegisterExecNode(factory func() IExecNode) {
	if factory == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.execFactories = append(b.execFactories, factory)
}

// Init 加载节点定义和蓝图目录，并初始化运行时依赖。
//
// 该方法兼容旧版 Blueprint 的初始化入口。
func (b *Blueprint) Init(execDefFilePath string, graphFilePath string, blueprintModule IBlueprintModule, logger ...IBlueprintLogger) error {
	b.mu.Lock()
	b.ensureLocked()
	b.module = blueprintModule
	b.execDefPath = execDefFilePath
	b.graphPath = graphFilePath
	if len(logger) > 0 {
		b.logger = logger[0]
		if traceLogger, ok := logger[0].(BlueprintTraceLogger); ok {
			b.traceLogger = traceLogger
		}
	}
	execFactories := append([]func() IExecNode(nil), b.execFactories...)
	b.mu.Unlock()

	registry := NewRegistry()
	factories := append(BuiltinExecNodeFactories(), execFactories...)
	if err := loadDefinitionDir(registry, execDefFilePath, factories); err != nil {
		return err
	}
	graphs, err := loadGraphDir(registry, graphFilePath)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.ensureLocked()
	for name, graph := range graphs {
		b.graphs[name] = graph
	}
	b.mu.Unlock()
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

	// 先编译函数图，普通图编译时可以直接绑定函数调用目标。
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
	present, _, err := probeGraphSchemaVersion(data)
	if err != nil {
		return GraphConfig{}, false, "", nil, err
	}
	var documentProbe struct {
		GraphName         string                     `json:"graphName"`
		FunctionID        string                     `json:"functionId,omitempty"`
		FunctionSignature graphDocumentFuncSignature `json:"functionSignature,omitempty"`
	}
	if present {
		if err := json.Unmarshal(data, &documentProbe); err != nil {
			return GraphConfig{}, false, "", nil, err
		}
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
