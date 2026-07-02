package blueprint

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Registry 保存节点名称到节点定义的映射。
type Registry struct {
	definitions map[string]*NodeDefinition
}

// NewRegistry 创建空节点定义注册表。
func NewRegistry() *Registry {
	return &Registry{definitions: map[string]*NodeDefinition{}}
}

// Register 注册一个节点定义。
//
// 已存在同名定义时返回 false，避免加载阶段静默覆盖。
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

// Get 按节点名称查找节点定义。
func (r *Registry) Get(name string) *NodeDefinition {
	if r == nil {
		return nil
	}
	return r.definitions[name]
}

// NodeConfig 是编译器使用的节点配置。
//
// 它同时兼容旧格式和新版文档格式转换后的中间结构。
type NodeConfig struct {
	ID                  string         `json:"id"`
	Class               string         `json:"class"`
	PortDefault         map[int]any    `json:"-"`
	RawDefault          map[string]any `json:"port_defaultv"`
	FunctionID          string         `json:"functionId,omitempty"`
	FunctionName        string         `json:"functionName,omitempty"`
	FunctionInputTypes  []string       `json:"functionInputTypes,omitempty"`
	FunctionOutputTypes []string       `json:"functionOutputTypes,omitempty"`
}

// EdgeConfig 描述两个节点端口之间的一条连接。
type EdgeConfig struct {
	SourceNodeID string `json:"source_node_id"`
	DesNodeID    string `json:"des_node_id"`
	SourcePortID int    `json:"source_port_id"`
	DesPortID    int    `json:"des_port_id"`
}

// GraphConfig 是编译蓝图图所需的完整配置。
type GraphConfig struct {
	Nodes     []NodeConfig     `json:"nodes"`
	Edges     []EdgeConfig     `json:"edges"`
	Variables []VariableConfig `json:"variables"`
	Functions map[string]*CompiledGraph
}

// VariableConfig 描述蓝图实例变量的初始值。
type VariableConfig struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// ParseGraphConfigJSON 解析蓝图 JSON。
//
// 新版文档格式会先转换为 GraphConfig，旧格式则直接反序列化。
func ParseGraphConfigJSON(data []byte) (GraphConfig, error) {
	var documentProbe struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &documentProbe); err == nil && documentProbe.SchemaVersion > 0 {
		var document graphDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return GraphConfig{}, err
		}
		config, _, err := graphDocumentToConfig(document)
		return config, err
	}

	var config GraphConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return GraphConfig{}, err
	}
	for index := range config.Nodes {
		config.Nodes[index].PortDefault = parsePortDefaults(config.Nodes[index].RawDefault)
	}
	return config, nil
}

// CompileGraph 将 GraphConfig 编译为可执行的只读图结构。
//
// 编译阶段会预处理节点、连接、变量和函数，以减少运行期查找开销。
func CompileGraph(registry *Registry, config GraphConfig) (*CompiledGraph, error) {
	nodes := make(map[string]*ExecNode, len(config.Nodes))
	nodeDefaults := make(map[string]map[int]any, len(config.Nodes))
	nodeOrder := make([]*ExecNode, 0, len(config.Nodes))
	entrances := map[int64]*ExecNode{}
	variables := make(map[string]VariableConfig, len(config.Variables))
	for _, variable := range config.Variables {
		variables[variable.Name] = variable
	}

	for _, nodeConfig := range config.Nodes {
		nodeName, entranceID, isEntrance := parseEntranceClass(nodeConfig.Class)
		if nodeConfig.Class == "FunctionEntry" {
			entranceID = FunctionEntranceID
			isEntrance = true
		}
		definition := registry.Get(nodeName)
		if definition == nil {
			var dynamicErr error
			definition, dynamicErr = dynamicDefinition(nodeConfig, variables)
			if dynamicErr != nil {
				return nil, dynamicErr
			}
			nodeName = definition.Name
		}

		node := NewExecNode(nodeConfig.ID, definition)
		node.Index = len(nodeOrder)
		node.VariableName = variableNameFromClass(nodeConfig.Class)
		node.FunctionID = nodeConfig.FunctionID
		node.FunctionName = nodeConfig.FunctionName
		node.FunctionGraph = resolveFunctionGraph(config.Functions, nodeConfig.FunctionID, nodeConfig.FunctionName)
		node.IsEntrance = isEntrance
		nodes[nodeConfig.ID] = node
		nodeOrder = append(nodeOrder, node)
		if isEntrance {
			entrances[entranceID] = node
		}
		nodeDefaults[nodeConfig.ID] = nodeConfig.PortDefault
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
			source.NextInPort[edge.SourcePortID] = edge.DesPortID
			dest.BeConnect = true
			continue
		}

		if edge.DesPortID < 0 || edge.DesPortID >= len(dest.PreInPort) {
			return nil, fmt.Errorf("destination node %s in port %d not found", edge.DesNodeID, edge.DesPortID)
		}
		dest.PreInPort[edge.DesPortID] = &PrePortNode{Node: source, OutPortID: edge.SourcePortID}
	}

	for _, node := range nodeOrder {
		defaultInputs, defaultInputSet, err := compileDefaultInputs(node.Definition.InPorts, node.PreInPort, nodeDefaults[node.ID])
		if err != nil {
			return nil, fmt.Errorf("node %s defaults: %w", node.ID, err)
		}
		node.DefaultInputs = defaultInputs
		node.DefaultInputSet = defaultInputSet
		node.InputBindings = compileInputBindings(node)
	}

	return &CompiledGraph{Entrances: entrances, Variables: variables, Functions: config.Functions, NodeCount: len(nodeOrder)}, nil
}

func compileInputBindings(node *ExecNode) []InputBinding {
	if node == nil || node.Definition == nil {
		return nil
	}
	bindings := make([]InputBinding, 0, len(node.Definition.DataInPortIndexes))
	for _, inputPortID := range node.Definition.DataInPortIndexes {
		if inputPortID < 0 {
			continue
		}
		if inputPortID < len(node.PreInPort) {
			pre := node.PreInPort[inputPortID]
			if pre != nil {
				bindings = append(bindings, InputBinding{
					Kind:              InputBindingProducer,
					InputPortID:       inputPortID,
					Producer:          pre.Node,
					ProducerOutPortID: pre.OutPortID,
					RecomputeProducer: pre.Node != nil && !pre.Node.BeConnect && !pre.Node.IsEntrance,
				})
				continue
			}
		}
		if inputPortID < len(node.DefaultInputSet) && node.DefaultInputSet[inputPortID] {
			bindings = append(bindings, InputBinding{
				Kind:        InputBindingDefault,
				InputPortID: inputPortID,
				DefaultPort: node.DefaultInputs[inputPortID],
			})
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	return bindings
}

func resolveFunctionGraph(functions map[string]*CompiledGraph, functionID string, functionName string) *CompiledGraph {
	if len(functions) == 0 {
		return nil
	}
	if functionID != "" {
		if graph := functions[functionID]; graph != nil {
			return graph
		}
	}
	if functionName != "" {
		return functions[functionName]
	}
	return nil
}

func compileDefaultInputs(inPorts []IPort, preInPorts []*PrePortNode, defaults map[int]any) ([]IPort, []bool, error) {
	if len(inPorts) == 0 || len(defaults) == 0 {
		return nil, nil, nil
	}
	defaultInputs := make([]IPort, len(inPorts))
	defaultInputSet := make([]bool, len(inPorts))
	for index, value := range defaults {
		if index < 0 || index >= len(inPorts) {
			continue
		}
		if index < len(preInPorts) && preInPorts[index] != nil {
			continue
		}
		port := inPorts[index]
		if port == nil || port.IsPortExec() {
			continue
		}
		clone := port.Clone()
		if err := clone.setAnyValue(value); err != nil {
			return nil, nil, fmt.Errorf("input port %d: %w", index, err)
		}
		defaultInputs[index] = clone
		defaultInputSet[index] = true
	}
	return defaultInputs, defaultInputSet, nil
}

// dynamicDefinition 为函数、变量等动态节点生成节点定义。
func dynamicDefinition(nodeConfig NodeConfig, variables map[string]VariableConfig) (*NodeDefinition, error) {
	switch nodeConfig.Class {
	case "FunctionEntry":
		return functionEntryDefinition(nodeConfig.FunctionInputTypes)
	case "FunctionReturn":
		return functionReturnDefinition(nodeConfig.FunctionOutputTypes)
	case "FunctionCall":
		return functionCallDefinition(nodeConfig.FunctionInputTypes, nodeConfig.FunctionOutputTypes)
	default:
		if definition := builtinDynamicDefinition(nodeConfig.Class); definition != nil {
			return definition, nil
		}
		return dynamicVariableDefinition(nodeConfig.Class, variables)
	}
}

func dynamicVariableDefinition(className string, variables map[string]VariableConfig) (*NodeDefinition, error) {
	varName := variableNameFromClass(className)
	if varName == "" {
		return nil, fmt.Errorf("%s node has not been registered", className)
	}
	variable, ok := variables[varName]
	if !ok {
		return nil, fmt.Errorf("variable %s not found", varName)
	}
	port, err := newPortFromDataType(variable.Type)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(className, "Get_") {
		return NewNodeDefinition("GetVariable", func() IExecNode { return &GetVariableNode{} }, nil, []IPort{port}), nil
	}
	return NewNodeDefinition("SetVariable", func() IExecNode { return &SetVariableNode{} }, []IPort{NewPortExec(), port}, []IPort{NewPortExec(), port.Clone()}), nil
}

func variableNameFromClass(className string) string {
	if strings.HasPrefix(className, "Get_") {
		return strings.TrimPrefix(className, "Get_")
	}
	if strings.HasPrefix(className, "Set_") {
		return strings.TrimPrefix(className, "Set_")
	}
	return ""
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
		n.NextInPort = append(n.NextInPort, 0)
	}
}
