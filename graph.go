package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	GraphSchemaVersion          = 1
	maxDynamicSequenceOutputs   = 256
	maxFunctionSignatureInputs  = 128
	maxFunctionSignatureOutputs = 128
	maxLegacyPortsPerNode       = 4096
)

type GraphDocument struct {
	SchemaVersion     int                    `json:"schemaVersion"`
	GraphName         string                 `json:"graphName"`
	FunctionID        string                 `json:"functionId,omitempty"`
	Nodes             []GraphNode            `json:"nodes"`
	Connections       []GraphConnection      `json:"connections"`
	Groups            []GraphGroup           `json:"groups"`
	Variables         []GraphVariable        `json:"variables"`
	VariableGroups    []GraphVariableGroup   `json:"variableGroups"`
	FunctionSignature GraphFunctionSignature `json:"functionSignature,omitempty"`
	View              GraphView              `json:"view"`
	Legacy            *GraphLegacyState      `json:"legacy,omitempty"`
}

type GraphNode struct {
	ID         string                 `json:"id"`
	TypeID     string                 `json:"typeId"`
	Position   GraphPosition          `json:"position"`
	Values     map[string]interface{} `json:"values"`
	Properties GraphNodeProperties    `json:"properties,omitempty"`
}

type GraphNodeProperties struct {
	Label              string                 `json:"label,omitempty"`
	VariableID         string                 `json:"variableId,omitempty"`
	VariableAccess     string                 `json:"variableAccess,omitempty"`
	DynamicOutputCount int                    `json:"dynamicOutputCount,omitempty"`
	FunctionRole       string                 `json:"functionRole,omitempty"`
	FunctionID         string                 `json:"functionId,omitempty"`
	FunctionName       string                 `json:"functionName,omitempty"`
	FunctionSource     string                 `json:"functionSource,omitempty"`
	FunctionSignature  GraphFunctionSignature `json:"functionSignature,omitempty"`
	LegacyClass        string                 `json:"legacyClass,omitempty"`
	LegacyModule       string                 `json:"legacyModule,omitempty"`
	LegacyInputs       []GraphLegacyPort      `json:"legacyInputs,omitempty"`
	LegacyOutputs      []GraphLegacyPort      `json:"legacyOutputs,omitempty"`
}

type GraphFunctionSignature struct {
	Inputs  []GraphFunctionSignaturePort `json:"inputs,omitempty"`
	Outputs []GraphFunctionSignaturePort `json:"outputs,omitempty"`
}

type GraphFunctionSignaturePort struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type GraphLegacyState struct {
	Format               string                                 `json:"format,omitempty"`
	Time                 string                                 `json:"time,omitempty"`
	HiddenNodes          []legacyNode                           `json:"hiddenNodes,omitempty"`
	HiddenEdges          []legacyEdge                           `json:"hiddenEdges,omitempty"`
	HiddenEdgeOrdinals   []int                                  `json:"hiddenEdgeOrdinals,omitempty"`
	Groups               []legacyGroup                          `json:"groups,omitempty"`
	Variables            []map[string]interface{}               `json:"variables,omitempty"`
	ResidualNodeDefaults map[string]GraphLegacyResidualDefaults `json:"residualNodeDefaults,omitempty"`
	ExtraRootFields      map[string]json.RawMessage             `json:"extraRootFields,omitempty"`
	ExtraNodeFields      map[string]GraphLegacyNodeExtraFields  `json:"extraNodeFields,omitempty"`
	ExtraEdgeFields      map[string]map[string]json.RawMessage  `json:"extraEdgeFields,omitempty"`
}

type GraphLegacyNodeExtraFields struct {
	Class  string                     `json:"class"`
	Fields map[string]json.RawMessage `json:"fields"`
}

type GraphLegacyResidualDefaults struct {
	Class  string                 `json:"class"`
	Values map[string]interface{} `json:"values"`
}

type GraphLegacyPort struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type GraphPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type GraphConnection struct {
	Source                 string `json:"source"`
	SourceOutput           string `json:"sourceOutput"`
	Target                 string `json:"target"`
	TargetInput            string `json:"targetInput"`
	EntryConnectionVisible bool   `json:"entryConnectionVisible,omitempty"`
	LegacyEdgeID           string `json:"legacyEdgeId,omitempty"`
	LegacyOrdinal          *int   `json:"legacyOrdinal,omitempty"`
}

type GraphGroup struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Width   float64  `json:"width"`
	Height  float64  `json:"height"`
	NodeIDs []string `json:"nodeIds"`
}

type GraphVariable struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	DefaultValue interface{} `json:"defaultValue"`
	GroupID      string      `json:"groupId"`
	Description  string      `json:"description,omitempty"`
}

type GraphVariableGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

type GraphView struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

type ValidationIssue struct {
	Severity string   `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	NodeID   string   `json:"nodeId,omitempty"`
	NodeIDs  []string `json:"nodeIds,omitempty"`
}

type portDefinition struct {
	Inputs  map[string]string
	Outputs map[string]string
}

func functionPortKey(prefix string, port GraphFunctionSignaturePort, index int) string {
	source := strings.TrimSpace(port.ID)
	if source == "" {
		source = strings.TrimSpace(port.Name)
	}
	if source == "" {
		source = fmt.Sprintf("%d", index+1)
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

func functionNodePortDefinition(node GraphNode) portDefinition {
	signature := node.Properties.FunctionSignature
	inputs := map[string]string{}
	outputs := map[string]string{}
	switch node.TypeID {
	case "origin.function.entry":
		outputs["exec"] = "exec"
		for index, port := range signature.Inputs {
			outputs[functionPortKey("input", port, index)] = port.Type
		}
	case "origin.function.return":
		inputs["exec"] = "exec"
		for index, port := range signature.Outputs {
			inputs[functionPortKey("output", port, index)] = port.Type
		}
	case "origin.function.call":
		inputs["exec"] = "exec"
		outputs["exec"] = "exec"
		for index, port := range signature.Inputs {
			inputs[functionPortKey("input", port, index)] = port.Type
		}
		for index, port := range signature.Outputs {
			outputs[functionPortKey("output", port, index)] = port.Type
		}
	}
	return portDefinition{Inputs: inputs, Outputs: outputs}
}

func timerFunctionNodePortDefinition(node GraphNode) portDefinition {
	inputs := map[string]string{"exec": "exec", "time": "integer", "looping": "boolean", "firstDelay": "integer"}
	for index, port := range node.Properties.FunctionSignature.Inputs {
		inputs[functionPortKey("input", port, index)] = port.Type
	}
	return portDefinition{Inputs: inputs, Outputs: map[string]string{"then": "exec", "timerHandle": "timerhandle"}}
}

var graphNodePorts = map[string]portDefinition{
	"origin.event.begin": {
		Outputs: map[string]string{"exec": "exec"},
	},
	"origin.flow.for-loop": {
		Inputs:  map[string]string{"exec": "exec", "start": "integer", "end": "integer"},
		Outputs: map[string]string{"body": "exec", "index": "integer", "completed": "exec"},
	},
	"origin.flow.branch": {
		Inputs:  map[string]string{"exec": "exec", "condition": "boolean"},
		Outputs: map[string]string{"true": "exec", "false": "exec"},
	},
	"origin.cast.integer-string": {
		Inputs:  map[string]string{"value": "integer"},
		Outputs: map[string]string{"result": "string"},
	},
	"origin.cast.float-string": {
		Inputs: map[string]string{"value": "float"}, Outputs: map[string]string{"result": "string"},
	},
	"origin.action.print": {
		Inputs:  map[string]string{"exec": "exec", "value": "string"},
		Outputs: map[string]string{"exec": "exec"},
	},
	"origin.math.add-integer": {
		Inputs: map[string]string{"a": "integer", "b": "integer"}, Outputs: map[string]string{"result": "integer"},
	},
	"origin.math.subtract-integer": {
		Inputs: map[string]string{"a": "integer", "b": "integer", "absolute": "boolean"}, Outputs: map[string]string{"result": "integer"},
	},
	"origin.math.multiply-integer": {
		Inputs: map[string]string{"a": "integer", "b": "integer"}, Outputs: map[string]string{"result": "integer"},
	},
	"origin.math.divide-integer": {
		Inputs: map[string]string{"a": "integer", "b": "integer", "round": "boolean"}, Outputs: map[string]string{"result": "integer"},
	},
	"origin.math.modulo-integer": {
		Inputs: map[string]string{"a": "integer", "b": "integer"}, Outputs: map[string]string{"result": "integer"},
	},
	"origin.math.random-integer": {
		Inputs: map[string]string{"seed": "integer", "min": "integer", "max": "integer"}, Outputs: map[string]string{"result": "integer"},
	},
	"origin.flow.sequence": {
		Inputs: map[string]string{"exec": "exec"}, Outputs: map[string]string{"then0": "exec", "then1": "exec", "then2": "exec"},
	},
	"origin.flow.greater-integer": {
		Inputs: map[string]string{"exec": "exec", "orEqual": "boolean", "a": "integer", "b": "integer"}, Outputs: map[string]string{"false": "exec", "true": "exec"},
	},
	"origin.flow.less-integer": {
		Inputs: map[string]string{"exec": "exec", "orEqual": "boolean", "a": "integer", "b": "integer"}, Outputs: map[string]string{"false": "exec", "true": "exec"},
	},
	"origin.flow.equal-integer": {
		Inputs: map[string]string{"exec": "exec", "a": "integer", "b": "integer"}, Outputs: map[string]string{"false": "exec", "true": "exec"},
	},
	"origin.array.get-integer": {
		Inputs: map[string]string{"array": "array", "index": "integer"}, Outputs: map[string]string{"value": "integer"},
	},
	"origin.array.get-string": {
		Inputs: map[string]string{"array": "array", "index": "integer"}, Outputs: map[string]string{"value": "string"},
	},
	"origin.array.length": {
		Inputs: map[string]string{"array": "array"}, Outputs: map[string]string{"length": "integer"},
	},
	"origin.array.create-integer": {
		Inputs: map[string]string{"items": "array"}, Outputs: map[string]string{"array": "array"},
	},
	"origin.array.create-integer-new": {
		Inputs: map[string]string{"items": "array"}, Outputs: map[string]string{"array": "array"},
	},
	"origin.array.create-string": {
		Inputs: map[string]string{"items": "array"}, Outputs: map[string]string{"array": "array"},
	},
	"origin.array.create-string-new": {
		Inputs: map[string]string{"items": "array"}, Outputs: map[string]string{"array": "array"},
	},
	"origin.array.append-string": {
		Inputs: map[string]string{"array": "array", "value": "string"}, Outputs: map[string]string{"array": "array"},
	},
	"origin.array.append-integer": {
		Inputs: map[string]string{"array": "array", "value": "integer"}, Outputs: map[string]string{"array": "array"},
	},
	"origin.result.append-integer": {
		Inputs: map[string]string{"exec": "exec", "value": "integer"}, Outputs: map[string]string{"exec": "exec"},
	},
	"origin.result.append-string": {
		Inputs: map[string]string{"exec": "exec", "value": "string"}, Outputs: map[string]string{"exec": "exec"},
	},
	"origin.event.entry-array": {
		Outputs: map[string]string{"exec": "exec", "objectId": "integer", "params": "array"},
	},
	"origin.event.entry-two-integers": {
		Outputs: map[string]string{"exec": "exec", "objectId": "integer", "param1": "integer", "param2": "integer"},
	},
	"origin.flow.delay":      {Inputs: map[string]string{"exec": "exec", "duration": "integer"}, Outputs: map[string]string{"completed": "exec"}},
	"origin.timer.clear":     {Inputs: map[string]string{"exec": "exec", "timerHandle": "timerhandle", "cancelRunningCallback": "boolean"}, Outputs: map[string]string{"then": "exec", "success": "boolean"}},
	"origin.timer.pause":     {Inputs: map[string]string{"exec": "exec", "timerHandle": "timerhandle"}, Outputs: map[string]string{"then": "exec", "success": "boolean"}},
	"origin.timer.unpause":   {Inputs: map[string]string{"exec": "exec", "timerHandle": "timerhandle"}, Outputs: map[string]string{"then": "exec", "success": "boolean"}},
	"origin.timer.is-active": {Inputs: map[string]string{"timerHandle": "timerhandle"}, Outputs: map[string]string{"active": "boolean"}},
	"origin.timer.is-paused": {Inputs: map[string]string{"timerHandle": "timerhandle"}, Outputs: map[string]string{"paused": "boolean"}},
	"origin.timer.is-valid":  {Inputs: map[string]string{"timerHandle": "timerhandle"}, Outputs: map[string]string{"valid": "boolean"}},
	"origin.timer.remaining": {Inputs: map[string]string{"timerHandle": "timerhandle"}, Outputs: map[string]string{"remaining": "integer"}},
	"origin.timer.elapsed":   {Inputs: map[string]string{"timerHandle": "timerhandle"}, Outputs: map[string]string{"elapsed": "integer"}},
	"origin.flow.foreach-integer-array": {
		Inputs: map[string]string{"exec": "exec", "array": "array"}, Outputs: map[string]string{"body": "exec", "completed": "exec", "index": "integer", "value": "integer"},
	},
	"origin.flow.probability": {
		Inputs: map[string]string{"exec": "exec", "probability": "integer"}, Outputs: map[string]string{"miss": "exec", "hit": "exec"},
	},
	"origin.flow.range-compare": {
		Inputs: map[string]string{"exec": "exec", "value": "integer", "ranges": "array"}, Outputs: map[string]string{"otherwise": "exec"},
	},
	"origin.flow.equal-switch": {
		Inputs: map[string]string{"exec": "exec", "value": "integer", "cases": "array"}, Outputs: map[string]string{"otherwise": "exec"},
	},
	"origin.flow.equal-switch-new": {
		Inputs: map[string]string{"exec": "exec", "value": "integer", "cases": "array"}, Outputs: map[string]string{"otherwise": "exec"},
	},
	"origin.debug.output": {
		Inputs: map[string]string{"exec": "exec", "integer": "integer", "string": "string", "array": "array"}, Outputs: map[string]string{"exec": "exec"},
	},
	"origin.literal.string": {
		Inputs: map[string]string{"value": "string"}, Outputs: map[string]string{"value": "string"},
	},
	"origin.math.add-float": {
		Inputs: map[string]string{"a": "float", "b": "float"}, Outputs: map[string]string{"result": "float"},
	},
	"origin.math.subtract-float": {
		Inputs: map[string]string{"a": "float", "b": "float"}, Outputs: map[string]string{"result": "float"},
	},
	"origin.math.multiply-float": {
		Inputs: map[string]string{"a": "float", "b": "float"}, Outputs: map[string]string{"result": "float"},
	},
	"origin.math.divide-float": {
		Inputs: map[string]string{"a": "float", "b": "float"}, Outputs: map[string]string{"result": "float"},
	},
	"origin.compare.greater-integer": {
		Inputs: map[string]string{"a": "integer", "b": "integer"}, Outputs: map[string]string{"result": "boolean", "a": "integer", "b": "integer"},
	},
	"origin.flow.while": {
		Inputs: map[string]string{"exec": "exec", "condition": "boolean"}, Outputs: map[string]string{"body": "exec", "completed": "exec"},
	},
	"origin.flow.for-loop-break": {
		Inputs: map[string]string{"exec": "exec", "start": "integer", "end": "integer", "break": "exec"}, Outputs: map[string]string{"body": "exec", "index": "integer", "completed": "exec"},
	},
	"origin.flow.foreach-array": {
		Inputs: map[string]string{"exec": "exec", "array": "array"}, Outputs: map[string]string{"body": "exec", "completed": "exec", "value": "any", "index": "integer"},
	},
	"origin.string.split": {
		Inputs: map[string]string{"exec": "exec", "text": "string", "delimiter": "string"}, Outputs: map[string]string{"exec": "exec", "array": "array"},
	},
	"origin.array.get-any": {
		Inputs: map[string]string{"array": "array", "index": "integer"}, Outputs: map[string]string{"value": "any"},
	},
	"origin.cast.any-string": {
		Inputs: map[string]string{"exec": "exec", "value": "any"}, Outputs: map[string]string{"exec": "exec", "valid": "boolean", "result": "string"},
	},
}

type dynamicBranchSpec struct {
	controlInput     string
	outputPrefix     string
	outputStartIndex int
	maxBranches      int
}

var dynamicBranchSpecs = map[string]dynamicBranchSpec{
	"origin.flow.range-compare":    {controlInput: "ranges", outputPrefix: "case", outputStartIndex: 1, maxBranches: 4},
	"origin.flow.equal-switch":     {controlInput: "cases", outputPrefix: "case", outputStartIndex: 1, maxBranches: 4},
	"origin.flow.equal-switch-new": {controlInput: "cases", outputPrefix: "case", outputStartIndex: 1, maxBranches: 50},
}

func applyDynamicBranchOutputs(node GraphNode, definition portDefinition) portDefinition {
	spec, ok := dynamicBranchSpecs[node.TypeID]
	if !ok {
		return definition
	}
	outputs := make(map[string]string, len(definition.Outputs)+spec.maxBranches)
	for key, value := range definition.Outputs {
		outputs[key] = value
	}
	for index := 0; index < spec.maxBranches; index++ {
		outputs[fmt.Sprintf("%s%d", spec.outputPrefix, spec.outputStartIndex+index)] = "exec"
	}
	return portDefinition{Inputs: definition.Inputs, Outputs: outputs}
}

func (a *App) ValidateGraph(content string) ([]ValidationIssue, error) {
	var document GraphDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return nil, fmt.Errorf("decode graph document: %w", err)
	}
	return validateGraph(document), nil
}

func validateGraph(document GraphDocument) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if document.SchemaVersion != GraphSchemaVersion {
		issues = append(issues, ValidationIssue{Severity: "error", Code: "schema.unsupported", Message: fmt.Sprintf("不支持的蓝图版本：%d", document.SchemaVersion)})
	}

	if len(document.FunctionSignature.Inputs) > maxFunctionSignatureInputs || len(document.FunctionSignature.Outputs) > maxFunctionSignatureOutputs {
		issues = append(issues, ValidationIssue{Severity: "error", Code: "function.signature-limit", Message: "Function signature exceeds the safe port limit"})
	}

	variables := make(map[string]GraphVariable, len(document.Variables))
	variableNames := make(map[string]bool, len(document.Variables))
	variableTypes := map[string]bool{"boolean": true, "integer": true, "float": true, "string": true, "array": true, "timerhandle": true}
	variableGroups := map[string]bool{"default": true}
	variableGroupNames := map[string]bool{"Default": true}
	for _, group := range document.VariableGroups {
		if group.ID == "" || group.Name == "" {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable-group.invalid", Message: "变量分组缺少 ID 或名称"})
			continue
		}
		if variableGroups[group.ID] && group.ID != "default" {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable-group.duplicate-id", Message: "变量分组 ID 重复：" + group.ID})
		}
		if variableGroupNames[group.Name] && !(group.ID == "default" && group.Name == "Default") {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable-group.duplicate-name", Message: "变量分组名称重复：" + group.Name})
		}
		variableGroups[group.ID] = true
		variableGroupNames[group.Name] = true
	}
	for _, variable := range document.Variables {
		if variable.ID == "" || variable.Name == "" {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable.invalid", Message: "变量缺少 ID 或名称"})
			continue
		}
		if _, exists := variables[variable.ID]; exists {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable.duplicate-id", Message: "变量 ID 重复：" + variable.ID})
		}
		if variableNames[variable.Name] {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable.duplicate-name", Message: "变量名称重复：" + variable.Name})
		}
		if !variableTypes[variable.Type] {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable.unknown-type", Message: "未知变量类型：" + variable.Type})
		}
		groupID := variable.GroupID
		if groupID == "" {
			groupID = "default"
		}
		if !variableGroups[groupID] {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "variable.missing-group", Message: "变量引用了不存在的分组：" + groupID})
		}
		variables[variable.ID] = variable
		variableNames[variable.Name] = true
	}

	nodes := make(map[string]GraphNode, len(document.Nodes))
	ports := make(map[string]portDefinition, len(document.Nodes))
	for _, node := range document.Nodes {
		if node.ID == "" {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "node.missing-id", Message: "存在缺少 ID 的结点"})
			continue
		}
		if _, exists := nodes[node.ID]; exists {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "node.duplicate-id", Message: "结点 ID 重复：" + node.ID, NodeID: node.ID})
			continue
		}
		nodes[node.ID] = node
		definition, known := graphNodePorts[node.TypeID]
		if node.TypeID == "origin.flow.sequence" {
			count := node.Properties.DynamicOutputCount
			if count == 0 {
				count = 3
			}
			if count < 1 || count > maxDynamicSequenceOutputs {
				issues = append(issues, ValidationIssue{Severity: "error", Code: "node.dynamic-output-count", Message: fmt.Sprintf("Sequence output count must be between 1 and %d", maxDynamicSequenceOutputs), NodeID: node.ID})
				continue
			}
			outputs := make(map[string]string, count)
			for index := 0; index < count; index++ {
				outputs[fmt.Sprintf("then%d", index)] = "exec"
			}
			definition = portDefinition{Inputs: map[string]string{"exec": "exec"}, Outputs: outputs}
			known = true
		}
		if strings.HasPrefix(node.TypeID, "origin.function.") {
			if len(node.Properties.FunctionSignature.Inputs) > maxFunctionSignatureInputs || len(node.Properties.FunctionSignature.Outputs) > maxFunctionSignatureOutputs {
				issues = append(issues, ValidationIssue{Severity: "error", Code: "function.signature-limit", Message: "Function signature exceeds the safe port limit", NodeID: node.ID})
				continue
			}
			definition = functionNodePortDefinition(node)
			known = true
		}
		if node.TypeID == "origin.timer.set-by-function" {
			if len(node.Properties.FunctionSignature.Inputs) > maxFunctionSignatureInputs || len(node.Properties.FunctionSignature.Outputs) > maxFunctionSignatureOutputs {
				issues = append(issues, ValidationIssue{Severity: "error", Code: "function.signature-limit", Message: "Function signature exceeds the safe port limit", NodeID: node.ID})
				continue
			}
			definition = timerFunctionNodePortDefinition(node)
			known = true
			if strings.TrimSpace(node.Properties.FunctionID) == "" {
				issues = append(issues, ValidationIssue{
					Severity: "error",
					Code:     "timer.function-missing",
					Message:  "按函数设置定时器节点尚未选择回调函数",
					NodeID:   node.ID,
				})
			}
		}
		if node.TypeID == "origin.variable.get" || node.TypeID == "origin.variable.set" {
			variable, exists := variables[node.Properties.VariableID]
			if !exists {
				issues = append(issues, ValidationIssue{Severity: "error", Code: "variable.missing", Message: "变量结点引用了不存在的变量", NodeID: node.ID})
				continue
			}
			if node.TypeID == "origin.variable.get" {
				definition = portDefinition{Outputs: map[string]string{"value": variable.Type}}
			} else {
				definition = portDefinition{Inputs: map[string]string{"exec": "exec", "value": variable.Type}, Outputs: map[string]string{"exec": "exec", "value": variable.Type}}
			}
			known = true
		}
		if node.TypeID == "origin.legacy.placeholder" {
			if len(node.Properties.LegacyInputs)+len(node.Properties.LegacyOutputs) > maxLegacyPortsPerNode {
				issues = append(issues, ValidationIssue{Severity: "error", Code: "node.port-limit", Message: "Legacy node port count exceeds the safe limit", NodeID: node.ID})
				continue
			}
			inputs := make(map[string]string, len(node.Properties.LegacyInputs))
			outputs := make(map[string]string, len(node.Properties.LegacyOutputs))
			for _, port := range node.Properties.LegacyInputs {
				inputs[port.Key] = port.Type
			}
			for _, port := range node.Properties.LegacyOutputs {
				outputs[port.Key] = port.Type
			}
			definition = portDefinition{Inputs: inputs, Outputs: outputs}
			known = true
			issues = append(issues, ValidationIssue{Severity: "warning", Code: "node.legacy-placeholder", Message: "老版本结点已保留，但当前不可执行：" + node.Properties.LegacyClass, NodeID: node.ID})
		}
		if !known && (len(node.Properties.LegacyInputs) > 0 || len(node.Properties.LegacyOutputs) > 0) {
			if len(node.Properties.LegacyInputs)+len(node.Properties.LegacyOutputs) > maxLegacyPortsPerNode {
				issues = append(issues, ValidationIssue{Severity: "error", Code: "node.port-limit", Message: "Legacy node port count exceeds the safe limit", NodeID: node.ID})
				continue
			}
			inputs := make(map[string]string, len(node.Properties.LegacyInputs))
			outputs := make(map[string]string, len(node.Properties.LegacyOutputs))
			for _, port := range node.Properties.LegacyInputs {
				inputs[port.Key] = port.Type
			}
			for _, port := range node.Properties.LegacyOutputs {
				outputs[port.Key] = port.Type
			}
			definition = portDefinition{Inputs: inputs, Outputs: outputs}
			known = true
		}
		if !known {
			continue
		}
		definition = applyDynamicBranchOutputs(node, definition)
		ports[node.ID] = definition
	}

	for _, connection := range document.Connections {
		source, sourceExists := nodes[connection.Source]
		target, targetExists := nodes[connection.Target]
		if !sourceExists || !targetExists {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "connection.dangling", Message: "连线引用了不存在的结点"})
			continue
		}
		if _, sourceKnown := ports[source.ID]; !sourceKnown {
			continue
		}
		if _, targetKnown := ports[target.ID]; !targetKnown {
			continue
		}
		sourceType, sourcePortExists := ports[source.ID].Outputs[connection.SourceOutput]
		targetType, targetPortExists := ports[target.ID].Inputs[connection.TargetInput]
		if !sourcePortExists || !targetPortExists {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "connection.missing-port", Message: "连线引用了不存在的端口", NodeID: target.ID})
			continue
		}
		if sourceType != targetType && sourceType != "any" && targetType != "any" {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "connection.type-mismatch", Message: fmt.Sprintf("端口类型不匹配：%s 不能连接到 %s", sourceType, targetType), NodeID: target.ID})
		}
	}
	issues = append(issues, validateExecutionFlow(nodes, ports, document.Connections)...)

	return issues
}

func validateExecutionFlow(nodes map[string]GraphNode, ports map[string]portDefinition, connections []GraphConnection) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	executable := make(map[string]bool)
	entries := make(map[string]bool)
	execEdges := make(map[string][]string)
	for nodeID, definition := range ports {
		hasExecInput := hasPortType(definition.Inputs, "exec")
		hasExecOutput := hasPortType(definition.Outputs, "exec")
		if hasExecInput || hasExecOutput {
			executable[nodeID] = true
		}
		if !hasExecInput && hasExecOutput {
			entries[nodeID] = true
		}
	}
	if len(executable) == 0 {
		return issues
	}
	for _, connection := range connections {
		sourceDefinition, sourceKnown := ports[connection.Source]
		targetDefinition, targetKnown := ports[connection.Target]
		if !sourceKnown || !targetKnown {
			continue
		}
		if sourceDefinition.Outputs[connection.SourceOutput] == "exec" && targetDefinition.Inputs[connection.TargetInput] == "exec" {
			execEdges[connection.Source] = append(execEdges[connection.Source], connection.Target)
		}
	}
	if len(entries) == 0 {
		issues = append(issues, ValidationIssue{Severity: "warning", Code: "flow.missing-entry", Message: "蓝图存在可执行结点，但没有入口结点", NodeIDs: sortedMapKeys(executable)})
		return issues
	}

	reachable := make(map[string]bool)
	entryReachable := make(map[string]map[string]bool)
	dataInputs := make(map[string][]GraphConnection)
	for _, connection := range connections {
		sourceDefinition, sourceKnown := ports[connection.Source]
		targetDefinition, targetKnown := ports[connection.Target]
		if !sourceKnown || !targetKnown {
			continue
		}
		sourceType := sourceDefinition.Outputs[connection.SourceOutput]
		targetType := targetDefinition.Inputs[connection.TargetInput]
		if sourceType == "" || targetType == "" || sourceType == "exec" || targetType == "exec" {
			continue
		}
		dataInputs[connection.Target] = append(dataInputs[connection.Target], connection)
	}
	markReachable := func(nodeID, entryID string) bool {
		reachable[nodeID] = true
		if entryReachable[nodeID] == nil {
			entryReachable[nodeID] = make(map[string]bool)
		}
		if entryReachable[nodeID][entryID] {
			return false
		}
		entryReachable[nodeID][entryID] = true
		return true
	}
	execVisited := make(map[string]bool)
	dataVisited := make(map[string]bool)
	for entryID := range entries {
		execStack := []string{entryID}
		for len(execStack) > 0 {
			nodeID := execStack[len(execStack)-1]
			execStack = execStack[:len(execStack)-1]
			markReachable(nodeID, entryID)
			key := entryID + "\x00" + nodeID
			if execVisited[key] {
				continue
			}
			execVisited[key] = true

			dataStack := make([]string, 0, len(dataInputs[nodeID]))
			for _, connection := range dataInputs[nodeID] {
				dataStack = append(dataStack, connection.Source)
			}
			for len(dataStack) > 0 {
				dataNodeID := dataStack[len(dataStack)-1]
				dataStack = dataStack[:len(dataStack)-1]
				if entries[dataNodeID] && dataNodeID != entryID {
					continue
				}
				dataKey := entryID + "\x00" + dataNodeID
				if dataVisited[dataKey] {
					continue
				}
				dataVisited[dataKey] = true
				markReachable(dataNodeID, entryID)
				for _, connection := range dataInputs[dataNodeID] {
					dataStack = append(dataStack, connection.Source)
				}
			}
			execStack = append(execStack, execEdges[nodeID]...)
		}
	}
	for nodeID := range executable {
		if !reachable[nodeID] {
			label := nodes[nodeID].Properties.Label
			if label == "" {
				label = nodeID
			}
			issues = append(issues, ValidationIssue{Severity: "error", Code: "flow.unreachable-node", Message: "结点不可达，不可能从任何入口执行到：" + label, NodeID: nodeID})
		}
	}
	for _, connection := range connections {
		sourceDefinition, sourceKnown := ports[connection.Source]
		targetDefinition, targetKnown := ports[connection.Target]
		if !sourceKnown || !targetKnown {
			continue
		}
		sourceType := sourceDefinition.Outputs[connection.SourceOutput]
		targetType := targetDefinition.Inputs[connection.TargetInput]
		if sourceType == "" || targetType == "" || sourceType == "exec" || targetType == "exec" {
			continue
		}
		if !entrySetsOverlap(entryReachable[connection.Source], entryReachable[connection.Target]) {
			issues = append(issues, ValidationIssue{Severity: "error", Code: "flow.cross-entry-data", Message: "不同入口分支之间存在参数交叉连接", NodeID: connection.Target})
		}
	}

	const (
		cycleUnvisited uint8 = iota
		cycleVisiting
		cycleVisited
	)
	cycleState := make(map[string]uint8)
	cycleReported := make(map[string]bool)
	type cycleFrame struct {
		nodeID string
		next   int
	}
	for nodeID := range entries {
		if cycleState[nodeID] != cycleUnvisited || !reachable[nodeID] {
			continue
		}
		cycleState[nodeID] = cycleVisiting
		stack := []cycleFrame{{nodeID: nodeID}}
		for len(stack) > 0 {
			frame := &stack[len(stack)-1]
			nextNodes := execEdges[frame.nodeID]
			if frame.next >= len(nextNodes) {
				cycleState[frame.nodeID] = cycleVisited
				stack = stack[:len(stack)-1]
				continue
			}
			next := nextNodes[frame.next]
			frame.next++
			if !reachable[next] || cycleState[next] == cycleVisited {
				continue
			}
			if cycleState[next] == cycleVisiting {
				if !cycleReported[next] {
					issues = append(issues, ValidationIssue{Severity: "error", Code: "flow.possible-cycle", Message: "从入口开始可能产生直接或间接死循环", NodeID: next})
					cycleReported[next] = true
				}
				continue
			}
			cycleState[next] = cycleVisiting
			stack = append(stack, cycleFrame{nodeID: next})
		}
	}
	return issues
}

func entrySetsOverlap(a, b map[string]bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for key := range a {
		if b[key] {
			return true
		}
	}
	return false
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func hasPortType(ports map[string]string, portType string) bool {
	for _, value := range ports {
		if value == portType {
			return true
		}
	}
	return false
}
