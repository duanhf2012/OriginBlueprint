package blueprint

import (
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
	if b.closed {
		b.mu.Unlock()
		return ErrBlueprintClosed
	}
	if len(b.instances) != 0 || len(b.executions) != 0 {
		b.mu.Unlock()
		return ErrBlueprintInUse
	}
	nextLogger := b.logger
	nextTraceLogger := b.traceLogger
	if len(logger) > 0 {
		nextLogger = logger[0]
		nextTraceLogger = nil
		if traceLogger, ok := logger[0].(BlueprintTraceLogger); ok {
			nextTraceLogger = traceLogger
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
	defer b.mu.Unlock()
	b.ensureLocked()
	if b.closed {
		return ErrBlueprintClosed
	}
	if len(b.instances) != 0 || len(b.executions) != 0 {
		return ErrBlueprintInUse
	}
	b.module = blueprintModule
	b.logger = nextLogger
	b.traceLogger = nextTraceLogger
	b.execDefPath = execDefFilePath
	b.graphPath = graphFilePath
	b.graphs = graphs
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

// loadedGraphFile 是目录加载的中间产物：新版文档保留原始 graphDocument，
// 待宏索引建立后统一展开宏引用并转换为 GraphConfig；legacy 文档直接给 config。
type loadedGraphFile struct {
	path       string
	name       string
	aliases    []string
	config     GraphConfig
	isFunction bool
	document   *graphDocument
	data       []byte
}

func loadGraphDir(registry *Registry, dir string) (map[string]*CompiledGraph, error) {
	files := make([]loadedGraphFile, 0)
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
		file, err := parseGraphFile(data, dir, path)
		if err != nil {
			return wrapBlueprintStageError(BlueprintStageParse, path, err)
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 宏索引：.obpm 文档按 macroId 提供解析来源；重复 macroId 属于用户错误，直接失败。
	macroData := map[string][]byte{}
	macroOwners := map[string]string{}
	for _, file := range files {
		if file.document == nil || file.macroID() == "" || !isMacroFile(file.path) {
			continue
		}
		macroID := file.macroID()
		if owner, exists := macroOwners[macroID]; exists {
			return nil, fmt.Errorf("macro id %q from %s conflicts with %s", macroID, file.path, owner)
		}
		macroOwners[macroID] = file.path
		macroData[macroID] = file.data
	}
	resolveMacro := func(macroID string) (*graphDocument, error) {
		data, ok := macroData[strings.TrimSpace(macroID)]
		if !ok {
			return nil, fmt.Errorf("macro %s not found in graph directory", macroID)
		}
		var document graphDocument
		if err := decodeGraphDocument(data, &document); err != nil {
			return nil, err
		}
		return &document, nil
	}

	// 展开宏引用并统一转换为 GraphConfig（引用图和 .obpm 宏自身的嵌套引用都在此处理）。
	for index := range files {
		file := &files[index]
		if file.document == nil {
			continue
		}
		if len(file.document.MacroRefs) != 0 {
			if err := expandMacroRefs(file.document, resolveMacro, nil); err != nil {
				return nil, wrapBlueprintStageError(BlueprintStageParse, file.path, err)
			}
		}
		config, isFunction, err := graphDocumentToConfig(*file.document)
		if err != nil {
			return nil, wrapBlueprintStageError(BlueprintStageParse, file.path, err)
		}
		if strings.EqualFold(filepath.Ext(file.path), ".obpf") {
			isFunction = true
		}
		config.IsFunction = isFunction
		file.config = config
		file.isFunction = isFunction
	}

	// 先编译函数图，普通图编译时可以直接绑定函数调用目标。
	graphNameOwners := make(map[string]string, len(files))
	functionKeyOwners := make(map[string]string)
	for _, file := range files {
		if owner, exists := graphNameOwners[file.name]; exists && owner != file.path {
			return nil, fmt.Errorf("graph name %q from %s conflicts with %s", file.name, file.path, owner)
		}
		graphNameOwners[file.name] = file.path
		if !file.isFunction {
			continue
		}
		keys := make([]string, 0, len(file.aliases)+1)
		keys = append(keys, file.name)
		keys = append(keys, file.aliases...)
		for _, key := range keys {
			if owner, exists := functionKeyOwners[key]; exists {
				if owner != file.path {
					return nil, fmt.Errorf("function key %q from %s conflicts with %s", key, file.path, owner)
				}
				continue
			}
			functionKeyOwners[key] = file.path
		}
	}

	functions := map[string]*CompiledGraph{}
	for _, file := range files {
		if !file.isFunction {
			continue
		}
		graph, err := CompileGraph(registry, file.config)
		if err != nil {
			return nil, wrapBlueprintStageError(BlueprintStageCompile, file.path, err)
		}
		if err := validateLoadedGraphContract(graph, true, false); err != nil {
			return nil, wrapBlueprintStageError(BlueprintStageCompile, file.path, err)
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
		if !file.isFunction {
			continue
		}
		if err := bindAndValidateFunctionCalls(graphs[file.name], true); err != nil {
			return nil, wrapBlueprintStageError(BlueprintStageCompile, file.path, err)
		}
	}
	for _, file := range files {
		if file.isFunction {
			continue
		}
		file.config.Functions = functions
		graph, err := CompileGraph(registry, file.config)
		if err != nil {
			return nil, wrapBlueprintStageError(BlueprintStageCompile, file.path, err)
		}
		allowEmptyLegacyPlaceholder := file.config.Legacy && len(graph.Nodes) == 0
		if err := validateLoadedGraphContract(graph, false, allowEmptyLegacyPlaceholder); err != nil {
			return nil, wrapBlueprintStageError(BlueprintStageCompile, file.path, err)
		}
		graphs[file.name] = graph
	}
	return graphs, nil
}

func validateLoadedGraphContract(graph *CompiledGraph, function bool, allowEmptyLegacyPlaceholder bool) error {
	if graph == nil {
		return fmt.Errorf("compiled graph is nil")
	}
	if !function {
		if len(graph.Entrances) == 0 && !allowEmptyLegacyPlaceholder {
			return fmt.Errorf("graph has no entrance")
		}
		return nil
	}
	if graph.Entrances[FunctionEntranceID] == nil {
		return fmt.Errorf("function graph has no FunctionEntry")
	}
	for _, node := range graph.Nodes {
		if node != nil && node.Definition != nil && node.Definition.ControlKind == ControlFunctionReturn {
			return nil
		}
	}
	return fmt.Errorf("function graph has no FunctionReturn")
}

func isGraphFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".vgf", ".obp", ".obpf", ".obpm":
		return true
	default:
		return false
	}
}

func isMacroFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".obpm")
}

// macroID 返回文档声明的宏身份；只有 .obpm 宏源文件会用它建立宏索引。
func (f *loadedGraphFile) macroID() string {
	if f.document == nil {
		return ""
	}
	return strings.TrimSpace(f.document.MacroID)
}

func parseGraphFile(data []byte, root string, path string) (loadedGraphFile, error) {
	present, _, err := probeGraphSchemaVersion(data)
	if err != nil {
		return loadedGraphFile{}, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !present {
		config, err := ParseGraphConfigJSON(data)
		return loadedGraphFile{path: path, name: name, config: config}, err
	}
	var document graphDocument
	if err := decodeGraphDocument(data, &document); err != nil {
		return loadedGraphFile{}, err
	}
	// .obpf 强制按函数图命名/编译；.obpm 宏源文件按普通图独立编译（校验宏自身），
	// 同时由 loadGraphDir 建立宏索引供引用图编译期内联。
	if strings.EqualFold(filepath.Ext(path), ".obpf") {
		if graphName := strings.TrimSpace(document.GraphName); graphName != "" {
			name = graphName
		}
	}
	return loadedGraphFile{
		path:     path,
		name:     name,
		aliases:  graphFunctionAliases(document, root, path),
		document: &document,
		data:     data,
	}, nil
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
