package blueprint

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestGraphDocumentVariableWithoutScopeDefaultsToExecution(t *testing.T) {
	config, err := ParseGraphConfigJSON([]byte(`{
		"schemaVersion":1,
		"graphName":"legacy-obp",
		"nodes":[],
		"connections":[],
		"variables":[{"id":"count","name":"Count","type":"integer","defaultValue":3}],
		"groups":[],
		"variableGroups":[],
		"view":{"x":0,"y":0,"zoom":1}
	}`))
	if err != nil {
		t.Fatalf("ParseGraphConfigJSON failed: %v", err)
	}
	if len(config.Variables) != 1 || config.Variables[0].Scope != "" {
		t.Fatalf("variables = %#v, want omitted execution scope", config.Variables)
	}
	compiled, err := CompileGraph(NewRegistry(), config)
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	if got := compiled.variablePlans[0].Scope; got != VariableScopeExecution {
		t.Fatalf("scope = %q, want %q", got, VariableScopeExecution)
	}
}

func TestCompilerRejectsInstanceVariablesInFunctions(t *testing.T) {
	_, err := CompileGraph(NewRegistry(), GraphConfig{
		IsFunction: true,
		Variables: []VariableConfig{{
			ID: "shared", Name: "Shared", Type: "integer", Scope: VariableScopeInstance,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot use instance scope") {
		t.Fatalf("error = %v, want function instance scope rejection", err)
	}
}

func TestHotReloadPreservesInstanceVariableByStableIDAndType(t *testing.T) {
	registry := vmNativeRegistry()
	oldGraph, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{ID: "stable", Name: "Count", Type: "integer", Scope: VariableScopeInstance}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "set", Class: "Set_Count"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "set", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile old graph failed: %v", err)
	}
	newGraph, err := CompileGraph(registry, GraphConfig{
		Variables: []VariableConfig{{ID: "stable", Name: "Renamed", Type: "integer", Scope: VariableScopeInstance}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "get", Class: "Get_Renamed"},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("compile new graph failed: %v", err)
	}

	bp := &Blueprint{}
	bp.AddCompiledGraph("hot", oldGraph)
	graphID := bp.Create("hot")
	if _, err := bp.Do(graphID, 1, 37); err != nil {
		t.Fatalf("old Do failed: %v", err)
	}
	(&hotReloadPlan{blueprint: bp, graphs: map[string]*CompiledGraph{"hot": newGraph}}).apply()
	returns, err := bp.Do(graphID, 1, 0)
	if err != nil {
		t.Fatalf("new Do failed: %v", err)
	}
	assertVMIntReturns(t, returns, 37)
}

func TestInstanceVariableConcurrentAccessIsRaceSafe(t *testing.T) {
	compiled, err := CompileGraph(vmNativeRegistry(), GraphConfig{
		Variables: []VariableConfig{{ID: "count", Name: "Count", Type: "integer", Scope: VariableScopeInstance}},
		Nodes: []NodeConfig{
			{ID: "entry", Class: "VMEntry_1"},
			{ID: "set", Class: "Set_Count"},
			{ID: "get", Class: "Get_Count"},
			{ID: "result", Class: "VMReturnPort"},
		},
		Edges: []EdgeConfig{
			{SourceNodeID: "entry", SourcePortID: 0, DesNodeID: "set", DesPortID: 0},
			{SourceNodeID: "entry", SourcePortID: 1, DesNodeID: "set", DesPortID: 1},
			{SourceNodeID: "set", SourcePortID: 0, DesNodeID: "result", DesPortID: 0},
			{SourceNodeID: "get", SourcePortID: 0, DesNodeID: "result", DesPortID: 1},
		},
	})
	if err != nil {
		t.Fatalf("CompileGraph failed: %v", err)
	}
	bp := &Blueprint{}
	bp.AddCompiledGraph("shared", compiled)
	graphID := bp.Create("shared")

	const count = 32
	var wait sync.WaitGroup
	errors := make(chan error, count)
	for index := 1; index <= count; index++ {
		wait.Add(1)
		go func(value int) {
			defer wait.Done()
			returns, err := bp.Do(graphID, 1, value)
			if err != nil {
				errors <- err
				return
			}
			if len(returns) != 1 || returns[0].IntVal < 1 || returns[0].IntVal > count {
				errors <- fmt.Errorf("returns = %#v", returns)
			}
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
