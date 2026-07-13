package blueprint

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistryLoadsNodeDefinitionsAndBindsRegisteredExec(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"name":"TestRecorder",
			"inputs":[
				{"type":"exec","port_id":0},
				{"type":"data","data_type":"int","port_id":1}
			],
			"outputs":[]
		}
	]`), []func() IExecNode{
		func() IExecNode { return &testRecorder{} },
	})
	if err != nil {
		t.Fatalf("LoadDefinitionsJSON failed: %v", err)
	}

	definition := registry.Get("TestRecorder")
	if definition == nil {
		t.Fatalf("definition not registered")
	}
	if len(definition.InPorts) != 2 {
		t.Fatalf("in ports = %d, want 2", len(definition.InPorts))
	}
	if !definition.InPorts[0].IsPortExec() {
		t.Fatalf("input 0 is not exec")
	}
	if _, ok := definition.InPorts[1].GetInt(); !ok {
		t.Fatalf("input 1 is not int")
	}
}

func TestRegistryReportsMissingExecImplementation(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{"name":"MissingImpl","inputs":[],"outputs":[]}
	]`), nil)
	if err == nil {
		t.Fatalf("LoadDefinitionsJSON succeeded, want missing implementation error")
	}
}

func TestRegistryExtendsDynamicBranchExecOutputs(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"name":"EqualSwitch",
			"inputs":[
				{"type":"exec","port_id":0},
				{"type":"data","data_type":"Integer","port_id":1},
				{"type":"data","data_type":"Array","port_id":2}
			],
			"outputs":[
				{"type":"exec","port_id":0},
				{"type":"exec","port_id":1},
				{"type":"exec","port_id":2}
			]
		}
	]`), []func() IExecNode{
		func() IExecNode { return &EqualSwitch{} },
	})
	if err != nil {
		t.Fatalf("LoadDefinitionsJSON failed: %v", err)
	}
	definition := registry.Get("EqualSwitch")
	if definition == nil {
		t.Fatalf("definition not registered")
	}
	if len(definition.OutPorts) != 52 {
		t.Fatalf("out ports = %d, want 52", len(definition.OutPorts))
	}
	if !definition.OutPorts[51].IsPortExec() {
		t.Fatalf("case50 output should be exec")
	}
}

func TestRegistryLoadsEqualSwitchNewSchemaWithoutLegacyPortIDs(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"id": "origin.flow.equal-switch-new",
			"title": "Equal Switch New",
			"category": "Flow",
			"inputs": [
				{"key": "exec", "label": "", "type": "exec"},
				{"key": "value", "label": "Value", "type": "data", "data_type": "Integer"},
				{"key": "cases", "label": "Cases", "type": "data", "data_type": "Array"}
			],
			"outputs": [
				{"key": "otherwise", "label": "Otherwise", "type": "exec"}
			],
			"dynamicBranch": {
				"controlInput": "cases",
				"defaultOutput": "otherwise",
				"outputPrefix": "case",
				"outputStartIndex": 1,
				"maxBranches": 50,
				"outputTemplate": {"label": "", "type": "exec"}
			}
		}
	]`), []func() IExecNode{
		func() IExecNode { return &EqualSwitch{} },
	})
	if err != nil {
		t.Fatalf("LoadDefinitionsJSON failed: %v", err)
	}

	definition := registry.Get("EqualSwitch")
	if definition == nil {
		t.Fatalf("definition not registered")
	}
	if len(definition.InPorts) != 3 {
		t.Fatalf("in ports = %d, want 3", len(definition.InPorts))
	}
	if !definition.InPorts[0].IsPortExec() {
		t.Fatalf("input 0 is not exec")
	}
	if _, ok := definition.InPorts[1].GetInt(); !ok {
		t.Fatalf("input 1 is not int")
	}
	if _, ok := definition.InPorts[2].GetArray(); !ok {
		t.Fatalf("input 2 is not array")
	}
	if len(definition.OutPorts) != 52 {
		t.Fatalf("out ports = %d, want 52", len(definition.OutPorts))
	}
	if !definition.OutPorts[0].IsPortExec() || !definition.OutPorts[51].IsPortExec() {
		t.Fatalf("otherwise and case50 outputs should be exec")
	}
}

func TestRegistryLoadsNewArraySchemasWithoutLegacyPortIDs(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"id": "origin.array.create-integer-new",
			"title": "Create Int Array New",
			"category": "Array",
			"inputs": [
				{"key": "items", "label": "", "type": "data", "data_type": "Array"}
			],
			"outputs": [
				{"key": "array", "label": "Array", "type": "data", "data_type": "Array"}
			]
		},
		{
			"id": "origin.array.create-string-new",
			"title": "Create String Array New",
			"category": "Array",
			"inputs": [
				{"key": "items", "label": "", "type": "data", "data_type": "Array"}
			],
			"outputs": [
				{"key": "array", "label": "Array", "type": "data", "data_type": "Array"}
			]
		}
	]`), []func() IExecNode{
		func() IExecNode { return &CreateIntArray{} },
		func() IExecNode { return &CreateStringArray{} },
	})
	if err != nil {
		t.Fatalf("LoadDefinitionsJSON failed: %v", err)
	}

	for _, name := range []string{"CreateIntArray", "CreateStringArray"} {
		definition := registry.Get(name)
		if definition == nil {
			t.Fatalf("%s definition not registered", name)
		}
		if len(definition.InPorts) != 1 {
			t.Fatalf("%s in ports = %d, want 1", name, len(definition.InPorts))
		}
		if _, ok := definition.InPorts[0].GetArray(); !ok {
			t.Fatalf("%s input 0 is not array", name)
		}
		if len(definition.OutPorts) != 1 {
			t.Fatalf("%s out ports = %d, want 1", name, len(definition.OutPorts))
		}
		if _, ok := definition.OutPorts[0].GetArray(); !ok {
			t.Fatalf("%s output 0 is not array", name)
		}
	}
}

func TestRegistryMapsSchemaPortKeysWhenLegacyNameIsPresent(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"id": "origin.flow.equal-switch-new",
			"name": "EqualSwitch",
			"inputs": [
				{"key": "exec", "type": "exec"},
				{"key": "value", "type": "data", "data_type": "Integer"},
				{"key": "cases", "type": "data", "data_type": "Array"}
			],
			"outputs": [
				{"key": "otherwise", "type": "exec"}
			]
		}
	]`), []func() IExecNode{
		func() IExecNode { return &EqualSwitch{} },
	})
	if err != nil {
		t.Fatalf("LoadDefinitionsJSON failed: %v", err)
	}

	definition := registry.Get("EqualSwitch")
	if definition == nil {
		t.Fatalf("definition not registered")
	}
	if len(definition.InPorts) != 3 {
		t.Fatalf("in ports = %d, want 3", len(definition.InPorts))
	}
	if !definition.InPorts[0].IsPortExec() {
		t.Fatalf("input 0 is not exec")
	}
	if _, ok := definition.InPorts[1].GetInt(); !ok {
		t.Fatalf("input 1 is not int")
	}
	if _, ok := definition.InPorts[2].GetArray(); !ok {
		t.Fatalf("input 2 is not array")
	}
	if len(definition.OutPorts) != 52 {
		t.Fatalf("out ports = %d, want 52", len(definition.OutPorts))
	}
}

func TestRegistryReportsUnmappedSchemaPortKey(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"id": "origin.unknown.keyed-node",
			"name": "EqualSwitch",
			"inputs": [
				{"key": "exec", "type": "exec"}
			],
			"outputs": []
		}
	]`), []func() IExecNode{
		func() IExecNode { return &EqualSwitch{} },
	})
	if err == nil || !strings.Contains(err.Error(), `port key "exec" has no port_id mapping`) {
		t.Fatalf("LoadDefinitionsJSON err = %v, want unmapped key error", err)
	}
}

func TestRegistryReportsSchemaPortKeyAndPortIDMismatch(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"id": "origin.flow.equal-switch-new",
			"name": "EqualSwitch",
			"inputs": [
				{"key": "value", "type": "data", "data_type": "Integer", "port_id": 9}
			],
			"outputs": []
		}
	]`), []func() IExecNode{
		func() IExecNode { return &EqualSwitch{} },
	})
	if err == nil || !strings.Contains(err.Error(), `port key "value" maps to port_id 1 but declares 9`) {
		t.Fatalf("LoadDefinitionsJSON err = %v, want key/port_id mismatch error", err)
	}
}

func TestRegistryReportsInvalidPortIDValues(t *testing.T) {
	tests := []struct {
		name    string
		portID  string
		wantErr string
	}{
		{name: "non-integer", portID: `1.5`, wantErr: "invalid non-integer port_id 1.5"},
		{name: "negative", portID: `-1`, wantErr: "invalid negative port_id -1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := NewRegistry()
			err := registry.LoadDefinitionsJSON([]byte(`[
				{
					"name": "TestRecorder",
					"inputs": [
						{"type": "exec", "port_id": `+test.portID+`}
					],
					"outputs": []
				}
			]`), []func() IExecNode{
				func() IExecNode { return &testRecorder{} },
			})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadDefinitionsJSON err = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestBuildPortsRejectsUnsafePortLayouts(t *testing.T) {
	tests := []struct {
		name    string
		ports   []PortDefinition
		wantErr string
	}{
		{
			name:    "negative port id",
			ports:   []PortDefinition{{PortType: "exec", PortID: -1}},
			wantErr: "port_id -1 must be nonnegative",
		},
		{
			name: "duplicate port id",
			ports: []PortDefinition{
				{PortType: "exec", PortID: 0},
				{PortType: "exec", PortID: 0},
			},
			wantErr: "duplicate port_id 0",
		},
		{
			name:    "sparse port id above limit",
			ports:   []PortDefinition{{PortType: "exec", PortID: 4096}},
			wantErr: "port_id 4096 exceeds maximum 4095",
		},
		{
			name:    "port count above limit",
			ports:   make([]PortDefinition, 4097),
			wantErr: "port count 4097 exceeds maximum 4096",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildPorts(test.ports)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("buildPorts err = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestRegistryLenientDefinitionsCannotBypassPortLimits(t *testing.T) {
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"name": "TestRecorder",
			"inputs": [{"type":"exec", "port_id":4096}],
			"outputs": [],
		},
	]`), []func() IExecNode{
		func() IExecNode { return &testRecorder{} },
	})
	if err == nil || !strings.Contains(err.Error(), "port_id 4096 exceeds maximum 4095") {
		t.Fatalf("LoadDefinitionsJSON err = %v, want lenient port limit error", err)
	}
}

func TestRegistryLenientDefinitionsStopScanningAtTotalPortLimit(t *testing.T) {
	ports := strings.Repeat(`{"type":"exec","port_id":0},`, 4097)
	registry := NewRegistry()
	err := registry.LoadDefinitionsJSON([]byte(`[
		{
			"name": "TestRecorder",
			"inputs": [`+ports+`],
			"outputs": [],
		},
	]`), []func() IExecNode{
		func() IExecNode { return &testRecorder{} },
	})
	if err == nil || !strings.Contains(err.Error(), "total port count 4097 exceeds maximum 4096") {
		t.Fatalf("LoadDefinitionsJSON err = %v, want bounded lenient scan error", err)
	}
}

func TestSplitTopLevelObjectsStopsAtLimit(t *testing.T) {
	objects, err := splitTopLevelObjectsLimited(`{}{}{}{}{}more-data-that-must-not-be-scanned`, 4, "total port count")
	if err == nil || !strings.Contains(err.Error(), "total port count 5 exceeds maximum 4") {
		t.Fatalf("splitTopLevelObjectsLimited objects=%d err=%v, want early count error", len(objects), err)
	}
}

func TestRegistryRejectsTotalNodePortCountAboveLimit(t *testing.T) {
	inputs := make([]PortDefinition, 2049)
	outputs := make([]PortDefinition, 2048)
	for index := range inputs {
		inputs[index] = PortDefinition{PortType: "exec", PortID: index}
	}
	for index := range outputs {
		outputs[index] = PortDefinition{PortType: "exec", PortID: index}
	}

	registry := NewRegistry()
	config, err := json.Marshal([]ExecDefinitionConfig{{
		Name:    "TestRecorder",
		Inputs:  inputs,
		Outputs: outputs,
	}})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	err = registry.LoadDefinitionsJSON(config, []func() IExecNode{
		func() IExecNode { return &testRecorder{} },
	})
	if err == nil || !strings.Contains(err.Error(), "total port count 4097 exceeds maximum 4096") {
		t.Fatalf("LoadDefinitionsJSON err = %v, want total port count limit", err)
	}
}
