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
	mu            sync.RWMutex
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
	timerMu    sync.Mutex
	variables  map[string]IPort
	variableMu sync.RWMutex
}

// AddCompiledGraph registers an already compiled graph under a runtime name.
func (b *Blueprint) AddCompiledGraph(name string, graph *CompiledGraph) {
	if name == "" || graph == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	b.graphs[name] = graph
}

// Create allocates an executable graph instance and returns its graph id.
//
// The returned instance shares the compiled execution tree but owns variables
// and timer ids. A return value of 0 means the graph name was not registered.
func (b *Blueprint) Create(graphName string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
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
	b.mu.RLock()
	instance := b.instances[graphID]
	if instance == nil {
		b.mu.RUnlock()
		return nil, nil
	}
	name := instance.name
	compiled := instance.compiled
	module := instance.module
	variables := instance.variables
	variableMu := &instance.variableMu
	b.mu.RUnlock()

	graph := NewGraph(compiled)
	graph.name = name
	graph.graphID = graphID
	graph.module = module
	graph.instance = instance
	graph.variables = variables
	graph.variableMu = variableMu
	return graph.Do(entranceID, args...)
}

// TriggerEvent is a convenience wrapper used by timer callbacks.
func (b *Blueprint) TriggerEvent(graphID int64, eventID int64, args ...any) error {
	_, err := b.Do(graphID, eventID, args...)
	return err
}

// ReleaseGraph removes an instance and asks the host module to cancel timers.
func (b *Blueprint) ReleaseGraph(graphID int64) {
	b.mu.Lock()
	instance := b.instances[graphID]
	delete(b.instances, graphID)
	b.mu.Unlock()

	if instance != nil && instance.module != nil {
		instance.timerMu.Lock()
		timerIDs := make([]uint64, 0, len(instance.timers))
		for timerID := range instance.timers {
			timerIDs = append(timerIDs, timerID)
		}
		instance.timers = map[uint64]struct{}{}
		instance.timerMu.Unlock()
		for _, timerID := range timerIDs {
			id := timerID
			instance.module.CancelTimerId(graphID, &id)
		}
	}
}

// CancelTimerId preserves the old blueprint API for external timer modules.
func (b *Blueprint) CancelTimerId(graphID int64, timerID *uint64) bool {
	if timerID == nil {
		return false
	}
	id := *timerID
	b.mu.RLock()
	module := b.module
	cancelTimer := b.cancelTimer
	instance := b.instances[graphID]
	b.mu.RUnlock()

	if module != nil {
		module.CancelTimerId(graphID, timerID)
	} else if cancelTimer != nil {
		cancelTimer(timerID)
	}

	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	delete(instance.timers, id)
	instance.timerMu.Unlock()
	return true
}

// StartHotReload keeps the old facade method available.
//
// The new engine compiles immutable graph trees; file-watcher based hot reload
// can be layered on top later. For now this returns a no-op stop function.
func (b *Blueprint) StartHotReload() (func(), error) {
	b.mu.RLock()
	execDefPath := b.execDefPath
	graphPath := b.graphPath
	execFactories := append([]func() IExecNode(nil), b.execFactories...)
	b.mu.RUnlock()

	if execDefPath == "" || graphPath == "" {
		return func() {}, nil
	}
	registry := NewRegistry()
	factories := append(BuiltinExecNodeFactories(), execFactories...)
	if err := loadDefinitionDir(registry, execDefPath, factories); err != nil {
		return nil, err
	}
	graphs, err := loadGraphDir(registry, graphPath)
	if err != nil {
		return nil, err
	}
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.ensureLocked()
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
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.logger
}

// GetGraphName returns the registered graph name for an instance.
func (b *Blueprint) GetGraphName(graphID int64) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	instance := b.instances[graphID]
	if instance == nil {
		return ""
	}
	return instance.name
}

func (b *Blueprint) ensure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
}

func (b *Blueprint) ensureLocked() {
	if b.graphs == nil {
		b.graphs = map[string]*CompiledGraph{}
	}
	if b.instances == nil {
		b.instances = map[int64]*GraphInstance{}
	}
}

func (b *Blueprint) mustGraph(graphID int64) (*GraphInstance, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	graph := b.instances[graphID]
	if graph == nil {
		return nil, fmt.Errorf("can not find graph:%d", graphID)
	}
	return graph, nil
}
