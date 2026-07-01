package golang

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Blueprint is the legacy-compatible facade used by server modules.
//
// Graph definitions are compiled once and shared. Create allocates a lightweight
// GraphInstance that owns variables and timer bookkeeping for one server object.
type Blueprint struct {
	graphs        map[string]*CompiledGraph
	instances     map[int64]*GraphInstance
	execFactories []func() IExecNode
	module        IBlueprintModule
	cancelTimer   func(*uint64) bool
	logger        IBlueprintLogger
	execDefPath   string
	graphPath     string
	seedID        int64
}

// GraphInstance stores state that must not be shared between Create calls.
type GraphInstance struct {
	name       string
	compiled   *CompiledGraph
	graphID    int64
	module     IBlueprintModule
	timers     map[uint64]struct{}
	variables  map[string]IPort
	variableMu sync.RWMutex
}

// AddCompiledGraph registers an already compiled graph under a runtime name.
func (b *Blueprint) AddCompiledGraph(name string, graph *CompiledGraph) {
	b.ensure()
	if name == "" || graph == nil {
		return
	}
	b.graphs[name] = graph
}

// Create allocates an executable graph instance and returns its graph id.
//
// The returned instance shares the compiled execution tree but owns variables
// and timer ids. A return value of 0 means the graph name was not registered.
func (b *Blueprint) Create(graphName string) int64 {
	b.ensure()
	compiled := b.graphs[graphName]
	if compiled == nil {
		return 0
	}
	graphID := atomic.AddInt64(&b.seedID, 1)
	b.instances[graphID] = &GraphInstance{
		name:      graphName,
		compiled:  compiled,
		graphID:   graphID,
		module:    b.module,
		timers:    map[uint64]struct{}{},
		variables: initialVariables(compiled),
	}
	return graphID
}

// Do executes one entrance on an existing graph instance.
//
// Each Do call creates an execution session so concurrent async continuations do
// not share transient node contexts. Instance variables are shared deliberately.
func (b *Blueprint) Do(graphID int64, entranceID int64, args ...any) (PortArray, error) {
	b.ensure()
	instance := b.instances[graphID]
	if instance == nil {
		return nil, nil
	}
	graph := NewGraph(instance.compiled)
	graph.name = instance.name
	graph.graphID = instance.graphID
	graph.module = instance.module
	graph.instance = instance
	graph.variables = instance.variables
	graph.variableMu = &instance.variableMu
	return graph.Do(entranceID, args...)
}

// TriggerEvent is a convenience wrapper used by timer callbacks.
func (b *Blueprint) TriggerEvent(graphID int64, eventID int64, args ...any) error {
	_, err := b.Do(graphID, eventID, args...)
	return err
}

// ReleaseGraph removes an instance and asks the host module to cancel timers.
func (b *Blueprint) ReleaseGraph(graphID int64) {
	b.ensure()
	instance := b.instances[graphID]
	if instance != nil && instance.module != nil {
		for timerID := range instance.timers {
			id := timerID
			instance.module.CancelTimerId(graphID, &id)
		}
	}
	delete(b.instances, graphID)
}

// CancelTimerId preserves the old blueprint API for external timer modules.
func (b *Blueprint) CancelTimerId(graphID int64, timerID *uint64) bool {
	b.ensure()
	if timerID == nil {
		return false
	}
	id := *timerID
	if b.module != nil {
		b.module.CancelTimerId(graphID, timerID)
	} else if b.cancelTimer != nil {
		b.cancelTimer(timerID)
	}

	instance := b.instances[graphID]
	if instance == nil {
		return false
	}
	delete(instance.timers, id)
	return true
}

// StartHotReload keeps the old facade method available.
//
// The new engine compiles immutable graph trees; file-watcher based hot reload
// can be layered on top later. For now this returns a no-op stop function.
func (b *Blueprint) StartHotReload() (func(), error) {
	b.ensure()
	if b.execDefPath == "" || b.graphPath == "" {
		return func() {}, nil
	}
	registry := NewRegistry()
	factories := append(BuiltinExecNodeFactories(), b.execFactories...)
	if err := loadDefinitionDir(registry, b.execDefPath, factories); err != nil {
		return nil, err
	}
	graphs, err := loadGraphDir(registry, b.graphPath)
	if err != nil {
		return nil, err
	}
	return func() {
		b.graphs = graphs
		for _, instance := range b.instances {
			if compiled := graphs[instance.name]; compiled != nil {
				instance.compiled = compiled
			}
		}
	}, nil
}

// GetLogger returns the optional logger passed to Init.
func (b *Blueprint) GetLogger() IBlueprintLogger {
	return b.logger
}

// GetGraphName returns the registered graph name for an instance.
func (b *Blueprint) GetGraphName(graphID int64) string {
	b.ensure()
	instance := b.instances[graphID]
	if instance == nil {
		return ""
	}
	return instance.name
}

func (b *Blueprint) ensure() {
	if b.graphs == nil {
		b.graphs = map[string]*CompiledGraph{}
	}
	if b.instances == nil {
		b.instances = map[int64]*GraphInstance{}
	}
}

func (b *Blueprint) mustGraph(graphID int64) (*GraphInstance, error) {
	b.ensure()
	graph := b.instances[graphID]
	if graph == nil {
		return nil, fmt.Errorf("can not find graph:%d", graphID)
	}
	return graph, nil
}
