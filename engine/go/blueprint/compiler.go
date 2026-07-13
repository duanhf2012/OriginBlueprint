package blueprint

import (
	"bytes"
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
	Legacy    bool `json:"-"`
}

type compiledExecEdge struct {
	source       *ExecNode
	destination  *ExecNode
	sourcePortID int
	destPortID   int
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
	present, _, err := probeGraphSchemaVersion(data)
	if err != nil {
		return GraphConfig{}, err
	}
	if present {
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
	config.Legacy = true
	for index := range config.Nodes {
		config.Nodes[index].PortDefault = parsePortDefaults(config.Nodes[index].RawDefault)
	}
	return config, nil
}

func probeGraphSchemaVersion(data []byte) (bool, int, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false, 0, err
	}
	raw, present := fields["schemaVersion"]
	if !present {
		return false, 0, nil
	}
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("1")) {
		return true, 0, fmt.Errorf("unsupported schemaVersion %s: expected integer 1", bytes.TrimSpace(raw))
	}
	return true, 1, nil
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
	execEdges := make([]compiledExecEdge, 0, len(config.Edges))
	for _, variable := range config.Variables {
		if strings.TrimSpace(variable.Name) == "" {
			return nil, fmt.Errorf("variable name is empty")
		}
		if _, exists := variables[variable.Name]; exists {
			return nil, fmt.Errorf("duplicate variable %q", variable.Name)
		}
		port, err := newPortFromDataType(variable.Type)
		if err != nil {
			return nil, fmt.Errorf("variable %s: %w", variable.Name, err)
		}
		if variable.Value != nil {
			if err := port.setAnyValue(variable.Value); err != nil {
				return nil, fmt.Errorf("variable %s default: %w", variable.Name, err)
			}
		}
		variables[variable.Name] = variable
	}

	for _, nodeConfig := range config.Nodes {
		if strings.TrimSpace(nodeConfig.ID) == "" {
			return nil, fmt.Errorf("node id is empty")
		}
		if _, exists := nodes[nodeConfig.ID]; exists {
			return nil, fmt.Errorf("duplicate node id %q", nodeConfig.ID)
		}
		nodeName, entranceID, isEntrance := parseEntranceClass(nodeConfig.Class)
		if nodeConfig.Class == "FunctionEntry" {
			entranceID = FunctionEntranceID
			isEntrance = true
		}
		definition := registry.Get(nodeName)
		if nodeConfig.Class == "SetTimerByFunction" {
			definition = nil
		}
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
			if _, exists := entrances[entranceID]; exists {
				return nil, fmt.Errorf("duplicate entrance id %d", entranceID)
			}
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
		if edge.SourcePortID < 0 || edge.SourcePortID >= len(source.Definition.OutPorts) {
			return nil, fmt.Errorf("source node %s out port %d not found", edge.SourceNodeID, edge.SourcePortID)
		}
		if edge.DesPortID < 0 || edge.DesPortID >= len(dest.Definition.InPorts) {
			return nil, fmt.Errorf("destination node %s in port %d not found", edge.DesNodeID, edge.DesPortID)
		}
		sourcePort := source.Definition.OutPorts[edge.SourcePortID]
		targetPort := dest.Definition.InPorts[edge.DesPortID]
		if portIsNil(sourcePort) || portIsNil(targetPort) {
			return nil, fmt.Errorf("connection %s:%d -> %s:%d uses nil port", edge.SourceNodeID, edge.SourcePortID, edge.DesNodeID, edge.DesPortID)
		}
		if !config.Legacy && sourcePort.IsPortExec() != targetPort.IsPortExec() {
			return nil, fmt.Errorf("connection %s:%d -> %s:%d mixes exec and data ports", edge.SourceNodeID, edge.SourcePortID, edge.DesNodeID, edge.DesPortID)
		}
		if !config.Legacy && !sourcePort.IsPortExec() && !portsCompatible(sourcePort, targetPort) {
			return nil, fmt.Errorf("connection %s:%d -> %s:%d has incompatible data ports", edge.SourceNodeID, edge.SourcePortID, edge.DesNodeID, edge.DesPortID)
		}

		if sourcePort.IsPortExec() {
			if !config.Legacy && edge.SourcePortID < len(source.Next) && source.Next[edge.SourcePortID] != nil {
				return nil, fmt.Errorf("source node %s exec port %d has multiple targets", edge.SourceNodeID, edge.SourcePortID)
			}
			execEdges = append(execEdges, compiledExecEdge{source: source, destination: dest, sourcePortID: edge.SourcePortID, destPortID: edge.DesPortID})
			source.ensureNext(edge.SourcePortID)
			source.Next[edge.SourcePortID] = dest
			source.NextInPort[edge.SourcePortID] = edge.DesPortID
			dest.BeConnect = true
			continue
		}

		if dest.PreInPort[edge.DesPortID] != nil {
			return nil, fmt.Errorf("destination node %s in port %d has multiple producers", edge.DesNodeID, edge.DesPortID)
		}
		dest.PreInPort[edge.DesPortID] = &PrePortNode{Node: source, OutPortID: edge.SourcePortID}
	}
	if err := validateDataDependencyCycles(nodeOrder); err != nil {
		return nil, err
	}
	if !config.Legacy {
		if err := validateExecCycles(nodeOrder, execEdges); err != nil {
			return nil, err
		}
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

func validateDataDependencyCycles(nodes []*ExecNode) error {
	indegree := make(map[*ExecNode]int, len(nodes))
	adjacency := make(map[*ExecNode][]*ExecNode, len(nodes))
	for _, node := range nodes {
		indegree[node] = 0
	}
	for _, consumer := range nodes {
		for _, producer := range consumer.PreInPort {
			if producer == nil || producer.Node == nil {
				continue
			}
			adjacency[producer.Node] = append(adjacency[producer.Node], consumer)
			indegree[consumer]++
		}
	}
	queue := make([]*ExecNode, 0, len(nodes))
	for _, node := range nodes {
		if indegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	processed := 0
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		processed++
		for _, consumer := range adjacency[node] {
			indegree[consumer]--
			if indegree[consumer] == 0 {
				queue = append(queue, consumer)
			}
		}
	}
	if processed == len(nodes) {
		return nil
	}
	ids := make([]string, 0, len(nodes)-processed)
	for _, node := range nodes {
		if indegree[node] > 0 {
			ids = append(ids, node.ID)
		}
	}
	return fmt.Errorf("data dependency cycle: %s", strings.Join(ids, ", "))
}

func validateExecCycles(nodes []*ExecNode, edges []compiledExecEdge) error {
	candidateBreak := make(map[int]bool)
	baseAdjacency := make(map[*ExecNode][]compiledExecEdge, len(nodes))
	baseIndegree := make(map[*ExecNode]int, len(nodes))
	for _, node := range nodes {
		baseIndegree[node] = 0
	}
	for index, edge := range edges {
		isBreak := edge.destination != nil && edge.destination.Definition != nil && edge.destination.Definition.Name == "ForLoopBreak" && edge.destPortID == 3
		if isBreak {
			candidateBreak[index] = true
			continue
		}
		baseAdjacency[edge.source] = append(baseAdjacency[edge.source], edge)
		baseIndegree[edge.destination]++
	}
	roots := make([]*ExecNode, 0, len(nodes))
	for _, node := range nodes {
		if baseIndegree[node] == 0 {
			roots = append(roots, node)
		}
	}

	allowedBreak := make(map[int]bool)
	for index := range candidateBreak {
		edge := edges[index]
		loop := edge.destination
		if edge.source == loop && edge.sourcePortID == 0 {
			allowedBreak[index] = true
			continue
		}
		bodyStarts := make([]*ExecNode, 0, 1)
		for _, next := range baseAdjacency[loop] {
			if next.sourcePortID == 0 {
				bodyStarts = append(bodyStarts, next.destination)
			}
		}
		bodyReach := reachableExecNodes(bodyStarts, baseAdjacency, nil)
		if !bodyReach[edge.source] {
			continue
		}
		reachableWithoutBody := reachableExecNodes(roots, baseAdjacency, func(candidate compiledExecEdge) bool {
			return candidate.source == loop && candidate.sourcePortID == 0
		})
		if !reachableWithoutBody[edge.source] {
			allowedBreak[index] = true
		}
	}

	indegree := make(map[*ExecNode]int, len(nodes))
	adjacency := make(map[*ExecNode][]*ExecNode, len(nodes))
	for _, node := range nodes {
		indegree[node] = 0
	}
	for index, edge := range edges {
		if allowedBreak[index] {
			continue
		}
		adjacency[edge.source] = append(adjacency[edge.source], edge.destination)
		indegree[edge.destination]++
	}
	queue := make([]*ExecNode, 0, len(nodes))
	for _, node := range nodes {
		if indegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	processed := 0
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		processed++
		for _, next := range adjacency[node] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if processed == len(nodes) {
		return nil
	}
	ids := make([]string, 0, len(nodes)-processed)
	for _, node := range nodes {
		if indegree[node] > 0 {
			ids = append(ids, node.ID)
		}
	}
	return fmt.Errorf("exec cycle: %s", strings.Join(ids, ", "))
}

func reachableExecNodes(starts []*ExecNode, adjacency map[*ExecNode][]compiledExecEdge, skip func(compiledExecEdge) bool) map[*ExecNode]bool {
	reached := make(map[*ExecNode]bool, len(starts))
	queue := append([]*ExecNode(nil), starts...)
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		if node == nil || reached[node] {
			continue
		}
		reached[node] = true
		for _, edge := range adjacency[node] {
			if skip != nil && skip(edge) {
				continue
			}
			queue = append(queue, edge.destination)
		}
	}
	return reached
}

func portsCompatible(source, target IPort) bool {
	sourcePort, sourceBuiltin := source.(*Port)
	targetPort, targetBuiltin := target.(*Port)
	if !sourceBuiltin || !targetBuiltin {
		return true
	}
	return sourcePort.kind == portKindAny || targetPort.kind == portKindAny || sourcePort.kind == targetPort.kind
}

func portIsNil(port IPort) bool {
	if port == nil {
		return true
	}
	builtin, ok := port.(*Port)
	return ok && builtin == nil
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
	case "SetTimerByFunction":
		return setTimerByFunctionDefinition(nodeConfig.FunctionInputTypes)
	default:
		if definition := dynamicSequenceDefinition(nodeConfig.Class); definition != nil {
			return definition, nil
		}
		if definition := builtinDynamicDefinition(nodeConfig.Class); definition != nil {
			return definition, nil
		}
		return dynamicVariableDefinition(nodeConfig.Class, variables)
	}
}

func dynamicSequenceDefinition(className string) *NodeDefinition {
	if !strings.HasPrefix(className, "SequenceDynamic") {
		return nil
	}
	count, err := strconv.Atoi(strings.TrimPrefix(className, "SequenceDynamic"))
	if err != nil || count <= 0 {
		return nil
	}
	return NewNodeDefinition("Sequence", func() IExecNode { return &Sequence{} }, []IPort{NewPortExec()}, execPortList(count))
}

func execPortList(count int) []IPort {
	ports := make([]IPort, count)
	for index := range ports {
		ports[index] = NewPortExec()
	}
	return ports
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
