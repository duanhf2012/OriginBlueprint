package blueprint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ExecDefinitionConfig 对应节点定义 JSON 中的一项节点声明。
type ExecDefinitionConfig struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Inputs  []PortDefinition `json:"inputs"`
	Outputs []PortDefinition `json:"outputs"`
}

// PortDefinition 描述节点定义中的单个端口。
type PortDefinition struct {
	Key      string `json:"key"`
	PortType string `json:"type"`
	DataType string `json:"data_type"`
	PortID   int    `json:"port_id"`
	// HasPortID 用于区分旧 port_id 缺省和显式声明为 0。
	HasPortID bool `json:"-"`
}

func (p *PortDefinition) UnmarshalJSON(data []byte) error {
	var raw struct {
		Key      string `json:"key"`
		PortType string `json:"type"`
		DataType string `json:"data_type"`
		PortID   any    `json:"port_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	portID, hasPortID, err := parseJSONPortID(raw.PortID)
	if err != nil {
		return err
	}
	*p = PortDefinition{
		Key:       raw.Key,
		PortType:  raw.PortType,
		DataType:  raw.DataType,
		PortID:    portID,
		HasPortID: hasPortID,
	}
	return nil
}

// LoadDefinitionsJSON 加载节点定义文件并注册可创建的节点。
//
// factories 用于把 JSON 定义绑定到具体 Go 执行节点。
func (r *Registry) LoadDefinitionsJSON(data []byte, factories []func() IExecNode) error {
	if err := preflightDefinitionPortCounts(data); err != nil {
		return err
	}
	var configs []ExecDefinitionConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		var rawArray []json.RawMessage
		if json.Unmarshal(data, &rawArray) == nil {
			// 标准 JSON 数组里的字段类型错误应直接暴露；宽松解析只用于历史非标准节点文件。
			return err
		}
		var lenientErr error
		configs, lenientErr = parseLenientDefinitions(string(data))
		if lenientErr != nil {
			return lenientErr
		}
		if len(configs) == 0 {
			return err
		}
	}
	explicitNames := make(map[string]bool, len(configs))
	for _, config := range configs {
		if strings.TrimSpace(config.Name) == "" {
			continue
		}
		nodeName, _, _ := parseEntranceClass(config.Name)
		explicitNames[nodeName] = true
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
		derivedFromSchemaID := false
		if strings.TrimSpace(config.Name) == "" {
			// 新编辑器 schema 使用 id/key；执行器仍按旧 class 注册，因此已知 id-only 节点在这里桥接。
			var ok bool
			config, ok = executableConfigForSchemaID(config.ID)
			if !ok {
				continue
			}
			derivedFromSchemaID = true
		}
		nodeName, _, _ := parseEntranceClass(config.Name)
		if derivedFromSchemaID && (explicitNames[nodeName] || registryHasDefinition(r, nodeName)) {
			// 同一个 nodes 文件同时有新旧定义时，优先使用显式旧 name/port_id 定义。
			continue
		}
		factory := factoryByName[nodeName]
		if factory == nil {
			return fmt.Errorf("exec %s has not been registered", nodeName)
		}
		if err := validateTotalNodePortCount(len(config.Inputs), len(config.Outputs)); err != nil {
			return fmt.Errorf("exec %s ports: %w", nodeName, err)
		}

		inputConfigs, err := normalizePortDefinitions(config.Inputs, schemaPortIndexes(config.ID, false))
		if err != nil {
			return fmt.Errorf("exec %s input ports: %w", nodeName, err)
		}
		outputConfigs, err := normalizePortDefinitions(config.Outputs, schemaPortIndexes(config.ID, true))
		if err != nil {
			return fmt.Errorf("exec %s output ports: %w", nodeName, err)
		}

		inPorts, err := buildPorts(inputConfigs)
		if err != nil {
			return fmt.Errorf("exec %s input ports: %w", nodeName, err)
		}
		dynamicOutputs := dynamicBranchDefinitionOutputs(config.Name, outputConfigs)
		if err := validateTotalNodePortCount(len(inputConfigs), len(dynamicOutputs)); err != nil {
			return fmt.Errorf("exec %s ports: %w", nodeName, err)
		}
		outPorts, err := buildPorts(dynamicOutputs)
		if err != nil {
			return fmt.Errorf("exec %s output ports: %w", nodeName, err)
		}
		if !r.Register(NewNodeDefinition(nodeName, factory, inPorts, outPorts)) {
			return fmt.Errorf("exec %s already registered", nodeName)
		}
	}

	return nil
}

type definitionJSONFrame struct {
	delim          json.Delim
	expectingKey   bool
	pendingKey     string
	rootArray      bool
	nodeObject     bool
	portArray      bool
	nodeFrameIndex int
	nodePortCount  int
}

func preflightDefinitionPortCounts(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	frames := make([]definitionJSONFrame, 0, 8)
	for {
		token, err := decoder.Token()
		if err != nil {
			// Syntax compatibility is decided by the regular and lenient parsers.
			return nil
		}

		if delim, ok := token.(json.Delim); ok {
			if delim == '}' || delim == ']' {
				if len(frames) != 0 {
					frames = frames[:len(frames)-1]
				}
				continue
			}

			parentIndex := len(frames) - 1
			parentKey := ""
			if parentIndex >= 0 && frames[parentIndex].delim == '{' && !frames[parentIndex].expectingKey {
				parentKey = frames[parentIndex].pendingKey
				frames[parentIndex].expectingKey = true
				frames[parentIndex].pendingKey = ""
			}

			frame := definitionJSONFrame{delim: delim, expectingKey: delim == '{', nodeFrameIndex: -1}
			if delim == '[' {
				frame.rootArray = len(frames) == 0
				if parentIndex >= 0 && frames[parentIndex].nodeObject && (parentKey == "inputs" || parentKey == "outputs") {
					frame.portArray = true
					frame.nodeFrameIndex = parentIndex
				}
			}
			if delim == '{' && parentIndex >= 0 {
				frame.nodeObject = frames[parentIndex].rootArray
			}
			if err := countDefinitionPortElement(frames, parentIndex); err != nil {
				return err
			}
			frames = append(frames, frame)
			continue
		}

		if len(frames) == 0 {
			continue
		}
		top := &frames[len(frames)-1]
		if top.delim == '{' {
			if top.expectingKey {
				key, ok := token.(string)
				if ok {
					top.pendingKey = key
					top.expectingKey = false
				}
			} else {
				top.expectingKey = true
				top.pendingKey = ""
			}
			continue
		}
		if err := countDefinitionPortElement(frames, len(frames)-1); err != nil {
			return err
		}
	}
}

func countDefinitionPortElement(frames []definitionJSONFrame, parentIndex int) error {
	if parentIndex < 0 || parentIndex >= len(frames) || !frames[parentIndex].portArray {
		return nil
	}
	nodeIndex := frames[parentIndex].nodeFrameIndex
	if nodeIndex < 0 || nodeIndex >= len(frames) {
		return nil
	}
	frames[nodeIndex].nodePortCount++
	return validateMaximum("total port count", frames[nodeIndex].nodePortCount, maxNodePortCount)
}

func registryHasDefinition(r *Registry, name string) bool {
	return r != nil && r.Get(name) != nil
}

func parseJSONPortID(value any) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	switch typed := value.(type) {
	case float64:
		if math.Trunc(typed) != typed {
			return 0, false, fmt.Errorf("invalid non-integer port_id %v", value)
		}
		portID := int(typed)
		if portID < 0 {
			return 0, false, fmt.Errorf("invalid negative port_id %d", portID)
		}
		if portID > maxNodePortID {
			return 0, false, fmt.Errorf("port_id %d exceeds maximum %d", portID, maxNodePortID)
		}
		return portID, true, nil
	case string:
		portID, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false, fmt.Errorf("invalid port_id %q", typed)
		}
		if portID < 0 {
			return 0, false, fmt.Errorf("invalid negative port_id %d", portID)
		}
		if portID > maxNodePortID {
			return 0, false, fmt.Errorf("port_id %d exceeds maximum %d", portID, maxNodePortID)
		}
		return portID, true, nil
	default:
		return 0, false, fmt.Errorf("invalid port_id %v", value)
	}
}

func schemaPortIndexes(id string, output bool) map[string]int {
	spec, ok := documentNodeSpecs[id]
	if !ok {
		return nil
	}
	if output {
		return spec.outputs
	}
	return spec.inputs
}

func normalizePortDefinitions(configs []PortDefinition, keyIndexes map[string]int) ([]PortDefinition, error) {
	result := make([]PortDefinition, 0, len(configs))
	for _, config := range configs {
		key := strings.TrimSpace(config.Key)
		if key != "" {
			// 新 schema 端口使用稳定 key；执行器需要数字 port_id，因此在边界处显式转换并校验漂移。
			index, exists := keyIndexes[key]
			if !exists && !config.HasPortID {
				return nil, fmt.Errorf("port key %q has no port_id mapping", key)
			}
			if exists {
				if config.HasPortID && config.PortID != index {
					return nil, fmt.Errorf("port key %q maps to port_id %d but declares %d", key, index, config.PortID)
				}
				config.PortID = index
				config.HasPortID = true
			}
		}
		result = append(result, config)
	}
	return result, nil
}

func executableConfigForSchemaID(id string) (ExecDefinitionConfig, bool) {
	switch id {
	case "origin.flow.range-compare":
		return ExecDefinitionConfig{
			ID:   id,
			Name: "RangeCompare",
			Inputs: []PortDefinition{
				{PortType: "exec", PortID: 0},
				{PortType: "data", DataType: "Integer", PortID: 1},
				{PortType: "data", DataType: "Array", PortID: 2},
			},
			Outputs: []PortDefinition{
				{PortType: "exec", PortID: 0},
				{PortType: "exec", PortID: 1},
			},
		}, true
	case "origin.flow.equal-switch-new":
		return ExecDefinitionConfig{
			ID:   id,
			Name: "EqualSwitch",
			Inputs: []PortDefinition{
				{PortType: "exec", PortID: 0},
				{PortType: "data", DataType: "Integer", PortID: 1},
				{PortType: "data", DataType: "Array", PortID: 2},
			},
			Outputs: []PortDefinition{
				{PortType: "exec", PortID: 0},
			},
		}, true
	case "origin.array.create-integer-new":
		return ExecDefinitionConfig{
			ID:   id,
			Name: "CreateIntArray",
			Inputs: []PortDefinition{
				{PortType: "data", DataType: "Array", PortID: 0},
			},
			Outputs: []PortDefinition{
				{PortType: "data", DataType: "Array", PortID: 0},
			},
		}, true
	case "origin.array.create-string-new":
		return ExecDefinitionConfig{
			ID:   id,
			Name: "CreateStringArray",
			Inputs: []PortDefinition{
				{PortType: "data", DataType: "Array", PortID: 0},
			},
			Outputs: []PortDefinition{
				{PortType: "data", DataType: "Array", PortID: 0},
			},
		}, true
	default:
		return ExecDefinitionConfig{}, false
	}
}

func dynamicBranchDefinitionOutputs(name string, outputs []PortDefinition) []PortDefinition {
	maxBranch := 0
	switch name {
	case "RangeCompare":
		maxBranch = 4
	case "EqualSwitch":
		maxBranch = 50
	default:
		return outputs
	}
	result := append([]PortDefinition(nil), outputs...)
	seen := make(map[int]bool, len(result))
	for _, output := range result {
		seen[output.PortID] = true
	}
	for portID := 0; portID <= maxBranch+1; portID++ {
		if !seen[portID] {
			result = append(result, PortDefinition{PortType: "exec", PortID: portID})
		}
	}
	return result
}

func parseLenientDefinitions(text string) ([]ExecDefinitionConfig, error) {
	objects := splitTopLevelObjects(text)
	configs := make([]ExecDefinitionConfig, 0, len(objects))
	for _, object := range objects {
		name := firstStringField(object, "name")
		if name == "" {
			continue
		}
		inputs, err := parseLenientPorts(arrayField(object, "inputs"), 0)
		if err != nil {
			return nil, fmt.Errorf("exec %s input ports: %w", name, err)
		}
		outputs, err := parseLenientPorts(arrayField(object, "outputs"), len(inputs))
		if err != nil {
			return nil, fmt.Errorf("exec %s output ports: %w", name, err)
		}
		configs = append(configs, ExecDefinitionConfig{
			Name:    name,
			Inputs:  inputs,
			Outputs: outputs,
		})
	}
	return configs, nil
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

func splitTopLevelObjectsLimited(text string, maximum int, field string) ([]string, error) {
	objects := make([]string, 0, min(maximum, 16))
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
				if len(objects) >= maximum {
					return nil, fmt.Errorf("%s %d exceeds maximum %d", field, len(objects)+1, maximum)
				}
				objects = append(objects, text[start:index+1])
				start = -1
			}
		}
	}
	return objects, nil
}

func firstStringField(text, field string) string {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*"([^"]*)"`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func firstIntField(text, field string) (int, bool) {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(field) + `"\s*:\s*(-?\d+)`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return value, true
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

func parseLenientPorts(text string, existingPortCount int) ([]PortDefinition, error) {
	remaining := maxNodePortCount - existingPortCount
	if remaining < 0 {
		return nil, fmt.Errorf("total port count %d exceeds maximum %d", existingPortCount, maxNodePortCount)
	}
	objects, err := splitTopLevelObjectsLimited(text, remaining, "port count")
	if err != nil {
		return nil, fmt.Errorf("total port count %d exceeds maximum %d", maxNodePortCount+1, maxNodePortCount)
	}
	ports := make([]PortDefinition, 0, len(objects))
	for _, object := range objects {
		portID, ok := firstIntField(object, "port_id")
		if !ok {
			continue
		}
		ports = append(ports, PortDefinition{
			PortType: firstStringField(object, "type"),
			DataType: firstStringField(object, "data_type"),
			PortID:   portID,
		})
	}
	return ports, nil
}

func buildPorts(configs []PortDefinition) ([]IPort, error) {
	if err := validateMaximum("port count", len(configs), maxNodePortCount); err != nil {
		return nil, err
	}
	seen := make(map[int]struct{}, len(configs))
	maxPortID := -1
	for _, config := range configs {
		if config.PortID < 0 {
			return nil, fmt.Errorf("port_id %d must be nonnegative", config.PortID)
		}
		if config.PortID > maxNodePortID {
			return nil, fmt.Errorf("port_id %d exceeds maximum %d", config.PortID, maxNodePortID)
		}
		if _, exists := seen[config.PortID]; exists {
			return nil, fmt.Errorf("duplicate port_id %d", config.PortID)
		}
		seen[config.PortID] = struct{}{}
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
	case "timerhandle", "timer_handle":
		return NewPortTimerHandle(), nil
	default:
		return nil, fmt.Errorf("invalid data type %s", dataType)
	}
}
