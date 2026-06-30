package golang

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ExecDefinitionConfig struct {
	Name    string           `json:"name"`
	Inputs  []PortDefinition `json:"inputs"`
	Outputs []PortDefinition `json:"outputs"`
}

type PortDefinition struct {
	PortType string `json:"type"`
	DataType string `json:"data_type"`
	PortID   int    `json:"port_id"`
}

func (r *Registry) LoadDefinitionsJSON(data []byte, factories []func() IExecNode) error {
	var configs []ExecDefinitionConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return err
	}

	factoryByName := make(map[string]func() IExecNode, len(factories))
	for _, factory := range factories {
		if factory == nil {
			continue
		}
		exec := factory()
		if exec == nil {
			continue
		}
		factoryByName[exec.GetName()] = factory
	}

	for _, config := range configs {
		factory := factoryByName[config.Name]
		if factory == nil {
			return fmt.Errorf("exec %s has not been registered", config.Name)
		}

		inPorts, err := buildPorts(config.Inputs)
		if err != nil {
			return fmt.Errorf("exec %s input ports: %w", config.Name, err)
		}
		outPorts, err := buildPorts(config.Outputs)
		if err != nil {
			return fmt.Errorf("exec %s output ports: %w", config.Name, err)
		}
		if !r.Register(NewNodeDefinition(config.Name, factory, inPorts, outPorts)) {
			return fmt.Errorf("exec %s already registered", config.Name)
		}
	}

	return nil
}

func buildPorts(configs []PortDefinition) ([]IPort, error) {
	maxPortID := -1
	for _, config := range configs {
		if config.PortID > maxPortID {
			maxPortID = config.PortID
		}
	}
	if maxPortID < 0 {
		return nil, nil
	}

	ports := make([]IPort, maxPortID+1)
	for _, config := range configs {
		port, err := newPortFromConfig(config)
		if err != nil {
			return nil, err
		}
		ports[config.PortID] = port
	}
	return ports, nil
}

func newPortFromConfig(config PortDefinition) (IPort, error) {
	switch strings.ToLower(config.PortType) {
	case "exec":
		return NewPortExec(), nil
	case "data":
		return newPortFromDataType(config.DataType)
	default:
		return nil, fmt.Errorf("invalid port type %s", config.PortType)
	}
}

func newPortFromDataType(dataType string) (IPort, error) {
	switch strings.ToLower(dataType) {
	case "int", "integer":
		return NewPortInt(), nil
	case "array":
		return NewPortArray(), nil
	default:
		return nil, fmt.Errorf("invalid data type %s", dataType)
	}
}
