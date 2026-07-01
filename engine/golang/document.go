package golang

import (
	"fmt"
	"strings"
)

// graphDocument is the native editor document shape used by .obp and .obpf.
//
// It is intentionally private; the runtime converts it into GraphConfig so the
// executor has one compact format.
type graphDocument struct {
	SchemaVersion     int                        `json:"schemaVersion"`
	GraphName         string                     `json:"graphName"`
	FunctionID        string                     `json:"functionId,omitempty"`
	Nodes             []graphDocumentNode        `json:"nodes"`
	Connections       []graphDocumentConnection  `json:"connections"`
	Variables         []graphDocumentVariable    `json:"variables"`
	FunctionSignature graphDocumentFuncSignature `json:"functionSignature,omitempty"`
}

type graphDocumentNode struct {
	ID         string                  `json:"id"`
	TypeID     string                  `json:"typeId"`
	Values     map[string]any          `json:"values"`
	Properties graphDocumentProperties `json:"properties,omitempty"`
}

type graphDocumentProperties struct {
	VariableID        string                     `json:"variableId,omitempty"`
	FunctionID        string                     `json:"functionId,omitempty"`
	FunctionName      string                     `json:"functionName,omitempty"`
	FunctionSignature graphDocumentFuncSignature `json:"functionSignature,omitempty"`
	LegacyClass       string                     `json:"legacyClass,omitempty"`
	LegacyInputs      []graphDocumentLegacyPort  `json:"legacyInputs,omitempty"`
	LegacyOutputs     []graphDocumentLegacyPort  `json:"legacyOutputs,omitempty"`
}

type graphDocumentFuncSignature struct {
	Inputs  []graphDocumentFuncPort `json:"inputs,omitempty"`
	Outputs []graphDocumentFuncPort `json:"outputs,omitempty"`
}

type graphDocumentFuncPort struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type graphDocumentVariable struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	DefaultValue any    `json:"defaultValue"`
}

type graphDocumentLegacyPort struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}

type graphDocumentConnection struct {
	Source       string `json:"source"`
	SourceOutput string `json:"sourceOutput"`
	Target       string `json:"target"`
	TargetInput  string `json:"targetInput"`
}

// documentNodeSpec maps document string port keys to runtime numeric port ids.
type documentNodeSpec struct {
	class   string
	inputs  map[string]int
	outputs map[string]int
}

// documentNodeSpecs covers native editor nodes that correspond to implemented
// runtime nodes in this package.
var documentNodeSpecs = map[string]documentNodeSpec{
	"origin.event.entry-array":          {class: "Entrance_ArrayParam_2", outputs: map[string]int{"exec": 0, "objectId": 1, "params": 2}},
	"origin.event.entry-two-integers":   {class: "Entrance_IntParam_1", outputs: map[string]int{"exec": 0, "objectId": 1, "param1": 2, "param2": 3}},
	"origin.event.timer":                {class: "Entrance_Timer_3", outputs: map[string]int{"exec": 0, "timerId": 1, "params": 2}},
	"origin.debug.output":               {class: "DebugOutput", inputs: map[string]int{"exec": 0, "integer": 1, "string": 2, "array": 3}, outputs: map[string]int{"exec": 0}},
	"origin.cast.integer-string":        {class: "CastIntegerString", inputs: map[string]int{"value": 0}, outputs: map[string]int{"result": 0}},
	"origin.cast.float-string":          {class: "CastFloatString", inputs: map[string]int{"value": 0}, outputs: map[string]int{"result": 0}},
	"origin.cast.any-string":            {class: "CastAnyString", inputs: map[string]int{"exec": 0, "value": 1}, outputs: map[string]int{"exec": 0, "valid": 1, "result": 2}},
	"origin.literal.string":             {class: "LiteralString", inputs: map[string]int{"value": 0}, outputs: map[string]int{"value": 0}},
	"origin.math.add-integer":           {class: "AddInt", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.math.subtract-integer":      {class: "SubInt", inputs: map[string]int{"a": 0, "b": 1, "absolute": 2}, outputs: map[string]int{"result": 0}},
	"origin.math.multiply-integer":      {class: "MulInt", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.math.divide-integer":        {class: "DivInt", inputs: map[string]int{"a": 0, "b": 1, "round": 2}, outputs: map[string]int{"result": 0}},
	"origin.math.modulo-integer":        {class: "ModInt", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.math.random-integer":        {class: "RandNumber", inputs: map[string]int{"seed": 0, "min": 1, "max": 2}, outputs: map[string]int{"result": 0}},
	"origin.math.add-float":             {class: "AddFloat", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.math.subtract-float":        {class: "SubFloat", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.math.multiply-float":        {class: "MulFloat", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.math.divide-float":          {class: "DivFloat", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0}},
	"origin.compare.greater-integer":    {class: "CompareGreaterInteger", inputs: map[string]int{"a": 0, "b": 1}, outputs: map[string]int{"result": 0, "a": 1, "b": 2}},
	"origin.flow.sequence":              {class: "Sequence", inputs: map[string]int{"exec": 0}, outputs: map[string]int{"then0": 0, "then1": 1, "then2": 2}},
	"origin.flow.for-loop":              {class: "Foreach", inputs: map[string]int{"exec": 0, "start": 1, "end": 2}, outputs: map[string]int{"body": 0, "completed": 1, "index": 2}},
	"origin.flow.branch":                {class: "BoolIf", inputs: map[string]int{"exec": 0, "condition": 1}, outputs: map[string]int{"false": 0, "true": 1}},
	"origin.flow.greater-integer":       {class: "GreaterThanInteger", inputs: map[string]int{"exec": 0, "orEqual": 1, "a": 2, "b": 3}, outputs: map[string]int{"false": 0, "true": 1}},
	"origin.flow.less-integer":          {class: "LessThanInteger", inputs: map[string]int{"exec": 0, "orEqual": 1, "a": 2, "b": 3}, outputs: map[string]int{"false": 0, "true": 1}},
	"origin.flow.equal-integer":         {class: "EqualInteger", inputs: map[string]int{"exec": 0, "a": 1, "b": 2}, outputs: map[string]int{"false": 0, "true": 1}},
	"origin.flow.foreach-integer-array": {class: "ForeachIntArray", inputs: map[string]int{"exec": 0, "array": 1}, outputs: map[string]int{"body": 0, "completed": 1, "index": 2, "value": 3}},
	"origin.flow.while":                 {class: "WhileNode", inputs: map[string]int{"exec": 0, "condition": 1}, outputs: map[string]int{"body": 0, "completed": 1}},
	"origin.flow.for-loop-break":        {class: "ForLoopBreak", inputs: map[string]int{"exec": 0, "start": 1, "end": 2, "break": 3}, outputs: map[string]int{"body": 0, "index": 1, "completed": 2}},
	"origin.flow.foreach-array":         {class: "ForeachArray", inputs: map[string]int{"exec": 0, "array": 1}, outputs: map[string]int{"body": 0, "completed": 1, "value": 2, "index": 3}},
	"origin.flow.foreach-table-row":     {class: "ForeachTableRow", inputs: map[string]int{"exec": 0, "table": 1}, outputs: map[string]int{"body": 0, "completed": 1, "row": 2, "index": 3}},
	"origin.flow.probability":           {class: "Probability", inputs: map[string]int{"exec": 0, "probability": 1}, outputs: map[string]int{"miss": 0, "hit": 1}},
	"origin.flow.range-compare":         {class: "RangeCompare", inputs: map[string]int{"exec": 0, "value": 1, "ranges": 2}, outputs: switchOutputs()},
	"origin.flow.equal-switch":          {class: "EqualSwitch", inputs: map[string]int{"exec": 0, "value": 1, "cases": 2}, outputs: switchOutputs()},
	"origin.flow.equal-switch-new":      {class: "EqualSwitch", inputs: map[string]int{"exec": 0, "value": 1, "cases": 2}, outputs: switchOutputs()},
	"origin.array.get-integer":          {class: "GetArrayInt", inputs: map[string]int{"array": 0, "index": 1}, outputs: map[string]int{"value": 0}},
	"origin.array.get-string":           {class: "GetArrayString", inputs: map[string]int{"array": 0, "index": 1}, outputs: map[string]int{"value": 0}},
	"origin.array.get-any":              {class: "GetArrayAny", inputs: map[string]int{"array": 0, "index": 1}, outputs: map[string]int{"value": 0}},
	"origin.array.length":               {class: "GetArrayLen", inputs: map[string]int{"array": 0}, outputs: map[string]int{"length": 0}},
	"origin.array.create-integer":       {class: "CreateIntArray", inputs: map[string]int{"items": 0}, outputs: map[string]int{"array": 0}},
	"origin.array.create-integer-new":   {class: "CreateIntArray", inputs: map[string]int{"items": 0}, outputs: map[string]int{"array": 0}},
	"origin.array.create-string":        {class: "CreateStringArray", inputs: map[string]int{"items": 0}, outputs: map[string]int{"array": 0}},
	"origin.array.create-string-new":    {class: "CreateStringArray", inputs: map[string]int{"items": 0}, outputs: map[string]int{"array": 0}},
	"origin.array.append-string":        {class: "AppendStringToArray", inputs: map[string]int{"array": 0, "value": 1}, outputs: map[string]int{"array": 0}},
	"origin.array.append-integer":       {class: "AppendIntegerToArray", inputs: map[string]int{"array": 0, "value": 1}, outputs: map[string]int{"array": 0}},
	"origin.result.append-integer":      {class: "AppendIntReturn", inputs: map[string]int{"exec": 0, "value": 1}, outputs: map[string]int{"exec": 0}},
	"origin.result.append-string":       {class: "AppendStringReturn", inputs: map[string]int{"exec": 0, "value": 1}, outputs: map[string]int{"exec": 0}},
	"origin.timer.create":               {class: "CreateTimer", inputs: map[string]int{"exec": 0, "milliseconds": 1, "params": 2}, outputs: map[string]int{"exec": 0, "timerId": 1}},
	"origin.timer.close":                {class: "CloseTimer", inputs: map[string]int{"exec": 0, "timerId": 1}, outputs: map[string]int{"exec": 0}},
	"origin.io.file-path":               {class: "FilePath", inputs: map[string]int{"path": 0}, outputs: map[string]int{"file": 0}},
	"origin.io.save-file-path":          {class: "SaveFilePath", inputs: map[string]int{"path": 0}, outputs: map[string]int{"file": 0}},
	"origin.io.read-text":               {class: "ReadText", inputs: map[string]int{"exec": 0, "file": 1}, outputs: map[string]int{"exec": 0, "text": 1, "error": 2}},
	"origin.io.save-text":               {class: "SaveText", inputs: map[string]int{"exec": 0, "file": 1, "text": 2}, outputs: map[string]int{"exec": 0}},
	"origin.string.split":               {class: "StringSplit", inputs: map[string]int{"exec": 0, "text": 1, "delimiter": 2}, outputs: map[string]int{"exec": 0, "array": 1}},
	"origin.dictionary.set":             {class: "DictionarySet", inputs: map[string]int{"exec": 0, "dictionary": 1, "key": 2, "value": 3}, outputs: map[string]int{"exec": 0, "dictionary": 1}},
	"origin.dictionary.size":            {class: "DictionarySize", inputs: map[string]int{"dictionary": 0}, outputs: map[string]int{"size": 0}},
	"origin.dictionary.keys":            {class: "DictionaryKeys", inputs: map[string]int{"dictionary": 0}, outputs: map[string]int{"keys": 0}},
	"origin.table.read-csv":             {class: "ReadCSV", inputs: map[string]int{"exec": 0, "file": 1, "delimiter": 2, "header": 3}, outputs: map[string]int{"exec": 0, "table": 1, "error": 2}},
	"origin.table.save-csv":             {class: "SaveCSV", inputs: map[string]int{"exec": 0, "table": 1, "file": 2}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.row-count":            {class: "TableRowCount", inputs: map[string]int{"exec": 0, "table": 1}, outputs: map[string]int{"exec": 0, "count": 1}},
	"origin.table.headers":              {class: "TableHeaders", inputs: map[string]int{"exec": 0, "table": 1}, outputs: map[string]int{"exec": 0, "headers": 1}},
	"origin.table.merge":                {class: "TableMerge", inputs: map[string]int{"exec": 0, "left": 1, "right": 2, "key": 3}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.select-columns":       {class: "TableSelectColumns", inputs: map[string]int{"exec": 0, "table": 1, "columns": 2}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.print":                {class: "TablePrint", inputs: map[string]int{"exec": 0, "table": 1}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.sort":                 {class: "TableSort", inputs: map[string]int{"exec": 0, "table": 1, "column": 2, "ascending": 3}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.filter-equal":         {class: "TableFilterEqual", inputs: map[string]int{"exec": 0, "table": 1, "column": 2, "value": 3}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.rename-column":        {class: "TableRenameColumn", inputs: map[string]int{"exec": 0, "table": 1, "from": 2, "to": 3}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.drop-columns":         {class: "TableDropColumns", inputs: map[string]int{"exec": 0, "table": 1, "columns": 2}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.fill-empty":           {class: "TableFillEmpty", inputs: map[string]int{"exec": 0, "table": 1, "value": 2}, outputs: map[string]int{"exec": 0, "table": 1}},
	"origin.table.get-column":           {class: "TableGetColumn", inputs: map[string]int{"table": 0, "column": 1}, outputs: map[string]int{"values": 0}},
	"origin.table.preview":              {class: "TablePreview", inputs: map[string]int{"table": 0}, outputs: map[string]int{}},
}

func switchOutputs() map[string]int {
	return map[string]int{"otherwise": 0, "case0": 1, "case1": 2, "case2": 3, "case3": 4, "case4": 5}
}

// graphDocumentToConfig converts an editor document into executable GraphConfig.
//
// The boolean return marks documents that should be registered as callable
// functions by the directory loader.
func graphDocumentToConfig(document graphDocument) (GraphConfig, bool, error) {
	variables := make([]VariableConfig, 0, len(document.Variables))
	variableByID := make(map[string]graphDocumentVariable, len(document.Variables))
	for _, variable := range document.Variables {
		variableByID[variable.ID] = variable
		variables = append(variables, VariableConfig{Name: variable.Name, Type: variable.Type, Value: variable.DefaultValue})
	}

	nodes := make([]NodeConfig, 0, len(document.Nodes))
	specs := make(map[string]documentNodeSpec, len(document.Nodes))
	for _, node := range document.Nodes {
		config, spec, err := documentNodeToConfig(node, variableByID)
		if err != nil {
			return GraphConfig{}, false, err
		}
		nodes = append(nodes, config)
		specs[node.ID] = spec
	}

	edges := make([]EdgeConfig, 0, len(document.Connections))
	for _, connection := range document.Connections {
		sourceSpec, ok := specs[connection.Source]
		if !ok {
			return GraphConfig{}, false, fmt.Errorf("source node %s not found", connection.Source)
		}
		destSpec, ok := specs[connection.Target]
		if !ok {
			return GraphConfig{}, false, fmt.Errorf("destination node %s not found", connection.Target)
		}
		sourcePort, ok := sourceSpec.outputs[connection.SourceOutput]
		if !ok {
			return GraphConfig{}, false, fmt.Errorf("source node %s output %s not found", connection.Source, connection.SourceOutput)
		}
		destPort, ok := destSpec.inputs[connection.TargetInput]
		if !ok {
			return GraphConfig{}, false, fmt.Errorf("destination node %s input %s not found", connection.Target, connection.TargetInput)
		}
		edges = append(edges, EdgeConfig{
			SourceNodeID: connection.Source,
			SourcePortID: sourcePort,
			DesNodeID:    connection.Target,
			DesPortID:    destPort,
		})
	}

	return GraphConfig{Nodes: nodes, Edges: edges, Variables: variables}, document.FunctionID != "" || len(document.FunctionSignature.Inputs)+len(document.FunctionSignature.Outputs) > 0, nil
}

// documentNodeToConfig converts one editor node and preserves its port mapping.
func documentNodeToConfig(node graphDocumentNode, variables map[string]graphDocumentVariable) (NodeConfig, documentNodeSpec, error) {
	switch node.TypeID {
	case "origin.function.entry":
		signature := node.Properties.FunctionSignature
		return NodeConfig{ID: node.ID, Class: "FunctionEntry", FunctionInputTypes: signatureTypes(signature.Inputs), PortDefault: documentDefaults(node.Values, functionEntrySpec(signature).inputs)}, functionEntrySpec(signature), nil
	case "origin.function.return":
		signature := node.Properties.FunctionSignature
		return NodeConfig{ID: node.ID, Class: "FunctionReturn", FunctionOutputTypes: signatureTypes(signature.Outputs), PortDefault: documentDefaults(node.Values, functionReturnSpec(signature).inputs)}, functionReturnSpec(signature), nil
	case "origin.function.call":
		signature := node.Properties.FunctionSignature
		spec := functionCallSpec(signature)
		return NodeConfig{ID: node.ID, Class: "FunctionCall", FunctionID: node.Properties.FunctionID, FunctionName: node.Properties.FunctionName, FunctionInputTypes: signatureTypes(signature.Inputs), FunctionOutputTypes: signatureTypes(signature.Outputs), PortDefault: documentDefaults(node.Values, spec.inputs)}, spec, nil
	case "origin.variable.get", "origin.variable.set":
		variable, ok := variables[node.Properties.VariableID]
		if !ok {
			return NodeConfig{}, documentNodeSpec{}, fmt.Errorf("variable %s not found", node.Properties.VariableID)
		}
		if node.TypeID == "origin.variable.get" {
			return NodeConfig{ID: node.ID, Class: "Get_" + variable.Name}, documentNodeSpec{class: "Get_" + variable.Name, outputs: map[string]int{"value": 0}}, nil
		}
		spec := documentNodeSpec{class: "Set_" + variable.Name, inputs: map[string]int{"exec": 0, "value": 1}, outputs: map[string]int{"exec": 0, "value": 1}}
		return NodeConfig{ID: node.ID, Class: "Set_" + variable.Name, PortDefault: documentDefaults(node.Values, spec.inputs)}, spec, nil
	}

	if node.Properties.LegacyClass != "" {
		spec := legacyNodeSpec(node.Properties)
		return NodeConfig{ID: node.ID, Class: node.Properties.LegacyClass, PortDefault: documentDefaults(node.Values, spec.inputs)}, spec, nil
	}
	spec, ok := documentNodeSpecs[node.TypeID]
	if !ok {
		return NodeConfig{}, documentNodeSpec{}, fmt.Errorf("%s node has not been registered", node.TypeID)
	}
	return NodeConfig{ID: node.ID, Class: spec.class, PortDefault: documentDefaults(node.Values, spec.inputs)}, spec, nil
}

// legacyNodeSpec lets native documents keep executing old registered node classes.
func legacyNodeSpec(properties graphDocumentProperties) documentNodeSpec {
	inputs := make(map[string]int, len(properties.LegacyInputs))
	for index, port := range properties.LegacyInputs {
		inputs[port.Key] = index
	}
	outputs := make(map[string]int, len(properties.LegacyOutputs))
	for index, port := range properties.LegacyOutputs {
		outputs[port.Key] = index
	}
	return documentNodeSpec{class: properties.LegacyClass, inputs: inputs, outputs: outputs}
}

func signatureTypes(ports []graphDocumentFuncPort) []string {
	types := make([]string, 0, len(ports))
	for _, port := range ports {
		types = append(types, port.Type)
	}
	return types
}

func functionEntrySpec(signature graphDocumentFuncSignature) documentNodeSpec {
	outputs := map[string]int{"exec": 0}
	for index, port := range signature.Inputs {
		outputs[functionPortKey("input", port, index)] = index + 1
	}
	return documentNodeSpec{class: "FunctionEntry", outputs: outputs}
}

func functionReturnSpec(signature graphDocumentFuncSignature) documentNodeSpec {
	inputs := map[string]int{"exec": 0}
	for index, port := range signature.Outputs {
		inputs[functionPortKey("output", port, index)] = index + 1
	}
	return documentNodeSpec{class: "FunctionReturn", inputs: inputs}
}

func functionCallSpec(signature graphDocumentFuncSignature) documentNodeSpec {
	inputs := map[string]int{"exec": 0}
	outputs := map[string]int{"exec": 0}
	for index, port := range signature.Inputs {
		inputs[functionPortKey("input", port, index)] = index + 1
	}
	for index, port := range signature.Outputs {
		outputs[functionPortKey("output", port, index)] = index + 1
	}
	return documentNodeSpec{class: "FunctionCall", inputs: inputs, outputs: outputs}
}

// functionPortKey mirrors the editor's stable key generation for signature ports.
func functionPortKey(prefix string, port graphDocumentFuncPort, index int) string {
	source := strings.TrimSpace(port.ID)
	if source == "" {
		source = strings.TrimSpace(port.Name)
	}
	if source == "" {
		return fmt.Sprintf("%s_%d", prefix, index+1)
	}
	var builder strings.Builder
	for _, char := range source {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			builder.WriteRune(char)
		} else if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "-") {
			builder.WriteRune('-')
		}
	}
	key := strings.Trim(builder.String(), "-")
	if key == "" {
		key = fmt.Sprintf("%d", index+1)
	}
	return prefix + "_" + key
}

func documentDefaults(values map[string]any, inputs map[string]int) map[int]any {
	if len(values) == 0 || len(inputs) == 0 {
		return nil
	}
	defaults := map[int]any{}
	for key, value := range values {
		if index, ok := inputs[key]; ok {
			defaults[index] = value
		}
	}
	if len(defaults) == 0 {
		return nil
	}
	return defaults
}
