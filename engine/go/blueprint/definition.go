package blueprint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExecDefinitionConfig 对应节点定义 JSON 中的一项节点声明。
type ExecDefinitionConfig struct {
	Name    string           `json:"name"`
	Inputs  []PortDefinition `json:"inputs"`
	Outputs []PortDefinition `json:"outputs"`
}

// PortDefinition 描述节点定义中的单个端口。
type PortDefinition struct {
	PortType string `json:"type"`
	DataType string `json:"data_type"`
	PortID   int    `json:"port_id"`
}

// LoadDefinitionsJSON 加载节点定义文件并注册可创建的节点。
//
// factories 用于把 JSON 定义绑定到具体 Go 执行节点。
func (r *Registry) LoadDefinitionsJSON(data []byte, factories []func() IExecNode) error {
	var configs []ExecDefinitionConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		configs = parseLenientDefinitions(string(data))
		if len(configs) == 0 {
			return err
		}
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
		nodeName, _, _ := parseEntranceClass(config.Name)
		factory := factoryByName[nodeName]
		if factory == nil {
			return fmt.Errorf("exec %s has not been registered", nodeName)
		}

		inPorts, err := buildPorts(config.Inputs)
		if err != nil {
			return fmt.Errorf("exec %s input ports: %w", nodeName, err)
		}
		outPorts, err := buildPorts(config.Outputs)
		if err != nil {
			return fmt.Errorf("exec %s output ports: %w", nodeName, err)
		}
		if !r.Register(NewNodeDefinition(nodeName, factory, inPorts, outPorts)) {
			return fmt.Errorf("exec %s already registered", nodeName)
		}
	}

	return nil
}

func parseLenientDefinitions(text string) []ExecDefinitionConfig {
	objects := splitTopLevelObjects(text)
	configs := make([]ExecDefinitionConfig, 0, len(objects))
	for _, object := range objects {
		name := firstStringField(object, "name")
		if name == "" {
			continue
		}
		configs = append(configs, ExecDefinitionConfig{
			Name:    name,
			Inputs:  parseLenientPorts(arrayField(object, "inputs")),
			Outputs: parseLenientPorts(arrayField(object, "outputs")),
		})
	}
	return configs
}

func splitTopLevelObjects(text string) []string {
	var objects []string
	depth := 0
	start := -1
	for index, r := range text {
		switch r {
		case '{':
			if depth == 0 {
				start = index
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				objects = append(objects, text[start:index+1])
				start = -1
			}
		}
	}
	return objects
}

func firstStringField(text, field string) string {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*"([^"]*)"`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func firstIntField(text, field string) int {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*(-?\d+)`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return -1
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return value
}

func arrayField(text, field string) string {
	startRe := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*\[`)
	loc := startRe.FindStringIndex(text)
	if loc == nil {
		return ""
	}
	start := loc[1]
	depth := 1
	for index := start; index < len(text); index++ {
		switch text[index] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return text[start:index]
			}
		}
	}
	return ""
}

func parseLenientPorts(text string) []PortDefinition {
	objects := splitTopLevelObjects(text)
	ports := make([]PortDefinition, 0, len(objects))
	for _, object := range objects {
		portID := firstIntField(object, "port_id")
		if portID < 0 {
			continue
		}
		ports = append(ports, PortDefinition{
			PortType: firstStringField(object, "type"),
			DataType: firstStringField(object, "data_type"),
			PortID:   portID,
		})
	}
	return ports
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
	case "float":
		return NewPortFloat(), nil
	case "string", "str":
		return NewPortStr(), nil
	case "boolean", "bool":
		return NewPortBool(), nil
	case "array":
		return NewPortArray(), nil
	case "any":
		return NewPortAny(), nil
	default:
		return nil, fmt.Errorf("invalid data type %s", dataType)
	}
}
