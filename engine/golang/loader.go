package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type IBlueprintModule interface{}
type IBlueprintLogger interface{}

func (b *Blueprint) RegisterExecNode(factory func() IExecNode) {
	if factory == nil {
		return
	}
	b.execFactories = append(b.execFactories, factory)
}

func (b *Blueprint) Init(execDefFilePath string, graphFilePath string, blueprintModule IBlueprintModule, cancelTimer func(*uint64) bool, logger ...IBlueprintLogger) error {
	b.ensure()
	registry := NewRegistry()
	if err := loadDefinitionDir(registry, execDefFilePath, b.execFactories); err != nil {
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
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".json" {
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
	graphs := map[string]*CompiledGraph{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".vgf" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		config, err := ParseGraphConfigJSON(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		graph, err := CompileGraph(registry, config)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		graphs[name] = graph
		return nil
	})
	return graphs, err
}
