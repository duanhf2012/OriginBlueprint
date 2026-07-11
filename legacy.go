package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type legacyGraph struct {
	GraphName string                   `json:"graph_name"`
	Time      string                   `json:"time"`
	Nodes     []legacyNode             `json:"nodes"`
	Edges     []legacyEdge             `json:"edges"`
	Groups    []legacyGroup            `json:"groups"`
	Variables []map[string]interface{} `json:"variables"`
}

type legacyNode struct {
	ID           string                 `json:"id"`
	Class        string                 `json:"class"`
	Module       string                 `json:"module"`
	Position     []float64              `json:"pos"`
	PortDefaults map[string]interface{} `json:"port_defaultv"`
}

type legacyEdge struct {
	EdgeID                 string      `json:"edge_id,omitempty"`
	SourceNodeID           string      `json:"source_node_id"`
	SourceIndex            int         `json:"source_port_index"`
	SourcePortID           interface{} `json:"source_port_id"`
	TargetNodeID           string      `json:"des_node_id"`
	TargetIndex            int         `json:"des_port_index"`
	TargetPortID           interface{} `json:"des_port_id"`
	EntryConnectionVisible bool        `json:"entryConnectionVisible,omitempty"`
}

type legacyGroup struct {
	Title string   `json:"title"`
	Nodes []string `json:"nodes"`
}
type legacyNodeSpec struct {
	TypeID          string
	Inputs, Outputs []string
}

type runtimeLegacySpec struct {
	legacyNodeSpec
	InputPorts, OutputPorts []GraphLegacyPort
}

type legacyExportEdge struct {
	edge     legacyEdge
	ordinal  *int
	sequence int
}

type legacyRuntimeNodeDefinition struct {
	Name    string                 `json:"name"`
	ID      string                 `json:"id"`
	Inputs  []legacyPortDefinition `json:"inputs"`
	Outputs []legacyPortDefinition `json:"outputs"`
}

type legacyPortDefinition struct {
	PortID   interface{} `json:"port_id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	DataType string      `json:"data_type"`
}

var legacyNodeSpecs = map[string]legacyNodeSpec{
	"BeginNode":                  {"origin.event.begin", nil, []string{"exec"}},
	"ForLoop":                    {"origin.flow.for-loop", []string{"exec", "start", "end"}, []string{"body", "index", "completed"}},
	"Foreach":                    {"origin.flow.for-loop", []string{"exec", "start", "end"}, []string{"body", "completed", "index"}},
	"BranchNode":                 {"origin.flow.branch", []string{"exec", "condition"}, []string{"false", "true"}},
	"BoolIf":                     {"origin.flow.branch", []string{"exec", "condition"}, []string{"false", "true"}},
	"PrintNode":                  {"origin.action.print", []string{"exec", "value"}, []string{"exec"}},
	"int -> str":                 {"origin.cast.integer-string", []string{"value"}, []string{"result"}},
	"Integer2String":             {"origin.cast.integer-string", []string{"value"}, []string{"result"}},
	"float -> str":               {"origin.cast.float-string", []string{"value"}, []string{"result"}},
	"AddInt":                     {"origin.math.add-integer", []string{"a", "b"}, []string{"result"}},
	"+ (Integer)":                {"origin.math.add-integer", []string{"a", "b"}, []string{"result"}},
	"SubInt":                     {"origin.math.subtract-integer", []string{"a", "b", "absolute"}, []string{"result"}},
	"MulInt":                     {"origin.math.multiply-integer", []string{"a", "b"}, []string{"result"}},
	"DivInt":                     {"origin.math.divide-integer", []string{"a", "b", "round"}, []string{"result"}},
	"ModInt":                     {"origin.math.modulo-integer", []string{"a", "b"}, []string{"result"}},
	"RandNumber":                 {"origin.math.random-integer", []string{"seed", "min", "max"}, []string{"result"}},
	"Sequence":                   {"origin.flow.sequence", []string{"exec"}, []string{"then0", "then1", "then2"}},
	"GreaterThanInteger":         {"origin.flow.greater-integer", []string{"exec", "orEqual", "a", "b"}, []string{"false", "true"}},
	"LessThanInteger":            {"origin.flow.less-integer", []string{"exec", "orEqual", "a", "b"}, []string{"false", "true"}},
	"EqualInteger":               {"origin.flow.equal-integer", []string{"exec", "a", "b"}, []string{"false", "true"}},
	"RangeCompare":               {"origin.flow.range-compare", []string{"exec", "value", "ranges"}, []string{"otherwise", "case0", "case1", "case2", "case3", "case4"}},
	"EqualSwitch":                {"origin.flow.equal-switch", []string{"exec", "value", "cases"}, []string{"otherwise", "case0", "case1", "case2", "case3", "case4"}},
	"GetArrayInt":                {"origin.array.get-integer", []string{"array", "index"}, []string{"value"}},
	"GetArrayString":             {"origin.array.get-string", []string{"array", "index"}, []string{"value"}},
	"GetArrayLen":                {"origin.array.length", []string{"array"}, []string{"length"}},
	"CreateIntArray":             {"origin.array.create-integer", []string{"items"}, []string{"array"}},
	"CreateStringArray":          {"origin.array.create-string", []string{"items"}, []string{"array"}},
	"StringArray":                {"origin.array.create-string", []string{"items"}, []string{"array"}},
	"AppendStringToArray":        {"origin.array.append-string", []string{"array", "value"}, []string{"array"}},
	"AppendIntegerToArray":       {"origin.array.append-integer", []string{"array", "value"}, []string{"array"}},
	"AppendIntReturn":            {"origin.result.append-integer", []string{"exec", "value"}, []string{"exec"}},
	"AppendStringReturn":         {"origin.result.append-string", []string{"exec", "value"}, []string{"exec"}},
	"Entrance_ArrayParam_000002": {"origin.event.entry-array", nil, []string{"exec", "objectId", "params"}},
	"Entrance_IntParam_000001":   {"origin.event.entry-two-integers", nil, []string{"exec", "objectId", "param1", "param2"}},
	"ForeachIntArray":            {"origin.flow.foreach-integer-array", []string{"exec", "array"}, []string{"body", "completed", "index", "value"}},
	"Probability":                {"origin.flow.probability", []string{"exec", "probability"}, []string{"miss", "hit"}},
	"DebugOutput":                {"origin.debug.output", []string{"exec", "integer", "string", "array"}, []string{"exec"}},
	"StringNode":                 {"origin.literal.string", []string{"value"}, []string{"value"}},
	"AddNode":                    {"origin.math.add-float", []string{"a", "b"}, []string{"result"}},
	"MinusNode":                  {"origin.math.subtract-float", []string{"a", "b"}, []string{"result"}},
	"MultiplyNode":               {"origin.math.multiply-float", []string{"a", "b"}, []string{"result"}},
	"DivideNode":                 {"origin.math.divide-float", []string{"a", "b"}, []string{"result"}},
	"GreaterIntegerNode":         {"origin.compare.greater-integer", []string{"a", "b"}, []string{"result", "a", "b"}},
	"WhileNode":                  {"origin.flow.while", []string{"exec", "condition"}, []string{"body", "completed"}},
	"ForLoopWithBreak":           {"origin.flow.for-loop-break", []string{"exec", "start", "end", "break"}, []string{"body", "index", "completed"}},
	"Length (Array)":             {"origin.array.length", []string{"array"}, []string{"length"}},
	"ForEahcNode":                {"origin.flow.foreach-array", []string{"exec", "array"}, []string{"body", "index", "value", "completed"}},
	"Split":                      {"origin.string.split", []string{"exec", "text", "delimiter"}, []string{"exec", "array"}},
	"Get (Array)":                {"origin.array.get-any", []string{"array", "index"}, []string{"value"}},
	"Cast To":                    {"origin.cast.any-string", []string{"exec", "value"}, []string{"exec", "result"}},
	"CastingNode_str":            {"origin.cast.any-string", []string{"exec", "value"}, []string{"exec", "valid", "result"}},
}

var preferredLegacyExportClassByType = map[string]string{
	"origin.flow.for-loop":       "Foreach",
	"origin.flow.branch":         "BoolIf",
	"origin.cast.integer-string": "Integer2String",
	"origin.math.add-integer":    "AddInt",
	"origin.array.length":        "GetArrayLen",
	"origin.array.create-string": "CreateStringArray",
	"origin.cast.any-string":     "Cast To",
}

func (a *App) MigrateLegacyGraph(content string) (string, error) {
	document, err := migrateLegacyGraph([]byte(content))
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(document)
	return string(data), err
}

func (a *App) ExportLegacyGraph(content string) (string, error) {
	var document GraphDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return "", fmt.Errorf("decode graph document: %w", err)
	}
	data, err := exportLegacyGraph(document)
	return string(data), err
}

func migrateLegacyGraph(data []byte) (GraphDocument, error) {
	var legacy legacyGraph
	if err := json.Unmarshal(data, &legacy); err != nil {
		return GraphDocument{}, fmt.Errorf("decode legacy graph: %w", err)
	}
	runtimeSpecs := runtimeLegacyNodeSpecs()
	document := GraphDocument{
		SchemaVersion:  GraphSchemaVersion,
		GraphName:      legacy.GraphName,
		View:           GraphView{Zoom: 1},
		VariableGroups: []GraphVariableGroup{{ID: "default", Name: "Default"}},
		Legacy:         &GraphLegacyState{Format: "vgf", Time: legacy.Time, Groups: cloneLegacyGroups(legacy.Groups), Variables: cloneLegacyVariables(legacy.Variables)},
	}
	variableIDs := map[string]string{}
	groupIDs := map[string]string{"Default": "default"}
	for _, item := range legacy.Variables {
		name := fmt.Sprint(item["name"])
		groupName := fmt.Sprint(item["group"])
		if groupName == "" {
			groupName = "Default"
		}
		groupID := groupIDs[groupName]
		if groupID == "" {
			groupID = uuid.NewString()
			groupIDs[groupName] = groupID
			document.VariableGroups = append(document.VariableGroups, GraphVariableGroup{ID: groupID, Name: groupName})
		}
		id := uuid.NewString()
		variableIDs[name] = id
		document.Variables = append(document.Variables, GraphVariable{ID: id, Name: name, Type: legacyVariableType(item["type"]), DefaultValue: item["value"], GroupID: groupID})
	}
	nodeByID := map[string]int{}
	nodeSpecs := map[string]runtimeLegacySpec{}
	maxInputs, maxOutputs := map[string]int{}, map[string]int{}
	inputIndexes, outputIndexes := map[string]map[int]bool{}, map[string]map[int]bool{}
	for _, edge := range legacy.Edges {
		sourceIndex := legacyPortIndex(edge.SourcePortID, edge.SourceIndex)
		targetIndex := legacyPortIndex(edge.TargetPortID, edge.TargetIndex)
		if outputIndexes[edge.SourceNodeID] == nil {
			outputIndexes[edge.SourceNodeID] = map[int]bool{}
		}
		outputIndexes[edge.SourceNodeID][sourceIndex] = true
		if inputIndexes[edge.TargetNodeID] == nil {
			inputIndexes[edge.TargetNodeID] = map[int]bool{}
		}
		inputIndexes[edge.TargetNodeID][targetIndex] = true
		if current, exists := maxOutputs[edge.SourceNodeID]; !exists || sourceIndex > current {
			maxOutputs[edge.SourceNodeID] = sourceIndex
		}
		if current, exists := maxInputs[edge.TargetNodeID]; !exists || targetIndex > current {
			maxInputs[edge.TargetNodeID] = targetIndex
		}
	}
	fallbackSpecs := inferredRuntimeFallbackSpecs(legacy.Nodes, legacy.Edges, runtimeSpecs, maxInputs, maxOutputs)
	for _, item := range legacy.Nodes {
		spec, known := runtimeSpecs[item.Class]
		if item.Class == "EqualSwitch" && legacyEqualSwitchUsesExpandedBranches(item, maxOutputs[item.ID]) {
			// EqualSwitch 也是 equal-switch-new 的 legacy 导出 class。
			// 如果旧文件里已使用超过 case4 的分支，迁移回扩展后的新节点以保留高编号端口。
			spec = runtimeLegacySpec{legacyNodeSpec: legacyEqualSwitchNewSpec()}
			known = true
		}
		if !runtimeLegacySpecHasPorts(spec, false, inputIndexes[item.ID]) || !runtimeLegacySpecHasPorts(spec, true, outputIndexes[item.ID]) {
			known = false
		}
		properties := GraphNodeProperties{LegacyClass: item.Class, LegacyModule: item.Module}
		if strings.HasPrefix(item.Class, "Get_") || strings.HasPrefix(item.Class, "Set_") {
			access := "get"
			name := strings.TrimPrefix(item.Class, "Get_")
			if strings.HasPrefix(item.Class, "Set_") {
				access = "set"
				name = strings.TrimPrefix(item.Class, "Set_")
			}
			if id := variableIDs[name]; id != "" {
				spec.TypeID = "origin.variable." + access
				properties.VariableID = id
				properties.VariableAccess = access
				known = true
				if access == "get" {
					spec.Outputs = []string{"value"}
				} else {
					spec.Inputs = []string{"exec", "value"}
					spec.Outputs = []string{"exec", "value"}
				}
			}
		}
		if !known {
			if fallback, exists := fallbackSpecs[item.ID]; exists {
				spec = fallback
				known = true
			} else {
				document.Legacy.HiddenNodes = append(document.Legacy.HiddenNodes, cloneLegacyNode(item))
				continue
			}
		}
		if _, staticallyKnown := graphNodePorts[spec.TypeID]; !staticallyKnown {
			properties.LegacyInputs = legacyPortsFromRuntimeSpec(spec.Inputs, spec.InputPorts, "in")
			properties.LegacyOutputs = legacyPortsFromRuntimeSpec(spec.Outputs, spec.OutputPorts, "out")
		}
		nodeSpecs[item.ID] = spec
		position := GraphPosition{}
		if len(item.Position) > 0 {
			position.X = item.Position[0]
		}
		if len(item.Position) > 1 {
			position.Y = item.Position[1]
		}
		values, residualDefaults := mapLegacyNodeDefaults(item.PortDefaults, spec.Inputs)
		node := GraphNode{ID: item.ID, TypeID: spec.TypeID, Position: position, Values: values, Properties: properties}
		if len(residualDefaults) > 0 {
			if document.Legacy.ResidualNodeDefaults == nil {
				document.Legacy.ResidualNodeDefaults = map[string]GraphLegacyResidualDefaults{}
			}
			document.Legacy.ResidualNodeDefaults[item.ID] = GraphLegacyResidualDefaults{Class: item.Class, Values: residualDefaults}
		}
		document.Nodes = append(document.Nodes, node)
		nodeByID[item.ID] = len(document.Nodes) - 1
	}
	for ordinal, edge := range legacy.Edges {
		sourceIndex := legacyPortIndex(edge.SourcePortID, edge.SourceIndex)
		targetIndex := legacyPortIndex(edge.TargetPortID, edge.TargetIndex)
		sourceSpec := nodeSpecs[edge.SourceNodeID]
		targetSpec := nodeSpecs[edge.TargetNodeID]
		_, sourceExists := nodeByID[edge.SourceNodeID]
		_, targetExists := nodeByID[edge.TargetNodeID]
		if !sourceExists || !targetExists {
			document.Legacy.HiddenEdges = append(document.Legacy.HiddenEdges, cloneLegacyEdge(edge))
			document.Legacy.HiddenEdgeOrdinals = append(document.Legacy.HiddenEdgeOrdinals, ordinal)
			continue
		}
		sourceKey := indexedKey(sourceSpec.Outputs, sourceIndex, "out")
		targetKey := indexedKey(targetSpec.Inputs, targetIndex, "in")
		legacyOrdinal := ordinal
		document.Connections = append(document.Connections, GraphConnection{Source: edge.SourceNodeID, SourceOutput: sourceKey, Target: edge.TargetNodeID, TargetInput: targetKey, EntryConnectionVisible: edge.EntryConnectionVisible, LegacyEdgeID: edge.EdgeID, LegacyOrdinal: &legacyOrdinal})
	}
	for _, group := range legacy.Groups {
		positions := make([]GraphPosition, 0)
		ids := make([]string, 0)
		for _, id := range group.Nodes {
			if nodeIndex, ok := nodeByID[id]; ok {
				positions = append(positions, document.Nodes[nodeIndex].Position)
				ids = append(ids, id)
			}
		}
		if len(positions) > 0 {
			minX, maxX, minY, maxY := positions[0].X, positions[0].X, positions[0].Y, positions[0].Y
			for _, p := range positions[1:] {
				if p.X < minX {
					minX = p.X
				}
				if p.X > maxX {
					maxX = p.X
				}
				if p.Y < minY {
					minY = p.Y
				}
				if p.Y > maxY {
					maxY = p.Y
				}
			}
			document.Groups = append(document.Groups, GraphGroup{ID: uuid.NewString(), Title: group.Title, X: minX - 30, Y: minY - 45, Width: maxX - minX + 300, Height: maxY - minY + 180, NodeIDs: ids})
		}
	}
	return document, nil
}

func exportLegacyGraph(document GraphDocument) ([]byte, error) {
	runtimeSpecs := runtimeLegacyNodeSpecs()
	specByType := map[string]runtimeLegacySpec{}
	classByType := map[string]string{}
	classes := make([]string, 0, len(runtimeSpecs))
	for class := range runtimeSpecs {
		classes = append(classes, class)
	}
	sort.Strings(classes)
	for _, class := range classes {
		spec := runtimeSpecs[class]
		if _, exists := specByType[spec.TypeID]; !exists {
			specByType[spec.TypeID] = spec
			classByType[spec.TypeID] = class
		}
	}
	variablesByID := map[string]GraphVariable{}
	for _, variable := range document.Variables {
		variablesByID[variable.ID] = variable
	}

	legacy := legacyGraph{GraphName: document.GraphName, Groups: legacyGroups(document), Variables: legacyVariables(document)}
	if document.Legacy != nil {
		legacy.Time = document.Legacy.Time
	}

	nodeSpecs := map[string]runtimeLegacySpec{}
	nodeIDs := map[string]bool{}
	for _, node := range document.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return nil, fmt.Errorf("legacy export node id is empty")
		}
		if nodeIDs[node.ID] {
			return nil, fmt.Errorf("legacy export duplicate node id %q", node.ID)
		}
		class := node.Properties.LegacyClass
		module := node.Properties.LegacyModule
		spec := runtimeLegacySpec{}
		if node.TypeID == "origin.flow.equal-switch-new" {
			// 外部旧解析器仍期望 EqualSwitch class；新编辑器可创建 case1..case50。
			// 导出时使用更宽的端口表，避免静默丢失 case5+ 连线。
			class = "EqualSwitch"
			spec = runtimeLegacySpec{legacyNodeSpec: legacyEqualSwitchNewSpec()}
		}
		if node.TypeID == "origin.array.create-integer-new" {
			class = "CreateIntArray"
			spec = runtimeLegacySpec{legacyNodeSpec: legacyNodeSpecs[class]}
		}
		if node.TypeID == "origin.array.create-string-new" {
			class = "CreateStringArray"
			spec = runtimeLegacySpec{legacyNodeSpec: legacyNodeSpecs[class]}
		}
		if node.TypeID == "origin.variable.get" || node.TypeID == "origin.variable.set" {
			variable, exists := variablesByID[node.Properties.VariableID]
			if !exists || variable.Name == "" {
				return nil, fmt.Errorf("legacy export node %q references missing variable %q", node.ID, node.Properties.VariableID)
			}
			if node.TypeID == "origin.variable.get" {
				if class == "" {
					class = "Get_" + variable.Name
				}
				spec = runtimeLegacySpec{
					legacyNodeSpec: legacyNodeSpec{TypeID: node.TypeID, Outputs: []string{"value"}},
					OutputPorts:    []GraphLegacyPort{{Key: "value", Type: variable.Type}},
				}
			} else {
				if class == "" {
					class = "Set_" + variable.Name
				}
				spec = runtimeLegacySpec{
					legacyNodeSpec: legacyNodeSpec{TypeID: node.TypeID, Inputs: []string{"exec", "value"}, Outputs: []string{"exec", "value"}},
					InputPorts: []GraphLegacyPort{
						{Key: "exec", Type: "exec"},
						{Key: "value", Type: variable.Type},
					},
					OutputPorts: []GraphLegacyPort{
						{Key: "exec", Type: "exec"},
						{Key: "value", Type: variable.Type},
					},
				}
			}
			if module == "" {
				module = "nodes.VariableNode"
			}
		}
		if class == "" {
			class = preferredLegacyExportClassByType[node.TypeID]
		}
		if class == "" {
			class = classByType[node.TypeID]
		}
		if class == "" {
			return nil, fmt.Errorf("legacy export node %q type %q has no legacy class", node.ID, node.TypeID)
		}
		if spec.TypeID == "" {
			spec = runtimeSpecs[class]
		}
		if spec.TypeID == "" && (node.TypeID == "origin.legacy.placeholder" || len(node.Properties.LegacyInputs) > 0 || len(node.Properties.LegacyOutputs) > 0) {
			spec = runtimeLegacySpec{
				legacyNodeSpec: legacyNodeSpec{
					TypeID:  node.TypeID,
					Inputs:  legacyPortKeysFromPorts(node.Properties.LegacyInputs),
					Outputs: legacyPortKeysFromPorts(node.Properties.LegacyOutputs),
				},
				InputPorts:  node.Properties.LegacyInputs,
				OutputPorts: node.Properties.LegacyOutputs,
			}
		}
		if spec.TypeID == "" {
			spec = specByType[node.TypeID]
		}
		if spec.TypeID == "" {
			return nil, fmt.Errorf("legacy export node %q class %q has no legacy port specification", node.ID, class)
		}
		nodeSpecs[node.ID] = spec
		nodeIDs[node.ID] = true
		portDefaults := map[string]interface{}{}
		if document.Legacy != nil {
			if residual, exists := document.Legacy.ResidualNodeDefaults[node.ID]; exists {
				if residual.Class != class {
					return nil, fmt.Errorf("legacy export node %q residual defaults belong to class %q, not %q", node.ID, residual.Class, class)
				}
				portDefaults = cloneInterfaceMap(residual.Values)
			}
		}
		for key, value := range node.Values {
			if index, ok := legacyKeyIndex(spec.Inputs, key, "in"); ok {
				portDefaults[strconv.Itoa(index)] = value
			}
		}
		if module == "" {
			module = "tools.json_node_loader"
		}
		legacy.Nodes = append(legacy.Nodes, legacyNode{
			ID:           node.ID,
			Class:        class,
			Module:       module,
			Position:     []float64{node.Position.X, node.Position.Y},
			PortDefaults: portDefaults,
		})
	}
	if document.Legacy != nil {
		for _, node := range document.Legacy.HiddenNodes {
			if strings.TrimSpace(node.ID) == "" {
				return nil, fmt.Errorf("legacy export hidden node id is empty")
			}
			if nodeIDs[node.ID] {
				return nil, fmt.Errorf("legacy export duplicate node id %q", node.ID)
			}
			legacy.Nodes = append(legacy.Nodes, cloneLegacyNode(node))
			nodeIDs[node.ID] = true
		}
	}

	exportEdges := make([]legacyExportEdge, 0, len(document.Connections))
	usedOrdinals := map[int]string{}
	sequence := 0
	for connectionIndex, connection := range document.Connections {
		sourceSpec, sourceOK := nodeSpecs[connection.Source]
		targetSpec, targetOK := nodeSpecs[connection.Target]
		if !sourceOK || !targetOK {
			return nil, fmt.Errorf("legacy export connection %d references unmappable nodes %q -> %q", connectionIndex, connection.Source, connection.Target)
		}
		sourceIndex, sourcePortOK := legacyKeyIndex(sourceSpec.Outputs, connection.SourceOutput, "out")
		targetIndex, targetPortOK := legacyKeyIndex(targetSpec.Inputs, connection.TargetInput, "in")
		if !sourcePortOK || !targetPortOK {
			return nil, fmt.Errorf("legacy export connection %d has unmappable ports %q -> %q", connectionIndex, connection.SourceOutput, connection.TargetInput)
		}
		sourceType := runtimeLegacySpecPortType(sourceSpec, true, sourceIndex)
		targetType := runtimeLegacySpecPortType(targetSpec, false, targetIndex)
		if !legacyPortTypesCompatible(sourceType, targetType) && connection.LegacyOrdinal == nil {
			return nil, fmt.Errorf("legacy export connection %d has incompatible port types %q -> %q", connectionIndex, sourceType, targetType)
		}
		edgeID := connection.LegacyEdgeID
		if edgeID == "" && connection.LegacyOrdinal == nil {
			edgeID = uuid.NewString()
		}
		if err := registerLegacyExportOrdinal(usedOrdinals, connection.LegacyOrdinal, fmt.Sprintf("connection %d", connectionIndex)); err != nil {
			return nil, err
		}
		exportEdges = append(exportEdges, legacyExportEdge{edge: legacyEdge{
			EdgeID:                 edgeID,
			SourceNodeID:           connection.Source,
			SourceIndex:            sourceIndex,
			SourcePortID:           sourceIndex,
			TargetNodeID:           connection.Target,
			TargetIndex:            targetIndex,
			TargetPortID:           targetIndex,
			EntryConnectionVisible: connection.EntryConnectionVisible,
		}, ordinal: connection.LegacyOrdinal, sequence: sequence})
		sequence++
	}
	if document.Legacy != nil {
		hasHiddenOrdinals := len(document.Legacy.HiddenEdgeOrdinals) > 0
		if hasHiddenOrdinals && len(document.Legacy.HiddenEdgeOrdinals) != len(document.Legacy.HiddenEdges) {
			return nil, fmt.Errorf("legacy export hidden edge ordinals length %d does not match hidden edges %d", len(document.Legacy.HiddenEdgeOrdinals), len(document.Legacy.HiddenEdges))
		}
		for hiddenIndex, edge := range document.Legacy.HiddenEdges {
			if !nodeIDs[edge.SourceNodeID] || !nodeIDs[edge.TargetNodeID] {
				return nil, fmt.Errorf("legacy export hidden edge %q references missing nodes %q -> %q", edge.EdgeID, edge.SourceNodeID, edge.TargetNodeID)
			}
			var ordinal *int
			if hasHiddenOrdinals {
				value := document.Legacy.HiddenEdgeOrdinals[hiddenIndex]
				ordinal = &value
			}
			if err := registerLegacyExportOrdinal(usedOrdinals, ordinal, fmt.Sprintf("hidden edge %q", edge.EdgeID)); err != nil {
				return nil, err
			}
			exportEdges = append(exportEdges, legacyExportEdge{edge: cloneLegacyEdge(edge), ordinal: ordinal, sequence: sequence})
			sequence++
		}
	}
	sort.SliceStable(exportEdges, func(i, j int) bool {
		left, right := exportEdges[i], exportEdges[j]
		if left.ordinal == nil {
			return right.ordinal == nil && left.sequence < right.sequence
		}
		if right.ordinal == nil {
			return true
		}
		return *left.ordinal < *right.ordinal
	})
	legacy.Edges = make([]legacyEdge, 0, len(exportEdges))
	for _, item := range exportEdges {
		legacy.Edges = append(legacy.Edges, item.edge)
	}

	return json.MarshalIndent(legacy, "", "  ")
}

func registerLegacyExportOrdinal(used map[int]string, ordinal *int, owner string) error {
	if ordinal == nil {
		return nil
	}
	if *ordinal < 0 {
		return fmt.Errorf("legacy export %s has negative ordinal %d", owner, *ordinal)
	}
	if previous, exists := used[*ordinal]; exists {
		return fmt.Errorf("legacy export ordinal %d is shared by %s and %s", *ordinal, previous, owner)
	}
	used[*ordinal] = owner
	return nil
}

func canonicalLegacyDefaultIndex(raw string) (int, bool) {
	index, err := strconv.Atoi(raw)
	return index, err == nil && index >= 0 && strconv.Itoa(index) == raw
}

func mapLegacyNodeDefaults(defaults map[string]interface{}, inputs []string) (map[string]interface{}, map[string]interface{}) {
	values := map[string]interface{}{}
	residual := map[string]interface{}{}
	for rawIndex, value := range defaults {
		index, mapped := canonicalLegacyDefaultIndex(rawIndex)
		if mapped {
			if key, exists := legacyPortKeyAtIndex(inputs, index, "in"); exists {
				values[key] = value
				continue
			}
		}
		residual[rawIndex] = value
	}
	return values, residual
}

type inferredRuntimeFallbackPortSet struct {
	inputs  map[int]string
	outputs map[int]string
}

var hiddenLegacyClasses = map[string]bool{
	"FileNode":      true,
	"TableReader":   true,
	"PreviewTable":  true,
	"Keys (Dict)":   true,
	"Get (Dict)":    true,
	"Set (Dict)":    true,
	"Create (Dict)": true,
	"UnknownSource": true,
}

func inferredRuntimeFallbackSpecs(nodes []legacyNode, edges []legacyEdge, runtimeSpecs map[string]runtimeLegacySpec, maxInputs, maxOutputs map[string]int) map[string]runtimeLegacySpec {
	byID := map[string]legacyNode{}
	sets := map[string]*inferredRuntimeFallbackPortSet{}
	for _, node := range nodes {
		byID[node.ID] = node
		if _, known := runtimeSpecs[node.Class]; known || !shouldCreateRuntimeFallback(node) {
			continue
		}
		set := &inferredRuntimeFallbackPortSet{inputs: map[int]string{}, outputs: map[int]string{}}
		if maxInput, exists := maxInputs[node.ID]; exists {
			for index := 0; index <= maxInput; index++ {
				set.inputs[index] = "any"
			}
		}
		if maxOutput, exists := maxOutputs[node.ID]; exists {
			for index := 0; index <= maxOutput; index++ {
				set.outputs[index] = "any"
			}
		}
		for rawIndex := range node.PortDefaults {
			index, err := strconv.Atoi(rawIndex)
			if err == nil && index >= 0 {
				set.inputs[index] = "any"
			}
		}
		if isLegacyEntranceClass(node.Class) {
			set.outputs[0] = "exec"
		}
		if len(set.inputs) > 0 || len(set.outputs) > 0 || isLegacyEntranceClass(node.Class) {
			sets[node.ID] = set
		}
	}

	changed := true
	for changed {
		changed = false
		for _, edge := range edges {
			sourceIndex := legacyPortIndex(edge.SourcePortID, edge.SourceIndex)
			targetIndex := legacyPortIndex(edge.TargetPortID, edge.TargetIndex)
			sourceType := inferredLegacyOutputType(byID[edge.SourceNodeID], runtimeSpecs, sets, sourceIndex)
			targetType := inferredLegacyInputType(byID[edge.TargetNodeID], runtimeSpecs, sets, targetIndex)
			if set := sets[edge.SourceNodeID]; set != nil && assignInferredPortType(set.outputs, sourceIndex, targetType) {
				changed = true
			}
			if set := sets[edge.TargetNodeID]; set != nil && assignInferredPortType(set.inputs, targetIndex, sourceType) {
				changed = true
			}
		}
		for _, set := range sets {
			if set.inputs[0] == "exec" && assignInferredPortType(set.outputs, 0, "exec") {
				changed = true
			}
			if set.outputs[0] == "exec" && assignInferredPortType(set.inputs, 0, "exec") {
				changed = true
			}
		}
	}

	result := map[string]runtimeLegacySpec{}
	for id, set := range sets {
		inputs := inferredLegacyPorts(set.inputs, "in")
		outputs := inferredLegacyPorts(set.outputs, "out")
		result[id] = runtimeLegacySpec{
			legacyNodeSpec: legacyNodeSpec{
				TypeID:  "origin.legacy.placeholder",
				Inputs:  legacyPortKeysFromPorts(inputs),
				Outputs: legacyPortKeysFromPorts(outputs),
			},
			InputPorts:  inputs,
			OutputPorts: outputs,
		}
	}
	return result
}

func shouldCreateRuntimeFallback(node legacyNode) bool {
	class := strings.TrimSpace(node.Class)
	if class == "" || hiddenLegacyClasses[class] {
		return false
	}
	module := strings.TrimSpace(node.Module)
	return module == "" || module == "tools.json_node_loader"
}

func isLegacyEntranceClass(class string) bool {
	return strings.HasPrefix(strings.TrimSpace(class), "Entrance_")
}

func inferredLegacyInputType(node legacyNode, runtimeSpecs map[string]runtimeLegacySpec, sets map[string]*inferredRuntimeFallbackPortSet, index int) string {
	if set := sets[node.ID]; set != nil {
		return set.inputs[index]
	}
	if spec, known := runtimeSpecs[node.Class]; known {
		return runtimeLegacySpecPortType(spec, false, index)
	}
	return ""
}

func inferredLegacyOutputType(node legacyNode, runtimeSpecs map[string]runtimeLegacySpec, sets map[string]*inferredRuntimeFallbackPortSet, index int) string {
	if set := sets[node.ID]; set != nil {
		return set.outputs[index]
	}
	if spec, known := runtimeSpecs[node.Class]; known {
		return runtimeLegacySpecPortType(spec, true, index)
	}
	return ""
}

func assignInferredPortType(ports map[int]string, index int, portType string) bool {
	if index < 0 || portType == "" || portType == "any" {
		return false
	}
	current, exists := ports[index]
	if !exists {
		return false
	}
	if current != "" && current != "any" {
		return false
	}
	ports[index] = portType
	return current != portType
}

func inferredLegacyPorts(types map[int]string, prefix string) []GraphLegacyPort {
	indexes := make([]int, 0, len(types))
	for index := range types {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	ports := make([]GraphLegacyPort, 0, len(indexes))
	for _, index := range indexes {
		portType := types[index]
		if portType == "" {
			portType = "any"
		}
		key := fmt.Sprintf("%s%d", prefix, index)
		label := key
		if portType == "exec" {
			label = ""
		}
		ports = append(ports, GraphLegacyPort{Key: key, Label: label, Type: portType})
	}
	return ports
}

func runtimeLegacySpecPortType(spec runtimeLegacySpec, output bool, index int) string {
	keys := spec.Inputs
	ports := spec.InputPorts
	prefix := "in"
	staticPorts := map[string]string(nil)
	if output {
		keys = spec.Outputs
		ports = spec.OutputPorts
		prefix = "out"
	}
	key, exists := legacyPortKeyAtIndex(keys, index, prefix)
	if !exists {
		return ""
	}
	for _, port := range ports {
		if port.Key == key {
			return port.Type
		}
	}
	if definition, exists := graphNodePorts[spec.TypeID]; exists {
		if output {
			staticPorts = definition.Outputs
		} else {
			staticPorts = definition.Inputs
		}
		if portType := staticPorts[key]; portType != "" {
			return portType
		}
	}
	if output && strings.HasPrefix(key, "case") && strings.HasPrefix(spec.TypeID, "origin.flow.") {
		return "exec"
	}
	if key == "exec" || key == "body" || key == "completed" || key == "otherwise" || key == "true" || key == "false" || strings.HasPrefix(key, "then") {
		return "exec"
	}
	return "any"
}

func runtimeLegacySpecHasPort(spec runtimeLegacySpec, output bool, index int) bool {
	keys := spec.Inputs
	prefix := "in"
	if output {
		keys = spec.Outputs
		prefix = "out"
	}
	_, exists := legacyPortKeyAtIndex(keys, index, prefix)
	return exists
}

func runtimeLegacySpecHasPorts(spec runtimeLegacySpec, output bool, indexes map[int]bool) bool {
	for index := range indexes {
		if !runtimeLegacySpecHasPort(spec, output, index) {
			return false
		}
	}
	return true
}

func legacyPortTypesCompatible(source, target string) bool {
	if source == "" || target == "" {
		return false
	}
	if source == "any" || target == "any" {
		return true
	}
	if source == "exec" || target == "exec" {
		return source == target
	}
	return source == target
}

func runtimeLegacyNodeSpecs() map[string]runtimeLegacySpec {
	result := map[string]runtimeLegacySpec{}
	for name, spec := range legacyNodeSpecs {
		result[name] = runtimeLegacySpec{legacyNodeSpec: spec}
	}
	loadResult := loadRuntimeNodeSchemaDocumentsWithEmbedded(runtimeNodeDirectories())
	for _, document := range loadResult.Documents {
		// 这里服务于 legacy .vgf 的 class/port_id 映射，只从旧 name/port_id 定义推导。
		// 新 id/key schema 若需要 .vgf round-trip，应在静态映射或显式导出逻辑中维护。
		for _, definition := range parseLegacyRuntimeNodeDefinitions([]byte(document.Content)) {
			name := strings.TrimSpace(definition.Name)
			if name == "" {
				continue
			}
			if spec, exists := legacyNodeSpecs[name]; exists {
				result[name] = runtimeLegacySpec{legacyNodeSpec: spec}
				continue
			}
			inputs := generatedLegacyPorts(definition.Inputs, "in")
			outputs := generatedLegacyPorts(definition.Outputs, "out")
			result[name] = runtimeLegacySpec{
				legacyNodeSpec: legacyNodeSpec{
					TypeID:  legacyRuntimeTypeID(definition, name),
					Inputs:  legacyPortKeysFromPorts(inputs),
					Outputs: legacyPortKeysFromPorts(outputs),
				},
				InputPorts:  inputs,
				OutputPorts: outputs,
			}
		}
	}
	return result
}

func parseLegacyRuntimeNodeDefinitions(data []byte) []legacyRuntimeNodeDefinition {
	var array []legacyRuntimeNodeDefinition
	if err := json.Unmarshal(data, &array); err == nil {
		return array
	}
	var wrapped struct {
		Nodes []legacyRuntimeNodeDefinition `json:"nodes"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Nodes) > 0 {
		return wrapped.Nodes
	}
	var single legacyRuntimeNodeDefinition
	if err := json.Unmarshal(data, &single); err == nil && (single.Name != "" || single.ID != "") {
		return []legacyRuntimeNodeDefinition{single}
	}
	return nil
}

func generatedLegacyPorts(ports []legacyPortDefinition, prefix string) []GraphLegacyPort {
	type indexedPort struct {
		index int
		port  legacyPortDefinition
	}
	indexed := make([]indexedPort, 0, len(ports))
	for fallback, port := range ports {
		indexed = append(indexed, indexedPort{index: legacyPortIndex(port.PortID, fallback), port: port})
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].index < indexed[j].index })
	result := make([]GraphLegacyPort, 0, len(indexed))
	for _, item := range indexed {
		result = append(result, GraphLegacyPort{
			Key:   fmt.Sprintf("%s%d", prefix, item.index),
			Label: item.port.Name,
			Type:  legacyRuntimePortType(item.port),
		})
	}
	return result
}

func legacyPortKeysFromPorts(ports []GraphLegacyPort) []string {
	result := make([]string, 0, len(ports))
	for _, port := range ports {
		result = append(result, port.Key)
	}
	return result
}

func legacyEqualSwitchNewSpec() legacyNodeSpec {
	return legacyNodeSpec{
		TypeID:  "origin.flow.equal-switch-new",
		Inputs:  []string{"exec", "value", "cases"},
		Outputs: legacyDynamicCaseOutputs(50),
	}
}

func legacyDynamicCaseOutputs(maxBranches int) []string {
	outputs := make([]string, 0, maxBranches+2)
	outputs = append(outputs, "otherwise")
	// case0 是隐藏的 legacy 占位端口；保留它才能让 case1 导出为 source_port_id 2。
	for index := 0; index <= maxBranches; index++ {
		outputs = append(outputs, fmt.Sprintf("case%d", index))
	}
	return outputs
}

func legacyEqualSwitchUsesExpandedBranches(node legacyNode, maxOutput int) bool {
	if maxOutput > 5 {
		return true
	}
	if node.PortDefaults == nil {
		return false
	}
	cases, ok := node.PortDefaults["2"].([]interface{})
	return ok && len(cases) > 4
}

func legacyPortTypesFromPorts(ports []GraphLegacyPort) []string {
	result := make([]string, 0, len(ports))
	for _, port := range ports {
		result = append(result, port.Type)
	}
	return result
}

func legacyPortLabelsFromPorts(ports []GraphLegacyPort) []string {
	result := make([]string, 0, len(ports))
	for _, port := range ports {
		result = append(result, port.Label)
	}
	return result
}

func legacyRuntimePortType(port legacyPortDefinition) string {
	if strings.EqualFold(strings.TrimSpace(port.Type), "exec") {
		return "exec"
	}
	return legacyVariableType(port.DataType)
}

func legacyKeyIndex(keys []string, key, prefix string) (int, bool) {
	for index, candidate := range keys {
		if candidate == key {
			if portID, ok := indexedLegacyPortID(candidate, prefix); ok {
				return portID, true
			}
			return index, true
		}
	}
	return 0, false
}

func legacyPortKeyAtIndex(keys []string, index int, prefix string) (string, bool) {
	hasIndexedKeys := false
	for _, key := range keys {
		portID, indexed := indexedLegacyPortID(key, prefix)
		if !indexed {
			continue
		}
		hasIndexedKeys = true
		if portID == index {
			return key, true
		}
	}
	if !hasIndexedKeys && index >= 0 && index < len(keys) {
		return keys[index], true
	}
	return "", false
}

func indexedLegacyPortID(key, prefix string) (int, bool) {
	if !strings.HasPrefix(key, prefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(key, prefix)
	index, err := strconv.Atoi(raw)
	return index, err == nil && index >= 0 && strconv.Itoa(index) == raw
}

func legacyPortsFromRuntimeSpec(keys []string, ports []GraphLegacyPort, prefix string) []GraphLegacyPort {
	byKey := make(map[string]GraphLegacyPort, len(ports))
	for _, port := range ports {
		byKey[port.Key] = port
	}
	result := make([]GraphLegacyPort, 0, len(keys))
	for index, key := range keys {
		if port, exists := byKey[key]; exists {
			result = append(result, port)
			continue
		}
		result = append(result, GraphLegacyPort{Key: key, Label: key, Type: "any"})
		if key == fmt.Sprintf("%s%d", prefix, index) {
			result[len(result)-1].Label = ""
		}
	}
	return result
}

func legacyVariables(document GraphDocument) []map[string]interface{} {
	if document.Legacy != nil && document.Legacy.Variables != nil {
		return cloneLegacyVariables(document.Legacy.Variables)
	}
	result := make([]map[string]interface{}, 0, len(document.Variables))
	for _, variable := range document.Variables {
		result = append(result, map[string]interface{}{"name": variable.Name, "type": variable.Type, "value": variable.DefaultValue})
	}
	return result
}

func legacyGroups(document GraphDocument) []legacyGroup {
	result := make([]legacyGroup, 0, len(document.Groups))
	if document.Legacy != nil && document.Legacy.Groups != nil {
		visibleNodes := make(map[string]bool, len(document.Nodes))
		for _, node := range document.Nodes {
			visibleNodes[node.ID] = true
		}
		usedGroups := make([]bool, len(document.Groups))
		for _, legacyItem := range document.Legacy.Groups {
			visibleIDs := make([]string, 0, len(legacyItem.Nodes))
			hiddenIDs := make([]string, 0)
			for _, id := range legacyItem.Nodes {
				if visibleNodes[id] {
					visibleIDs = append(visibleIDs, id)
				} else {
					hiddenIDs = append(hiddenIDs, id)
				}
			}
			if len(visibleIDs) == 0 {
				result = append(result, legacyGroup{Title: legacyItem.Title, Nodes: append([]string(nil), legacyItem.Nodes...)})
				continue
			}
			if index := matchingGraphGroup(document.Groups, usedGroups, visibleIDs); index >= 0 {
				usedGroups[index] = true
				group := document.Groups[index]
				nodes := append([]string(nil), group.NodeIDs...)
				nodes = append(nodes, hiddenIDs...)
				result = append(result, legacyGroup{Title: group.Title, Nodes: nodes})
			}
		}
		for index, group := range document.Groups {
			if usedGroups[index] {
				continue
			}
			result = append(result, legacyGroup{Title: group.Title, Nodes: append([]string(nil), group.NodeIDs...)})
		}
		return result
	}
	for _, group := range document.Groups {
		result = append(result, legacyGroup{Title: group.Title, Nodes: append([]string(nil), group.NodeIDs...)})
	}
	return result
}

func matchingGraphGroup(groups []GraphGroup, used []bool, visibleIDs []string) int {
	for index, group := range groups {
		if index < len(used) && used[index] {
			continue
		}
		if sameStringSet(group.NodeIDs, visibleIDs) {
			return index
		}
	}
	return -1
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}

func legacyRuntimeTypeID(definition legacyRuntimeNodeDefinition, name string) string {
	if strings.TrimSpace(definition.ID) != "" {
		return strings.TrimSpace(definition.ID)
	}
	return "origin.custom." + legacySlug(name)
}

func legacySlug(value string) string {
	var builder strings.Builder
	previousDash := false
	for index, char := range strings.TrimSpace(value) {
		if index > 0 && char >= 'A' && char <= 'Z' {
			if !previousDash {
				builder.WriteByte('-')
				previousDash = true
			}
		}
		lower := char
		if lower >= 'A' && lower <= 'Z' {
			lower += 'a' - 'A'
		}
		if (lower >= 'a' && lower <= 'z') || (lower >= '0' && lower <= '9') {
			builder.WriteRune(lower)
			previousDash = false
			continue
		}
		if !previousDash && builder.Len() > 0 {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func cloneLegacyNode(node legacyNode) legacyNode {
	return legacyNode{ID: node.ID, Class: node.Class, Module: node.Module, Position: append([]float64(nil), node.Position...), PortDefaults: cloneInterfaceMap(node.PortDefaults)}
}

func cloneLegacyEdge(edge legacyEdge) legacyEdge {
	return edge
}

func cloneLegacyGroups(groups []legacyGroup) []legacyGroup {
	result := make([]legacyGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, legacyGroup{Title: group.Title, Nodes: append([]string(nil), group.Nodes...)})
	}
	return result
}

func cloneLegacyVariables(variables []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(variables))
	for _, variable := range variables {
		result = append(result, cloneInterfaceMap(variable))
	}
	return result
}

func cloneInterfaceMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func legacyVariableType(value interface{}) string {
	switch strings.ToLower(fmt.Sprint(value)) {
	case "bool", "boolean":
		return "boolean"
	case "int", "integer":
		return "integer"
	case "float", "double":
		return "float"
	case "array", "list":
		return "array"
	}
	return "string"
}
func legacyPortIndex(value interface{}, fallback int) int {
	if value == nil {
		return fallback
	}
	number, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil {
		return fallback
	}
	return number
}
func indexedKey(keys []string, index int, prefix string) string {
	if key, exists := legacyPortKeyAtIndex(keys, index, prefix); exists {
		return key
	}
	return fmt.Sprintf("%s%d", prefix, index)
}
func sortedPortIndexes(values map[int]bool) []int {
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}
