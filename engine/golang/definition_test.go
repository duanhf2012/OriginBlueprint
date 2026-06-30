package golang

import "testing"

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
