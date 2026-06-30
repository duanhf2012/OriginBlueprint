package golang

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Registry struct {
	definitions map[string]*NodeDefinition
}

func NewRegistry() *Registry {
	return &Registry{definitions: map[string]*NodeDefinition{}}
}

func (r *Registry) Register(definition *NodeDefinition) bool {
	if r == nil || definition == nil || definition.Name == "" {
		return false
	}
	if r.definitions == nil {
		r.definitions = map[string]*NodeDefinition{}
	}
	if _, exists := r.definitions[definition.Name]; exists {
		return false
	}
	r.definitions[definition.Name] = definition
	return true
}

func (r *Registry) Get(name string) *NodeDefinition {
	if r == nil {
		return nil
	}
	return r.definitions[name]
}

type NodeConfig struct {
	ID          string         `json:"id"`
	Class       string         `json:"class"`
	PortDefault map[int]any    `json:"-"`
	RawDefault  map[string]any `json:"port_defaultv"`
}

type EdgeConfig struct {
	SourceNodeID string `json:"source_node_id"`
	DesNodeID    string `json:"des_node_id"`
	SourcePortID int    `json:"source_port_id"`
	DesPortID    int    `json:"des_port_id"`
}

type GraphConfig struct {
	Nodes []NodeConfig `json:"nodes"`
	Edges []EdgeConfig `json:"edges"`
}

func ParseGraphConfigJSON(data []byte) (GraphConfig, error) {
	var config GraphConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return GraphConfig{}, err
	}
	for index := range config.Nodes {
		config.Nodes[index].PortDefault = parsePortDefaults(config.Nodes[index].RawDefault)
	}
	return config, nil
}

func CompileGraph(registry *Registry, config GraphConfig) (*CompiledGraph, error) {
	nodes := make(map[string]*ExecNode, len(config.Nodes))
	entrances := map[int64]*ExecNode{}

	for _, nodeConfig := range config.Nodes {
		nodeName, entranceID, isEntrance := parseEntranceClass(nodeConfig.Class)
		definition := registry.Get(nodeName)
		if definition == nil {
			return nil, fmt.Errorf("%s node has not been registered", nodeConfig.Class)
		}

		node := NewExecNode(nodeConfig.ID, definition)
		node.DefaultIn = nodeConfig.PortDefault
		node.IsEntrance = isEntrance
		nodes[nodeConfig.ID] = node
		if isEntrance {
			entrances[entranceID] = node
		}
	}

	for _, edge := range config.Edges {
		source := nodes[edge.SourceNodeID]
		dest := nodes[edge.DesNodeID]
		if source == nil {
			return nil, fmt.Errorf("source node %s not found", edge.SourceNodeID)
		}
		if dest == nil {
			return nil, fmt.Errorf("destination node %s not found", edge.DesNodeID)
		}

		if source.isOutPortExec(edge.SourcePortID) {
			source.ensureNext(edge.SourcePortID)
			source.Next[edge.SourcePortID] = dest
			dest.BeConnect = true
			continue
		}

		if edge.DesPortID < 0 || edge.DesPortID >= len(dest.PreInPort) {
			return nil, fmt.Errorf("destination node %s in port %d not found", edge.DesNodeID, edge.DesPortID)
		}
		dest.PreInPort[edge.DesPortID] = &PrePortNode{Node: source, OutPortID: edge.SourcePortID}
	}

	return &CompiledGraph{Entrances: entrances}, nil
}

func parsePortDefaults(raw map[string]any) map[int]any {
	if len(raw) == 0 {
		return nil
	}
	defaults := make(map[int]any, len(raw))
	for key, value := range raw {
		index, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		defaults[index] = value
	}
	return defaults
}

func parseEntranceClass(className string) (string, int64, bool) {
	index := strings.LastIndex(className, "_")
	if index < 0 || index == len(className)-1 {
		return className, 0, false
	}

	entranceID, err := strconv.ParseInt(className[index+1:], 10, 64)
	if err != nil {
		return className, 0, false
	}
	return className[:index], entranceID, true
}

func (n *ExecNode) isOutPortExec(index int) bool {
	if n == nil || n.Definition == nil || index < 0 || index >= len(n.Definition.OutPorts) {
		return false
	}
	port := n.Definition.OutPorts[index]
	return port != nil && port.IsPortExec()
}

func (n *ExecNode) ensureNext(index int) {
	for len(n.Next) <= index {
		n.Next = append(n.Next, nil)
	}
}
