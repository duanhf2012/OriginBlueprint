package golang

import (
	"fmt"
	"sync/atomic"
)

type Blueprint struct {
	graphs        map[string]*CompiledGraph
	instances     map[int64]*GraphInstance
	execFactories []func() IExecNode
	seedID        int64
}

type GraphInstance struct {
	name     string
	compiled *CompiledGraph
}

func (b *Blueprint) AddCompiledGraph(name string, graph *CompiledGraph) {
	b.ensure()
	if name == "" || graph == nil {
		return
	}
	b.graphs[name] = graph
}

func (b *Blueprint) Create(graphName string) int64 {
	b.ensure()
	compiled := b.graphs[graphName]
	if compiled == nil {
		return 0
	}
	graphID := atomic.AddInt64(&b.seedID, 1)
	b.instances[graphID] = &GraphInstance{name: graphName, compiled: compiled}
	return graphID
}

func (b *Blueprint) Do(graphID int64, entranceID int64, args ...any) (PortArray, error) {
	b.ensure()
	instance := b.instances[graphID]
	if instance == nil {
		return nil, nil
	}
	graph := NewGraph(instance.compiled)
	graph.name = instance.name
	return graph.Do(entranceID, args...)
}

func (b *Blueprint) TriggerEvent(graphID int64, eventID int64, args ...any) error {
	_, err := b.Do(graphID, eventID, args...)
	return err
}

func (b *Blueprint) ReleaseGraph(graphID int64) {
	b.ensure()
	delete(b.instances, graphID)
}

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
