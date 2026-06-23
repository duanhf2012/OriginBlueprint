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

type legacyRuntimeNodeDefinition struct {
	Name    string                 `json:"name"`
	ID      string                 `json:"id"`
	Inputs  []legacyPortDefinition `json:"inputs"`
	Outputs []legacyPortDefinition `json:"outputs"`
}

type legacyPortDefinition struct {
	PortID interface{} `json:"port_id"`
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
	"Entrance_Timer_000003":      {"origin.event.timer", nil, []string{"exec", "timerId", "params"}},
	"CreateTimer":                {"origin.timer.create", []string{"exec", "milliseconds", "params"}, []string{"exec", "timerId"}},
	"CloseTimer":                 {"origin.timer.close", []string{"exec", "timerId"}, []string{"exec"}},
	"ForeachIntArray":            {"origin.flow.foreach-integer-array", []string{"exec", "array"}, []string{"body", "completed", "index", "value"}},
	"Probability":                {"origin.flow.probability", []string{"exec", "probability"}, []string{"miss", "hit"}},
	"DebugOutput":                {"origin.debug.output", []string{"exec", "integer", "string", "array"}, []string{"exec"}},
	"FileNode":                   {"origin.io.file-path", []string{"path"}, []string{"file"}},
	"FileReadNode":               {"origin.io.read-text", []string{"exec", "file"}, []string{"exec", "text", "error"}},
	"SaveFileNode":               {"origin.io.save-file-path", []string{"path"}, []string{"file"}},
	"StringNode":                 {"origin.literal.string", []string{"value"}, []string{"value"}},
	"AddNode":                    {"origin.math.add-float", []string{"a", "b"}, []string{"result"}},
	"MinusNode":                  {"origin.math.subtract-float", []string{"a", "b"}, []string{"result"}},
	"MultiplyNode":               {"origin.math.multiply-float", []string{"a", "b"}, []string{"result"}},
	"DivideNode":                 {"origin.math.divide-float", []string{"a", "b"}, []string{"result"}},
	"GreaterIntegerNode":         {"origin.compare.greater-integer", []string{"a", "b"}, []string{"result", "a", "b"}},
	"WhileNode":                  {"origin.flow.while", []string{"exec", "condition"}, []string{"body", "completed"}},
	"ForLoopWithBreak":           {"origin.flow.for-loop-break", []string{"exec", "start", "end", "break"}, []string{"body", "index", "completed"}},
	"Length (Array)":             {"origin.array.length", []string{"array"}, []string{"length"}},
	"TableReader":                {"origin.table.read-csv", []string{"exec", "file", "delimiter", "header"}, []string{"exec", "table", "error"}},
	"Save Table":                 {"origin.table.save-csv", []string{"exec", "table", "file"}, []string{"exec", "table"}},
	"Table Rows Count":           {"origin.table.row-count", []string{"exec", "table"}, []string{"exec", "count"}},
	"TableHeader":                {"origin.table.headers", []string{"exec", "table"}, []string{"exec", "headers"}},
	"MergeTable":                 {"origin.table.merge", []string{"exec", "left", "right", "key"}, []string{"exec", "table"}},
	"Sub Table":                  {"origin.table.select-columns", []string{"exec", "table", "columns"}, []string{"exec", "table"}},
	"Print Table":                {"origin.table.print", []string{"exec", "table"}, []string{"exec", "table"}},
	"ForEahcNode":                {"origin.flow.foreach-array", []string{"exec", "array"}, []string{"body", "index", "value", "completed"}},
	"PreviewTable":               {"origin.table.preview", []string{"table"}, nil},
	"Split":                      {"origin.string.split", []string{"exec", "text", "delimiter"}, []string{"exec", "array"}},
	"Get (Array)":                {"origin.array.get-any", []string{"array", "index"}, []string{"value"}},
	"Cast To":                    {"origin.cast.any-string", []string{"exec", "value"}, []string{"exec", "result"}},
	"CastingNode_str":            {"origin.cast.any-string", []string{"exec", "value"}, []string{"exec", "valid", "result"}},
	"Set (Dict)":                 {"origin.dictionary.set", []string{"exec", "dictionary", "key", "value"}, []string{"exec", "dictionary"}},
	"Size (Dict)":                {"origin.dictionary.size", []string{"dictionary"}, []string{"size"}},
	"Keys (Dict)":                {"origin.dictionary.keys", []string{"dictionary"}, []string{"keys"}},
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
	nodeSpecs := map[string]legacyNodeSpec{}
	maxInputs, maxOutputs := map[string]int{}, map[string]int{}
	for _, edge := range legacy.Edges {
		sourceIndex := legacyPortIndex(edge.SourcePortID, edge.SourceIndex)
		targetIndex := legacyPortIndex(edge.TargetPortID, edge.TargetIndex)
		if sourceIndex > maxOutputs[edge.SourceNodeID] {
			maxOutputs[edge.SourceNodeID] = sourceIndex
		}
		if targetIndex > maxInputs[edge.TargetNodeID] {
			maxInputs[edge.TargetNodeID] = targetIndex
		}
	}
	for _, item := range legacy.Nodes {
		spec, known := runtimeSpecs[item.Class]
		if (len(spec.Inputs) > 0 && maxInputs[item.ID] >= len(spec.Inputs)) || (len(spec.Outputs) > 0 && maxOutputs[item.ID] >= len(spec.Outputs)) {
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
			document.Legacy.HiddenNodes = append(document.Legacy.HiddenNodes, cloneLegacyNode(item))
			continue
		}
		if _, staticallyKnown := graphNodePorts[spec.TypeID]; !staticallyKnown {
			properties.LegacyInputs = legacyPortsFromKeys(spec.Inputs)
			properties.LegacyOutputs = legacyPortsFromKeys(spec.Outputs)
		}
		nodeSpecs[item.ID] = spec
		position := GraphPosition{}
		if len(item.Position) > 0 {
			position.X = item.Position[0]
		}
		if len(item.Position) > 1 {
			position.Y = item.Position[1]
		}
		node := GraphNode{ID: item.ID, TypeID: spec.TypeID, Position: position, Values: map[string]interface{}{}, Properties: properties}
		for rawIndex, value := range item.PortDefaults {
			index, _ := strconv.Atoi(rawIndex)
			if index >= 0 && index < len(spec.Inputs) {
				node.Values[spec.Inputs[index]] = value
			}
		}
		document.Nodes = append(document.Nodes, node)
		nodeByID[item.ID] = len(document.Nodes) - 1
	}
	for _, edge := range legacy.Edges {
		sourceIndex := legacyPortIndex(edge.SourcePortID, edge.SourceIndex)
		targetIndex := legacyPortIndex(edge.TargetPortID, edge.TargetIndex)
		sourceSpec := nodeSpecs[edge.SourceNodeID]
		targetSpec := nodeSpecs[edge.TargetNodeID]
		_, sourceExists := nodeByID[edge.SourceNodeID]
		_, targetExists := nodeByID[edge.TargetNodeID]
		if !sourceExists || !targetExists {
			document.Legacy.HiddenEdges = append(document.Legacy.HiddenEdges, cloneLegacyEdge(edge))
			continue
		}
		sourceKey := indexedKey(sourceSpec.Outputs, sourceIndex, "out")
		targetKey := indexedKey(targetSpec.Inputs, targetIndex, "in")
		document.Connections = append(document.Connections, GraphConnection{Source: edge.SourceNodeID, SourceOutput: sourceKey, Target: edge.TargetNodeID, TargetInput: targetKey, EntryConnectionVisible: edge.EntryConnectionVisible})
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
	specByType := map[string]legacyNodeSpec{}
	classByType := map[string]string{}
	for class, spec := range runtimeSpecs {
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

	nodeSpecs := map[string]legacyNodeSpec{}
	nodeIDs := map[string]bool{}
	for _, node := range document.Nodes {
		class := node.Properties.LegacyClass
		module := node.Properties.LegacyModule
		spec := legacyNodeSpec{}
		if node.TypeID == "origin.flow.equal-switch-new" {
			class = "EqualSwitch"
			spec = legacyNodeSpecs[class]
		}
		if node.TypeID == "origin.array.create-integer-new" {
			class = "CreateIntArray"
			spec = legacyNodeSpecs[class]
		}
		if node.TypeID == "origin.array.create-string-new" {
			class = "CreateStringArray"
			spec = legacyNodeSpecs[class]
		}
		if node.TypeID == "origin.variable.get" || node.TypeID == "origin.variable.set" {
			variable, exists := variablesByID[node.Properties.VariableID]
			if !exists || variable.Name == "" {
				continue
			}
			if node.TypeID == "origin.variable.get" {
				if class == "" {
					class = "Get_" + variable.Name
				}
				spec = legacyNodeSpec{TypeID: node.TypeID, Outputs: []string{"value"}}
			} else {
				if class == "" {
					class = "Set_" + variable.Name
				}
				spec = legacyNodeSpec{TypeID: node.TypeID, Inputs: []string{"exec", "value"}, Outputs: []string{"exec", "value"}}
			}
			if module == "" {
				module = "nodes.VariableNode"
			}
		}
		if class == "" {
			class = classByType[node.TypeID]
		}
		if class == "" {
			continue
		}
		if spec.TypeID == "" {
			spec = runtimeSpecs[class]
		}
		if spec.TypeID == "" {
			spec = specByType[node.TypeID]
		}
		if spec.TypeID == "" {
			continue
		}
		nodeSpecs[node.ID] = spec
		nodeIDs[node.ID] = true
		portDefaults := map[string]interface{}{}
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
			legacy.Nodes = append(legacy.Nodes, cloneLegacyNode(node))
			nodeIDs[node.ID] = true
		}
	}

	for _, connection := range document.Connections {
		sourceSpec, sourceOK := nodeSpecs[connection.Source]
		targetSpec, targetOK := nodeSpecs[connection.Target]
		if !sourceOK || !targetOK {
			continue
		}
		sourceIndex, sourcePortOK := legacyKeyIndex(sourceSpec.Outputs, connection.SourceOutput, "out")
		targetIndex, targetPortOK := legacyKeyIndex(targetSpec.Inputs, connection.TargetInput, "in")
		if !sourcePortOK || !targetPortOK {
			continue
		}
		legacy.Edges = append(legacy.Edges, legacyEdge{
			EdgeID:                 uuid.NewString(),
			SourceNodeID:           connection.Source,
			SourceIndex:            sourceIndex,
			SourcePortID:           sourceIndex,
			TargetNodeID:           connection.Target,
			TargetIndex:            targetIndex,
			TargetPortID:           targetIndex,
			EntryConnectionVisible: connection.EntryConnectionVisible,
		})
	}
	if document.Legacy != nil {
		for _, edge := range document.Legacy.HiddenEdges {
			if nodeIDs[edge.SourceNodeID] && nodeIDs[edge.TargetNodeID] {
				legacy.Edges = append(legacy.Edges, cloneLegacyEdge(edge))
			}
		}
	}

	return json.MarshalIndent(legacy, "", "  ")
}

func runtimeLegacyNodeSpecs() map[string]legacyNodeSpec {
	result := map[string]legacyNodeSpec{}
	loadResult := loadRuntimeNodeSchemaDocuments(runtimeNodeDirectories())
	for _, document := range loadResult.Documents {
		for _, definition := range parseLegacyRuntimeNodeDefinitions([]byte(document.Content)) {
			name := strings.TrimSpace(definition.Name)
			if name == "" {
				continue
			}
			if spec, exists := legacyNodeSpecs[name]; exists {
				result[name] = spec
				continue
			}
			result[name] = legacyNodeSpec{
				TypeID:  legacyRuntimeTypeID(definition, name),
				Inputs:  generatedLegacyPortKeys(definition.Inputs, "in"),
				Outputs: generatedLegacyPortKeys(definition.Outputs, "out"),
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

func generatedLegacyPortKeys(ports []legacyPortDefinition, prefix string) []string {
	indexes := make([]int, 0, len(ports))
	for fallback, port := range ports {
		indexes = append(indexes, legacyPortIndex(port.PortID, fallback))
	}
	sort.Ints(indexes)
	keys := make([]string, 0, len(indexes))
	for _, index := range indexes {
		keys = append(keys, fmt.Sprintf("%s%d", prefix, index))
	}
	return keys
}

func legacyKeyIndex(keys []string, key, prefix string) (int, bool) {
	for index, candidate := range keys {
		if candidate == key {
			return index, true
		}
	}
	if strings.HasPrefix(key, prefix) {
		index, err := strconv.Atoi(strings.TrimPrefix(key, prefix))
		return index, err == nil
	}
	return 0, false
}

func legacyPortsFromKeys(keys []string) []GraphLegacyPort {
	result := make([]GraphLegacyPort, 0, len(keys))
	for _, key := range keys {
		result = append(result, GraphLegacyPort{Key: key, Label: key, Type: "any"})
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
	if document.Legacy != nil && document.Legacy.Groups != nil {
		return cloneLegacyGroups(document.Legacy.Groups)
	}
	result := make([]legacyGroup, 0, len(document.Groups))
	for _, group := range document.Groups {
		result = append(result, legacyGroup{Title: group.Title, Nodes: append([]string(nil), group.NodeIDs...)})
	}
	return result
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
	case "file":
		return "file"
	case "dataframe", "table":
		return "table"
	case "dict", "dictionary", "map":
		return "dictionary"
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
	if index >= 0 && index < len(keys) {
		return keys[index]
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
